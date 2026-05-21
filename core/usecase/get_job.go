// core/usecase/get_job.go
package usecase

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type GetJob struct {
	Jobs ports.JobsRepository
}

type GetJobInput struct {
	JobID  domain.JobID
	UserID domain.UserID
}

func (uc *GetJob) Execute(ctx context.Context, in GetJobInput) (*domain.Job, error) {
	if in.JobID == "" || in.UserID == "" {
		return nil, domain.ErrInvalidInput
	}
	return uc.Jobs.GetForUser(ctx, in.JobID, in.UserID)
}
