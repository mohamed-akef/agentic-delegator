// core/adapter/http/middleware.go
package http

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"

	"agentic-delegator/core/domain"
)

// RequestLogger emits one structured log line per request with method, path,
// status, duration, and the chi request ID for correlation.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", chimw.GetReqID(r.Context()),
		)
	})
}

type contextKey int

// UserIDKey is the exported context key for the authenticated user ID.
// Exported so that tests can inject a user ID directly into the context.
const UserIDKey contextKey = 1

// UserResolver looks up a user from an HTTP request via session cookie or
// bearer API key.
type UserResolver interface {
	Resolve(r *http.Request) (domain.UserID, error)
}

// BearerOrSession populates a UserID in the request context. On failure, it
// short-circuits with 401.
func BearerOrSession(resolver UserResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uid, err := resolver.Resolve(r)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), UserIDKey, uid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserFromContext returns the userID set by BearerOrSession.
func UserFromContext(ctx context.Context) (domain.UserID, bool) {
	v, ok := ctx.Value(UserIDKey).(domain.UserID)
	return v, ok
}

// BearerToken extracts the value after "Bearer " in the Authorization header.
func BearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(h, "Bearer ")
}
