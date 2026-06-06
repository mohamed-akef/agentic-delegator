// core/usecase/cancel_job_test.go
package usecase_test

import (
	"context"
	"testing"
	"time"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

func TestCancelJob_running(t *testing.T) {
	ctx := context.Background()
	clock := testutil.NewFakeClock(time.Unix(1000, 0))
	jobs := testutil.NewFakeJobsRepo()
	runner := testutil.NewFakeRunnerService()

	j := domain.NewJob("j_1", "u_1", "o/r", "main", "agentic/x", domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "", clock.Now())
	_ = j.MarkRunning("ctr_1", clock.Now())
	_ = jobs.Create(ctx, j)
	runner.SetAlive("ctr_1", true)

	uc := &usecase.CancelJob{Jobs: jobs, Runner: runner, Clock: clock}
	if err := uc.Execute(ctx, usecase.CancelJobInput{JobID: "j_1", UserID: "u_1"}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	saved, _ := jobs.Get(ctx, "j_1")
	if saved.Status != domain.JobStatusCancelled {
		t.Fatalf("want cancelled, got %s", saved.Status)
	}
	if alive, _ := runner.Inspect(ctx, "ctr_1"); alive {
		t.Fatalf("container should have been stopped")
	}
}

func TestCancelJob_otherUserGetsNotFound(t *testing.T) {
	ctx := context.Background()
	clock := testutil.NewFakeClock(time.Unix(1000, 0))
	jobs := testutil.NewFakeJobsRepo()
	j := domain.NewJob("j_1", "u_1", "o/r", "main", "agentic/x", domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "", clock.Now())
	_ = jobs.Create(ctx, j)

	uc := &usecase.CancelJob{Jobs: jobs, Runner: testutil.NewFakeRunnerService(), Clock: clock}
	err := uc.Execute(ctx, usecase.CancelJobInput{JobID: "j_1", UserID: "u_2"})
	if err != domain.ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestCancelJob_alreadyTerminal(t *testing.T) {
	ctx := context.Background()
	clock := testutil.NewFakeClock(time.Unix(1000, 0))
	jobs := testutil.NewFakeJobsRepo()
	j := domain.NewJob("j_1", "u_1", "o/r", "main", "agentic/x", domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "", clock.Now())
	_ = j.MarkRunning("ctr_1", clock.Now())
	_ = j.MarkSucceeded("https://pr", clock.Now())
	_ = jobs.Create(ctx, j)

	uc := &usecase.CancelJob{Jobs: jobs, Runner: testutil.NewFakeRunnerService(), Clock: clock}
	if err := uc.Execute(ctx, usecase.CancelJobInput{JobID: "j_1", UserID: "u_1"}); err != domain.ErrInvalidState {
		t.Fatalf("want ErrInvalidState, got %v", err)
	}
}
