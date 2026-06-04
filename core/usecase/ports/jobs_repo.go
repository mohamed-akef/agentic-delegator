// core/usecase/ports/jobs_repo.go
package ports

import (
	"context"

	"agentic-delegator/core/domain"
)

// JobsRepository is the outbound port for persisting Jobs. Adapter
// implementations (postgres, in-memory fake) must scope every query by
// UserID for multi-tenant safety, except where explicitly noted.
type JobsRepository interface {
	Create(ctx context.Context, j *domain.Job) error

	// Get returns the job regardless of owner. Use only from trusted callers
	// (the runner completion path needs cross-user access).
	Get(ctx context.Context, id domain.JobID) (*domain.Job, error)

	// GetForUser returns the job iff it belongs to userID. Otherwise ErrNotFound.
	GetForUser(ctx context.Context, id domain.JobID, userID domain.UserID) (*domain.Job, error)

	// ListForUser returns the most recent `limit` jobs for userID. limit<=0 means no cap.
	ListForUser(ctx context.Context, userID domain.UserID, limit int) ([]*domain.Job, error)

	// ListByStatus returns jobs in the given status across all users.
	// Used at startup to reattach orphaned containers.
	ListByStatus(ctx context.Context, status domain.JobStatus) ([]*domain.Job, error)

	Update(ctx context.Context, j *domain.Job) error

	// CountActiveForUser returns jobs in queued+running for this user.
	CountActiveForUser(ctx context.Context, userID domain.UserID) (int, error)

	// CountActiveGlobal returns jobs in queued+running across all users.
	CountActiveGlobal(ctx context.Context) (int, error)
}
