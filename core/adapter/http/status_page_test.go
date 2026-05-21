// core/adapter/http/status_page_test.go
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

func TestStatusPage_rendersJob(t *testing.T) {
	jobs := testutil.NewFakeJobsRepo()
	j := domain.NewJob("j_1", "u_1", "o/r", "main", "agentic/x", domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "", time.Unix(1000, 0))
	j.LogPath = "/tmp/x.log"
	_ = jobs.Create(context.Background(), j)

	page := adhttp.NewStatusPage(&usecase.GetJob{Jobs: jobs})

	// Wrap to inject the userID, since the page expects it in context.
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		_ = ctx
		// In the real router, BearerOrSession populates userID. For this test, do it directly.
		page.Render(w, r.WithContext(injectUser(r.Context(), "u_1")), "j_1")
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/jobs/j_1", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Job j_1") {
		t.Fatalf("body missing job id: %s", rec.Body.String())
	}
}

// injectUser places a userID into the request context using the same key the
// middleware uses. Uses the exported UserIDKey from the adapter package.
func injectUser(ctx context.Context, uid domain.UserID) context.Context {
	return context.WithValue(ctx, adhttp.UserIDKey, uid)
}
