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
	"time"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type Config struct {
	Image         string   // e.g. "ghcr.io/<owner>/agentic-delegator-runner:latest"
	EntryOverride []string // optional: override the image's entrypoint for tests (nil = default)
	CPUs          string   // e.g. "2"
	MemoryMB      int      // e.g. 2048
	PidsLimit     int      // max processes in the container (0 = adapter default)
	Network       string   // runner bridge network for egress filtering; "" => no --network
	DNS           []string // public DNS resolvers; emitted as --dns only when Network != ""
	WorkDirHost   string   // host directory where per-job workspaces are created (e.g. /var/lib/delegator/work)
	// MaxJobDuration bounds a single container's run time. After it elapses the
	// container is killed and the job reported as failed. 0 = adapter default.
	MaxJobDuration time.Duration
}

// logTailBytes is how much of the tail of the log we keep for the webhook payload.
const logTailBytes = 8 << 10 // 8 KiB

type Runner struct {
	cfg Config
	mu  sync.Mutex
}

func New(cfg Config) *Runner {
	if cfg.WorkDirHost == "" {
		cfg.WorkDirHost = filepath.Join(os.TempDir(), "agentic-delegator")
	}
	if cfg.PidsLimit == 0 {
		cfg.PidsLimit = 512
	}
	if cfg.MaxJobDuration == 0 {
		cfg.MaxJobDuration = 30 * time.Minute
	}
	_ = os.MkdirAll(cfg.WorkDirHost, 0o700)
	// MkdirAll is umask-subject and does not tighten a pre-existing dir, so chmod
	// explicitly: WorkDirHost is the host-exposure gate for the world-traversable
	// per-job dirs (0777 workspace, 0711 secrets) created under it.
	_ = os.Chmod(cfg.WorkDirHost, 0o700)
	return &Runner{cfg: cfg}
}

var _ ports.RunnerService = (*Runner)(nil)

// Start spawns a container and supervises it in a goroutine. The onComplete
// callback fires from the supervisor goroutine after the container exits.
func (r *Runner) Start(ctx context.Context, spec ports.RunnerStartSpec, onComplete func(ports.RunnerResult)) (string, error) {
	jobDir := filepath.Join(r.cfg.WorkDirHost, string(spec.JobID))
	if err := os.MkdirAll(jobDir, 0o777); err != nil {
		_ = os.RemoveAll(jobDir)
		return "", err
	}
	// The container runs as root (uid 0) but, under --cap-drop=ALL, lacks
	// CAP_DAC_OVERRIDE — so it cannot bypass DAC checks to write into a host
	// directory it does not own. Force the workspace mode to 0777 (MkdirAll is
	// subject to umask) so the container can write its artifacts (.pr-url,
	// .notification-webhook, the clone) into the bind mount regardless of which
	// uid it runs as. Exposure is contained: the parent WorkDirHost is 0700, so
	// this per-job dir is not reachable by other host users, and it is removed
	// when the job completes.
	if err := os.Chmod(jobDir, 0o777); err != nil {
		_ = os.RemoveAll(jobDir)
		return "", err
	}

	// Secrets are delivered via a read-only bind-mounted sibling dir, not -e env.
	secretsDir, err := writeSecrets(r.cfg.WorkDirHost, string(spec.JobID), spec.GitCreds.Token, spec.Anthropic.APIKey)
	if err != nil {
		_ = os.RemoveAll(jobDir)
		_ = os.RemoveAll(secretsDir)
		return "", err
	}

	args := buildRunArgs(r.cfg, spec, jobDir, secretsDir)

	out, err := exec.CommandContext(ctx, "docker", args...).Output()
	if err != nil {
		// supervise never runs if docker run fails, so clean both dirs here
		// (no plaintext tokens left on disk).
		_ = os.RemoveAll(jobDir)
		_ = os.RemoveAll(secretsDir)
		return "", fmt.Errorf("docker run: %w", err)
	}
	containerID := strings.TrimSpace(string(out))

	// Supervise the container in a goroutine.
	go r.supervise(containerID, jobDir, secretsDir, spec.LogPath, spec.JobID, onComplete)

	return containerID, nil
}

