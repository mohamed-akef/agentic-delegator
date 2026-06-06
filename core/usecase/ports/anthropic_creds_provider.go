// core/usecase/ports/anthropic_creds_provider.go
package ports

import (
	"context"

	"agentic-delegator/core/domain"
)

// AnthropicCredentialsProvider returns the Anthropic credential to pass into
// the runner. Today it reads from the SecretsRepository, but the abstraction
// lets us add OAuth-style sources later.
type AnthropicCredentialsProvider interface {
	For(ctx context.Context, userID domain.UserID) (domain.AnthropicCreds, error)
}
