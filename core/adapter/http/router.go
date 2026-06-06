// core/adapter/http/router.go
package http

import (
	"context"
	nethttp "net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"agentic-delegator/core/adapter/http/gen"
	"agentic-delegator/core/presenter/static"
)

// RouteMounter lets the composition root mount auth/webhook routes without the
// http adapter importing the auth/ghapp adapters (preserves the SRP boundary).
type RouteMounter interface {
	RegisterRoutes(r chi.Router)
}

// Deps bundles everything a Router needs to wire handlers. The composition
// root constructs this; each handler reads only the fields it needs.
type Deps struct {
	Resolver        UserResolver
	JobsHandler     *JobsHandler
	SettingsHandler *SettingsHandler
	StatusPage      *StatusPage
	Dashboard       *DashboardHandler
	Routes          RouteMounter // mounts /login, /auth/*, /webhooks/github
	// HealthCheck backs /healthz; nil means liveness-only (always 200).
	HealthCheck func(context.Context) error
}

// NewRouter builds and returns the chi router with all API routes mounted.
func NewRouter(deps Deps) chi.Router {
	r := chi.NewRouter()
	r.Use(chimw.RequestID, chimw.RealIP, RequestLogger, chimw.Recoverer)

	// liveness/readiness — no auth
	r.Get("/healthz", func(w nethttp.ResponseWriter, req *nethttp.Request) {
		if deps.HealthCheck != nil {
			ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
			defer cancel()
			if err := deps.HealthCheck(ctx); err != nil {
				nethttp.Error(w, "unhealthy", nethttp.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(nethttp.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// public (no auth)
	r.Get("/", deps.Dashboard.Landing)
	r.Handle("/static/*", nethttp.StripPrefix("/static/", static.Handler()))
	// auth + GitHub-App routes (/login, /auth/github/callback,
	// /auth/github-app/*, /webhooks/github)
	if deps.Routes != nil {
		deps.Routes.RegisterRoutes(r)
	}

	// authenticated routes
	r.Group(func(api chi.Router) {
		api.Use(BearerOrSession(deps.Resolver))
		gen.HandlerFromMux(handlerImpl{
			jobs:     deps.JobsHandler,
			settings: deps.SettingsHandler,
		}, api)
		// Routes not in the OpenAPI-generated mux, mounted directly.
		api.Post("/api/jobs/{id}/cancel", func(w nethttp.ResponseWriter, r *nethttp.Request) {
			deps.JobsHandler.Cancel(w, r, chi.URLParam(r, "id"))
		})
		api.Delete("/settings/api-keys/{id}", func(w nethttp.ResponseWriter, r *nethttp.Request) {
			deps.SettingsHandler.RevokeAPIKey(w, r, chi.URLParam(r, "id"))
		})
		api.Get("/dashboard", deps.Dashboard.Dashboard)
		api.Get("/settings", deps.Dashboard.Settings)
		api.Get("/jobs/{id}", func(w nethttp.ResponseWriter, r *nethttp.Request) {
			deps.StatusPage.Render(w, r, chi.URLParam(r, "id"))
		})
		api.Get("/jobs/{id}/log", func(w nethttp.ResponseWriter, r *nethttp.Request) {
			deps.StatusPage.LogTail(w, r, chi.URLParam(r, "id"))
		})
	})

	return r
}
