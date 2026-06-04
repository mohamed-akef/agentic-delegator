// core/domain/job.go
package domain

import "time"

type JobID string

type JobStatus string

const (
	JobStatusQueued    JobStatus = "queued"
	JobStatusRunning   JobStatus = "running"
	JobStatusSucceeded JobStatus = "succeeded"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

// Job is the central domain entity. It tracks the lifecycle of one
// agentic-delegator task from submission through completion.
type Job struct {
	ID            JobID
	UserID        UserID
	Status        JobStatus
	Repo          string
	BaseBranch    string
	WorkBranch    string
	Spec          SpecSource
	ModelOverride string
	ContainerID   string // populated when MarkRunning
	PRURL         string // populated on MarkSucceeded
	Error         string // populated on MarkFailed
	LogPath       string // filesystem path the runner streams stdout to
	CreatedAt     time.Time
	StartedAt     *time.Time
	FinishedAt    *time.Time
}

func NewJob(id JobID, userID UserID, repo, baseBranch, workBranch string, spec SpecSource, modelOverride string, now time.Time) *Job {
	return &Job{
		ID:            id,
		UserID:        userID,
		Status:        JobStatusQueued,
		Repo:          repo,
		BaseBranch:    baseBranch,
		WorkBranch:    workBranch,
		Spec:          spec,
		ModelOverride: modelOverride,
		CreatedAt:     now,
	}
}

func (j *Job) MarkRunning(containerID string, now time.Time) error {
	if j.Status != JobStatusQueued {
		return ErrInvalidState
	}
	t := now
	j.Status = JobStatusRunning
	j.ContainerID = containerID
	j.StartedAt = &t
	return nil
}

func (j *Job) MarkSucceeded(prURL string, now time.Time) error {
	if j.Status != JobStatusRunning {
		return ErrInvalidState
	}
	t := now
	j.Status = JobStatusSucceeded
	j.PRURL = prURL
	j.FinishedAt = &t
	return nil
}

func (j *Job) MarkFailed(reason string, now time.Time) error {
	if j.IsTerminal() {
		return ErrInvalidState
	}
	t := now
	j.Status = JobStatusFailed
	j.Error = reason
	j.FinishedAt = &t
	return nil
}

func (j *Job) MarkCancelled(now time.Time) error {
	if j.Status != JobStatusQueued && j.Status != JobStatusRunning {
		return ErrInvalidState
	}
	t := now
	j.Status = JobStatusCancelled
	j.FinishedAt = &t
	return nil
}

func (j *Job) IsTerminal() bool {
	switch j.Status {
	case JobStatusSucceeded, JobStatusFailed, JobStatusCancelled:
		return true
	}
	return false
}
