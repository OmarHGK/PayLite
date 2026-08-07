package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/OmarHGK/paylite/internal/config"
	"github.com/OmarHGK/paylite/internal/db"
	"github.com/OmarHGK/paylite/internal/handlers"
)

func main() {
	client := db.Connect("mongodb://localhost:27017")

	stripe.Key = cfg.StripeSecretKey

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	client := db.Connect(cfg.MongoURI)

	accountHandler := handlers.NewAccountHandler(client)
	transferHandler := handlers.NewTransferHandler(client)
	paymentHandler := handlers.NewPaymentHandler(client, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("POST /accounts", accountHandler.CreateAccount)
	mux.HandleFunc("GET /accounts/{id}", accountHandler.GetAccount)
	mux.HandleFunc("POST /transfers", transferHandler.CreateTransfer)

	log.Println("starting server on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
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
