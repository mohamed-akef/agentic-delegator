//go:build integration

// core/adapter/docker/secrets_perms_integration_test.go
package docker_test

import (
	"os/exec"
	"strings"
	"testing"
)

// TestSecretsDir_0711ReadableByCapDropRunner_0700Not is the regression guard for
// the secret-isolation blocker: a uid-0 container under --cap-drop=ALL (no
// CAP_DAC_READ_SEARCH) is in the "other" DAC class relative to the non-root
// orchestrator that owns the secrets dir, so it needs the dir's SEARCH bit. A
// 0711 dir grants it; a 0700 dir does not (the secrets would be unreadable and
// every job would fail). Uses a Docker named volume so the fs has faithful
// guest DAC — a macOS bind mount would falsely pass via Docker Desktop's
// virtiofs, which ignores guest permissions.
func TestSecretsDir_0711ReadableByCapDropRunner_0700Not(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	const vol = "delegator-secrets-perms-test"
	_ = exec.Command("docker", "volume", "rm", "-f", vol).Run()
	if out, err := exec.Command("docker", "volume", "create", vol).CombinedOutput(); err != nil {
		t.Skipf("cannot create docker volume: %v: %s", err, out)
	}
	defer exec.Command("docker", "volume", "rm", "-f", vol).Run()

	// As root (with caps), build the two layouts owned by a non-root uid.
	setup := `mkdir -p /data/d700 /data/d711 && ` +
		`printf TOK > /data/d700/gh-token && printf TOK > /data/d711/gh-token && ` +
		`chown -R 1000:1000 /data/d700 /data/d711 && ` +
		`chmod 0700 /data/d700 && chmod 0711 /data/d711 && ` +
		`chmod 0644 /data/d700/gh-token /data/d711/gh-token`
	if out, err := exec.Command("docker", "run", "--rm", "-v", vol+":/data", "alpine:3.20", "sh", "-c", setup).CombinedOutput(); err != nil {
		t.Fatalf("setup: %v: %s", err, out)
	}

	// 0711 dir: the no-caps uid-0 runner CAN read the leaf by known name.
	out, err := exec.Command("docker", "run", "--rm", "--cap-drop=ALL", "-v", vol+":/data:ro",
		"alpine:3.20", "cat", "/data/d711/gh-token").CombinedOutput()
	if err != nil {
		t.Fatalf("0711 secrets dir must be readable by the --cap-drop=ALL runner: %v: %s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "TOK" {
		t.Fatalf("0711 read: want TOK, got %q", got)
	}

	// 0700 dir: the same runner CANNOT traverse it — this is the blocker we fixed.
	if out, err := exec.Command("docker", "run", "--rm", "--cap-drop=ALL", "-v", vol+":/data:ro",
		"alpine:3.20", "cat", "/data/d700/gh-token").CombinedOutput(); err == nil {
		t.Fatalf("0700 secrets dir must NOT be readable by the --cap-drop=ALL runner, but cat succeeded: %s", out)
	}
}
