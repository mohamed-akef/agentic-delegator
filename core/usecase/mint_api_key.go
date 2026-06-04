// core/usecase/mint_api_key.go
package usecase

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type MintAPIKey struct {
	Keys  ports.APIKeysRepository
	IDGen ports.IDGenerator
	Clock ports.Clock
}

type MintAPIKeyInput struct {
	UserID domain.UserID
	Name   string
}

type MintAPIKeyOutput struct {
	Key       *domain.APIKey
	Plaintext string // only returned once — caller must show user and discard
}

func (uc *MintAPIKey) Execute(ctx context.Context, in MintAPIKeyInput) (*MintAPIKeyOutput, error) {
	if in.UserID == "" || in.Name == "" {
		return nil, domain.ErrInvalidInput
	}

	plain, prefix := uc.IDGen.NewAPIKeyPlaintext()
	// For MVP, store the plaintext directly as the "hash" — bcrypt happens in
	// the Postgres adapter where the real KDF lives (Plan 02 wires it). Tests
	// see a deterministic "hash" so they can assert.
	hash := domain.APIKeyHash([]byte(plain))

	k := domain.NewAPIKey(
		domain.APIKeyID(uc.IDGen.NewAPIKeyID()),
		in.UserID, in.Name, prefix, hash,
		uc.Clock.Now(),
	)
	if err := uc.Keys.Create(ctx, k); err != nil {
		return nil, err
	}
	return &MintAPIKeyOutput{Key: k, Plaintext: plain}, nil
}
