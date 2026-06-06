// core/usecase/handle_runner_completion.go
package usecase

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type HandleRunnerCompletion struct {
	Jobs  ports.JobsRepository
	Clock ports.Clock

	// Webhook is optional. When set and the runner reported a notification
	// webhook URL, the completion is dispatched after the job is persisted.
	Webhook *DispatchCompletionWebhook
}

func (uc *HandleRunnerCompletion) Execute(ctx context.Context, res ports.RunnerResult) error {
	if res.JobID == "" {
		return domain.ErrInvalidInput
	}

	job, err := uc.Jobs.Get(ctx, res.JobID)
	if err != nil {
		return err
	}

	now := uc.Clock.Now()
	if res.ExitCode == 0 {
		if err := job.MarkSucceeded(res.PRURL, now); err != nil {
			return err
		}
	} else {
		reason := res.Error
		if reason == "" {
			reason = "runner exited with non-zero code"
		}
		if err := job.MarkFailed(reason, now); err != nil {
			return err
		}
	}
	if err := uc.Jobs.Update(ctx, job); err != nil {
		return err
	}

	// Fire the completion webhook if the repo configured one. Delivery failures
	// are non-fatal: the job state is already persisted and authoritative.
	if uc.Webhook != nil && res.NotificationWebhook != "" {
		_ = uc.Webhook.Execute(ctx, DispatchCompletionWebhookInput{
			URL:     res.NotificationWebhook,
			Job:     job,
			LogTail: res.LogTail,
		})
	}
	return nil
}
