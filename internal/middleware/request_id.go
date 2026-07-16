package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
)

type contextKey string

const requestIDKey contextKey = "requestID"

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.NewString()
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		r = r.WithContext(ctx)

		slog.Info("request started",
			"request_id", id,
			"method", r.Method,
			"path", r.URL.Path,
		)
		next.ServeHTTP(w, r)
		slog.Info("request finished",
			"request_id", id,
			"method", r.Method,
			"path", r.URL.Path,
		)
	})
}

func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}
