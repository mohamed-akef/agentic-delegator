// core/adapter/keyhash/keyhash.go
//
// Package keyhash wraps an APIKeysRepository so that minted keys are bcrypt-
// hashed before they are persisted. This is the write-side counterpart to the
// resolver's bcrypt.CompareHashAndPassword on the read side: MintAPIKey builds
// a domain.APIKey whose Hash field carries the *plaintext* key, and this
// wrapper turns that into a bcrypt digest at the composition seam — mirroring
// how the secrets repo is AES-wrapped in the composition root.
package keyhash

import (
	"context"
	"time"

	"golang.org/x/crypto/bcrypt"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

// Repo wraps a ports.APIKeysRepository, bcrypting the Hash on Create and
// passing every other call straight through.
type Repo struct {
	inner ports.APIKeysRepository
}

// New returns a Repo wrapping inner.
func New(inner ports.APIKeysRepository) *Repo { return &Repo{inner: inner} }

var _ ports.APIKeysRepository = (*Repo)(nil)

func (r *Repo) Create(ctx context.Context, k *domain.APIKey) error {
	digest, err := bcrypt.GenerateFromPassword([]byte(k.Hash), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	clone := *k
	clone.Hash = domain.APIKeyHash(digest)
	return r.inner.Create(ctx, &clone)
}

func (r *Repo) GetByPrefix(ctx context.Context, prefix string) ([]*domain.APIKey, error) {
	return r.inner.GetByPrefix(ctx, prefix)
}

func (r *Repo) ListForUser(ctx context.Context, userID domain.UserID) ([]*domain.APIKey, error) {
	return r.inner.ListForUser(ctx, userID)
}

func (r *Repo) Delete(ctx context.Context, id domain.APIKeyID, userID domain.UserID) error {
	return r.inner.Delete(ctx, id, userID)
}

func (r *Repo) RecordUsed(ctx context.Context, id domain.APIKeyID, t time.Time) error {
	return r.inner.RecordUsed(ctx, id, t)
}
