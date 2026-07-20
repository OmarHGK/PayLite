package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/OmarHGK/paylite/internal/models"
)

type TransferHandler struct {
	Client              *mongo.Client
	AccountsCollection  *mongo.Collection
	TransfersCollection *mongo.Collection
	LedgerCollection    *mongo.Collection
}

func NewTransferHandler(client *mongo.Client) *TransferHandler {
	paylite := client.Database("paylite")
	return &TransferHandler{
		Client:              client,
		AccountsCollection:  paylite.Collection("accounts"),
		TransfersCollection: paylite.Collection("transfers"),
		LedgerCollection:    paylite.Collection("ledger_entries"),
	}
}

type CreateTransferRequest struct {
	FromAccount    string `json:"from_account"`
	ToAccount      string `json:"to_account"`
	AmountCents    int64  `json:"amount_cents"`
	IdempotencyKey string `json:"idempotency_key"`
}

var errInsufficientFunds = errors.New("insufficient funds")

func (h *TransferHandler) CreateTransfer(w http.ResponseWriter, r *http.Request) {
	var req CreateTransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	fromID, err := bson.ObjectIDFromHex(req.FromAccount)
	if err != nil {
		http.Error(w, "Invalid from_account ID", http.StatusBadRequest)
		return
	}

	toID, err := bson.ObjectIDFromHex(req.ToAccount)
	if err != nil {
		http.Error(w, "Invalid to_account ID", http.StatusBadRequest)
		return
	}

	if req.AmountCents <= 0 {
		http.Error(w, "amount_cents must be positive", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var existing models.Transfer
	err = h.TransfersCollection.FindOne(ctx, bson.M{"idempotency_key": req.IdempotencyKey}).Decode(&existing)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(existing)
		return
	}

	if !errors.Is(err, mongo.ErrNoDocuments) {
		http.Error(w, "Failed to check idempotency key", http.StatusInternalServerError)
		return
	}

	session, err := h.Client.StartSession()
	if err != nil {
		http.Error(w, "Failed to start session", http.StatusInternalServerError)
		return
	}
	defer session.EndSession(ctx)

	result, err := session.WithTransaction(ctx, func(sessCtx context.Context) (interface{}, error) {
		now := time.Now()

		debitFilter := bson.M{
			"_id":           fromID,
			"balance_cents": bson.M{"$gte": req.AmountCents},
		}
		debitUpdate := bson.M{
			"$inc": bson.M{"balance_cents": -req.AmountCents},
			"$set": bson.M{"updated_at": now},
		}
		debitResult, err := h.AccountsCollection.UpdateOne(sessCtx, debitFilter, debitUpdate)
		if err != nil {
			return nil, err
		}
		if debitResult.MatchedCount == 0 {
			return nil, errInsufficientFunds
		}
		creditFilter := bson.M{"_id": toID}
		creditUpdate := bson.M{
			"$inc": bson.M{"balance_cents": req.AmountCents},
			"$set": bson.M{"updated_at": now},
		}
		creditResult, err := h.AccountsCollection.UpdateOne(sessCtx, creditFilter, creditUpdate)
		if err != nil {
			return nil, err
		}
		if creditResult.MatchedCount == 0 {
			return nil, errors.New("to_account not found")
		}

		var fromAccount, toAccount models.Account

		if err := h.AccountsCollection.FindOne(sessCtx, bson.M{"_id": fromID}).Decode(&fromAccount); err != nil {
			return nil, err
		}
		if err := h.AccountsCollection.FindOne(sessCtx, bson.M{"_id": toID}).Decode(&toAccount); err != nil {
			return nil, err
		}

		transfer := models.Transfer{
			FromAccountID:  fromID,
			ToAccountID:    toID,
			AmountCents:    req.AmountCents,
			Status:         "completed",
			IdempotencyKey: req.IdempotencyKey,
			CreatedAt:      now,
		}

		insertResult, err := h.TransfersCollection.InsertOne(sessCtx, transfer)
		if err != nil {
			return nil, err
		}

		transferID := insertResult.InsertedID.(bson.ObjectID)
		transfer.ID = transferID

		debitEntry := models.LedgerEntry{
			TransferID:        transferID,
			AccountID:         fromID,
			Direction:         models.DirectionDebit,
			AmountCents:       -req.AmountCents,
			Currency:          fromAccount.Currency,
			BalanceAfterCents: fromAccount.BalanceCents,
			CreatedAt:         now,
		}

		creditEntry := models.LedgerEntry{
			TransferID:        transferID,
			AccountID:         toID,
			Direction:         models.DirectionCredit,
			AmountCents:       req.AmountCents,
			Currency:          toAccount.Currency,
			BalanceAfterCents: toAccount.BalanceCents,
			CreatedAt:         now,
		}

		_, err = h.LedgerCollection.InsertMany(sessCtx, []interface{}{debitEntry, creditEntry})
		if err != nil {
			return nil, err
		}

		return transfer, nil
	})

	if err != nil {
		if errors.Is(err, errInsufficientFunds) {
			http.Error(w, "Insufficient funds", http.StatusUnprocessableEntity)
			return
		}

		if mongo.IsDuplicateKeyError(err) {
			var existing models.Transfer
			ferr := h.TransfersCollection.FindOne(ctx, bson.M{"idempotency_key": req.IdempotencyKey}).Decode(&existing)
			if ferr == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(existing)
				return
			}
		}

		http.Error(w, "Failed to create transfer", http.StatusInternalServerError)
		return
	}

	transfer := result.(models.Transfer)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(transfer)
}