// buildRunArgs constructs the full `docker run` argument list for one job. It is
// pure (no side effects) so the flag composition can be unit-tested without a
// docker daemon. Image must remain the last non-EntryOverride argument.
//
// Egress to private/link-local/metadata ranges is blocked at the host
// DOCKER-USER firewall layer (keyed on the runner bridge subnet); public egress
// is intentionally allowed (clone, Anthropic API, PR). The --network/--dns flags
// are emitted only when a runner network is configured; dev/local/CI leave it
// empty and run unfiltered, exactly as before.
func buildRunArgs(cfg Config, spec ports.RunnerStartSpec, jobDir, secretsDir string) []string {
	args := []string{"run", "-d",
		// Isolation hardening: drop all Linux capabilities, forbid privilege
		// escalation, and cap the process count.
		"--cap-drop=ALL",
		"--security-opt", "no-new-privileges",
		"--pids-limit", fmt.Sprintf("%d", cfg.PidsLimit),
		"-e", "JOB_ID=" + string(spec.JobID),
		"-e", "REPO=" + spec.Repo,
		"-e", "BASE_BRANCH=" + spec.BaseBranch,
		"-e", "WORK_BRANCH=" + spec.WorkBranch,
		// GH_TOKEN / ANTHROPIC_API_KEY are NOT passed as -e (they'd be visible via
		// docker inspect). They are delivered through the read-only secrets mount
		// below and read from files by the entrypoint.
		"-e", "MODEL_OVERRIDE=" + spec.Model,
		"-e", "SPEC_TYPE=" + string(spec.Spec.Type),
		"-e", "SPEC_VALUE=" + spec.Spec.Value,
		"-v", jobDir + ":/workspace",
		"-v", secretsDir + ":" + secretsMountPath + ":ro",
	}
	if cfg.CPUs != "" {
		args = append(args, "--cpus", cfg.CPUs)
	}
	if cfg.MemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", cfg.MemoryMB))
	}
	if cfg.Network != "" {
		args = append(args, "--network", cfg.Network)
		for _, d := range cfg.DNS {
			args = append(args, "--dns", d)
		}
	}
	args = append(args, cfg.Image)
	args = append(args, cfg.EntryOverride...)
	return args
}

func (r *Runner) supervise(containerID, jobDir, secretsDir, logPath string, jobID domain.JobID, onComplete func(ports.RunnerResult)) {
	// 1. Wait for the container to finish (bounded by MaxJobDuration), then
	// stream logs to logPath. If the deadline elapses we kill the container and
	// fall through with a non-zero exit so the job is marked failed.
	timedOut := false
	waitCtx, cancel := context.WithTimeout(context.Background(), r.cfg.MaxJobDuration)
	defer cancel()
	if err := exec.CommandContext(waitCtx, "docker", "wait", containerID).Run(); err != nil {
		if waitCtx.Err() == context.DeadlineExceeded {
			timedOut = true
			_ = exec.Command("docker", "kill", containerID).Run()
		}
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err == nil {
		defer logFile.Close()
		logsCmd := exec.Command("docker", "logs", containerID)
		logsCmd.Stdout = logFile
		logsCmd.Stderr = logFile
		_ = logsCmd.Run()
	}

	// 2. Get the exit code.
	exitCode := 0
	if timedOut {
		exitCode = 124 // conventional timeout exit code
	} else {
		exitOut, _ := exec.Command("docker", "inspect", "--format", "{{.State.ExitCode}}", containerID).Output()
		_, _ = fmt.Sscanf(strings.TrimSpace(string(exitOut)), "%d", &exitCode)
	}

	// 3. Pick up artifacts the runner wrote before we tear the workspace down.
	prURL := readTrimmed(filepath.Join(jobDir, ".pr-url"))
	notifyURL := readTrimmed(filepath.Join(jobDir, ".notification-webhook"))
	logTail := tailFile(logPath, logTailBytes)

	// 4. Remove the container, its workspace, and its secrets dir now that we've
	// extracted everything we need (covers the timeout-kill and cancel paths,
	// since docker kill unblocks the wait above and falls through to here).
	_ = exec.Command("docker", "rm", "-f", containerID).Run()
	_ = os.RemoveAll(jobDir)
	_ = os.RemoveAll(secretsDir)

	res := ports.RunnerResult{
		JobID:               jobID,
		ExitCode:            exitCode,
		PRURL:               prURL,
		NotificationWebhook: notifyURL,
		LogTail:             logTail,
	}
	switch {
	case timedOut:
		res.Error = fmt.Sprintf("runner exceeded max duration %s", r.cfg.MaxJobDuration)
	case exitCode != 0:
		res.Error = fmt.Sprintf("runner exited with code %d", exitCode)
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

func readTrimmed(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// tailFile returns the last max bytes of the file, starting at a line boundary.
func tailFile(path string, max int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(b) > max {
		b = b[len(b)-max:]
		if i := strings.IndexByte(string(b), '\n'); i >= 0 && i+1 < len(b) {
			b = b[i+1:]
		}
	}
	return string(b)
}
