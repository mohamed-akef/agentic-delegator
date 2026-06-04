// core/usecase/get_job_test.go
package usecase_test

import (
	"context"
	"testing"
	"time"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

func TestGetJob_ownerCanRead(t *testing.T) {
	ctx := context.Background()
	jobs := testutil.NewFakeJobsRepo()
	j := domain.NewJob("j_1", "u_1", "o/r", "main", "agentic/x", domain.SpecSource{Type: domain.SourceTypePath, Value: "specs/x.md"}, "", time.Unix(1000, 0))
	_ = jobs.Create(ctx, j)

	uc := &usecase.GetJob{Jobs: jobs}
	got, err := uc.Execute(ctx, usecase.GetJobInput{JobID: "j_1", UserID: "u_1"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got.ID != "j_1" {
		t.Fatalf("wrong job returned")
	}
}

func TestGetJob_nonOwnerSees404(t *testing.T) {
	ctx := context.Background()
	jobs := testutil.NewFakeJobsRepo()
	j := domain.NewJob("j_1", "u_1", "o/r", "main", "agentic/x", domain.SpecSource{Type: domain.SourceTypePath, Value: "specs/x.md"}, "", time.Unix(1000, 0))
	_ = jobs.Create(ctx, j)

	uc := &usecase.GetJob{Jobs: jobs}
	_, err := uc.Execute(ctx, usecase.GetJobInput{JobID: "j_1", UserID: "u_2"})
	if err != domain.ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
