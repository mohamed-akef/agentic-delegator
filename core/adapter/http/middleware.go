// core/adapter/http/middleware.go
package http

import (
	"context"
	"net/http"
	"strings"

	"agentic-delegator/core/domain"
)

type contextKey int

// UserIDKey is the exported context key for the authenticated user ID.
// Exported so that tests can inject a user ID directly into the context.
const UserIDKey contextKey = 1

// UserResolver looks up a user from an HTTP request. Editions implement this;
// for selfhost it's the admin, for SaaS it's a session or bearer-key lookup.
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
