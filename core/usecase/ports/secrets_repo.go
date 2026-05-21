// core/usecase/ports/secrets_repo.go
package ports

import (
	"context"

	"agentic-delegator/core/domain"
)

// SecretsRepository stores per-user Anthropic credentials (encrypted at
// rest by the adapter). The interface returns plaintext value objects;
// encryption happens inside the adapter.
type SecretsRepository interface {
	SetAnthropicCreds(ctx context.Context, userID domain.UserID, creds domain.AnthropicCreds) error
	GetAnthropicCreds(ctx context.Context, userID domain.UserID) (domain.AnthropicCreds, error)
	DeleteAnthropicCreds(ctx context.Context, userID domain.UserID) error
}
