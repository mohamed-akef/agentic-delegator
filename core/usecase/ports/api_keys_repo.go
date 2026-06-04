// core/usecase/ports/api_keys_repo.go
package ports

import (
	"context"
	"time"

	"agentic-delegator/core/domain"
)

type APIKeysRepository interface {
	Create(ctx context.Context, k *domain.APIKey) error

	// GetByPrefix returns all keys with this prefix (typically the first 8
	// chars of the plaintext). Caller bcrypt-checks each candidate's Hash
	// against the supplied plaintext to find a match.
	GetByPrefix(ctx context.Context, prefix string) ([]*domain.APIKey, error)

	ListForUser(ctx context.Context, userID domain.UserID) ([]*domain.APIKey, error)

	// Delete removes the key iff it belongs to userID. Otherwise ErrNotFound.
	Delete(ctx context.Context, id domain.APIKeyID, userID domain.UserID) error

	RecordUsed(ctx context.Context, id domain.APIKeyID, at time.Time) error
}
