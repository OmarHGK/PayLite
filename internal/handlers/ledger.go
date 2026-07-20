package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/OmarHGK/paylite/internal/models"
)

type LedgerHandler struct {
	LedgerCollection *mongo.Collection
}

func NewLedgerHandler(client *mongo.Client) *LedgerHandler {
	return &LedgerHandler{
		LedgerCollection: client.Database("paylite").Collection("ledger_entries"),
	}
}

func (h *LedgerHandler) GetAccountStatement(w http.ResponseWriter, r *http.Request) {
	rawID := r.PathValue("id")

	accountID, err := bson.ObjectIDFromHex(rawID)
	if err != nil {
		http.Error(w, "Invalid account ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	filter := bson.M{"account_id": accountID}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}})

	cursor, err := h.LedgerCollection.Find(ctx, filter, opts)
	if err != nil {
		http.Error(w, "Failed to query ledger", http.StatusInternalServerError)
		return
	}
	defer cursor.Close(ctx)
	var entries []models.LedgerEntry
	if err := cursor.All(ctx, &entries); err != nil {
		http.Error(w, "Failed to decode ledger entries", http.StatusInternalServerError)
		return
	}

	if entries == nil {
		entries = []models.LedgerEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func (h *LedgerHandler) GetTransferEntries(w http.ResponseWriter, r *http.Request) {
	rawID := r.PathValue("id")

	transferID, err := bson.ObjectIDFromHex(rawID)
	if err != nil {
		http.Error(w, "Invalid transfer ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	filter := bson.M{"transfer_id": transferID}

	cursor, err := h.LedgerCollection.Find(ctx, filter)
	if err != nil {
		http.Error(w, "Failed to query ledger", http.StatusInternalServerError)
		return
	}
	defer cursor.Close(ctx)

	var entries []models.LedgerEntry
	if err := cursor.All(ctx, &entries); err != nil {
		http.Error(w, "Failed to decode ledger entries", http.StatusInternalServerError)
		return
	}

	if len(entries) == 0 {
		http.Error(w, "No ledger entries found for this transfer", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}
