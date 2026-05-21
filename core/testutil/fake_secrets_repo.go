// core/testutil/fake_secrets_repo.go
package testutil

import (
	"context"
	"sync"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type FakeSecretsRepo struct {
	mu sync.Mutex
	m  map[domain.UserID]domain.AnthropicCreds
}

func NewFakeSecretsRepo() *FakeSecretsRepo {
	return &FakeSecretsRepo{m: map[domain.UserID]domain.AnthropicCreds{}}
}

var _ ports.SecretsRepository = (*FakeSecretsRepo)(nil)

func (r *FakeSecretsRepo) SetAnthropicCreds(ctx context.Context, userID domain.UserID, c domain.AnthropicCreds) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[userID] = c
	return nil
}

func (r *FakeSecretsRepo) GetAnthropicCreds(ctx context.Context, userID domain.UserID) (domain.AnthropicCreds, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.m[userID]
	if !ok {
		return domain.AnthropicCreds{}, domain.ErrNotFound
	}
	return c, nil
}

func (r *FakeSecretsRepo) DeleteAnthropicCreds(ctx context.Context, userID domain.UserID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.m[userID]; !ok {
		return domain.ErrNotFound
	}
	delete(r.m, userID)
	return nil
}
