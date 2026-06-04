// core/adapter/http/dashboard_handler.go
package http

import (
	"errors"
	"net/http"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/presenter/templ/pages"
	"agentic-delegator/core/usecase"
	"agentic-delegator/core/usecase/ports"
)

type DashboardHandler struct {
	list     *usecase.ListJobs
	keys     ports.APIKeysRepository
	secrets  ports.SecretsRepository
	resolver UserResolver
}

func NewDashboardHandler(list *usecase.ListJobs, keys ports.APIKeysRepository, secrets ports.SecretsRepository, resolver UserResolver) *DashboardHandler {
	return &DashboardHandler{list: list, keys: keys, secrets: secrets, resolver: resolver}
}

func (h *DashboardHandler) Landing(w http.ResponseWriter, r *http.Request) {
	// If the visitor already has a valid session/bearer, skip the marketing
	// page and send them to the dashboard.
	if h.resolver != nil {
		if _, err := h.resolver.Resolve(r); err == nil {
			http.Redirect(w, r, "/dashboard", http.StatusFound)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.Landing().Render(r.Context(), w)
}

func (h *DashboardHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	uid, _ := UserFromContext(r.Context())
	js, err := h.list.Execute(r.Context(), usecase.ListJobsInput{UserID: uid})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.Dashboard(js).Render(r.Context(), w)
}

func (h *DashboardHandler) Settings(w http.ResponseWriter, r *http.Request) {
	uid, _ := UserFromContext(r.Context())
	keys, err := h.keys.ListForUser(r.Context(), uid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, errAn := h.secrets.GetAnthropicCreds(r.Context(), uid)
	hasAnthropic := errAn == nil
	if errAn != nil && !errors.Is(errAn, domain.ErrNotFound) {
		http.Error(w, errAn.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.Settings(keys, hasAnthropic).Render(r.Context(), w)
}
