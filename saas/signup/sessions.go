//go:build saas

// saas/signup/sessions.go
package signup

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"time"

	"agentic-delegator/core/domain"
)

// SessionStore is the slice of saas/store.SessionsRepo we need.
type SessionStore interface {
	Create(ctx context.Context, id []byte, userID domain.UserID, expires time.Time) error
	ResolveUser(ctx context.Context, id []byte) (domain.UserID, error)
	Delete(ctx context.Context, id []byte) error
}

const (
	cookieName   = "agdsess"
	cookieMaxAge = 30 * 24 * time.Hour
)

type Sessions struct {
	store SessionStore
}

func NewSessions(store SessionStore) *Sessions { return &Sessions{store: store} }

// Login mints a session, sets the cookie, and persists the session record.
func (s *Sessions) Login(ctx context.Context, w http.ResponseWriter, userID domain.UserID) error {
	id := make([]byte, 32)
	if _, err := rand.Read(id); err != nil {
		return err
	}
	expires := time.Now().Add(cookieMaxAge).UTC()
	if err := s.store.Create(ctx, id, userID, expires); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    encodeCookieValue(id),
		Expires:  expires,
		MaxAge:   int(cookieMaxAge.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   false, // set to true behind TLS via deploy config
		Path:     "/",
	})
	return nil
}

func (s *Sessions) Logout(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	c, err := r.Cookie(cookieName)
	if err == nil {
		if id, ok := decodeCookieValue(c.Value); ok {
			_ = s.store.Delete(ctx, id)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", MaxAge: -1, Path: "/"})
	return nil
}

func (s *Sessions) UserFromRequest(r *http.Request) (domain.UserID, error) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return "", errors.New("no session cookie")
	}
	id, ok := decodeCookieValue(c.Value)
	if !ok {
		return "", errors.New("invalid session cookie")
	}
	return s.store.ResolveUser(r.Context(), id)
}

func encodeCookieValue(id []byte) string {
	return hexEncode(id)
}

func decodeCookieValue(v string) ([]byte, bool) {
	b, err := hexDecode(v)
	if err != nil {
		return nil, false
	}
	return b, len(b) == 32
}
