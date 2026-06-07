// core/adapter/docker/secrets_test.go
package docker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteSecrets_modesAndContent(t *testing.T) {
	work := t.TempDir()
	secretsDir, err := writeSecrets(work, "j1", "ghs_TOKEN", "sk-KEY")
	if err != nil {
		t.Fatalf("writeSecrets: %v", err)
	}
	if want := filepath.Join(work, "j1.secrets"); secretsDir != want {
		t.Fatalf("secretsDir: want %q, got %q", want, secretsDir)
	}

	di, err := os.Stat(secretsDir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	// 0711 (NOT 0700): the uid-0 --cap-drop=ALL runner needs the search bit to
	// traverse into the dir; host exposure is gated by the 0700 WorkDirHost parent.
	if got := di.Mode().Perm(); got != 0o711 {
		t.Fatalf("secrets dir mode: want 0711, got %o", got)
	}

	for name, want := range map[string]string{"gh-token": "ghs_TOKEN", "anthropic-key": "sk-KEY"} {
		p := filepath.Join(secretsDir, name)
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if got := fi.Mode().Perm(); got != 0o644 {
			t.Fatalf("%s mode: want 0644, got %o", name, got)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(b) != want { // raw, no trailing newline
			t.Fatalf("%s content: want %q, got %q", name, want, string(b))
		}
	}
}

func TestSweepOrphanSecrets_removesNonRunningKeepsRunningAndLogs(t *testing.T) {
	work := t.TempDir()
	for _, d := range []string{"j1.secrets", "j2.secrets", "logs"} {
		if err := os.MkdirAll(filepath.Join(work, d), 0o711); err != nil {
			t.Fatal(err)
		}
	}

	removed := SweepOrphanSecrets(work, map[string]bool{"j1": true})

	if _, err := os.Stat(filepath.Join(work, "j1.secrets")); err != nil {
		t.Fatalf("running job j1.secrets must be kept: %v", err)
	}
	if _, err := os.Stat(filepath.Join(work, "j2.secrets")); !os.IsNotExist(err) {
		t.Fatalf("non-running job j2.secrets must be removed (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(work, "logs")); err != nil {
		t.Fatalf("logs sibling must never be swept: %v", err)
	}
	if len(removed) != 1 || filepath.Base(removed[0]) != "j2.secrets" {
		t.Fatalf("removed: want [.../j2.secrets], got %v", removed)
	}
}
