package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/OmarHGK/paylite/internal/db"
	"github.com/OmarHGK/paylite/internal/handlers"
	"github.com/OmarHGK/paylite/internal/middleware"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}
	client := db.Connect(mongoURI)

	accountHandler := handlers.NewAccountHandler(client)
	transferHandler := handlers.NewTransferHandler(client)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("POST /accounts", accountHandler.CreateAccount)
	mux.HandleFunc("GET /accounts/{id}", accountHandler.GetAccount)
	mux.HandleFunc("POST /transfers", transferHandler.CreateTransfer)

	wrappedMux := middleware.RequestID(mux)

	server := &http.Server{
		Addr:    ":8080",
		Handler: wrappedMux,
	}

	// Run the server in its own goroutine, so main() can continue on to
	// wait for a shutdown signal without blocking here.
	go func() {
		slog.Info("starting server", "port", 8080)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
		}
	}()

	// Block here until we receive SIGINT (Ctrl+C) or SIGTERM (what
	// docker stop sends to a container's main process).
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	slog.Info("shutdown signal received, starting graceful shutdown")

	// Give in-flight requests up to 10 seconds to finish before giving up.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
	}

	if err := client.Disconnect(shutdownCtx); err != nil {
		slog.Error("failed to disconnect from mongo", "error", err)
	}

	slog.Info("shutdown complete")
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
