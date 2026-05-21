package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	adhttp "agentic-delegator/core/adapter/http"
	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

func TestStatusPage_rendersHTML(t *testing.T) {
	jobs := testutil.NewFakeJobsRepo()
	j := domain.NewJob("j_1", "u_1", "o/r", "main", "agentic/x", domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "", time.Unix(1000, 0))
	j.LogPath = "/tmp/agentic-delegator-status-test-nonexistent.log"
	_ = jobs.Create(context.Background(), j)

	page := adhttp.NewStatusPage(&usecase.GetJob{Jobs: jobs})

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), adhttp.UserIDKey, domain.UserID("u_1"))
		page.Render(w, r.WithContext(ctx), "j_1")
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/jobs/j_1", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Job ") || !strings.Contains(body, "j_1") {
		t.Fatalf("body missing job id: %s", body[:min(300, len(body))])
	}
	if !strings.Contains(body, "<!doctype html>") {
		t.Fatalf("not HTML: %s", body[:min(300, len(body))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
