// core/usecase/reattach_running_jobs_test.go
package usecase_test

import (
	"context"
	"testing"
	"time"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

func TestReattachRunningJobs_aliveStays(t *testing.T) {
	ctx := context.Background()
	jobs := testutil.NewFakeJobsRepo()
	runner := testutil.NewFakeRunnerService()
	clock := testutil.NewFakeClock(time.Unix(1000, 0))

	j := domain.NewJob("j_1", "u_1", "o/r", "main", "agentic/x", domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "", clock.Now())
	_ = j.MarkRunning("ctr_alive", clock.Now())
	_ = jobs.Create(ctx, j)

	// Mark the container alive in the runner without going through Start.
	runner.SetAlive("ctr_alive", true)

	uc := &usecase.ReattachRunningJobs{Jobs: jobs, Runner: runner, Clock: clock}
	if err := uc.Execute(ctx); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	got, _ := jobs.Get(ctx, "j_1")
	if got.Status != domain.JobStatusRunning {
		t.Fatalf("alive container's job should stay running, got %s", got.Status)
	}
}

func TestReattachRunningJobs_deadGetsMarkedFailed(t *testing.T) {
	ctx := context.Background()
	jobs := testutil.NewFakeJobsRepo()
	runner := testutil.NewFakeRunnerService()
	clock := testutil.NewFakeClock(time.Unix(1000, 0))

	j := domain.NewJob("j_2", "u_1", "o/r", "main", "agentic/x", domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "", clock.Now())
	_ = j.MarkRunning("ctr_dead", clock.Now())
	_ = jobs.Create(ctx, j)
	// Container is not alive in the runner.

	uc := &usecase.ReattachRunningJobs{Jobs: jobs, Runner: runner, Clock: clock}
	if err := uc.Execute(ctx); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	got, _ := jobs.Get(ctx, "j_2")
	if got.Status != domain.JobStatusFailed {
		t.Fatalf("dead container's job should be failed, got %s", got.Status)
	}
	if got.Error == "" {
		t.Fatalf("error reason should be set")
	}
}
