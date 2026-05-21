// core/usecase/dispatch_completion_webhook_test.go
package usecase_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

func TestDispatchCompletionWebhook_postsExpectedPayload(t *testing.T) {
	ctx := context.Background()
	disp := testutil.NewFakeWebhookDispatcher()

	uc := &usecase.DispatchCompletionWebhook{Dispatcher: disp}

	j := domain.NewJob("j_1", "u_1", "o/r", "main", "agentic/x", domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "", time.Unix(1000, 0))
	_ = j.MarkRunning("ctr_a", time.Unix(1100, 0))
	_ = j.MarkSucceeded("https://example/pr/1", time.Unix(1200, 0))

	err := uc.Execute(ctx, usecase.DispatchCompletionWebhookInput{
		URL:     "https://hook.example/dest",
		Job:     j,
		LogTail: "build ok",
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(disp.Calls) != 1 {
		t.Fatalf("want 1 dispatch, got %d", len(disp.Calls))
	}
	var got map[string]any
	if err := json.Unmarshal(disp.Calls[0].Payload, &got); err != nil {
		t.Fatalf("payload not valid json: %v", err)
	}
	if got["event"] != "job.completed" {
		t.Fatalf("event field wrong: %v", got["event"])
	}
	if got["log_tail"] != "build ok" {
		t.Fatalf("log_tail not threaded through: %v", got["log_tail"])
	}
}

func TestDispatchCompletionWebhook_skippedOnEmptyURL(t *testing.T) {
	disp := testutil.NewFakeWebhookDispatcher()
	uc := &usecase.DispatchCompletionWebhook{Dispatcher: disp}
	err := uc.Execute(context.Background(), usecase.DispatchCompletionWebhookInput{URL: ""})
	if err != nil {
		t.Fatalf("empty URL should be a no-op, got %v", err)
	}
	if len(disp.Calls) != 0 {
		t.Fatalf("expected no dispatches for empty URL")
	}
}
