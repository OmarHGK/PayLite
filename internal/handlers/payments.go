package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/OmarHGK/paylite/internal/models"
	"github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/paymentintent"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type PaymentHandler struct {
	AccountCollection *mongo.Collection
	PaymentCollection *mongo.Collection
	Logger            *slog.Logger
}

func NewPaymentHandler(client *mongo.Client, logger *slog.Logger) *PaymentHandler {
	return &PaymentHandler{
		AccountCollection: client.Database("paylite").Collection("accounts"),
		PaymentCollection: client.Database("paylite").Collection("payments"),
		Logger:            logger,
	}
}

type CreatePaymentRequest struct {
	AccountID   string `json:"account_id"`   // hex string of a MongoDB ObjectID
	AmountCents int64  `json:"amount_cents"` // e.g. 5000 = $50.00
	Currency    string `json:"currency"`     // e.g. "usd"
}

type CreatePaymentResponse struct {
	PaymentID    string `json:"payment_id"`
	ClientSecret string `json:"client_secret"`
	AmountCents  int64  `json:"amount_cents"`
	Currency     string `json:"currency"`
	Status       string `json:"status"`
}

type ConfirmPaymentRequest struct {
	PaymentID      string `json:"payment_id"`
	StripeIntentID string `json:"stripe_intent_id"`
}

type ConfirmPaymentResponse struct {
	Success      bool   `json:"success"`
	PaymentID    string `json:"payment_id"`
	Status       string `json:"status"`
	BalanceCents int64  `json:"balance_cents"`
	Message      string `json:"message"`
}

