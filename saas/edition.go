//go:build saas

// saas/edition.go
package saas

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/runtime"
	"agentic-delegator/core/usecase/ports"
	"agentic-delegator/saas/ghapp"
	"agentic-delegator/saas/signup"
	"agentic-delegator/saas/tenancy"
)

// Compile-time assertion: Edition implements runtime.Edition.
var _ runtime.Edition = (*Edition)(nil)

type Edition struct {
	resolver       *tenancy.Resolver
	repoCreds      ports.RepoCredentialsProvider
	anthropicCreds ports.AnthropicCredentialsProvider
	oauth          *signup.OAuth
	install        *ghapp.InstallHandler
	webhook        *ghapp.WebhookHandler
}

func New(
	resolver *tenancy.Resolver,
	repoCreds ports.RepoCredentialsProvider,
	anthropicCreds ports.AnthropicCredentialsProvider,
	oauth *signup.OAuth,
	install *ghapp.InstallHandler,
	webhook *ghapp.WebhookHandler,
) *Edition {
	return &Edition{
		resolver: resolver, repoCreds: repoCreds, anthropicCreds: anthropicCreds,
		oauth: oauth, install: install, webhook: webhook,
	}
}

func (e *Edition) Name() string { return "saas" }

func (e *Edition) RegisterRoutes(r chi.Router) {
	r.Get("/login", e.oauth.Login)
	r.Get("/auth/github/callback", e.oauth.Callback)
	r.Get("/auth/github-app/install", e.install.Install)
	r.Get("/auth/github-app/callback", e.install.Callback)
	r.Post("/webhooks/github", e.webhook.Handle)
}

func (e *Edition) ResolveUser(r *http.Request) (domain.UserID, error) {
	return e.resolver.Resolve(r)
}

func (e *Edition) RepoCredentialsProvider() ports.RepoCredentialsProvider {
	return e.repoCreds
}

func (e *Edition) AnthropicCredentialsProvider() ports.AnthropicCredentialsProvider {
	return e.anthropicCreds
}

func (e *Edition) Bootstrap(ctx context.Context) error { return nil }
