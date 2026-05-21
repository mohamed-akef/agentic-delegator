// core/testutil/fake_jobs_repo.go
package testutil

import (
	"context"
	"sort"
	"sync"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

// FakeJobsRepo is an in-memory ports.JobsRepository. Stores copies on write
// and returns copies on read to avoid aliasing bugs in tests.
type FakeJobsRepo struct {
	mu sync.Mutex
	m  map[domain.JobID]*domain.Job
}

func NewFakeJobsRepo() *FakeJobsRepo {
	return &FakeJobsRepo{m: map[domain.JobID]*domain.Job{}}
}

var _ ports.JobsRepository = (*FakeJobsRepo)(nil)

func (r *FakeJobsRepo) Create(ctx context.Context, j *domain.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.m[j.ID]; exists {
		return domain.ErrConflict
	}
	clone := *j
	r.m[j.ID] = &clone
	return nil
}

func (r *FakeJobsRepo) Get(ctx context.Context, id domain.JobID) (*domain.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.m[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	clone := *j
	return &clone, nil
}

func (r *FakeJobsRepo) GetForUser(ctx context.Context, id domain.JobID, userID domain.UserID) (*domain.Job, error) {
	j, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if j.UserID != userID {
		return nil, domain.ErrNotFound
	}
	return j, nil
}

func (r *FakeJobsRepo) ListForUser(ctx context.Context, userID domain.UserID, limit int) ([]*domain.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.Job
	for _, j := range r.m {
		if j.UserID == userID {
			clone := *j
			out = append(out, &clone)
		}
	}
	sort.Slice(out, func(i, k int) bool { return out[i].CreatedAt.After(out[k].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *FakeJobsRepo) ListByStatus(ctx context.Context, status domain.JobStatus) ([]*domain.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.Job
	for _, j := range r.m {
		if j.Status == status {
			clone := *j
			out = append(out, &clone)
		}
	}
	return out, nil
}

func (r *FakeJobsRepo) Update(ctx context.Context, j *domain.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.m[j.ID]; !ok {
		return domain.ErrNotFound
	}
	clone := *j
	r.m[j.ID] = &clone
	return nil
}

func (r *FakeJobsRepo) CountActiveForUser(ctx context.Context, userID domain.UserID) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, j := range r.m {
		if j.UserID == userID && (j.Status == domain.JobStatusQueued || j.Status == domain.JobStatusRunning) {
			n++
		}
	}
	return n, nil
}

func (r *FakeJobsRepo) CountActiveGlobal(ctx context.Context) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, j := range r.m {
		if j.Status == domain.JobStatusQueued || j.Status == domain.JobStatusRunning {
			n++
		}
	}
	return n, nil
}
