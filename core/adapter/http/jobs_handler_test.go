// core/adapter/http/jobs_handler_test.go
package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	adhttp "agentic-delegator/core/adapter/http"
	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

type stubResolver struct {
	uid domain.UserID
	err error
}

func (s stubResolver) Resolve(r *http.Request) (domain.UserID, error) { return s.uid, s.err }

func newRouter(t *testing.T) (chi.Router, *testutil.FakeJobsRepo) {
	t.Helper()
	jobs := testutil.NewFakeJobsRepo()
	clock := testutil.NewFakeClock(time.Unix(1000, 0))
	idg := &testutil.FakeIDGenerator{}

	enqueue := &usecase.EnqueueJob{
		Jobs:           jobs,
		RepoCreds:      testutil.NewFakeRepoCredsProvider(domain.GitCreds{Token: "x"}),
		AnthropicCreds: testutil.NewFakeAnthropicCredsProvider(domain.AnthropicCreds{APIKey: "y"}),
		Runner:         testutil.NewFakeRunnerService(),
		IDGen:          idg,
		Clock:          clock,
	}
	get := &usecase.GetJob{Jobs: jobs}
	list := &usecase.ListJobs{Jobs: jobs}

	r := adhttp.NewRouter(adhttp.Deps{
		Resolver:        stubResolver{uid: "u_1"},
		JobsHandler:     adhttp.NewJobsHandler(enqueue, get, list),
		SettingsHandler: adhttp.NewSettingsHandler(nil, nil, nil), // not exercised here
	})
	return r, jobs
}

func TestPOSTJobs_enqueues(t *testing.T) {
	r, jobs := newRouter(t)
	body := `{"repo":"owner/repo","base_branch":"main","work_branch":"agentic/x","spec_source":"specs/x.md","source_type":"path"}`
	req := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		JobID     string `json:"job_id"`
		StatusURL string `json:"status_url"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.JobID == "" {
		t.Fatalf("job_id missing in response: %s", rec.Body.String())
	}
	// confirm it's persisted
	got, _ := jobs.Get(context.Background(), domain.JobID(out.JobID))
	if got == nil {
		t.Fatalf("job not persisted")
	}
}

func TestGETJobsID_returnsJob(t *testing.T) {
	r, jobs := newRouter(t)
	// seed one
	j := domain.NewJob("j_seed", "u_1", "o/r", "main", "agentic/x",
		domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "", time.Unix(1000, 0))
	j.LogPath = "/tmp/x.log"
	_ = jobs.Create(context.Background(), j)

	req := httptest.NewRequest(http.MethodGet, "/api/jobs/j_seed", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
}
