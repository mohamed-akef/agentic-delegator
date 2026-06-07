// core/adapter/docker/secrets.go
package docker

import (
	"os"
	"path/filepath"
	"strings"
)

// secretsMountPath is where the per-job secrets dir is bind-mounted (read-only)
// inside the runner container. The entrypoint reads gh-token / anthropic-key
// from here instead of from -e env vars.
const secretsMountPath = "/run/delegator-secrets"

// writeSecrets creates the per-job secrets dir and writes the GitHub token and
// Anthropic key as raw files (no trailing newline).
//
// Modes are load-bearing: the dir is 0711 (search-only) — NOT 0700 — so the
// container, which runs as uid 0 under --cap-drop=ALL (no CAP_DAC_READ_SEARCH)
// and is therefore in the "other" DAC class, can traverse into it to read the
// 0644 files. Host exposure is gated by the 0700 WorkDirHost parent, not by
// this dir's mode. (A 0700 dir would make the secrets unreadable by the
// container and fail every job.) MkdirAll/WriteFile are umask-subject, so each
// mode is set explicitly with Chmod.
func writeSecrets(workDirHost, jobID, ghToken, anthropicKey string) (string, error) {
	secretsDir := filepath.Join(workDirHost, jobID+".secrets")
	if err := os.MkdirAll(secretsDir, 0o711); err != nil {
		return "", err
	}
	if err := os.Chmod(secretsDir, 0o711); err != nil {
		return "", err
	}
	if err := writeSecretFile(filepath.Join(secretsDir, "gh-token"), ghToken); err != nil {
		return "", err
	}
	if err := writeSecretFile(filepath.Join(secretsDir, "anthropic-key"), anthropicKey); err != nil {
		return "", err
	}
	return secretsDir, nil
}

func writeSecretFile(path, val string) error {
	if err := os.WriteFile(path, []byte(val), 0o644); err != nil {
		return err
	}
	return os.Chmod(path, 0o644)
}

// SweepOrphanSecrets removes per-job "*.secrets" dirs under workDirHost whose
// job ID is not in running. It bounds the lifetime of secrets orphaned by an
// orchestrator restart (reattach skips supervise for still-alive jobs, so their
// cleanup never runs). Best-effort: per-entry errors are ignored. The glob
// matches only "*.secrets" — never the sibling "logs" dir. Returns the dirs
// removed (for logging).
func SweepOrphanSecrets(workDirHost string, running map[string]bool) []string {
	matches, _ := filepath.Glob(filepath.Join(workDirHost, "*.secrets"))
	var removed []string
	for _, dir := range matches {
		jobID := strings.TrimSuffix(filepath.Base(dir), ".secrets")
		if running[jobID] {
			continue
		}
		if os.RemoveAll(dir) == nil {
			removed = append(removed, dir)
		}
	}
	return removed
}
