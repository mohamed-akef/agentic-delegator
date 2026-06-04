// core/usecase/ports/repo_creds_provider.go
package ports

import (
	"context"

	"agentic-delegator/core/domain"
)

// RepoCredentialsProvider returns short-lived git credentials for a user+repo.
// Edition-specific: selfhost returns the admin's PAT, SaaS mints a fresh
// GitHub App installation token.
type RepoCredentialsProvider interface {
	For(ctx context.Context, userID domain.UserID, repo string) (domain.GitCreds, error)
}
