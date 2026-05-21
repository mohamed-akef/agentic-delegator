// core/runtime/selfhost/edition.go
package selfhost

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

// AdminUserID is the fixed UserID assigned to the single admin user in
// selfhost mode. Anything that reaches a user-resolution boundary uses this.
const AdminUserID domain.UserID = "u_admin"

// Edition implements runtime.Edition for the OSS selfhost binary.
type Edition struct {
	repoCreds      ports.RepoCredentialsProvider
	anthropicCreds ports.AnthropicCredentialsProvider
	bootstrap      *AdminBootstrap
	adminKeyHash   []byte // bcrypt hash of the admin API key; populated at init
}

func New(
	repoCreds ports.RepoCredentialsProvider,
	anthropicCreds ports.AnthropicCredentialsProvider,
	bootstrap *AdminBootstrap,
	adminKeyHash []byte,
) *Edition {
	return &Edition{
		repoCreds:      repoCreds,
		anthropicCreds: anthropicCreds,
		bootstrap:      bootstrap,
		adminKeyHash:   adminKeyHash,
	}
}

func (e *Edition) Name() string { return "selfhost" }

func (e *Edition) RegisterRoutes(r chi.Router) {
	r.Get("/admin/setup", e.bootstrap.SetupPageHandler)
	r.Post("/admin/setup/pat", e.bootstrap.SetPATHandler)
}

func (e *Edition) ResolveUser(r *http.Request) (domain.UserID, error) {
	token := extractBearer(r)
	if token == "" {
		return "", errors.New("missing bearer token")
	}
	if !e.bootstrap.CompareAdminKey(token) {
		return "", errors.New("invalid bearer token")
	}
	return AdminUserID, nil
}

func (e *Edition) RepoCredentialsProvider() ports.RepoCredentialsProvider {
	return e.repoCreds
}

func (e *Edition) AnthropicCredentialsProvider() ports.AnthropicCredentialsProvider {
	return e.anthropicCreds
}

func (e *Edition) Bootstrap(ctx context.Context) error {
	return e.bootstrap.EnsureAdminUser(ctx)
}

func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) < 7 || h[:7] != "Bearer " {
		return ""
	}
	return h[7:]
}
