package handlers_test

// This is an integration test file — it runs against a real MongoDB instance.
// It does NOT use mocks. This matches how your existing codebase is structured
// (real client, real collections, real transactions).
//
// PREREQUISITES:
//   - A running MongoDB replica set (required for transactions).
//     Your existing docker-compose.yml already provides this.
//
// HOW TO RUN:
//   # With docker-compose running:
//   MONGO_URI="mongodb://localhost:27017/?replicaSet=rs0" go test ./internal/handlers/... -v
//
//   # Run only ledger tests:
//   MONGO_URI="mongodb://localhost:27017/?replicaSet=rs0" go test ./internal/handlers/... -v -run TestLedger

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/OmarHGK/paylite/internal/db"
	"github.com/OmarHGK/paylite/internal/handlers"
	"github.com/OmarHGK/paylite/internal/models"
)

// ── Test helpers ──────────────────────────────────────────────────────────────

// testDB holds all the shared state for one test run.
// We create it once in TestMain and share it across all tests.
type testDB struct {
	client              *mongo.Client
	accountsCollection  *mongo.Collection
	transfersCollection *mongo.Collection
	ledgerCollection    *mongo.Collection
}

var tdb *testDB

// TestMain is the entry point Go calls before running any test in this package.
// We use it to:
//  1. Connect to MongoDB once (avoids reconnecting for every test).
//  2. Call InitCollections so the ledger_entries collection + indexes exist.
//  3. Run all tests.
//  4. Disconnect cleanly when done.
func TestMain(m *testing.M) {
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		// Default matches the URI your docker-compose replica set exposes.
		mongoURI = "mongodb://localhost:27017/?replicaSet=rs0"
	}

	client := db.Connect(mongoURI)
	db.InitCollections(client) // creates ledger_entries + indexes if not there yet

	paylite := client.Database("paylite")
	tdb = &testDB{
		client:              client,
		accountsCollection:  paylite.Collection("accounts"),
		transfersCollection: paylite.Collection("transfers"),
		ledgerCollection:    paylite.Collection("ledger_entries"),
	}

	// Run all tests and capture the exit code.
	code := m.Run()

	// Disconnect before exiting so the connection is closed cleanly.
	_ = client.Disconnect(context.Background())
	os.Exit(code)
}

// cleanCollections wipes accounts, transfers, and ledger_entries before each
// test so tests are fully isolated from one another.
// Call this at the top of every test with: defer cleanCollections(t)
func cleanCollections(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// DeleteMany with an empty filter deletes every document in the collection.
	_, _ = tdb.accountsCollection.DeleteMany(ctx, bson.M{})
	_, _ = tdb.transfersCollection.DeleteMany(ctx, bson.M{})
	_, _ = tdb.ledgerCollection.DeleteMany(ctx, bson.M{})
}

