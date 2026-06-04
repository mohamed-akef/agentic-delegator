// core/adapter/docker/runner.go
package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"agentic-delegator/core/usecase/ports"
)

type Config struct {
	Image         string   // e.g. "ghcr.io/<owner>/agentic-delegator-runner:latest"
	EntryOverride []string // optional: override the image's entrypoint for tests (nil = default)
	CPUs          string   // e.g. "2"
	MemoryMB      int      // e.g. 2048
	WorkDirHost   string   // host directory where per-job workspaces are created (e.g. /var/lib/delegator/work)
}

type Runner struct {
	cfg Config
	mu  sync.Mutex
}

func New(cfg Config) *Runner {
	if cfg.WorkDirHost == "" {
		cfg.WorkDirHost = filepath.Join(os.TempDir(), "agentic-delegator")
	}
	_ = os.MkdirAll(cfg.WorkDirHost, 0o755)
	return &Runner{cfg: cfg}
}

var _ ports.RunnerService = (*Runner)(nil)

// Start spawns a container and supervises it in a goroutine. The onComplete
// callback fires from the supervisor goroutine after the container exits.
func (r *Runner) Start(ctx context.Context, spec ports.RunnerStartSpec, onComplete func(ports.RunnerResult)) (string, error) {
	jobDir := filepath.Join(r.cfg.WorkDirHost, string(spec.JobID))
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		return "", err
	}

	args := []string{"run", "-d",
		"-e", "JOB_ID=" + string(spec.JobID),
		"-e", "REPO=" + spec.Repo,
		"-e", "BASE_BRANCH=" + spec.BaseBranch,
		"-e", "WORK_BRANCH=" + spec.WorkBranch,
		"-e", "GH_TOKEN=" + spec.GitCreds.Token,
		"-e", "ANTHROPIC_API_KEY=" + spec.Anthropic.APIKey,
		"-e", "MODEL_OVERRIDE=" + spec.Model,
		"-e", "SPEC_TYPE=" + string(spec.Spec.Type),
		"-e", "SPEC_VALUE=" + spec.Spec.Value,
		"-v", jobDir + ":/workspace",
	}
	if r.cfg.CPUs != "" {
		args = append(args, "--cpus", r.cfg.CPUs)
	}
	if r.cfg.MemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", r.cfg.MemoryMB))
	}
	args = append(args, r.cfg.Image)
	if len(r.cfg.EntryOverride) > 0 {
		args = append(args, r.cfg.EntryOverride...)
	}

	out, err := exec.CommandContext(ctx, "docker", args...).Output()
	if err != nil {
		return "", fmt.Errorf("docker run: %w", err)
	}
	containerID := strings.TrimSpace(string(out))

	// Supervise the container in a goroutine.
	go r.supervise(containerID, jobDir, spec.LogPath, spec.JobID, onComplete)

	return containerID, nil
}

func (r *Runner) supervise(containerID, jobDir, logPath string, jobID interface{}, onComplete func(ports.RunnerResult)) {
	// 1. Wait for the container to finish, then stream logs to logPath.
	// docker wait blocks until the container exits.
	_ = exec.Command("docker", "wait", containerID).Run()

	logFile, err := os.Create(logPath)
	if err == nil {
		defer logFile.Close()
		logsCmd := exec.Command("docker", "logs", containerID)
		logsCmd.Stdout = logFile
		logsCmd.Stderr = logFile
		_ = logsCmd.Run()
	}

	// 2. Get the exit code
	exitOut, _ := exec.Command("docker", "inspect", "--format", "{{.State.ExitCode}}", containerID).Output()
	exitCode := 0
	_, _ = fmt.Sscanf(strings.TrimSpace(string(exitOut)), "%d", &exitCode)

	// 3. Remove the container now that we've extracted what we need.
	_ = exec.Command("docker", "rm", "-f", containerID).Run()

	// 3. Pick up PR URL if the runner wrote one
	prURL := ""
	if b, err := os.ReadFile(filepath.Join(jobDir, ".pr-url")); err == nil {
		prURL = strings.TrimSpace(string(b))
	}

	res := ports.RunnerResult{
		ExitCode: exitCode,
		PRURL:    prURL,
	}
	if exitCode != 0 {
		res.Error = fmt.Sprintf("runner exited with code %d", exitCode)
	}
	if jid, ok := jobID.(interface{ String() string }); ok {
		_ = jid
	}

	if onComplete != nil {
		onComplete(res)
	}
}

func (r *Runner) Inspect(ctx context.Context, containerID string) (bool, error) {
	out, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.State.Running}}", containerID).Output()
	if err != nil {
		// Docker inspect returns non-zero if the container doesn't exist.
		return false, nil
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

func (r *Runner) Stop(ctx context.Context, containerID string) error {
	_ = exec.CommandContext(ctx, "docker", "kill", containerID).Run()
	return nil
}
