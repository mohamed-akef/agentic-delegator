// core/usecase/enqueue_job.go
package usecase

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type EnqueueJob struct {
	Jobs           ports.JobsRepository
	RepoCreds      ports.RepoCredentialsProvider
	AnthropicCreds ports.AnthropicCredentialsProvider
	Runner         ports.RunnerService
	IDGen          ports.IDGenerator
	Clock          ports.Clock

	// LogDir is the private directory (mode 0700) where per-job log files are
	// written, named by the non-guessable job ID. Used when the request does not
	// supply an explicit LogPath.
	LogDir string

	MaxConcurrentPerUser int // 0 = unlimited
	MaxConcurrentGlobal  int // 0 = unlimited

	// OnComplete is invoked by the runner adapter when a container exits.
	// Plan 03 wires this to the HandleRunnerCompletion use case. For tests,
	// the field can stay nil.
	OnComplete func(ports.RunnerResult)
}

type EnqueueJobInput struct {
	UserID        domain.UserID
	Repo          string
	BaseBranch    string
	WorkBranch    string
	Spec          domain.SpecSource
	ModelOverride string
	LogPath       string
}

type EnqueueJobOutput struct {
	JobID  domain.JobID
	Status domain.JobStatus
}

func (uc *EnqueueJob) Execute(ctx context.Context, in EnqueueJobInput) (*EnqueueJobOutput, error) {
	if in.UserID == "" || in.Repo == "" || in.BaseBranch == "" || in.WorkBranch == "" || !in.Spec.Valid() {
		return nil, domain.ErrInvalidInput
	}

	now := uc.Clock.Now()
	job := domain.NewJob(
		domain.JobID(uc.IDGen.NewJobID()),
		in.UserID,
		in.Repo, in.BaseBranch, in.WorkBranch,
		in.Spec, in.ModelOverride, now,
	)
	// Log path: caller may pin one; otherwise derive a private, non-guessable
	// path under LogDir from the job ID (avoids the user-controlled work-branch
	// name landing in a world-readable /tmp file).
	logPath := in.LogPath
	if logPath == "" {
		logPath = filepath.Join(uc.LogDir, string(job.ID)+".log")
	}
	job.LogPath = logPath

	if err := uc.Jobs.Create(ctx, job); err != nil {
		return nil, err
	}

	// Concurrency caps: if exceeded, leave queued and return without starting.
	if uc.MaxConcurrentPerUser > 0 {
		n, err := uc.Jobs.CountActiveForUser(ctx, in.UserID)
		if err != nil {
			return nil, err
		}
		if n > uc.MaxConcurrentPerUser {
			return &EnqueueJobOutput{JobID: job.ID, Status: job.Status}, nil
		}
	}
	if uc.MaxConcurrentGlobal > 0 {
		n, err := uc.Jobs.CountActiveGlobal(ctx)
		if err != nil {
			return nil, err
		}
		if n > uc.MaxConcurrentGlobal {
			return &EnqueueJobOutput{JobID: job.ID, Status: job.Status}, nil
		}
	}

	// Missing credentials are a user-config precondition, not a missing resource:
	// surface them as invalid input (400) rather than letting ErrNotFound (404)
	// leak out of POST /api/jobs.
	gitCreds, err := uc.RepoCreds.For(ctx, in.UserID, in.Repo)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, fmt.Errorf("%w: repository credentials not available (is the GitHub App installed on %s?)", domain.ErrInvalidInput, in.Repo)
		}
		return nil, err
	}
	anth, err := uc.AnthropicCreds.For(ctx, in.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, fmt.Errorf("%w: Anthropic API key not configured", domain.ErrInvalidInput)
		}
		return nil, err
	}

	spec := ports.RunnerStartSpec{
		JobID:      job.ID,
		Repo:       in.Repo,
		BaseBranch: in.BaseBranch,
		WorkBranch: in.WorkBranch,
		Spec:       in.Spec,
		GitCreds:   gitCreds,
		Anthropic:  anth,
		Model:      in.ModelOverride,
		LogPath:    in.LogPath,
	}

	containerID, startErr := uc.Runner.Start(ctx, spec, uc.OnComplete)
	if startErr != nil {
		_ = job.MarkFailed(startErr.Error(), uc.Clock.Now())
		_ = uc.Jobs.Update(ctx, job)
		return nil, startErr
	}

	if err := job.MarkRunning(containerID, uc.Clock.Now()); err != nil {
		return nil, err
	}
	if err := uc.Jobs.Update(ctx, job); err != nil {
		return nil, err
	}
	return &EnqueueJobOutput{JobID: job.ID, Status: job.Status}, nil
}