// createTestAccount inserts an account directly into MongoDB and returns its ID
// as a hex string. This is a helper — it bypasses the HTTP handler so tests
// don't depend on CreateAccount working correctly in order to test the ledger.
func createTestAccount(t *testing.T, ownerName string, balanceCents int64, currency string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	account := models.Account{
		OwnerName:    ownerName,
		BalanceCents: balanceCents,
		Currency:     currency,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	result, err := tdb.accountsCollection.InsertOne(ctx, account)
	if err != nil {
		t.Fatalf("createTestAccount: failed to insert account: %v", err)
	}

	return result.InsertedID.(bson.ObjectID).Hex()
}

// doTransfer fires POST /transfers via the real HTTP handler and returns the
// response recorder so callers can inspect the status code and body.
func doTransfer(t *testing.T, handler *handlers.TransferHandler, fromID, toID string, amountCents int64, idempotencyKey string) *httptest.ResponseRecorder {
	t.Helper()

	body := fmt.Sprintf(
		`{"from_account":%q,"to_account":%q,"amount_cents":%d,"idempotency_key":%q}`,
		fromID, toID, amountCents, idempotencyKey,
	)

	req := httptest.NewRequest(http.MethodPost, "/transfers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.CreateTransfer(rr, req)
	return rr
}

// fetchLedgerEntries returns all ledger entries that match the given filter.
func fetchLedgerEntries(t *testing.T, filter bson.M) []models.LedgerEntry {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := tdb.ledgerCollection.Find(ctx, filter)
	if err != nil {
		t.Fatalf("fetchLedgerEntries: Find failed: %v", err)
	}
	defer cursor.Close(ctx)

	var entries []models.LedgerEntry
	if err := cursor.All(ctx, &entries); err != nil {
		t.Fatalf("fetchLedgerEntries: cursor.All failed: %v", err)
	}
	return entries
}

// fetchAccount returns the current state of an account from MongoDB.
func fetchAccount(t *testing.T, idHex string) models.Account {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	objectID, err := bson.ObjectIDFromHex(idHex)
	if err != nil {
		t.Fatalf("fetchAccount: invalid ID %q: %v", idHex, err)
	}

	var account models.Account
	err = tdb.accountsCollection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&account)
	if err != nil {
		t.Fatalf("fetchAccount: FindOne failed: %v", err)
	}
	return account
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// Test 1: Every transfer must produce exactly 2 ledger entries.
//
// Why 2? Double-entry bookkeeping: one debit (money out of sender) and
// one credit (money into receiver). Never 1, never 3.
func TestLedger_TransferProducesTwoEntries(t *testing.T) {
	cleanCollections(t)

	fromID := createTestAccount(t, "Alice", 50000, "USD") // $500.00
	toID := createTestAccount(t, "Bob", 10000, "USD")     // $100.00

	handler := handlers.NewTransferHandler(tdb.client)
	rr := doTransfer(t, handler, fromID, toID, 10000, "key-test1")

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d — body: %s", rr.Code, rr.Body.String())
	}

	// Fetch ALL ledger entries (filter is empty = all documents).
	entries := fetchLedgerEntries(t, bson.M{})

	if len(entries) != 2 {
		t.Errorf("expected exactly 2 ledger entries, got %d", len(entries))
	}
}

// Test 2: The double-entry invariant — debit + credit must sum to zero.
//
// This is the mathematical heart of a ledger. If this breaks, money is
// being created or destroyed somewhere in the system.
func TestLedger_DebitAndCreditSumToZero(t *testing.T) {
	cleanCollections(t)

	fromID := createTestAccount(t, "Alice", 50000, "USD")
	toID := createTestAccount(t, "Bob", 10000, "USD")

	handler := handlers.NewTransferHandler(tdb.client)
	rr := doTransfer(t, handler, fromID, toID, 10000, "key-test2")

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d — body: %s", rr.Code, rr.Body.String())
	}

	entries := fetchLedgerEntries(t, bson.M{})
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d — cannot check invariant", len(entries))
	}

	// Sum all amount_cents values. For one transfer:
	//   debit  entry: -10000
	//   credit entry: +10000
	//   total:          0
	var total int64
	for _, e := range entries {
		total += e.AmountCents
	}

	if total != 0 {
		t.Errorf("double-entry invariant violated: sum of amount_cents = %d, want 0", total)
	}
}

