// core/adapter/http/status_page.go
package http

import (
	"fmt"
	"net/http"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase"
)

// StatusPage is a placeholder presenter for GET /jobs/{id}.
// Plan 03 replaces this with templ-rendered HTML + HTMX log polling.
type StatusPage struct {
	get *usecase.GetJob
}

func NewStatusPage(get *usecase.GetJob) *StatusPage { return &StatusPage{get: get} }

// Render is the placeholder handler for GET /jobs/{id}.
// Plan 03 replaces this with templ-rendered HTML + HTMX log polling.
func (p *StatusPage) Render(w http.ResponseWriter, r *http.Request, id string) {
	uid, _ := UserFromContext(r.Context())
	j, err := p.get.Execute(r.Context(), usecase.GetJobInput{JobID: domain.JobID(id), UserID: uid})
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "Job %s\nStatus: %s\nRepo: %s\nBranch: %s\nPR: %s\nError: %s\n",
		j.ID, j.Status, j.Repo, j.WorkBranch, j.PRURL, j.Error)
}
