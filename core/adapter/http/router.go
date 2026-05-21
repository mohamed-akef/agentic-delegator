// core/adapter/http/router.go
package http

import (
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"agentic-delegator/core/adapter/http/gen"
)

// Deps bundles everything a Router needs to wire handlers. The composition
// root constructs this; each handler reads only the fields it needs.
type Deps struct {
	Resolver        UserResolver
	JobsHandler     *JobsHandler
	SettingsHandler *SettingsHandler
}

// NewRouter builds and returns the chi router with all API routes mounted.
func NewRouter(deps Deps) chi.Router {
	r := chi.NewRouter()
	r.Use(chimw.RequestID, chimw.Recoverer, chimw.RealIP)

	// authenticated /api and /settings routes
	r.Group(func(api chi.Router) {
		api.Use(BearerOrSession(deps.Resolver))
		gen.HandlerFromMux(handlerImpl{
			jobs:     deps.JobsHandler,
			settings: deps.SettingsHandler,
		}, api)
	})

	return r
}
