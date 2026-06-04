// core/usecase/list_jobs.go
package usecase

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type ListJobs struct {
	Jobs ports.JobsRepository
}

type ListJobsInput struct {
	UserID domain.UserID
	Limit  int // 0 = default 50
}

func (uc *ListJobs) Execute(ctx context.Context, in ListJobsInput) ([]*domain.Job, error) {
	if in.UserID == "" {
		return nil, domain.ErrInvalidInput
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 50
	}
	return uc.Jobs.ListForUser(ctx, in.UserID, limit)
}
