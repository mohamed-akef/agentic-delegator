// core/adapter/http/auth/resolver.go
package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

// SessionResolver is how we resolve a cookie-borne session.
type SessionResolver interface {
	UserFromRequest(r *http.Request) (domain.UserID, error)
}

type Resolver struct {
	sessions SessionResolver
	keys     ports.APIKeysRepository
}

func NewResolver(sessions SessionResolver, keys ports.APIKeysRepository) *Resolver {
	return &Resolver{sessions: sessions, keys: keys}
}

func (r *Resolver) Resolve(req *http.Request) (domain.UserID, error) {
	// 1) Try bearer key (used by the skill from CLI)
	if h := req.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		uid, err := r.resolveBearer(req.Context(), strings.TrimPrefix(h, "Bearer "))
		if err == nil {
			return uid, nil
		}
	}
	// 2) Try session cookie (used by the dashboard)
	if uid, err := r.sessions.UserFromRequest(req); err == nil {
		return uid, nil
	}
	return "", errors.New("no valid auth")
}

func (r *Resolver) resolveBearer(ctx context.Context, plain string) (domain.UserID, error) {
	if len(plain) < 8 {
		return "", errors.New("short")
	}
	prefix := plain[:8]
	candidates, err := r.keys.GetByPrefix(ctx, prefix)
	if err != nil {
		return "", err
	}
	for _, k := range candidates {
		if err := bcrypt.CompareHashAndPassword([]byte(k.Hash), []byte(plain)); err == nil {
			_ = r.keys.RecordUsed(ctx, k.ID, time.Now().UTC())
			return k.UserID, nil
		}
	}
	return "", errors.New("no match")
}
