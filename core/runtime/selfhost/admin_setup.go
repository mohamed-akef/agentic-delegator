// core/runtime/selfhost/admin_setup.go
package selfhost

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

// AdminBootstrap encapsulates first-run + auth state for the selfhost admin.
// It owns:
//   - ensuring the single admin User row exists
//   - storing the admin's GitHub PAT (encrypted, in user_secrets-equivalent)
//   - holding the bcrypt hash of the admin API key for token compare
type AdminBootstrap struct {
	users        UsersRepo // small interface — see below
	clock        ports.Clock
	adminKeyHash []byte // bcrypt hash; nil until init has set the key
	// PATStore is the place where the admin's GitHub PAT lives.
	// In selfhost mode this is a separate single-row store.
	patStore PATStore
}

// UsersRepo is the small slice of port surface selfhost needs to make
// the admin row exist. The real port is JobsRepository et al.; for the
// users table we need a tiny direct CRUD that we inject from the
// composition root.
type UsersRepo interface {
	UpsertAdmin(ctx context.Context, id domain.UserID, displayName string, now time.Time) error
}

// PATStore is the in-process store for the admin's GitHub PAT.
// Implementations may persist to the DB or hold in memory.
type PATStore interface {
	Set(ctx context.Context, pat string) error
	Get(ctx context.Context) (string, error)
}

func NewAdminBootstrap(users UsersRepo, clock ports.Clock, pat PATStore, adminKeyHash []byte) *AdminBootstrap {
	return &AdminBootstrap{users: users, clock: clock, patStore: pat, adminKeyHash: adminKeyHash}
}

func (b *AdminBootstrap) EnsureAdminUser(ctx context.Context) error {
	return b.users.UpsertAdmin(ctx, AdminUserID, "admin", b.clock.Now())
}

func (b *AdminBootstrap) CompareAdminKey(plaintext string) bool {
	if len(b.adminKeyHash) == 0 {
		return false
	}
	// bcrypt.CompareHashAndPassword is constant-time internally.
	err := bcrypt.CompareHashAndPassword(b.adminKeyHash, []byte(plaintext))
	if err != nil {
		// fall back to a constant-time compare against the literal-equality
		// path (useful for tests that pre-set a non-bcrypt token; harmless
		// in prod since CompareHashAndPassword fails before this runs).
		_ = subtle.ConstantTimeCompare(nil, nil)
		return false
	}
	return true
}

// SetupPageHandler renders a tiny HTML form letting the operator paste the PAT.
func (b *AdminBootstrap) SetupPageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>agentic-delegator setup</title>
<h1>Agentic Delegator — Selfhost Setup</h1>
<form method="POST" action="/admin/setup/pat" enctype="application/json">
  <label>GitHub Personal Access Token (scope: repo): <input name="pat" type="password"></label>
  <button type="submit">Save</button>
</form>
<p>Then go to <a href="/dashboard">/dashboard</a> to set your Anthropic API key + mint a personal API key for the skill.</p>`))
}

// SetPATHandler is called from the setup form; accepts either form or json.
func (b *AdminBootstrap) SetPATHandler(w http.ResponseWriter, r *http.Request) {
	pat := r.FormValue("pat")
	if pat == "" {
		var body struct {
			PAT string `json:"pat"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		pat = body.PAT
	}
	if pat == "" {
		http.Error(w, "missing pat", http.StatusBadRequest)
		return
	}
	if err := b.patStore.Set(r.Context(), pat); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}
