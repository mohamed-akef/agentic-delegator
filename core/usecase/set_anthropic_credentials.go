// core/usecase/set_anthropic_credentials.go
package usecase

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type SetAnthropicCredentials struct {
	Secrets ports.SecretsRepository
}

type SetAnthropicCredentialsInput struct {
	UserID domain.UserID
	APIKey string
}

func (uc *SetAnthropicCredentials) Execute(ctx context.Context, in SetAnthropicCredentialsInput) error {
	if in.UserID == "" || in.APIKey == "" {
		return domain.ErrInvalidInput
	}
	return uc.Secrets.SetAnthropicCreds(ctx, in.UserID, domain.AnthropicCreds{APIKey: in.APIKey})
}
