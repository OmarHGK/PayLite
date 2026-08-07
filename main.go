package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/OmarHGK/paylite/internal/config"
	"github.com/OmarHGK/paylite/internal/db"
	"github.com/OmarHGK/paylite/internal/handlers"
	"github.com/stripe/stripe-go/v79"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	stripe.Key = cfg.StripeSecretKey

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	client := db.Connect(cfg.MongoURI)
	db.InitCollections(client)

	accountHandler := handlers.NewAccountHandler(client)
	transferHandler := handlers.NewTransferHandler(client)
	paymentHandler := handlers.NewPaymentHandler(client, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("POST /accounts", accountHandler.CreateAccount)
	mux.HandleFunc("GET /accounts/{id}", accountHandler.GetAccount)
	mux.HandleFunc("POST /transfers", transferHandler.CreateTransfer)
	mux.HandleFunc("POST /payments", paymentHandler.CreatePayment)
	mux.HandleFunc("POST /payments/confirm", paymentHandler.ConfirmPayment)

	handler := requestIDMiddleware(mux)

	log.Printf("starting server on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, handler); err != nil {
		log.Fatal(err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = "no-id"
		}
		ctx := context.WithValue(r.Context(), "request_id", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
