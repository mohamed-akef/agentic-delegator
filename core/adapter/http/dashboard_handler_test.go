// core/adapter/http/dashboard_handler_test.go
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

func TestDashboard_listsJobs(t *testing.T) {
	jobs := testutil.NewFakeJobsRepo()
	_ = jobs.Create(context.Background(),
		domain.NewJob("j_x", "u_1", "o/r", "main", "agentic/x",
			domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "",
			time.Unix(1000, 0)))

	h := adhttp.NewDashboardHandler(
		&usecase.ListJobs{Jobs: jobs},
		testutil.NewFakeAPIKeysRepo(),
		testutil.NewFakeSecretsRepo(),
	)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req = req.WithContext(context.WithValue(req.Context(), adhttp.UserIDKey, domain.UserID("u_1")))
	rec := httptest.NewRecorder()
	h.Dashboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "j_x") {
		t.Fatalf("body missing job id")
	}
}
