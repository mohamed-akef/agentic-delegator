// core/adapter/ghapp/repo_creds.go
package ghapp

import (
	"context"

	"agentic-delegator/core/adapter/postgres"
	"agentic-delegator/core/domain"
)

// InstallationsLookup is the slice of postgres.InstallationsRepo we depend on.
type InstallationsLookup interface {
	ByUserAndRepo(ctx context.Context, userID domain.UserID, repo string) (*postgres.Installation, error)
}

// RepoCredsProvider implements ports.RepoCredentialsProvider using GitHub App
// installation tokens minted on demand via AppClient.
type RepoCredsProvider struct {
	app    *AppClient
	lookup InstallationsLookup
}

// NewRepoCredsProvider returns a RepoCredsProvider backed by the given AppClient and lookup.
func NewRepoCredsProvider(app *AppClient, lookup InstallationsLookup) *RepoCredsProvider {
	return &RepoCredsProvider{app: app, lookup: lookup}
}

// For resolves the GitHub App installation for userID/repo and mints a fresh token.
// Returns domain.ErrNotFound if no installation covers the repo.
func (p *RepoCredsProvider) For(ctx context.Context, userID domain.UserID, repo string) (domain.GitCreds, error) {
	inst, err := p.lookup.ByUserAndRepo(ctx, userID, repo)
	if err != nil {
		return domain.GitCreds{}, err
	}
	tok, exp, err := p.app.InstallationToken(ctx, inst.InstallationID)
	if err != nil {
		return domain.GitCreds{}, err
	}
	return domain.GitCreds{Token: tok, ExpiresAt: exp}, nil
}
