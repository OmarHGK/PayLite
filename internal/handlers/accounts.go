package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"errors"

	"github.com/OmarHGK/paylite/internal/models"
)

type AccountHandler struct {
	Collection *mongo.Collection
}

func NewAccountHandler(client *mongo.Client) *AccountHandler {
	collection := client.Database("paylite").Collection("accounts")
	return &AccountHandler{
		Collection: collection,
	}
}

type CreateAccountRequest struct {
	OwnerName    string `json:"owner_name"`
	Currency     string `json:"currency"`
	BalanceCents int64  `json:"balance_cents"`
}

func (h *AccountHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	now := time.Now()
	account := models.Account{
		OwnerName:    req.OwnerName,
		Currency:     req.Currency,
		BalanceCents: req.BalanceCents,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	result, err := h.Collection.InsertOne(ctx, account)
	if err != nil {
		http.Error(w, "Failed to create account", http.StatusInternalServerError)
		return
	}
	account.ID = result.InsertedID.(bson.ObjectID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(account)
}
func (h *AccountHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
	idParam := r.PathValue("id")

	objectID, err := bson.ObjectIDFromHex(idParam)
	if err != nil {
		http.Error(w, "invalid account id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var account models.Account
	err = h.Collection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&account)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			http.Error(w, "account not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to fetch account", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(account)
}
