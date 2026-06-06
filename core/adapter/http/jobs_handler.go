// core/adapter/http/jobs_handler.go
package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"agentic-delegator/core/adapter/http/gen"
	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase"
)

// JobsHandler handles the /api/jobs routes.
type JobsHandler struct {
	enqueue *usecase.EnqueueJob
	get     *usecase.GetJob
	list    *usecase.ListJobs
	cancel  *usecase.CancelJob
}

func NewJobsHandler(enqueue *usecase.EnqueueJob, get *usecase.GetJob, list *usecase.ListJobs, cancel *usecase.CancelJob) *JobsHandler {
	return &JobsHandler{enqueue: enqueue, get: get, list: list, cancel: cancel}
}

// Cancel handles POST /api/jobs/{id}/cancel.
func (h *JobsHandler) Cancel(w http.ResponseWriter, r *http.Request, id string) {
	uid, _ := UserFromContext(r.Context())
	if err := h.cancel.Execute(r.Context(), usecase.CancelJobInput{JobID: domain.JobID(id), UserID: uid}); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// EnqueueJob handles POST /api/jobs.
func (h *JobsHandler) EnqueueJob(w http.ResponseWriter, r *http.Request) {
	uid, _ := UserFromContext(r.Context())

	var body struct {
		Repo          string `json:"repo"`
		BaseBranch    string `json:"base_branch"`
		WorkBranch    string `json:"work_branch"`
		SpecSource    string `json:"spec_source"`
		SourceType    string `json:"source_type"`
		ModelOverride string `json:"model_override"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	out, err := h.enqueue.Execute(r.Context(), usecase.EnqueueJobInput{
		UserID:        uid,
		Repo:          body.Repo,
		BaseBranch:    body.BaseBranch,
		WorkBranch:    body.WorkBranch,
		Spec:          domain.SpecSource{Type: domain.SourceType(body.SourceType), Value: body.SpecSource},
		ModelOverride: body.ModelOverride,
		// LogPath is derived server-side (private dir + job ID) by the use case.
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"job_id":     string(out.JobID),
		"status_url": "/jobs/" + string(out.JobID),
	})
}

// GetJob handles GET /api/jobs/{id}.
func (h *JobsHandler) GetJob(w http.ResponseWriter, r *http.Request, id string) {
	uid, _ := UserFromContext(r.Context())
	j, err := h.get.Execute(r.Context(), usecase.GetJobInput{JobID: domain.JobID(id), UserID: uid})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, j)
}

// ListJobs handles GET /api/jobs.
func (h *JobsHandler) ListJobs(w http.ResponseWriter, r *http.Request, params gen.ListJobsParams) {
	uid, _ := UserFromContext(r.Context())
	limit := 50
	if params.Limit != nil && *params.Limit > 0 {
		limit = *params.Limit
	}
	js, err := h.list.Execute(r.Context(), usecase.ListJobsInput{UserID: uid, Limit: limit})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, js)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// writeDomainError maps sentinel domain errors to HTTP status codes so API
// clients can distinguish bad input (400) from missing (404), forbidden (403),
// and conflicts (409).
func writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeJSONError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrForbidden):
		writeJSONError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrInvalidState):
		writeJSONError(w, http.StatusConflict, err.Error())
	default:
		writeJSONError(w, http.StatusBadRequest, err.Error())
	}
}

// handlerImpl bridges the generated ServerInterface to our handler structs.
// It is used by router.go's HandlerFromMux call.
type handlerImpl struct {
	jobs     *JobsHandler
	settings *SettingsHandler
}

func (h handlerImpl) ListJobs(w http.ResponseWriter, r *http.Request, params gen.ListJobsParams) {
	h.jobs.ListJobs(w, r, params)
}
func (h handlerImpl) EnqueueJob(w http.ResponseWriter, r *http.Request) {
	h.jobs.EnqueueJob(w, r)
}
func (h handlerImpl) GetJob(w http.ResponseWriter, r *http.Request, id string) {
	h.jobs.GetJob(w, r, id)
}
func (h handlerImpl) SetAnthropicCredentials(w http.ResponseWriter, r *http.Request) {
	h.settings.SetAnthropic(w, r)
}
func (h handlerImpl) MintAPIKey(w http.ResponseWriter, r *http.Request) {
	h.settings.MintAPIKey(w, r)
}

// Compile-time assertion: handlerImpl must satisfy gen.ServerInterface exactly.
var _ gen.ServerInterface = handlerImpl{}
