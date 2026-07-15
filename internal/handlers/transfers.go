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
}

func NewTransferHandler(client *mongo.Client) *TransferHandler {
	db := client.Database("paylite")
	return &TransferHandler{
		Client:              client,
		AccountsCollection:  db.Collection("accounts"),
		TransfersCollection: db.Collection("transfers"),
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

	result, err := session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (interface{}, error) {
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

		transfer.ID = insertResult.InsertedID.(bson.ObjectID)
		return transfer, nil
	})

	if err != nil {
		if errors.Is(err, errInsufficientFunds) {
			http.Error(w, "Insufficient funds", http.StatusBadRequest)
			return
		}
		http.Error(w, "Failed to create transfer", http.StatusInternalServerError)
		return
	}

	transfer := result.(models.Transfer)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(transfer)
}
