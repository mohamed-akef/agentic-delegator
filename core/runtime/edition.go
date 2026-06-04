// core/runtime/edition.go
package runtime

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

// Edition is the variability surface between OSS selfhost and SaaS.
// Core never imports either implementation directly; cmd/* wires the right one.
type Edition interface {
	Name() string // "selfhost" | "saas"

	// RegisterRoutes lets an edition mount its own routes (admin setup,
	// signup, GitHub-App webhooks, etc.) onto the chi router.
	RegisterRoutes(r chi.Router)

	// ResolveUser turns an HTTP request into a UserID. Selfhost returns the
	// single admin's UserID for any authenticated request; SaaS resolves a
	// session cookie or bearer API key to the owning user.
	ResolveUser(r *http.Request) (domain.UserID, error)

	// RepoCredentialsProvider returns the port impl that yields short-lived
	// git creds for cloning/pushing on a user+repo's behalf.
	RepoCredentialsProvider() ports.RepoCredentialsProvider

	// AnthropicCredentialsProvider returns the port impl that yields the
	// Anthropic credential for a user.
	AnthropicCredentialsProvider() ports.AnthropicCredentialsProvider

	// Bootstrap runs once at startup. Selfhost: ensures the admin user row
	// exists. SaaS: no-op.
	Bootstrap(ctx context.Context) error
}
