// core/usecase/handle_runner_completion_test.go
package usecase_test

import (
	"context"
	"testing"
	"time"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
	"agentic-delegator/core/usecase/ports"
)

func setupRunningJob(t *testing.T) (*testutil.FakeJobsRepo, *testutil.FakeClock) {
	t.Helper()
	clock := testutil.NewFakeClock(time.Unix(1000, 0))
	jobs := testutil.NewFakeJobsRepo()
	j := domain.NewJob("j_1", "u_1", "o/r", "main", "agentic/x", domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "", clock.Now())
	_ = j.MarkRunning("ctr_a", clock.Now())
	_ = jobs.Create(context.Background(), j)
	return jobs, clock
}

func TestHandleRunnerCompletion_success(t *testing.T) {
	ctx := context.Background()
	jobs, clock := setupRunningJob(t)
	clock.Advance(30 * time.Second)

	uc := &usecase.HandleRunnerCompletion{Jobs: jobs, Clock: clock}
	err := uc.Execute(ctx, ports.RunnerResult{JobID: "j_1", ExitCode: 0, PRURL: "https://example/pr/1"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	saved, _ := jobs.Get(ctx, "j_1")
	if saved.Status != domain.JobStatusSucceeded {
		t.Fatalf("want succeeded, got %s", saved.Status)
	}
	if saved.PRURL != "https://example/pr/1" {
		t.Fatalf("pr_url not set")
	}
}

func TestHandleRunnerCompletion_failure(t *testing.T) {
	ctx := context.Background()
	jobs, clock := setupRunningJob(t)
	clock.Advance(30 * time.Second)

	uc := &usecase.HandleRunnerCompletion{Jobs: jobs, Clock: clock}
	err := uc.Execute(ctx, ports.RunnerResult{JobID: "j_1", ExitCode: 2, Error: "compilation failed"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	saved, _ := jobs.Get(ctx, "j_1")
	if saved.Status != domain.JobStatusFailed {
		t.Fatalf("want failed, got %s", saved.Status)
	}
	if saved.Error != "compilation failed" {
		t.Fatalf("error not recorded")
	}
}

func TestHandleRunnerCompletion_dispatchesWebhook(t *testing.T) {
	ctx := context.Background()
	jobs, clock := setupRunningJob(t)
	clock.Advance(30 * time.Second)

	hooks := testutil.NewFakeWebhookDispatcher()
	uc := &usecase.HandleRunnerCompletion{
		Jobs:    jobs,
		Clock:   clock,
		Webhook: &usecase.DispatchCompletionWebhook{Dispatcher: hooks},
	}
	err := uc.Execute(ctx, ports.RunnerResult{
		JobID:               "j_1",
		ExitCode:            0,
		PRURL:               "https://example/pr/1",
		NotificationWebhook: "https://hooks.example/x",
		LogTail:             "done",
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(hooks.Calls) != 1 {
		t.Fatalf("want 1 webhook call, got %d", len(hooks.Calls))
	}
	if hooks.Calls[0].URL != "https://hooks.example/x" {
		t.Fatalf("webhook url mismatch: %s", hooks.Calls[0].URL)
	}
}

func TestHandleRunnerCompletion_noWebhookWhenURLEmpty(t *testing.T) {
	ctx := context.Background()
	jobs, clock := setupRunningJob(t)
	clock.Advance(30 * time.Second)

	hooks := testutil.NewFakeWebhookDispatcher()
	uc := &usecase.HandleRunnerCompletion{
		Jobs:    jobs,
		Clock:   clock,
		Webhook: &usecase.DispatchCompletionWebhook{Dispatcher: hooks},
	}
	_ = uc.Execute(ctx, ports.RunnerResult{JobID: "j_1", ExitCode: 0}) // no NotificationWebhook
	if len(hooks.Calls) != 0 {
		t.Fatalf("expected no webhook calls, got %d", len(hooks.Calls))
	}
}

func TestHandleRunnerCompletion_unknownJob(t *testing.T) {
	uc := &usecase.HandleRunnerCompletion{
		Jobs:  testutil.NewFakeJobsRepo(),
		Clock: testutil.NewFakeClock(time.Unix(1000, 0)),
	}
	err := uc.Execute(context.Background(), ports.RunnerResult{JobID: "j_nope", ExitCode: 0})
	if err != domain.ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
