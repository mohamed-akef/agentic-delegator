// core/runtime/selfhost/repo_creds_test.go
package selfhost_test

import (
	"context"
	"testing"

	"agentic-delegator/core/runtime/selfhost"
)

type fakePAT struct {
	pat string
	err error
}

func (f *fakePAT) Set(ctx context.Context, pat string) error { f.pat = pat; return nil }
func (f *fakePAT) Get(ctx context.Context) (string, error)   { return f.pat, f.err }

func TestRepoCreds_returnsPATAsToken(t *testing.T) {
	store := &fakePAT{pat: "ghp_xxx"}
	p := selfhost.NewRepoCredsProvider(store)
	creds, err := p.For(context.Background(), "u_admin", "owner/repo")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if creds.Token != "ghp_xxx" {
		t.Fatalf("token mismatch: %q", creds.Token)
	}
}
