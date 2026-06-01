// core/adapter/http/router.go
package http

import (
	nethttp "net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"agentic-delegator/core/adapter/http/gen"
	"agentic-delegator/core/presenter/static"
)

// EditionRouteMounter is the slice of the runtime.Edition interface the
// router needs. Defined here so router doesn't import core/runtime.
type EditionRouteMounter interface {
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
	Edition         EditionRouteMounter // calls Edition.RegisterRoutes
}

// NewRouter builds and returns the chi router with all API routes mounted.
func NewRouter(deps Deps) chi.Router {
	r := chi.NewRouter()
	r.Use(chimw.RequestID, chimw.Recoverer, chimw.RealIP)

	// public (no auth)
	r.Get("/", deps.Dashboard.Landing)
	r.Handle("/static/*", nethttp.StripPrefix("/static/", static.Handler()))
	// edition-specific routes (selfhost: /admin/setup; saas: /login, etc.)
	if deps.Edition != nil {
		deps.Edition.RegisterRoutes(r)
	}

	// authenticated routes
	r.Group(func(api chi.Router) {
		api.Use(BearerOrSession(deps.Resolver))
		gen.HandlerFromMux(handlerImpl{
			jobs:     deps.JobsHandler,
			settings: deps.SettingsHandler,
		}, api)
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
