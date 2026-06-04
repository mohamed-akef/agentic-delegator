// core/runtime/selfhost/repo_creds.go
package selfhost

import (
	"context"
	"time"

	"agentic-delegator/core/domain"
)

type RepoCredsProvider struct {
	pat PATStore
}

func NewRepoCredsProvider(pat PATStore) *RepoCredsProvider {
	return &RepoCredsProvider{pat: pat}
}

func (p *RepoCredsProvider) For(ctx context.Context, userID domain.UserID, repo string) (domain.GitCreds, error) {
	tok, err := p.pat.Get(ctx)
	if err != nil {
		return domain.GitCreds{}, err
	}
	// PAT has no expiry tracked; zero ExpiresAt = "never expired" (per domain.GitCreds.Expired).
	_ = time.Time{}
	return domain.GitCreds{Token: tok}, nil
}
