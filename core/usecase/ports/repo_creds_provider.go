// core/usecase/ports/repo_creds_provider.go
package ports

import (
	"context"

	"agentic-delegator/core/domain"
)

// RepoCredentialsProvider returns short-lived git credentials for a user+repo.
// Mints fresh short-lived git credentials for a user+repo via the GitHub App.
type RepoCredentialsProvider interface {
	For(ctx context.Context, userID domain.UserID, repo string) (domain.GitCreds, error)
}
