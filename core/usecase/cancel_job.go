// core/usecase/cancel_job.go
package usecase

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

// CancelJob stops a running job's container (if any) and marks it cancelled.
// It is user-scoped: a user can only cancel their own jobs.
type CancelJob struct {
	Jobs   ports.JobsRepository
	Runner ports.RunnerService
	Clock  ports.Clock
}

type CancelJobInput struct {
	JobID  domain.JobID
	UserID domain.UserID
}

func (uc *CancelJob) Execute(ctx context.Context, in CancelJobInput) error {
	if in.JobID == "" || in.UserID == "" {
		return domain.ErrInvalidInput
	}
	job, err := uc.Jobs.GetForUser(ctx, in.JobID, in.UserID)
	if err != nil {
		return err
	}
	if job.IsTerminal() {
		return domain.ErrInvalidState
	}
	if job.ContainerID != "" {
		_ = uc.Runner.Stop(ctx, job.ContainerID)
	}
	if err := job.MarkCancelled(uc.Clock.Now()); err != nil {
		return err
	}
	return uc.Jobs.Update(ctx, job)
}
