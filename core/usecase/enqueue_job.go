// core/usecase/enqueue_job.go
package usecase

import (
	"context"

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
	if in.UserID == "" || in.Repo == "" || in.BaseBranch == "" || in.WorkBranch == "" || !in.Spec.Valid() || in.LogPath == "" {
		return nil, domain.ErrInvalidInput
	}

	now := uc.Clock.Now()
	job := domain.NewJob(
		domain.JobID(uc.IDGen.NewJobID()),
		in.UserID,
		in.Repo, in.BaseBranch, in.WorkBranch,
		in.Spec, in.ModelOverride, now,
	)
	job.LogPath = in.LogPath

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

	gitCreds, err := uc.RepoCreds.For(ctx, in.UserID, in.Repo)
	if err != nil {
		return nil, err
	}
	anth, err := uc.AnthropicCreds.For(ctx, in.UserID)
	if err != nil {
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
