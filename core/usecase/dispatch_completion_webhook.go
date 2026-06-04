// core/usecase/dispatch_completion_webhook.go
package usecase

import (
	"context"
	"encoding/json"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type DispatchCompletionWebhook struct {
	Dispatcher ports.WebhookDispatcher
}

type DispatchCompletionWebhookInput struct {
	URL     string
	Job     *domain.Job
	LogTail string
}

func (uc *DispatchCompletionWebhook) Execute(ctx context.Context, in DispatchCompletionWebhookInput) error {
	if in.URL == "" {
		return nil
	}
	if in.Job == nil {
		return domain.ErrInvalidInput
	}
	payload := map[string]any{
		"event":    "job.completed",
		"job":      in.Job,
		"log_tail": in.LogTail,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return uc.Dispatcher.Dispatch(ctx, in.URL, body)
}
