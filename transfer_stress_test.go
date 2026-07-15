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

func TestConcurrentTransfers(t *testing.T) {

	client := db.Connect("mongodb://localhost:27017")
	defer client.Disconnect(context.Background())

	accountHandler := handlers.NewAccountHandler(client)
	transferHandler := handlers.NewTransferHandler(client)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /accounts", accountHandler.CreateAccount)
	mux.HandleFunc("GET /accounts/{id}", accountHandler.GetAccount)
	mux.HandleFunc("POST /transfers", transferHandler.CreateTransfer)

	server := httptest.NewServer(mux)
	defer server.Close()

	const startingBalance = int64(1_000_000)
	aliceID := createAccount(t, server.URL, "StressAlice", startingBalance)
	bobID := createAccount(t, server.URL, "StressBob", 0)

	const numTransfers = 50
	const amountPerTransfer = int64(100)

	var wg sync.WaitGroup
	var successCount int64
	var failureCount int64

	for i := 0; i < numTransfers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

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

	wg.Wait()

	t.Logf("transfers: %d succeeded, %d failed", successCount, failureCount)

	if failureCount > 0 {
		t.Fatalf("expected all %d transfers to succeed, but %d failed", numTransfers, failureCount)
	}

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

	totalBefore := startingBalance
	totalAfter := aliceBalance + bobBalance
	if totalAfter != totalBefore {
		t.Fatalf("money was created or destroyed: total before = %d, total after = %d", totalBefore, totalAfter)
	}
}

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

var _ = time.Second