func (h *PaymentHandler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	requestID, _ := r.Context().Value("request_id").(string)
	logger := h.Logger.With("request_id", requestID, "endpoint", "CreatePayment")

	var req CreatePaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Error("failed to decode request body", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.AmountCents <= 0 {
		http.Error(w, "amount_cents must be greater than 0", http.StatusBadRequest)
		return
	}
	if req.Currency == "" {
		req.Currency = "usd"
	}

	accountID, err := bson.ObjectIDFromHex(req.AccountID)
	if err != nil {
		logger.Error("invalid account_id", "account_id", req.AccountID)
		http.Error(w, "Invalid account_id format", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var account models.Account
	err = h.AccountCollection.FindOne(ctx, bson.M{"_id": accountID}).Decode(&account)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			http.Error(w, "Account not found", http.StatusNotFound)
			return
		}
		logger.Error("database error looking up account", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	logger = logger.With("account_id", accountID.Hex())

	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(req.AmountCents),
		Currency: stripe.String(req.Currency),
		AutomaticPaymentMethods: &stripe.PaymentIntentAutomaticPaymentMethodsParams{
			Enabled:        stripe.Bool(true),
			AllowRedirects: stripe.String("never"), // disables redirect-based methods like Klarna
		},
		Metadata: map[string]string{
			"account_id":   accountID.Hex(),
			"account_name": account.OwnerName,
		},
	}

	ctx, cancel = context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	pi, err := paymentintent.New(params)
	if err != nil {
		logger.Error("failed to create Stripe Payment Intent", "error", err)
		http.Error(w, "Failed to create payment", http.StatusInternalServerError)
		return
	}

	logger = logger.With("stripe_intent_id", pi.ID)
	logger.Info("Stripe Payment Intent created", "amount_cents", req.AmountCents)

	now := time.Now()
	payment := models.Payment{
		AccountID:       accountID,
		StripePaymentID: pi.ID,
		AmountCents:     req.AmountCents,
		Currency:        req.Currency,
		Status:          "processing",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	ctx, cancel = context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	result, err := h.PaymentCollection.InsertOne(ctx, payment)
	if err != nil {
		logger.Error("failed to insert payment record", "error", err)
		http.Error(w, "Failed to store payment", http.StatusInternalServerError)
		return
	}

	payment.ID = result.InsertedID.(bson.ObjectID)
	logger.Info("payment record inserted", "payment_id", payment.ID.Hex())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(CreatePaymentResponse{
		PaymentID:    payment.ID.Hex(),
		ClientSecret: pi.ClientSecret,
		AmountCents:  req.AmountCents,
		Currency:     req.Currency,
		Status:       string(pi.Status),
	})
}

func (h *PaymentHandler) ConfirmPayment(w http.ResponseWriter, r *http.Request) {
	requestID, _ := r.Context().Value("request_id").(string)
	logger := h.Logger.With("request_id", requestID, "endpoint", "ConfirmPayment")

	var req ConfirmPaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Error("failed to decode request body", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	paymentID, err := bson.ObjectIDFromHex(req.PaymentID)
	if err != nil {
		http.Error(w, "Invalid payment_id format", http.StatusBadRequest)
		return
	}

	logger = logger.With("payment_id", paymentID.Hex(), "stripe_intent_id", req.StripeIntentID)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var payment models.Payment
	err = h.PaymentCollection.FindOne(ctx, bson.M{"_id": paymentID}).Decode(&payment)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			http.Error(w, "Payment not found", http.StatusNotFound)
			return
		}
		logger.Error("database error retrieving payment", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if payment.StripePaymentID != req.StripeIntentID {
		logger.Error("stripe_intent_id mismatch — possible tampering",
			"stored", payment.StripePaymentID, "received", req.StripeIntentID)
		http.Error(w, "Payment intent ID mismatch", http.StatusBadRequest)
		return
	}

	ctx, cancel = context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	pi, err := paymentintent.Get(req.StripeIntentID, nil)
	if err != nil {
		logger.Error("failed to fetch Payment Intent from Stripe", "error", err)
		http.Error(w, "Failed to verify payment with Stripe", http.StatusInternalServerError)
		return
	}

	logger.Info("Payment Intent status from Stripe", "status", pi.Status)

	if pi.Status != stripe.PaymentIntentStatusSucceeded {
		failureMsg := ""
		if pi.LastPaymentError != nil {
			failureMsg = pi.LastPaymentError.Msg
		}
		logger.Warn("payment not succeeded", "stripe_status", pi.Status, "failure", failureMsg)

		ctx, cancel = context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		h.PaymentCollection.UpdateOne(ctx, bson.M{"_id": paymentID}, bson.M{
			"$set": bson.M{"status": "failed", "failure_reason": failureMsg, "updated_at": time.Now()},
		})

		http.Error(w, fmt.Sprintf("Payment failed: %s", failureMsg), http.StatusPaymentRequired)
		return
	}

	ctx, cancel = context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	session, err := h.PaymentCollection.Database().Client().StartSession()
	if err != nil {
		logger.Error("failed to start MongoDB session", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sessCtx context.Context) (interface{}, error) {
		_, err := h.AccountCollection.UpdateOne(sessCtx,
			bson.M{"_id": payment.AccountID},
			bson.M{
				"$inc": bson.M{"balance_cents": payment.AmountCents},
				"$set": bson.M{"updated_at": time.Now()},
			},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to update account balance: %w", err)
		}

		_, err = h.PaymentCollection.UpdateOne(sessCtx,
			bson.M{"_id": paymentID},
			bson.M{"$set": bson.M{"status": "succeeded", "updated_at": time.Now()}},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to update payment status: %w", err)
		}

		return nil, nil
	})
	if err != nil {
		logger.Error("transaction failed", "error", err)
		http.Error(w, "Failed to finalise payment", http.StatusInternalServerError)
		return
	}

	logger.Info("balance credited and payment marked succeeded")

	ctx, cancel = context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var updated models.Account
	if err := h.AccountCollection.FindOne(ctx, bson.M{"_id": payment.AccountID}).Decode(&updated); err != nil {
		logger.Error("failed to fetch updated account", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ConfirmPaymentResponse{
		Success:      true,
		PaymentID:    payment.ID.Hex(),
		Status:       "succeeded",
		BalanceCents: updated.BalanceCents,
		Message:      fmt.Sprintf("Payment of %d cents successfully applied", payment.AmountCents),
	})

	logger.Info("payment confirmation complete", "new_balance_cents", updated.BalanceCents)
}
