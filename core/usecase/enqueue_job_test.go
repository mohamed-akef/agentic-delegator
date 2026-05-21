// core/usecase/enqueue_job_test.go
package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

type enqueueDeps struct {
	clock   *testutil.FakeClock
	jobs    *testutil.FakeJobsRepo
	runner  *testutil.FakeRunnerService
	repoCp  *testutil.FakeRepoCredsProvider
	anth    *testutil.FakeAnthropicCredsProvider
	idgen   *testutil.FakeIDGenerator
}

func newEnqueueUC(t *testing.T) (*usecase.EnqueueJob, *enqueueDeps) {
	t.Helper()
	deps := &enqueueDeps{
		clock:  testutil.NewFakeClock(time.Unix(1000, 0)),
		jobs:   testutil.NewFakeJobsRepo(),
		runner: testutil.NewFakeRunnerService(),
		repoCp: testutil.NewFakeRepoCredsProvider(domain.GitCreds{Token: "git-token"}),
		anth:   testutil.NewFakeAnthropicCredsProvider(domain.AnthropicCreds{APIKey: "sk-ant"}),
		idgen:  &testutil.FakeIDGenerator{},
	}
	uc := &usecase.EnqueueJob{
		Jobs:                 deps.jobs,
		RepoCreds:            deps.repoCp,
		AnthropicCreds:       deps.anth,
		Runner:               deps.runner,
		IDGen:                deps.idgen,
		Clock:                deps.clock,
		MaxConcurrentPerUser: 2,
		MaxConcurrentGlobal:  4,
	}
	return uc, deps
}

func validInput() usecase.EnqueueJobInput {
	return usecase.EnqueueJobInput{
		UserID:     "u_1",
		Repo:       "owner/repo",
		BaseBranch: "main",
		WorkBranch: "agentic/x",
		Spec:       domain.SpecSource{Type: domain.SourceTypePath, Value: "specs/x.md"},
		LogPath:    "/tmp/j_1.log",
	}
}

func TestEnqueueJob_happyPath(t *testing.T) {
	ctx := context.Background()
	uc, deps := newEnqueueUC(t)

	out, err := uc.Execute(ctx, validInput())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Status != domain.JobStatusRunning {
		t.Fatalf("status: want running, got %s", out.Status)
	}

	saved, _ := deps.jobs.Get(ctx, out.JobID)
	if saved.ContainerID == "" {
		t.Fatalf("container id not set")
	}
	if len(deps.runner.StartedSpecs) != 1 {
		t.Fatalf("expected 1 runner start, got %d", len(deps.runner.StartedSpecs))
	}
	if deps.runner.StartedSpecs[0].GitCreds.Token != "git-token" {
		t.Fatalf("git creds not threaded through to runner")
	}
}

func TestEnqueueJob_invalidInput(t *testing.T) {
	uc, _ := newEnqueueUC(t)
	_, err := uc.Execute(context.Background(), usecase.EnqueueJobInput{})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

func TestEnqueueJob_concurrencyCapPerUser_staysQueued(t *testing.T) {
	ctx := context.Background()
	uc, deps := newEnqueueUC(t)
	uc.MaxConcurrentPerUser = 1

	// Pre-populate one active job for the same user.
	preexisting := domain.NewJob("j_pre", "u_1", "owner/repo", "main", "agentic/pre", domain.SpecSource{Type: domain.SourceTypePath, Value: "specs/pre.md"}, "", time.Unix(500, 0))
	_ = preexisting.MarkRunning("ctr_pre", time.Unix(600, 0))
	_ = deps.jobs.Create(ctx, preexisting)

	out, err := uc.Execute(ctx, validInput())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Status != domain.JobStatusQueued {
		t.Fatalf("want queued (cap hit), got %s", out.Status)
	}
	if len(deps.runner.StartedSpecs) != 0 {
		t.Fatalf("runner should not have been called when capped")
	}
}

func TestEnqueueJob_repoCredsErrorPropagates(t *testing.T) {
	uc, deps := newEnqueueUC(t)
	deps.repoCp.Err = errors.New("github unreachable")

	_, err := uc.Execute(context.Background(), validInput())
	if err == nil {
		t.Fatalf("expected error from RepoCreds provider")
	}
}

func TestEnqueueJob_runnerStartErrorMarksFailed(t *testing.T) {
	ctx := context.Background()
	uc, deps := newEnqueueUC(t)
	deps.runner.StartErr = errors.New("docker down")

	out, err := uc.Execute(ctx, validInput())
	if err == nil {
		t.Fatalf("expected error from runner")
	}
	// The job should have been persisted as failed.
	if out != nil {
		t.Fatalf("output should be nil on failure")
	}
	jobs, _ := deps.jobs.ListByStatus(ctx, domain.JobStatusFailed)
	if len(jobs) != 1 {
		t.Fatalf("want 1 failed job, got %d", len(jobs))
	}
	if jobs[0].Error == "" {
		t.Fatalf("error reason not recorded")
	}
}
