// core/usecase/list_jobs_test.go
package usecase_test

import (
	"context"
	"testing"
	"time"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

func TestListJobs_returnsOnlyOwner(t *testing.T) {
	ctx := context.Background()
	jobs := testutil.NewFakeJobsRepo()
	_ = jobs.Create(ctx, domain.NewJob("j_1", "u_1", "o/r", "main", "agentic/a", domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "", time.Unix(1000, 0)))
	_ = jobs.Create(ctx, domain.NewJob("j_2", "u_2", "o/r", "main", "agentic/b", domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "", time.Unix(2000, 0)))
	_ = jobs.Create(ctx, domain.NewJob("j_3", "u_1", "o/r", "main", "agentic/c", domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "", time.Unix(3000, 0)))

	uc := &usecase.ListJobs{Jobs: jobs}
	got, err := uc.Execute(ctx, usecase.ListJobsInput{UserID: "u_1", Limit: 50})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 jobs for u_1, got %d", len(got))
	}
	// Most recent first.
	if got[0].ID != "j_3" {
		t.Fatalf("want j_3 first, got %s", got[0].ID)
	}
}
