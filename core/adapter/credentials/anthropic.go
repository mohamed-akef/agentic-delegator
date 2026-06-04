// core/adapter/credentials/anthropic.go
package credentials

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

// AnthropicCredsProvider yields a user's Anthropic credential from the secrets
// store. It implements ports.AnthropicCredentialsProvider.
type AnthropicCredsProvider struct {
	secrets ports.SecretsRepository
}

func NewAnthropicCredsProvider(secrets ports.SecretsRepository) *AnthropicCredsProvider {
	return &AnthropicCredsProvider{secrets: secrets}
}

func (p *AnthropicCredsProvider) For(ctx context.Context, userID domain.UserID) (domain.AnthropicCreds, error) {
	return p.secrets.GetAnthropicCreds(ctx, userID)
}