// Test 3: balance_after_cents must reflect the real account balance
// at the time the ledger entry was written.
//
// This is what makes the ledger useful as a statement — each line shows
// the running balance, not just the movement amount.
func TestLedger_BalanceAfterCentsIsCorrect(t *testing.T) {
	cleanCollections(t)

	fromID := createTestAccount(t, "Alice", 50000, "USD") // starts at $500.00
	toID := createTestAccount(t, "Bob", 10000, "USD")     // starts at $100.00

	const transferAmount int64 = 15000 // $150.00

	handler := handlers.NewTransferHandler(tdb.client)
	rr := doTransfer(t, handler, fromID, toID, transferAmount, "key-test3")

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d — body: %s", rr.Code, rr.Body.String())
	}

	// Get the real post-transfer balances directly from the accounts collection.
	fromAccount := fetchAccount(t, fromID)
	toAccount := fetchAccount(t, toID)

	// Expected:
	//   Alice: 50000 - 15000 = 35000
	//   Bob:   10000 + 15000 = 25000
	if fromAccount.BalanceCents != 35000 {
		t.Errorf("Alice balance: got %d, want 35000", fromAccount.BalanceCents)
	}
	if toAccount.BalanceCents != 25000 {
		t.Errorf("Bob balance: got %d, want 25000", toAccount.BalanceCents)
	}

	// Now check that the ledger entry snapshots match the real balances.
	fromObjID, _ := bson.ObjectIDFromHex(fromID)
	toObjID, _ := bson.ObjectIDFromHex(toID)

	debitEntries := fetchLedgerEntries(t, bson.M{"account_id": fromObjID, "direction": "debit"})
	if len(debitEntries) != 1 {
		t.Fatalf("expected 1 debit entry for Alice, got %d", len(debitEntries))
	}
	if debitEntries[0].BalanceAfterCents != fromAccount.BalanceCents {
		t.Errorf("debit entry balance_after_cents: got %d, want %d",
			debitEntries[0].BalanceAfterCents, fromAccount.BalanceCents)
	}

	creditEntries := fetchLedgerEntries(t, bson.M{"account_id": toObjID, "direction": "credit"})
	if len(creditEntries) != 1 {
		t.Fatalf("expected 1 credit entry for Bob, got %d", len(creditEntries))
	}
	if creditEntries[0].BalanceAfterCents != toAccount.BalanceCents {
		t.Errorf("credit entry balance_after_cents: got %d, want %d",
			creditEntries[0].BalanceAfterCents, toAccount.BalanceCents)
	}
}

// Test 4: Idempotency — sending the same idempotency_key twice must not
// create duplicate ledger entries.
//
// A retry should return the original transfer and leave the ledger untouched.
// After two calls with the same key, we still expect exactly 2 entries total.
func TestLedger_IdempotencyNoDuplicateEntries(t *testing.T) {
	cleanCollections(t)

	fromID := createTestAccount(t, "Alice", 50000, "USD")
	toID := createTestAccount(t, "Bob", 10000, "USD")

	handler := handlers.NewTransferHandler(tdb.client)
	const key = "key-idempotent"

	// First request — should succeed and create the transfer + 2 ledger entries.
	rr1 := doTransfer(t, handler, fromID, toID, 10000, key)
	if rr1.Code != http.StatusCreated {
		t.Fatalf("first request: expected 201, got %d — body: %s", rr1.Code, rr1.Body.String())
	}

	// Second request with the same key — should be a no-op (200 OK, not 201).
	rr2 := doTransfer(t, handler, fromID, toID, 10000, key)
	if rr2.Code != http.StatusOK {
		t.Fatalf("second request: expected 200 (idempotent), got %d — body: %s", rr2.Code, rr2.Body.String())
	}

	// After both calls, we must still have exactly 2 ledger entries — not 4.
	entries := fetchLedgerEntries(t, bson.M{})
	if len(entries) != 2 {
		t.Errorf("after idempotent retry: expected 2 ledger entries, got %d", len(entries))
	}
}

