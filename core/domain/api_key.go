// core/domain/api_key.go
package domain

import "time"

type APIKeyID string

// APIKeyHash is an opaque hash (typically bcrypt) of the plaintext key.
// The plaintext is never stored — only this hash and the prefix.
type APIKeyHash []byte

type APIKey struct {
	ID         APIKeyID
	UserID     UserID
	Name       string
	Prefix     string // first 8 chars of the plaintext key, kept for UI lookups
	Hash       APIKeyHash
	LastUsedAt *time.Time
	CreatedAt  time.Time
}

func NewAPIKey(id APIKeyID, userID UserID, name, prefix string, hash APIKeyHash, now time.Time) *APIKey {
	return &APIKey{
		ID:        id,
		UserID:    userID,
		Name:      name,
		Prefix:    prefix,
		Hash:      hash,
		CreatedAt: now,
	}
}

func (k *APIKey) RecordUsed(now time.Time) {
	t := now
	k.LastUsedAt = &t
}
