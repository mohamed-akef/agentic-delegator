// core/adapter/ghapp/install.go
package ghapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"agentic-delegator/core/adapter/postgres"
	"agentic-delegator/core/domain"
)

// InstallationsWriter is the slice of InstallationsRepo we need.
type InstallationsWriter interface {
	Upsert(ctx context.Context, i postgres.Installation) error
	Delete(ctx context.Context, id int64) error
}

// SessionResolver is how we know which user is initiating the install.
type SessionResolver interface {
	UserFromRequest(r *http.Request) (domain.UserID, error)
}

// InstallHandler handles the GitHub App install flow.
type InstallHandler struct {
	appSlug       string
	sessions      SessionResolver
	installations InstallationsWriter
	app           *AppClient
	httpClient    *http.Client
}

// NewInstallHandler returns an InstallHandler.
func NewInstallHandler(appSlug string, sessions SessionResolver, installs InstallationsWriter, app *AppClient) *InstallHandler {
	return &InstallHandler{
		appSlug:       appSlug,
		sessions:      sessions,
		installations: installs,
		app:           app,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Install handles GET /auth/github-app/install — sends the user to the App install page.
func (h *InstallHandler) Install(w http.ResponseWriter, r *http.Request) {
	url := fmt.Sprintf("https://github.com/apps/%s/installations/new", h.appSlug)
	http.Redirect(w, r, url, http.StatusFound)
}

// Callback handles GET /auth/github-app/callback?installation_id=N&setup_action=install.
func (h *InstallHandler) Callback(w http.ResponseWriter, r *http.Request) {
	uid, err := h.sessions.UserFromRequest(r)
	if err != nil {
		http.Error(w, "must be signed in", http.StatusUnauthorized)
		return
	}
	idStr := r.URL.Query().Get("installation_id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "missing installation_id", http.StatusBadRequest)
		return
	}

	// Fetch the installation details to populate repos + account.
	tok, _, err := h.app.InstallationToken(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	acct, repos, err := h.fetchAccountAndRepos(r.Context(), tok)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := h.installations.Upsert(r.Context(), postgres.Installation{
		InstallationID: id,
		UserID:         uid,
		AccountLogin:   acct,
		Repos:          repos,
		CreatedAt:      time.Now().UTC(),
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func (h *InstallHandler) fetchAccountAndRepos(ctx context.Context, token string) (string, []string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/installation/repositories", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", nil, fmt.Errorf("github api: %d", resp.StatusCode)
	}
	var body struct {
		TotalCount   int `json:"total_count"`
		Repositories []struct {
			FullName string `json:"full_name"`
			Owner    struct {
				Login string `json:"login"`
			} `json:"owner"`
		} `json:"repositories"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", nil, err
	}
	acct := ""
	repos := make([]string, 0, len(body.Repositories))
	for i, r := range body.Repositories {
		if i == 0 {
			acct = r.Owner.Login
		}
		repos = append(repos, r.FullName)
	}
	return acct, repos, nil
}