// Test 5: Concurrent stress test — fire N transfers simultaneously and verify
// that the ledger remains consistent under concurrency.
//
// Assertions after all goroutines finish:
//
//	a) Total ledger entries == N * 2 (each transfer produced exactly 2)
//	b) Sum of all amount_cents == 0  (global double-entry invariant holds)
//	c) No account balance went negative (no overdraft snuck through)
func TestLedger_ConcurrentTransfersConsistency(t *testing.T) {
	cleanCollections(t)

	// Give Alice enough balance to survive N concurrent transfers without
	// going negative (N=10, $10 each = $100 total needed).
	const N = 10
	const amountEach int64 = 1000 // $10.00 per transfer

	fromID := createTestAccount(t, "Alice", int64(N)*amountEach*2, "USD") // 2x buffer
	toID := createTestAccount(t, "Bob", 0, "USD")

	handler := handlers.NewTransferHandler(tdb.client)

	// Launch N goroutines simultaneously, each doing one transfer.
	// A WaitGroup lets us block until all goroutines finish.
	var wg sync.WaitGroup
	wg.Add(N)

	for i := 0; i < N; i++ {
		i := i // capture loop variable so each goroutine gets its own copy
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("stress-key-%d", i)
			rr := doTransfer(t, handler, fromID, toID, amountEach, key)
			// We accept 201 (new transfer) or 200 (idempotent replay).
			// Any 4xx or 5xx is a failure.
			if rr.Code != http.StatusCreated && rr.Code != http.StatusOK {
				// t.Errorf is safe to call from goroutines (t.Fatalf is not).
				t.Errorf("goroutine %d: unexpected status %d — body: %s", i, rr.Code, rr.Body.String())
			}
		}()
	}

	wg.Wait() // block until all N transfers are done

	// ── Assertion a: exactly N*2 ledger entries ───────────────────────────
	entries := fetchLedgerEntries(t, bson.M{})
	if len(entries) != N*2 {
		t.Errorf("concurrent test: expected %d ledger entries, got %d", N*2, len(entries))
	}

	// ── Assertion b: global sum of amount_cents == 0 ──────────────────────
	// For every transfer: debit(-X) + credit(+X) = 0.
	// Summing across all N transfers: the total must still be 0.
	var globalSum int64
	for _, e := range entries {
		globalSum += e.AmountCents
	}
	if globalSum != 0 {
		t.Errorf("concurrent test: global sum of amount_cents = %d, want 0", globalSum)
	}

	// ── Assertion c: no account went negative ─────────────────────────────
	fromAccount := fetchAccount(t, fromID)
	toAccount := fetchAccount(t, toID)

	if fromAccount.BalanceCents < 0 {
		t.Errorf("concurrent test: Alice balance went negative: %d", fromAccount.BalanceCents)
	}
	if toAccount.BalanceCents < 0 {
		t.Errorf("concurrent test: Bob balance went negative: %d", toAccount.BalanceCents)
	}

	// Sanity check: money is conserved. Total across both accounts must equal
	// the original total (Alice's starting balance + Bob's starting balance).
	originalTotal := int64(N)*amountEach*2 + 0 // Alice start + Bob start
	actualTotal := fromAccount.BalanceCents + toAccount.BalanceCents
	if actualTotal != originalTotal {
		t.Errorf("concurrent test: money not conserved — original total %d, actual total %d",
			originalTotal, actualTotal)
	}
}

// ── Statement endpoint tests ───────────────────────────────────────────────────

