// core/usecase/reattach_running_jobs.go
package usecase

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type ReattachRunningJobs struct {
	Jobs   ports.JobsRepository
	Runner ports.RunnerService
	Clock  ports.Clock
}

func (uc *ReattachRunningJobs) Execute(ctx context.Context) error {
	running, err := uc.Jobs.ListByStatus(ctx, domain.JobStatusRunning)
	if err != nil {
		return err
	}
	now := uc.Clock.Now()
	for _, j := range running {
		alive, err := uc.Runner.Inspect(ctx, j.ContainerID)
		if err != nil {
			return err
		}
		if alive {
			continue
		}
		_ = j.MarkFailed("api restarted while runner container gone", now)
		if err := uc.Jobs.Update(ctx, j); err != nil {
			return err
		}
	}
	return nil
}
