package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/OmarHGK/paylite/internal/db"
	"github.com/OmarHGK/paylite/internal/handlers"
)

// TestConcurrentTransfers fires many concurrent POST /transfers requests at
// a real running instance of the app (via httptest.NewServer, which starts
// an actual HTTP server on a real port) between the same two accounts, then
// checks that:
//  1. every successful transfer actually moved money
//  2. the combined total of both accounts is unchanged - i.e. no money was
//     created or destroyed by the concurrent writes.
//
// This is the "week 1 stress test, but over HTTP" required by task 02, and
// it's also what `go test ./... -race` needs an actual test to exercise.
//
// Requires: a real MongoDB replica set running and reachable at
// mongodb://localhost:27017 (same as `go run main.go` needs).
func TestConcurrentTransfers(t *testing.T) {
	// --- Setup: wire up the exact same handlers main.go uses ---
	client := db.Connect("mongodb://localhost:27017")
	defer client.Disconnect(context.Background())

	accountHandler := handlers.NewAccountHandler(client)
	transferHandler := handlers.NewTransferHandler(client)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /accounts", accountHandler.CreateAccount)
	mux.HandleFunc("GET /accounts/{id}", accountHandler.GetAccount)
	mux.HandleFunc("POST /transfers", transferHandler.CreateTransfer)

	// httptest.NewServer starts a real HTTP server on a real (random) port.
	// This is important: it means our test hits the app the same way a
	// real client would, over the network, not by calling Go functions
	// directly.
	server := httptest.NewServer(mux)
	defer server.Close()

	// --- Create two fresh accounts to run the stress test against ---
	const startingBalance = int64(1_000_000) // $10,000.00 in cents
	aliceID := createAccount(t, server.URL, "StressAlice", startingBalance)
	bobID := createAccount(t, server.URL, "StressBob", 0)

	// --- Fire N concurrent transfers ---
	const numTransfers = 50
	const amountPerTransfer = int64(100) // $1.00 each

	var wg sync.WaitGroup
	var successCount int64
	var failureCount int64

	for i := 0; i < numTransfers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			// Each request needs its OWN idempotency key. If they all
			// shared one key, we'd just be testing idempotency again
			// (49 of them would get rejected as duplicates) instead of
			// testing that concurrent, *different* transfers are all
			// applied safely.
			body := map[string]any{
				"from_account":    aliceID,
				"to_account":      bobID,
				"amount_cents":    amountPerTransfer,
				"idempotency_key": fmt.Sprintf("stress-test-%d", i),
			}
			payload, _ := json.Marshal(body)

			resp, err := http.Post(server.URL+"/transfers", "application/json", bytes.NewReader(payload))
			if err != nil {
				t.Errorf("transfer %d: request failed: %v", i, err)
				atomic.AddInt64(&failureCount, 1)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusCreated {
				atomic.AddInt64(&successCount, 1)
			} else {
				t.Errorf("transfer %d: unexpected status %d", i, resp.StatusCode)
				atomic.AddInt64(&failureCount, 1)
			}
		}(i)
	}

	// Block here until every goroutine above has called wg.Done().
	// Nothing after this line runs until all 50 requests have completed.
	wg.Wait()

	t.Logf("transfers: %d succeeded, %d failed", successCount, failureCount)

	if failureCount > 0 {
		t.Fatalf("expected all %d transfers to succeed, but %d failed", numTransfers, failureCount)
	}

	// --- Verify money was actually moved, and nothing was lost/duplicated ---
	aliceBalance := getBalance(t, server.URL, aliceID)
	bobBalance := getBalance(t, server.URL, bobID)

	expectedMoved := int64(successCount) * amountPerTransfer
	expectedAlice := startingBalance - expectedMoved
	expectedBob := expectedMoved

	if aliceBalance != expectedAlice {
		t.Errorf("alice balance = %d, want %d", aliceBalance, expectedAlice)
	}
	if bobBalance != expectedBob {
		t.Errorf("bob balance = %d, want %d", bobBalance, expectedBob)
	}

	// The real proof nothing was created or destroyed: the total across
	// both accounts must equal the total before the test ran, no matter
	// how the money moved between them.
	totalBefore := startingBalance
	totalAfter := aliceBalance + bobBalance
	if totalAfter != totalBefore {
		t.Fatalf("money was created or destroyed: total before = %d, total after = %d", totalBefore, totalAfter)
	}
}

// createAccount is a small helper: POST /accounts and return the new
// account's id as a string.
func createAccount(t *testing.T, baseURL, ownerName string, balanceCents int64) string {
	t.Helper()

	body := map[string]any{
		"owner_name":    ownerName,
		"currency":      "USD",
		"balance_cents": balanceCents,
	}
	payload, _ := json.Marshal(body)

	resp, err := http.Post(baseURL+"/accounts", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("failed to create account %s: %v", ownerName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("failed to create account %s: status %d", ownerName, resp.StatusCode)
	}

	var created struct {
		ID bson.ObjectID `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode created account: %v", err)
	}

	return created.ID.Hex()
}

// getBalance is a small helper: GET /accounts/{id} and return its
// balance_cents.
func getBalance(t *testing.T, baseURL, accountID string) int64 {
	t.Helper()

	resp, err := http.Get(baseURL + "/accounts/" + accountID)
	if err != nil {
		t.Fatalf("failed to fetch account %s: %v", accountID, err)
	}
	defer resp.Body.Close()

	var account struct {
		BalanceCents int64 `json:"balance_cents"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		t.Fatalf("failed to decode account %s: %v", accountID, err)
	}

	return account.BalanceCents
}

// keep time import used even if unused directly above in some edits
var _ = time.Second