// Test 6: GET /accounts/{id}/statement returns entries in chronological order.
//
// We make two transfers involving the same account and verify the statement
// comes back sorted oldest first with the correct number of entries.
func TestLedger_StatementEndpointReturnsChronologicalEntries(t *testing.T) {
	cleanCollections(t)

	aliceID := createTestAccount(t, "Alice", 100000, "USD") // $1000
	bobID := createTestAccount(t, "Bob", 50000, "USD")
	charlieID := createTestAccount(t, "Charlie", 0, "USD")

	transferHandler := handlers.NewTransferHandler(tdb.client)
	ledgerHandler := handlers.NewLedgerHandler(tdb.client)

	// Transfer 1: Alice → Bob
	rr := doTransfer(t, transferHandler, aliceID, bobID, 10000, "stmt-key-1")
	if rr.Code != http.StatusCreated {
		t.Fatalf("transfer 1: expected 201, got %d", rr.Code)
	}

	// Small sleep so created_at timestamps are distinct.
	time.Sleep(5 * time.Millisecond)

	// Transfer 2: Alice → Charlie
	rr = doTransfer(t, transferHandler, aliceID, charlieID, 5000, "stmt-key-2")
	if rr.Code != http.StatusCreated {
		t.Fatalf("transfer 2: expected 201, got %d", rr.Code)
	}

	// Hit the statement endpoint for Alice.
	req := httptest.NewRequest(http.MethodGet, "/accounts/"+aliceID+"/statement", nil)
	// httptest doesn't parse URL patterns, so we set PathValue manually.
	req.SetPathValue("id", aliceID)
	rr2 := httptest.NewRecorder()
	ledgerHandler.GetAccountStatement(rr2, req)

	if rr2.Code != http.StatusOK {
		t.Fatalf("statement endpoint: expected 200, got %d — body: %s", rr2.Code, rr2.Body.String())
	}

	// Decode the response body into a slice of LedgerEntry.
	var entries []models.LedgerEntry
	if err := decodeJSON(rr2, &entries); err != nil {
		t.Fatalf("failed to decode statement response: %v", err)
	}

	// Alice was the sender in both transfers → 2 debit entries on her statement.
	if len(entries) != 2 {
		t.Errorf("Alice's statement: expected 2 entries, got %d", len(entries))
	}

	// Entries must be sorted oldest first.
	if len(entries) == 2 && entries[0].CreatedAt.After(entries[1].CreatedAt) {
		t.Errorf("statement entries are not in chronological order")
	}
}

// Test 7: GET /transfers/{id}/entries returns exactly 2 entries for a transfer.
func TestLedger_TransferEntriesEndpoint(t *testing.T) {
	cleanCollections(t)

	fromID := createTestAccount(t, "Alice", 50000, "USD")
	toID := createTestAccount(t, "Bob", 0, "USD")

	transferHandler := handlers.NewTransferHandler(tdb.client)
	ledgerHandler := handlers.NewLedgerHandler(tdb.client)

	rr := doTransfer(t, transferHandler, fromID, toID, 10000, "entries-key-1")
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d — body: %s", rr.Code, rr.Body.String())
	}

	// Decode the transfer response to get the transfer ID.
	var transfer models.Transfer
	if err := decodeJSON(rr, &transfer); err != nil {
		t.Fatalf("failed to decode transfer response: %v", err)
	}

	transferIDHex := transfer.ID.Hex()

	// Hit GET /transfers/{id}/entries.
	req := httptest.NewRequest(http.MethodGet, "/transfers/"+transferIDHex+"/entries", nil)
	req.SetPathValue("id", transferIDHex)
	rr2 := httptest.NewRecorder()
	ledgerHandler.GetTransferEntries(rr2, req)

	if rr2.Code != http.StatusOK {
		t.Fatalf("entries endpoint: expected 200, got %d — body: %s", rr2.Code, rr2.Body.String())
	}

	var entries []models.LedgerEntry
	if err := decodeJSON(rr2, &entries); err != nil {
		t.Fatalf("failed to decode entries response: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("transfer entries endpoint: expected 2 entries, got %d", len(entries))
	}

	// One must be a debit, one a credit.
	directions := map[models.Direction]int{}
	for _, e := range entries {
		directions[e.Direction]++
	}
	if directions[models.DirectionDebit] != 1 || directions[models.DirectionCredit] != 1 {
		t.Errorf("expected 1 debit and 1 credit, got: %v", directions)
	}
}

// ── Utility ───────────────────────────────────────────────────────────────────

// decodeJSON decodes the JSON body of an httptest.ResponseRecorder into dst.
func decodeJSON(rr *httptest.ResponseRecorder, dst interface{}) error {
	return json.NewDecoder(rr.Body).Decode(dst)
}

// countDocuments returns how many documents match filter in the given collection.
// Kept here as a utility in case you want to add more assertions later.
func countDocuments(t *testing.T, coll *mongo.Collection, filter bson.M) int64 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		t.Fatalf("countDocuments: %v", err)
	}
	return count
}
