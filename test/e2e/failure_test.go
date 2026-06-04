// test/e2e/failure_test.go
package e2e

import (
	"net/http"
	"strings"
	"testing"

	"agentic-delegator/core/domain"
)

// TestEnqueue_RejectsUnauthenticated verifies the bearer/session gate: no
// credentials and a bogus key both get 401, and no job is started.
func TestEnqueue_RejectsUnauthenticated(t *testing.T) {
	h := newHarness(t, harnessOpts{})

	resp, _ := h.enqueue("", validSpecBody()) // no auth at all
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-auth enqueue: want 401, got %d (%s)", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()

	resp, _ = h.enqueue("agdkey_does_not_exist_0000", validSpecBody()) // bogus key
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad-key enqueue: want 401, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	if _, _, ok := h.runner.LastStarted(); ok {
		t.Fatal("a runner was started for an unauthenticated request")
	}
}

// TestEnqueue_MissingAnthropicCreds verifies that enqueuing without an
// Anthropic key set is rejected and never starts a runner.
func TestEnqueue_MissingAnthropicCreds(t *testing.T) {
	h := newHarness(t, harnessOpts{})
	h.login()
	h.seedInstallation()
	key := h.mintAPIKey("laptop") // note: no setAnthropicKey

	resp, _ := h.enqueue(key, validSpecBody())
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("enqueue without anthropic key: want 400, got %d (%s)", resp.StatusCode, readBody(t, resp))
	}
	resp.Body.Close()

	if _, _, ok := h.runner.LastStarted(); ok {
		t.Fatal("runner started despite missing Anthropic creds")
	}
}

// TestEnqueue_ConcurrencyCap verifies the per-user cap leaves the second job
// queued and does not start a runner for it.
func TestEnqueue_ConcurrencyCap(t *testing.T) {
	h := newHarness(t, harnessOpts{maxPerUser: 1})
	h.login()
	h.seedInstallation()
	h.setAnthropicKey("sk-ant-e2e-test")
	key := h.mintAPIKey("laptop")

	// First job starts and runs.
	resp, first := h.enqueue(key, validSpecBody())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first enqueue: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Second job is accepted but must stay queued (cap reached).
	body2 := validSpecBody()
	body2["work_branch"] = "agentic/second"
	resp, second := h.enqueue(key, body2)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second enqueue: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	if len(h.runner.StartedSpecs) != 1 {
		t.Fatalf("want exactly 1 runner started, got %d", len(h.runner.StartedSpecs))
	}

	_, firstJob := h.getJob(key, first.JobID)
	if firstJob.Status != domain.JobStatusRunning {
		t.Fatalf("first job status = %v, want running", firstJob.Status)
	}
	_, secondJob := h.getJob(key, second.JobID)
	if secondJob.Status != domain.JobStatusQueued {
		t.Fatalf("second job status = %v, want queued", secondJob.Status)
	}
}

// TestRunnerFailure_MarksJobFailed verifies the non-zero-exit completion path.
func TestRunnerFailure_MarksJobFailed(t *testing.T) {
	h := newHarness(t, harnessOpts{})
	h.login()
	h.seedInstallation()
	h.setAnthropicKey("sk-ant-e2e-test")
	key := h.mintAPIKey("laptop")

	_, enq := h.enqueue(key, validSpecBody())

	h.completeRunner(17, "", "clone failed: permission denied")

	_, job := h.getJob(key, enq.JobID)
	if job.Status != domain.JobStatusFailed {
		t.Fatalf("status = %v, want failed", job.Status)
	}
	if !strings.Contains(job.Error, "clone failed") {
		t.Fatalf("job error = %q, want it to mention the failure reason", job.Error)
	}
	if job.PRURL != "" {
		t.Fatalf("failed job should have no PR URL, got %q", job.PRURL)
	}
}
