// test/e2e/fullcycle_test.go
package e2e

import (
	"net/http"
	"strings"
	"testing"

	"agentic-delegator/core/domain"
)

// TestFullCycle_HappyPath drives the entire delegation workflow end to end:
// GitHub sign-in (session) -> App install redirect -> set Anthropic key + mint
// an API key -> enqueue a job with the bearer key -> runner completes -> the
// job reports succeeded with a PR link over both the JSON API and the status
// page. It traverses both auth paths (session and bearer) in one run.
func TestFullCycle_HappyPath(t *testing.T) {
	h := newHarness(t, harnessOpts{})

	// 1) /login redirects into the GitHub OAuth dance.
	resp := h.get(h.browser, "/login", "")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("/login: want 302, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); !strings.Contains(loc, "github.com/login/oauth/authorize") {
		t.Fatalf("/login Location = %q, want GitHub authorize URL", loc)
	}
	resp.Body.Close()

	// 2) Complete the OAuth callback -> session cookie + user row.
	h.login()

	// 3) The GitHub-App install redirect points at the app's install page.
	resp = h.get(h.browser, "/auth/github-app/install", "")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("install: want 302, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); !strings.Contains(loc, "github.com/apps/test-app/installations/new") {
		t.Fatalf("install Location = %q", loc)
	}
	resp.Body.Close()

	// The install *callback* needs live GitHub (App JWT + non-injectable
	// client), so we seed the installation row to represent the granted access.
	// Repo creds during enqueue come from the fake provider regardless.
	h.seedInstallation()

	// 4) Store the Anthropic key and mint a per-user API key (session path).
	h.setAnthropicKey("sk-ant-e2e-test")
	key := h.mintAPIKey("laptop")

	// 5) Enqueue a job with the bearer key (the path the /delegate skill uses).
	resp, enq := h.enqueue(key, validSpecBody())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("enqueue: want 200, got %d (%s)", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()
	if enq.JobID == "" {
		t.Fatal("enqueue returned empty job_id")
	}
	if enq.StatusURL != "/jobs/"+enq.JobID {
		t.Fatalf("status_url = %q, want /jobs/%s", enq.StatusURL, enq.JobID)
	}

	// The runner must have been started with the creds set above — proving the
	// set-key -> read-at-enqueue loop and the bearer auth both wired through.
	id, spec, ok := h.runner.LastStarted()
	if !ok {
		t.Fatal("runner was not started")
	}
	if id == "" {
		t.Fatal("empty container id")
	}
	if spec.Repo != "tester/sandbox" || spec.WorkBranch != "agentic/hello" {
		t.Fatalf("runner spec repo/branch unexpected: %+v", spec)
	}
	if spec.Anthropic.APIKey != "sk-ant-e2e-test" {
		t.Fatalf("runner got anthropic key %q, want the one set via /settings", spec.Anthropic.APIKey)
	}
	if spec.GitCreds.Token != "ghs_faketoken" {
		t.Fatalf("runner got git token %q", spec.GitCreds.Token)
	}

	// Job is running before completion.
	_, job := h.getJob(key, enq.JobID)
	if job == nil || job.Status != domain.JobStatusRunning {
		t.Fatalf("pre-completion status = %v, want running", statusOf(job))
	}

	// 6) Runner exits 0 with a PR URL -> completion use case marks succeeded.
	const prURL = "https://github.com/tester/sandbox/pull/1"
	h.completeRunner(0, prURL, "")

	resp, job = h.getJob(key, enq.JobID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get job: want 200, got %d", resp.StatusCode)
	}
	if job.Status != domain.JobStatusSucceeded {
		t.Fatalf("post-completion status = %v, want succeeded", job.Status)
	}
	if job.PRURL != prURL {
		t.Fatalf("job PRURL = %q, want %q", job.PRURL, prURL)
	}

	// 7) The job appears in the bearer-authenticated list.
	listResp := h.get(h.cli, "/api/jobs", key)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list jobs: want 200, got %d", listResp.StatusCode)
	}
	var list []domain.Job
	decodeJSON(t, listResp, &list)
	if !containsJob(list, enq.JobID) {
		t.Fatalf("job %s not in list (%d jobs)", enq.JobID, len(list))
	}

	// 8) The HTML status page (session path) renders the PR link.
	pageResp := h.get(h.browser, "/jobs/"+enq.JobID, "")
	body := readBody(t, pageResp)
	if pageResp.StatusCode != http.StatusOK {
		t.Fatalf("status page: want 200, got %d", pageResp.StatusCode)
	}
	if !strings.Contains(body, prURL) {
		t.Fatalf("status page does not render the PR URL")
	}
}

func statusOf(j *domain.Job) any {
	if j == nil {
		return "<nil job>"
	}
	return j.Status
}

func containsJob(list []domain.Job, id string) bool {
	for _, j := range list {
		if string(j.ID) == id {
			return true
		}
	}
	return false
}
