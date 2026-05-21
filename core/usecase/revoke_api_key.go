// core/usecase/revoke_api_key.go
package usecase

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type RevokeAPIKey struct {
	Keys ports.APIKeysRepository
}

type RevokeAPIKeyInput struct {
	ID     domain.APIKeyID
	UserID domain.UserID
}

func (uc *RevokeAPIKey) Execute(ctx context.Context, in RevokeAPIKeyInput) error {
	if in.ID == "" || in.UserID == "" {
		return domain.ErrInvalidInput
	}
	return uc.Keys.Delete(ctx, in.ID, in.UserID)
}
