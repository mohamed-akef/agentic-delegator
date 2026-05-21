// core/testutil/fake_api_keys_repo.go
package testutil

import (
	"context"
	"sync"
	"time"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type FakeAPIKeysRepo struct {
	mu sync.Mutex
	m  map[domain.APIKeyID]*domain.APIKey
}

func NewFakeAPIKeysRepo() *FakeAPIKeysRepo {
	return &FakeAPIKeysRepo{m: map[domain.APIKeyID]*domain.APIKey{}}
}

var _ ports.APIKeysRepository = (*FakeAPIKeysRepo)(nil)

func (r *FakeAPIKeysRepo) Create(ctx context.Context, k *domain.APIKey) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.m[k.ID]; ok {
		return domain.ErrConflict
	}
	clone := *k
	r.m[k.ID] = &clone
	return nil
}

func (r *FakeAPIKeysRepo) GetByPrefix(ctx context.Context, prefix string) ([]*domain.APIKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.APIKey
	for _, k := range r.m {
		if k.Prefix == prefix {
			clone := *k
			out = append(out, &clone)
		}
	}
	return out, nil
}

func (r *FakeAPIKeysRepo) ListForUser(ctx context.Context, userID domain.UserID) ([]*domain.APIKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.APIKey
	for _, k := range r.m {
		if k.UserID == userID {
			clone := *k
			out = append(out, &clone)
		}
	}
	return out, nil
}

func (r *FakeAPIKeysRepo) Delete(ctx context.Context, id domain.APIKeyID, userID domain.UserID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k, ok := r.m[id]
	if !ok || k.UserID != userID {
		return domain.ErrNotFound
	}
	delete(r.m, id)
	return nil
}

func (r *FakeAPIKeysRepo) RecordUsed(ctx context.Context, id domain.APIKeyID, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k, ok := r.m[id]
	if !ok {
		return domain.ErrNotFound
	}
	t := at
	k.LastUsedAt = &t
	return nil
}
