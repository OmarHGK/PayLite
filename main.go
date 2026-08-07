package main

import (
	"encoding/json"
	"log"
<<<<<<< HEAD
<<<<<<< HEAD
	"log/slog"
	"net/http"
	"os"
=======
	"net/http"
>>>>>>> main
=======
	"net/http"
>>>>>>> df0ac3420447ee516647febebc9116964d3aa784

	"github.com/OmarHGK/paylite/internal/config"
	"github.com/OmarHGK/paylite/internal/db"
	"github.com/OmarHGK/paylite/internal/handlers"
<<<<<<< HEAD
<<<<<<< HEAD
	"github.com/stripe/stripe-go/v79"
=======
>>>>>>> df0ac3420447ee516647febebc9116964d3aa784
)

func main() {
	client := db.Connect("mongodb://localhost:27017")

	stripe.Key = cfg.StripeSecretKey

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	client := db.Connect(cfg.MongoURI)
<<<<<<< HEAD
=======
)

func main() {
	client := db.Connect("mongodb://localhost:27017")
>>>>>>> main
=======
>>>>>>> df0ac3420447ee516647febebc9116964d3aa784

	accountHandler := handlers.NewAccountHandler(client)
	transferHandler := handlers.NewTransferHandler(client)
	paymentHandler := handlers.NewPaymentHandler(client, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("POST /accounts", accountHandler.CreateAccount)
	mux.HandleFunc("GET /accounts/{id}", accountHandler.GetAccount)
	mux.HandleFunc("POST /transfers", transferHandler.CreateTransfer)

<<<<<<< HEAD
<<<<<<< HEAD
	mux.HandleFunc("POST /payments", paymentHandler.CreatePayment)
	mux.HandleFunc("POST /payments/confirm", paymentHandler.ConfirmPayment)

	handler := requestIDMiddleware(mux)

	log.Printf("starting server on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, handler); err != nil {
=======
	log.Println("starting server on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
>>>>>>> main
=======
	log.Println("starting server on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
>>>>>>> df0ac3420447ee516647febebc9116964d3aa784
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
