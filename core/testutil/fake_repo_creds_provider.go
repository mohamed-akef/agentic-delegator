// core/testutil/fake_repo_creds_provider.go
package testutil

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type FakeRepoCredsProvider struct {
	Creds domain.GitCreds
	Err   error
}

func NewFakeRepoCredsProvider(c domain.GitCreds) *FakeRepoCredsProvider {
	return &FakeRepoCredsProvider{Creds: c}
}

var _ ports.RepoCredentialsProvider = (*FakeRepoCredsProvider)(nil)

func (p *FakeRepoCredsProvider) For(ctx context.Context, userID domain.UserID, repo string) (domain.GitCreds, error) {
	if p.Err != nil {
		return domain.GitCreds{}, p.Err
	}
	return p.Creds, nil
}
