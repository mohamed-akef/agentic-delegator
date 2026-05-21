// core/adapter/postgres/jobs_repo_test.go
//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"agentic-delegator/core/adapter/postgres"
	"agentic-delegator/core/domain"
)

func setupRepo(t *testing.T) *postgres.JobsRepo {
	t.Helper()
	db, err := postgres.Open(testDSN(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	// seed a user so foreign key holds
	ctx := context.Background()
	_, err = db.ExecContext(ctx,
		`INSERT INTO users (id, display_name) VALUES ('u_t1', 'tester') ON CONFLICT DO NOTHING`,
	)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	// wipe jobs from prior runs
	_, _ = db.ExecContext(ctx, `DELETE FROM jobs WHERE user_id = 'u_t1'`)
	return postgres.NewJobsRepo(db)
}

func newDomainJob() *domain.Job {
	return domain.NewJob(
		domain.JobID("j_t_"+time.Now().Format("150405.000000000")),
		"u_t1",
		"owner/repo", "main", "agentic/x",
		domain.SpecSource{Type: domain.SourceTypePath, Value: "specs/x.md"},
		"",
		time.Now().UTC(),
	)
}

func TestJobsRepo_createGetUpdate(t *testing.T) {
	repo := setupRepo(t)
	ctx := context.Background()

	j := newDomainJob()
	j.LogPath = "/tmp/" + string(j.ID) + ".log"

	if err := repo.Create(ctx, j); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Get(ctx, j.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != j.ID || got.Repo != j.Repo {
		t.Fatalf("Get returned wrong data: %+v", got)
	}

	// transition + update
	now := time.Now().UTC()
	_ = got.MarkRunning("ctr_abc", now)
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	again, _ := repo.Get(ctx, j.ID)
	if again.Status != domain.JobStatusRunning || again.ContainerID != "ctr_abc" {
		t.Fatalf("update not persisted: %+v", again)
	}
}

func TestJobsRepo_getForUserScoping(t *testing.T) {
	repo := setupRepo(t)
	ctx := context.Background()

	j := newDomainJob()
	j.LogPath = "/tmp/" + string(j.ID) + ".log"
	_ = repo.Create(ctx, j)

	if _, err := repo.GetForUser(ctx, j.ID, "u_t1"); err != nil {
		t.Fatalf("owner Get: %v", err)
	}
	if _, err := repo.GetForUser(ctx, j.ID, "u_other"); err == nil {
		t.Fatalf("non-owner GetForUser should fail")
	}
}

func TestJobsRepo_countActive(t *testing.T) {
	repo := setupRepo(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		j := newDomainJob()
		j.LogPath = "/tmp/" + string(j.ID) + ".log"
		if i < 2 {
			_ = j.MarkRunning("ctr"+string(rune('a'+i)), time.Now().UTC())
		}
		_ = repo.Create(ctx, j)
		_ = repo.Update(ctx, j)
	}

	n, err := repo.CountActiveForUser(ctx, "u_t1")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 3 {
		t.Fatalf("want 3 active (1 queued + 2 running), got %d", n)
	}
}
