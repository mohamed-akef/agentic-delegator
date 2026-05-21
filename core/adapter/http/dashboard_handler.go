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
	list    *usecase.ListJobs
	keys    ports.APIKeysRepository
	secrets ports.SecretsRepository
}

func NewDashboardHandler(list *usecase.ListJobs, keys ports.APIKeysRepository, secrets ports.SecretsRepository) *DashboardHandler {
	return &DashboardHandler{list: list, keys: keys, secrets: secrets}
}

func (h *DashboardHandler) Landing(w http.ResponseWriter, r *http.Request) {
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
