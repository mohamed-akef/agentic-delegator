// core/domain/job_test.go
package domain_test

import (
	"errors"
	"testing"
	"time"

	"agentic-delegator/core/domain"
)

func newTestJob(t *testing.T, now time.Time) *domain.Job {
	t.Helper()
	return domain.NewJob(
		"j_1", "u_1",
		"owner/repo", "main", "agentic/x",
		domain.SpecSource{Type: domain.SourceTypePath, Value: "specs/x.md"},
		"",
		now,
	)
}

func TestNewJob_initialStateIsQueued(t *testing.T) {
	now := time.Unix(1000, 0)
	j := newTestJob(t, now)

	if j.Status != domain.JobStatusQueued {
		t.Fatalf("status: want queued, got %s", j.Status)
	}
	if j.ID != "j_1" || j.UserID != "u_1" {
		t.Fatalf("ids not set")
	}
	if j.StartedAt != nil || j.FinishedAt != nil {
		t.Fatalf("timestamps should be nil on creation")
	}
	if !j.CreatedAt.Equal(now) {
		t.Fatalf("created_at not set")
	}
	if j.IsTerminal() {
		t.Fatalf("queued is not terminal")
	}
}

func TestJob_MarkRunning(t *testing.T) {
	now := time.Unix(1000, 0)
	then := now.Add(10 * time.Second)
	j := newTestJob(t, now)

	if err := j.MarkRunning("ctr_a", then); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if j.Status != domain.JobStatusRunning {
		t.Fatalf("status: want running, got %s", j.Status)
	}
	if j.ContainerID != "ctr_a" {
		t.Fatalf("container id not set")
	}
	if j.StartedAt == nil || !j.StartedAt.Equal(then) {
		t.Fatalf("started_at not set correctly")
	}
}

func TestJob_MarkRunning_fromTerminalFails(t *testing.T) {
	now := time.Unix(1000, 0)
	j := newTestJob(t, now)
	_ = j.MarkRunning("ctr_a", now)
	_ = j.MarkSucceeded("https://example/pr/1", now)

	err := j.MarkRunning("ctr_b", now)
	if !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("want ErrInvalidState, got %v", err)
	}
}

func TestJob_MarkSucceeded(t *testing.T) {
	now := time.Unix(1000, 0)
	j := newTestJob(t, now)
	_ = j.MarkRunning("ctr_a", now)

	finished := now.Add(time.Minute)
	if err := j.MarkSucceeded("https://example/pr/1", finished); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if j.Status != domain.JobStatusSucceeded {
		t.Fatalf("status: want succeeded, got %s", j.Status)
	}
	if j.PRURL != "https://example/pr/1" {
		t.Fatalf("pr_url not set")
	}
	if j.FinishedAt == nil || !j.FinishedAt.Equal(finished) {
		t.Fatalf("finished_at not set")
	}
	if !j.IsTerminal() {
		t.Fatalf("succeeded is terminal")
	}
}

func TestJob_MarkSucceeded_fromQueuedFails(t *testing.T) {
	j := newTestJob(t, time.Unix(1000, 0))
	err := j.MarkSucceeded("https://example/pr/1", time.Unix(2000, 0))
	if !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("want ErrInvalidState, got %v", err)
	}
}

func TestJob_MarkFailed_fromQueuedOrRunning(t *testing.T) {
	now := time.Unix(1000, 0)

	t.Run("from queued", func(t *testing.T) {
		j := newTestJob(t, now)
		if err := j.MarkFailed("boom", now); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if j.Status != domain.JobStatusFailed || j.Error != "boom" {
			t.Fatalf("not marked failed correctly")
		}
	})

	t.Run("from running", func(t *testing.T) {
		j := newTestJob(t, now)
		_ = j.MarkRunning("ctr_a", now)
		if err := j.MarkFailed("boom", now); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if j.Status != domain.JobStatusFailed {
			t.Fatalf("not failed")
		}
	})

	t.Run("from terminal fails", func(t *testing.T) {
		j := newTestJob(t, now)
		_ = j.MarkRunning("ctr_a", now)
		_ = j.MarkSucceeded("https://x/pr/1", now)
		if err := j.MarkFailed("boom", now); !errors.Is(err, domain.ErrInvalidState) {
			t.Fatalf("want ErrInvalidState, got %v", err)
		}
	})
}

func TestJob_MarkCancelled(t *testing.T) {
	now := time.Unix(1000, 0)

	t.Run("from queued ok", func(t *testing.T) {
		j := newTestJob(t, now)
		if err := j.MarkCancelled(now); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if j.Status != domain.JobStatusCancelled {
			t.Fatalf("not cancelled")
		}
	})

	t.Run("from running ok", func(t *testing.T) {
		j := newTestJob(t, now)
		_ = j.MarkRunning("ctr_a", now)
		if err := j.MarkCancelled(now); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})

	t.Run("from succeeded fails", func(t *testing.T) {
		j := newTestJob(t, now)
		_ = j.MarkRunning("ctr_a", now)
		_ = j.MarkSucceeded("https://x/pr/1", now)
		err := j.MarkCancelled(now)
		if !errors.Is(err, domain.ErrInvalidState) {
			t.Fatalf("want ErrInvalidState, got %v", err)
		}
	})
}

func TestJob_IsTerminal(t *testing.T) {
	tests := map[domain.JobStatus]bool{
		domain.JobStatusQueued:    false,
		domain.JobStatusRunning:   false,
		domain.JobStatusSucceeded: true,
		domain.JobStatusFailed:    true,
		domain.JobStatusCancelled: true,
	}
	for status, want := range tests {
		j := newTestJob(t, time.Unix(1000, 0))
		j.Status = status
		if got := j.IsTerminal(); got != want {
			t.Fatalf("status %s: want %v, got %v", status, want, got)
		}
	}
}
