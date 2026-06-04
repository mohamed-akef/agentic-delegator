// core/runtime/selfhost/anthropic_creds.go
package selfhost

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type AnthropicCredsProvider struct {
	secrets ports.SecretsRepository
}

func NewAnthropicCredsProvider(secrets ports.SecretsRepository) *AnthropicCredsProvider {
	return &AnthropicCredsProvider{secrets: secrets}
}

func (p *AnthropicCredsProvider) For(ctx context.Context, userID domain.UserID) (domain.AnthropicCreds, error) {
	return p.secrets.GetAnthropicCreds(ctx, userID)
}
