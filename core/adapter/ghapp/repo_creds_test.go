// core/adapter/ghapp/repo_creds_test.go
package ghapp_test

import (
	"context"
	"errors"
	"testing"

	"agentic-delegator/core/adapter/ghapp"
	"agentic-delegator/core/adapter/postgres"
	"agentic-delegator/core/domain"
)

type fakeLookup struct {
	byPair map[string]*postgres.Installation
}

func (f *fakeLookup) ByUserAndRepo(_ context.Context, uid domain.UserID, repo string) (*postgres.Installation, error) {
	if v, ok := f.byPair[string(uid)+"|"+repo]; ok {
		return v, nil
	}
	return nil, domain.ErrNotFound
}

func TestRepoCreds_propagatesNotFound(t *testing.T) {
	p := ghapp.NewRepoCredsProvider(nil, &fakeLookup{byPair: map[string]*postgres.Installation{}})
	_, err := p.For(context.Background(), "u_x", "owner/repo")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
