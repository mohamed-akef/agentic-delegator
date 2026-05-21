//go:build integration

// core/adapter/docker/runner_test.go
package docker_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agentic-delegator/core/adapter/docker"
	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

func TestDockerRunner_helloWorldExitsZero(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "j.log")

	r := docker.New(docker.Config{
		Image:         "alpine:3.20",
		EntryOverride: []string{"sh", "-c", `echo "hello $JOB_ID"; mkdir -p /workspace && echo "https://example/pr/1" > /workspace/.pr-url; exit 0`},
		CPUs:          "0.5",
		MemoryMB:      256,
	})

	done := make(chan ports.RunnerResult, 1)
	containerID, err := r.Start(context.Background(),
		ports.RunnerStartSpec{
			JobID:      domain.JobID("j_test_1"),
			Repo:       "owner/repo",
			BaseBranch: "main",
			WorkBranch: "agentic/x",
			Spec:       domain.SpecSource{Type: domain.SourceTypeInline, Value: "noop"},
			GitCreds:   domain.GitCreds{Token: "fake-token"},
			Anthropic:  domain.AnthropicCreds{APIKey: "sk-fake"},
			LogPath:    logPath,
		},
		func(res ports.RunnerResult) { done <- res },
	)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if containerID == "" {
		t.Fatalf("container id empty")
	}

	select {
	case res := <-done:
		if res.ExitCode != 0 {
			t.Fatalf("exit code: want 0, got %d (error=%q)", res.ExitCode, res.Error)
		}
		if res.PRURL != "https://example/pr/1" {
			t.Fatalf("pr_url mismatch: %q", res.PRURL)
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("runner did not complete in 30s")
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !contains(logBytes, "hello j_test_1") {
		t.Fatalf("log missing expected output:\n%s", logBytes)
	}
}

func contains(haystack []byte, needle string) bool {
	return string(haystack) != "" && (len(needle) == 0 || (indexOf(string(haystack), needle) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
