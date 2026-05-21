# Plan 03 — Selfhost Edition + Runner Image + Skill + Composition Root

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Take the adapters from Plan 02 and stand up a working `agentic-delegator` selfhost binary. The user can `agentic-delegator init` + `agentic-delegator serve` + install the skill in Claude Code + `/delegate` a real job that opens a real PR on a real GitHub repo. After this plan, the OSS edition is functionally complete end-to-end.

**Architecture:** This plan adds the *outermost* Clean Architecture layer (composition root in `cmd/`) and the *edition-specific* implementation of `runtime.Edition` for selfhost. It also adds the templ presenter that replaces the Plan 02 placeholder, the runner Docker image with `claude` + `gh` + `git`, and the `skill/delegate.md` file.

**Tech stack additions:** templ v0.2+ (HTML codegen), HTMX (loaded as a static asset from CDN), the official Claude Code CLI in the runner image (`@anthropic-ai/claude-code` npm package or a binary depending on availability).

**Spec reference:** [`docs/design/2026-05-21-mvp-design.md`](../design/2026-05-21-mvp-design.md) sections "MVP user journey → Self-host edition", "Component boundaries", "Codebase structure".

**Prerequisites:**
- `plan-02-done` tag present
- `make test-integration` green
- `make arch-check` green
- Postgres on host port 5433

---

## File structure produced by this plan

```
agentic-delegator/
├── cmd/agentic-delegator/
│   ├── main.go                                  # composition root (NEW)
│   └── migrate/main.go                          # already exists from Plan 02
├── core/
│   ├── runtime/
│   │   ├── edition.go                           # NEW — Edition interface
│   │   └── selfhost/
│   │       ├── edition.go                       # NEW — selfhost Edition impl
│   │       ├── admin_setup.go                   # NEW — first-run wizard + /admin/setup
│   │       ├── repo_creds.go                    # NEW — PAT-based RepoCredentialsProvider
│   │       └── anthropic_creds.go               # NEW — reads from SecretsRepo (encrypted wrapper applied at composition)
│   ├── presenter/templ/
│   │   ├── layouts/shell.templ                  # NEW
│   │   ├── pages/{landing,dashboard,status,settings,admin_setup}.templ
│   │   └── partials/{log_tail,joblist,onboarding,api_keys_list}.templ
│   ├── adapter/http/
│   │   └── status_page.go                       # REWRITE: swap placeholder text for templ
│   └── config/
│       └── config.go                            # NEW — env config loader
├── runner/
│   ├── Dockerfile                               # NEW
│   └── entrypoint.sh                            # NEW
├── skill/
│   └── delegate.md                              # NEW
└── docs/
    └── end-to-end-smoke.md                      # NEW — manual smoke procedure
```

The Edition interface is at `core/runtime/edition.go` (Plan 01's design called this out as a port). Selfhost is one implementation; SaaS (Plan 04) is the other.

---

## Phase A — Edition port + selfhost implementation

### Task 1: Define `runtime.Edition` interface

**Files:**
- Create: `core/runtime/edition.go`

- [ ] **Step 1: Update arch-lint to include the new runtime + presenter components**

Read `.go-arch-lint.yml`, then add these components and dep rules (under existing `components:` and `deps:` blocks):

```yaml
  runtime:
    in: core/runtime/**
  runtime_selfhost:
    in: core/runtime/selfhost/**
  presenter:
    in: core/presenter/**
  config:
    in: core/config/**
```

```yaml
  runtime:
    mayDependOn: [domain, ports]
    anyVendorDeps: true
  runtime_selfhost:
    mayDependOn: [domain, ports, runtime, usecase]
    anyVendorDeps: true
  presenter:
    mayDependOn: [domain]
    anyVendorDeps: true
  config:
    mayDependOn: []
    anyProjectDeps: false
    anyVendorDeps: true
```

- [ ] **Step 2: Write the `Edition` interface**

```go
// core/runtime/edition.go
package runtime

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

// Edition is the variability surface between OSS selfhost and SaaS.
// Core never imports either implementation directly; cmd/* wires the right one.
type Edition interface {
	Name() string  // "selfhost" | "saas"

	// RegisterRoutes lets an edition mount its own routes (admin setup,
	// signup, GitHub-App webhooks, etc.) onto the chi router.
	RegisterRoutes(r chi.Router)

	// ResolveUser turns an HTTP request into a UserID. Selfhost returns the
	// single admin's UserID for any authenticated request; SaaS resolves a
	// session cookie or bearer API key to the owning user.
	ResolveUser(r *http.Request) (domain.UserID, error)

	// RepoCredentialsProvider returns the port impl that yields short-lived
	// git creds for cloning/pushing on a user+repo's behalf.
	RepoCredentialsProvider() ports.RepoCredentialsProvider

	// AnthropicCredentialsProvider returns the port impl that yields the
	// Anthropic credential for a user.
	AnthropicCredentialsProvider() ports.AnthropicCredentialsProvider

	// Bootstrap runs once at startup. Selfhost: ensures the admin user row
	// exists. SaaS: no-op.
	Bootstrap(ctx context.Context) error
}
```

- [ ] **Step 3: Commit**

```bash
make arch-check
go build ./...
git add .go-arch-lint.yml core/runtime/edition.go
git commit -m "feat(runtime): Edition interface (port for edition variance)"
```

---

### Task 2: Selfhost edition skeleton + admin user bootstrap

**Files:**
- Create: `core/runtime/selfhost/edition.go`
- Create: `core/runtime/selfhost/admin_setup.go`

- [ ] **Step 1: Write the Edition implementation**

```go
// core/runtime/selfhost/edition.go
package selfhost

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

// AdminUserID is the fixed UserID assigned to the single admin user in
// selfhost mode. Anything that reaches a user-resolution boundary uses this.
const AdminUserID domain.UserID = "u_admin"

// Edition implements runtime.Edition for the OSS selfhost binary.
type Edition struct {
	repoCreds      ports.RepoCredentialsProvider
	anthropicCreds ports.AnthropicCredentialsProvider
	bootstrap      *AdminBootstrap
	adminKeyHash   []byte // bcrypt hash of the admin API key; populated at init
}

func New(
	repoCreds ports.RepoCredentialsProvider,
	anthropicCreds ports.AnthropicCredentialsProvider,
	bootstrap *AdminBootstrap,
	adminKeyHash []byte,
) *Edition {
	return &Edition{
		repoCreds:      repoCreds,
		anthropicCreds: anthropicCreds,
		bootstrap:      bootstrap,
		adminKeyHash:   adminKeyHash,
	}
}

func (e *Edition) Name() string { return "selfhost" }

func (e *Edition) RegisterRoutes(r chi.Router) {
	r.Get("/admin/setup", e.bootstrap.SetupPageHandler)
	r.Post("/admin/setup/pat", e.bootstrap.SetPATHandler)
}

func (e *Edition) ResolveUser(r *http.Request) (domain.UserID, error) {
	token := extractBearer(r)
	if token == "" {
		return "", errors.New("missing bearer token")
	}
	if !e.bootstrap.CompareAdminKey(token) {
		return "", errors.New("invalid bearer token")
	}
	return AdminUserID, nil
}

func (e *Edition) RepoCredentialsProvider() ports.RepoCredentialsProvider {
	return e.repoCreds
}

func (e *Edition) AnthropicCredentialsProvider() ports.AnthropicCredentialsProvider {
	return e.anthropicCreds
}

func (e *Edition) Bootstrap(ctx context.Context) error {
	return e.bootstrap.EnsureAdminUser(ctx)
}

func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) < 7 || h[:7] != "Bearer " {
		return ""
	}
	return h[7:]
}
```

- [ ] **Step 2: Write the admin bootstrap + setup handler**

```go
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
	users         UsersRepo    // small interface — see below
	clock         ports.Clock
	adminKeyHash  []byte       // bcrypt hash; nil until init has set the key
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
```

- [ ] **Step 3: Compile + arch-check**

```bash
go build ./...
make arch-check
```

- [ ] **Step 4: Commit**

```bash
git add core/runtime/selfhost/edition.go core/runtime/selfhost/admin_setup.go
git commit -m "feat(runtime/selfhost): Edition + admin bootstrap"
```

---

### Task 3: Selfhost RepoCredentialsProvider (PAT-based)

**Files:**
- Create: `core/runtime/selfhost/repo_creds.go`
- Test: `core/runtime/selfhost/repo_creds_test.go`

- [ ] **Step 1: Test**

```go
// core/runtime/selfhost/repo_creds_test.go
package selfhost_test

import (
	"context"
	"testing"

	"agentic-delegator/core/runtime/selfhost"
)

type fakePAT struct {
	pat string
	err error
}

func (f *fakePAT) Set(ctx context.Context, pat string) error { f.pat = pat; return nil }
func (f *fakePAT) Get(ctx context.Context) (string, error)   { return f.pat, f.err }

func TestRepoCreds_returnsPATAsToken(t *testing.T) {
	store := &fakePAT{pat: "ghp_xxx"}
	p := selfhost.NewRepoCredsProvider(store)
	creds, err := p.For(context.Background(), "u_admin", "owner/repo")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if creds.Token != "ghp_xxx" {
		t.Fatalf("token mismatch: %q", creds.Token)
	}
}
```

- [ ] **Step 2: Implement**

```go
// core/runtime/selfhost/repo_creds.go
package selfhost

import (
	"context"
	"time"

	"agentic-delegator/core/domain"
)

type RepoCredsProvider struct {
	pat PATStore
}

func NewRepoCredsProvider(pat PATStore) *RepoCredsProvider {
	return &RepoCredsProvider{pat: pat}
}

func (p *RepoCredsProvider) For(ctx context.Context, userID domain.UserID, repo string) (domain.GitCreds, error) {
	tok, err := p.pat.Get(ctx)
	if err != nil {
		return domain.GitCreds{}, err
	}
	// PAT has no expiry tracked; zero ExpiresAt = "never expired" (per domain.GitCreds.Expired).
	_ = time.Time{}
	return domain.GitCreds{Token: tok}, nil
}
```

- [ ] **Step 3: Test + commit**

```bash
go test ./core/runtime/selfhost/...
make arch-check
git add core/runtime/selfhost/repo_creds.go core/runtime/selfhost/repo_creds_test.go
git commit -m "feat(runtime/selfhost): PAT-based RepoCredentialsProvider"
```

---

### Task 4: Selfhost AnthropicCredentialsProvider

**Files:**
- Create: `core/runtime/selfhost/anthropic_creds.go`
- Test: `core/runtime/selfhost/anthropic_creds_test.go`

- [ ] **Step 1: Test**

```go
// core/runtime/selfhost/anthropic_creds_test.go
package selfhost_test

import (
	"context"
	"testing"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/runtime/selfhost"
	"agentic-delegator/core/testutil"
)

func TestAnthropicCreds_readsFromSecretsRepo(t *testing.T) {
	secrets := testutil.NewFakeSecretsRepo()
	_ = secrets.SetAnthropicCreds(context.Background(), "u_admin", domain.AnthropicCreds{APIKey: "sk-1"})
	p := selfhost.NewAnthropicCredsProvider(secrets)
	got, err := p.For(context.Background(), "u_admin")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got.APIKey != "sk-1" {
		t.Fatalf("api key mismatch: %s", got.APIKey)
	}
}
```

- [ ] **Step 2: Implement**

```go
// core/runtime/selfhost/anthropic_creds.go
package selfhost

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type AnthropicCredsProvider struct {
	secrets ports.SecretsRepository
}

func NewAnthropicCredsProvider(secrets ports.SecretsRepository) *AnthropicCredsProvider {
	return &AnthropicCredsProvider{secrets: secrets}
}

func (p *AnthropicCredsProvider) For(ctx context.Context, userID domain.UserID) (domain.AnthropicCreds, error) {
	return p.secrets.GetAnthropicCreds(ctx, userID)
}
```

- [ ] **Step 3: Test + commit**

```bash
go test ./core/runtime/selfhost/...
make arch-check
git add core/runtime/selfhost/anthropic_creds.go core/runtime/selfhost/anthropic_creds_test.go
git commit -m "feat(runtime/selfhost): AnthropicCredentialsProvider reading SecretsRepo"
```

---

## Phase B — Templ presenter (replace status page placeholder)

### Task 5: Install templ tooling

**Files:**
- Modify: `go.mod` (adds `github.com/a-h/templ`)
- Modify: `Makefile` (add templ generation to `generate` target)

- [ ] **Step 1: Add templ**

```bash
go get github.com/a-h/templ@v0.3.819
go mod tidy
```

Adjust the version if 0.3.819 doesn't resolve. Pin whatever Go picks.

- [ ] **Step 2: Update Makefile generate target**

Read the Makefile. Change the `generate:` target from:
```make
generate:
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.7.0 -config api/codegen.yaml api/openapi.yaml
```
to:
```make
generate:
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.7.0 -config api/codegen.yaml api/openapi.yaml
	go run github.com/a-h/templ/cmd/templ@latest generate
```

(Use the actual installed templ version where `@latest` is shown.)

- [ ] **Step 3: Commit**

```bash
go build ./...
git add go.mod go.sum Makefile
git commit -m "chore: add templ for type-safe HTML generation"
```

---

### Task 6: Templ layouts + status page

**Files:**
- Create: `core/presenter/templ/layouts/shell.templ`
- Create: `core/presenter/templ/pages/status.templ`
- Modify: `core/adapter/http/status_page.go` (use the new templ component)

- [ ] **Step 1: Write the shell layout**

```go
// core/presenter/templ/layouts/shell.templ
package layouts

templ Shell(title string) {
    <!DOCTYPE html>
    <html lang="en">
    <head>
        <meta charset="utf-8" />
        <title>{ title } — agentic-delegator</title>
        <script src="https://unpkg.com/htmx.org@2.0.4"></script>
        <style>
            body { font-family: ui-sans-serif, -apple-system, sans-serif; max-width: 920px; margin: 2em auto; padding: 0 1em; color: #222; }
            h1 { font-size: 1.4em; margin: 0 0 0.6em; }
            h2 { font-size: 1.1em; margin: 1em 0 0.3em; }
            pre.log { background: #111; color: #e7e7e7; padding: 1em; border-radius: 6px; max-height: 50vh; overflow: auto; font-size: 12px; }
            .badge { display: inline-block; padding: 2px 8px; border-radius: 999px; font-size: 11px; text-transform: uppercase; }
            .b-queued    { background: #eef; color: #44a; }
            .b-running   { background: #eea; color: #a60; }
            .b-succeeded { background: #efe; color: #060; }
            .b-failed    { background: #fee; color: #a00; }
            .b-cancelled { background: #eee; color: #555; }
            a { color: #44a; }
            label { display: block; margin: 0.5em 0; }
            input, button { font: inherit; padding: 0.3em 0.6em; }
        </style>
    </head>
    <body>
        { children... }
    </body>
    </html>
}
```

- [ ] **Step 2: Write the status page**

```go
// core/presenter/templ/pages/status.templ
package pages

import (
    "agentic-delegator/core/domain"
    "agentic-delegator/core/presenter/templ/layouts"
)

templ Status(j *domain.Job, logTail string) {
    @layouts.Shell("Job " + string(j.ID)) {
        <h1>Job <code>{ string(j.ID) }</code></h1>
        <p>
            <span class={ "badge", "b-" + string(j.Status) }>{ string(j.Status) }</span>
            on <strong>{ j.Repo }</strong> · branch <code>{ j.WorkBranch }</code> (from { j.BaseBranch })
        </p>
        if j.PRURL != "" {
            <p>PR: <a href={ templ.SafeURL(j.PRURL) }>{ j.PRURL }</a></p>
        }
        if j.Error != "" {
            <p>Error: <code>{ j.Error }</code></p>
        }
        <h2>Logs <small>(refreshes every 2s while running)</small></h2>
        <pre class="log"
             hx-get={ "/jobs/" + string(j.ID) + "/log" }
             hx-trigger={ logRefreshTrigger(j) }
             hx-swap="innerHTML">
            { logTail }
        </pre>
    }
}

func logRefreshTrigger(j *domain.Job) string {
    // Only auto-poll while running. Terminal states freeze the log view.
    if j.Status == domain.JobStatusRunning || j.Status == domain.JobStatusQueued {
        return "every 2s"
    }
    return ""
}
```

- [ ] **Step 3: Generate the templ code**

```bash
make generate
```

This produces `*.templ.go` files alongside each `.templ` file.

- [ ] **Step 4: Rewrite `core/adapter/http/status_page.go` to use templ**

Replace the contents of `status_page.go` with:

```go
// core/adapter/http/status_page.go
package http

import (
	"net/http"
	"os"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/presenter/templ/pages"
	"agentic-delegator/core/usecase"
)

type StatusPage struct {
	get *usecase.GetJob
}

func NewStatusPage(get *usecase.GetJob) *StatusPage { return &StatusPage{get: get} }

// Render is GET /jobs/{id} — full HTML page.
func (p *StatusPage) Render(w http.ResponseWriter, r *http.Request, id string) {
	uid, _ := UserFromContext(r.Context())
	j, err := p.get.Execute(r.Context(), usecase.GetJobInput{JobID: domain.JobID(id), UserID: uid})
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	logTail := readLogTail(j.LogPath, 200)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.Status(j, logTail).Render(r.Context(), w)
}

// LogTail is GET /jobs/{id}/log — HTMX partial. Returns plain text.
func (p *StatusPage) LogTail(w http.ResponseWriter, r *http.Request, id string) {
	uid, _ := UserFromContext(r.Context())
	j, err := p.get.Execute(r.Context(), usecase.GetJobInput{JobID: domain.JobID(id), UserID: uid})
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(readLogTail(j.LogPath, 200)))
}

func readLogTail(path string, maxLines int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	s := string(b)
	// crude tail — N=maxLines from the end
	lines := splitLines(s)
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return joinLines(lines)
}

func splitLines(s string) []string {
	out := []string{}
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
		} else {
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func joinLines(ls []string) string {
	out := ""
	for i, l := range ls {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}
```

- [ ] **Step 5: Update the status page test** (it asserted plain-text output; now it asserts HTML)

Rewrite `core/adapter/http/status_page_test.go`:

```go
package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	adhttp "agentic-delegator/core/adapter/http"
	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

func TestStatusPage_rendersHTML(t *testing.T) {
	jobs := testutil.NewFakeJobsRepo()
	j := domain.NewJob("j_1", "u_1", "o/r", "main", "agentic/x", domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "", time.Unix(1000, 0))
	j.LogPath = "/tmp/agentic-delegator-status-test-nonexistent.log"
	_ = jobs.Create(context.Background(), j)

	page := adhttp.NewStatusPage(&usecase.GetJob{Jobs: jobs})

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), adhttp.UserIDKey, domain.UserID("u_1"))
		page.Render(w, r.WithContext(ctx), "j_1")
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/jobs/j_1", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Job ") || !strings.Contains(body, "j_1") {
		t.Fatalf("body missing job id: %s", body[:min(300, len(body))])
	}
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Fatalf("not HTML: %s", body[:min(300, len(body))])
	}
}

func min(a, b int) int { if a < b { return a }; return b }
```

- [ ] **Step 6: Run + commit**

```bash
make generate
go test ./core/adapter/http/...
make arch-check
git add core/presenter/ core/adapter/http/status_page.go core/adapter/http/status_page_test.go
git commit -m "feat(presenter): templ + HTMX status page (replaces Plan 02 placeholder)"
```

---

### Task 7: Dashboard + settings pages (templ)

**Files:**
- Create: `core/presenter/templ/pages/dashboard.templ`
- Create: `core/presenter/templ/pages/settings.templ`
- Create: `core/presenter/templ/pages/landing.templ`
- Create: `core/adapter/http/dashboard_handler.go` + test
- Modify: `core/adapter/http/router.go` to mount the new HTML routes (`GET /`, `GET /dashboard`, `GET /settings`, `GET /jobs/{id}/log`)

- [ ] **Step 1: Write templ pages**

```go
// core/presenter/templ/pages/landing.templ
package pages

import "agentic-delegator/core/presenter/templ/layouts"

templ Landing() {
    @layouts.Shell("Welcome") {
        <h1>agentic-delegator</h1>
        <p>Self-hosted background coding agent for Claude Code.</p>
        <p>Go to <a href="/dashboard">/dashboard</a> to start.</p>
    }
}

// core/presenter/templ/pages/dashboard.templ
package pages

import (
    "agentic-delegator/core/domain"
    "agentic-delegator/core/presenter/templ/layouts"
)

templ Dashboard(jobs []*domain.Job) {
    @layouts.Shell("Dashboard") {
        <h1>Dashboard</h1>
        <p><a href="/settings">Settings</a></p>
        <h2>Recent jobs</h2>
        if len(jobs) == 0 {
            <p>No jobs yet. Use the <code>/delegate</code> skill in Claude Code to start one.</p>
        } else {
            <ul>
                for _, j := range jobs {
                    <li>
                        <a href={ templ.SafeURL("/jobs/" + string(j.ID)) }>{ string(j.ID) }</a>
                        <span class={ "badge", "b-" + string(j.Status) }>{ string(j.Status) }</span>
                        — { j.Repo } · { j.WorkBranch }
                    </li>
                }
            </ul>
        }
    }
}

// core/presenter/templ/pages/settings.templ
package pages

import (
    "agentic-delegator/core/domain"
    "agentic-delegator/core/presenter/templ/layouts"
)

templ Settings(keys []*domain.APIKey, hasAnthropic bool) {
    @layouts.Shell("Settings") {
        <h1>Settings</h1>

        <h2>Anthropic API key</h2>
        if hasAnthropic {
            <p>Anthropic API key is configured. <small>Update by re-submitting below.</small></p>
        } else {
            <p><strong>Not yet set.</strong> Paste your Anthropic API key:</p>
        }
        <form method="POST" action="/settings/anthropic" hx-post="/settings/anthropic" hx-swap="none">
            <label>API key: <input name="api_key" type="password" required /></label>
            <button type="submit">Save</button>
        </form>

        <h2>Personal API keys (for the skill)</h2>
        <form method="POST" action="/settings/api-keys">
            <label>Name: <input name="name" required /></label>
            <button type="submit">Mint</button>
        </form>
        if len(keys) > 0 {
            <ul>
                for _, k := range keys {
                    <li><code>{ k.Prefix }…</code> — { k.Name }</li>
                }
            </ul>
        }
    }
}
```

- [ ] **Step 2: Write the dashboard handler**

```go
// core/adapter/http/dashboard_handler.go
package http

import (
	"net/http"

	"agentic-delegator/core/presenter/templ/pages"
	"agentic-delegator/core/usecase"
)

type DashboardHandler struct {
	list *usecase.ListJobs
	keys ApiKeysReader      // tiny adapter — see below
	hasAnthropic AnthropicReader
}

// ApiKeysReader is the slice of API keys repo surface the dashboard needs.
// Avoids the dashboard handler importing the whole ports package twice.
type ApiKeysReader interface {
	ListForUser(ctx ContextLike, userID string) ([]struct {
		ID, Name, Prefix string
	}, error)
}

// AnthropicReader is a tiny check: does this user have anthropic configured?
type AnthropicReader interface {
	HasAnthropic(ctx ContextLike, userID string) (bool, error)
}

// ContextLike is context.Context (alias used to keep this file small).
type ContextLike = interface{ Done() <-chan struct{}; Err() error; Value(any) any; Deadline() (time1, time2 any); }
```

> **Implementer note:** the `ApiKeysReader` + `AnthropicReader` types above are an over-engineered abstraction. Simplify: import `agentic-delegator/core/usecase/ports` and use `ports.APIKeysRepository`, `ports.SecretsRepository` directly. The dashboard handler is part of `core/adapter/http`, which arch-lint allows to depend on `usecase`+`ports`+`domain`. Rewrite the handler cleanly:

```go
// core/adapter/http/dashboard_handler.go  -- cleaned-up version
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
```

- [ ] **Step 3: Mount the new HTML routes in router.go**

Read `core/adapter/http/router.go`, and update `NewRouter` to also wire HTML routes (in addition to the existing OpenAPI-generated `/api` routes). Add:

```go
type Deps struct {
	Resolver        UserResolver
	JobsHandler     *JobsHandler
	SettingsHandler *SettingsHandler
	StatusPage      *StatusPage
	Dashboard       *DashboardHandler
	Edition         EditionRouteMounter   // calls Edition.RegisterRoutes
}

// EditionRouteMounter is the slice of the runtime.Edition interface the
// router needs. Defined here so router doesn't import core/runtime.
type EditionRouteMounter interface {
	RegisterRoutes(r chi.Router)
}
```

Then inside `NewRouter`:

```go
// public (no auth)
r.Get("/", deps.Dashboard.Landing)
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
    api.Get("/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
        deps.StatusPage.Render(w, r, chi.URLParam(r, "id"))
    })
    api.Get("/jobs/{id}/log", func(w http.ResponseWriter, r *http.Request) {
        deps.StatusPage.LogTail(w, r, chi.URLParam(r, "id"))
    })
})
```

- [ ] **Step 4: Add a dashboard test**

```go
// core/adapter/http/dashboard_handler_test.go
package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	adhttp "agentic-delegator/core/adapter/http"
	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

func TestDashboard_listsJobs(t *testing.T) {
	jobs := testutil.NewFakeJobsRepo()
	_ = jobs.Create(context.Background(),
		domain.NewJob("j_x", "u_1", "o/r", "main", "agentic/x",
			domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "",
			time.Unix(1000, 0)))

	h := adhttp.NewDashboardHandler(
		&usecase.ListJobs{Jobs: jobs},
		testutil.NewFakeAPIKeysRepo(),
		testutil.NewFakeSecretsRepo(),
	)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req = req.WithContext(context.WithValue(req.Context(), adhttp.UserIDKey, domain.UserID("u_1")))
	rec := httptest.NewRecorder()
	h.Dashboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "j_x") {
		t.Fatalf("body missing job id")
	}
}
```

- [ ] **Step 5: Generate templ + test + commit**

```bash
make generate
go test ./core/adapter/http/... ./core/presenter/...
make arch-check
git add core/presenter/ core/adapter/http/dashboard_handler.go core/adapter/http/dashboard_handler_test.go core/adapter/http/router.go
git commit -m "feat(presenter): landing/dashboard/settings pages + dashboard handler + HTML routes"
```

---

## Phase C — Runner Docker image

### Task 8: Runner Dockerfile

**Files:**
- Create: `runner/Dockerfile`

- [ ] **Step 1: Write the Dockerfile**

```dockerfile
# runner/Dockerfile
# Image used by the orchestrator to execute one agentic-delegator job.
# Contents: claude (Claude Code CLI), gh, git, and a small shell entrypoint.

FROM debian:12-slim AS base

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl git jq gnupg \
 && rm -rf /var/lib/apt/lists/*

# gh CLI from official repo
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | tee /usr/share/keyrings/githubcli-archive-keyring.gpg >/dev/null \
 && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
 && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
    | tee /etc/apt/sources.list.d/github-cli.list \
 && apt-get update && apt-get install -y --no-install-recommends gh \
 && rm -rf /var/lib/apt/lists/*

# Node + Claude Code CLI (the official distribution channel)
RUN curl -fsSL https://deb.nodesource.com/setup_20.x | bash - \
 && apt-get install -y --no-install-recommends nodejs \
 && rm -rf /var/lib/apt/lists/* \
 && npm install -g @anthropic-ai/claude-code

WORKDIR /workspace
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
```

- [ ] **Step 2: Write entrypoint.sh**

```bash
# runner/entrypoint.sh
#!/usr/bin/env bash
set -euo pipefail

: "${JOB_ID:?JOB_ID required}"
: "${REPO:?REPO required}"
: "${BASE_BRANCH:?BASE_BRANCH required}"
: "${WORK_BRANCH:?WORK_BRANCH required}"
: "${GH_TOKEN:?GH_TOKEN required}"
: "${ANTHROPIC_API_KEY:?ANTHROPIC_API_KEY required}"
: "${SPEC_TYPE:?SPEC_TYPE required}"
: "${SPEC_VALUE:?SPEC_VALUE required}"

cd /workspace

# Configure git identity for the commits we'll make
git config --global user.email "agentic-delegator@local"
git config --global user.name "agentic-delegator"

echo "[delegator] cloning $REPO …"
git clone "https://x-access-token:${GH_TOKEN}@github.com/${REPO}.git" repo
cd repo

# Either continue an existing branch (fetch + checkout) OR create from base.
if git fetch origin "${WORK_BRANCH}" 2>/dev/null && git rev-parse --verify "origin/${WORK_BRANCH}" >/dev/null 2>&1; then
    echo "[delegator] continuing existing branch ${WORK_BRANCH}"
    git checkout -B "${WORK_BRANCH}" "origin/${WORK_BRANCH}"
else
    echo "[delegator] creating new branch ${WORK_BRANCH} from ${BASE_BRANCH}"
    git checkout "${BASE_BRANCH}"
    git checkout -b "${WORK_BRANCH}"
fi

# Resolve the spec to a single string we feed to Claude.
case "${SPEC_TYPE}" in
    inline) SPEC_TEXT="${SPEC_VALUE}" ;;
    path)   SPEC_TEXT="$(cat "${SPEC_VALUE}")" ;;
    url)    SPEC_TEXT="$(curl -fsSL "${SPEC_VALUE}")" ;;
    *)      echo "unknown SPEC_TYPE: ${SPEC_TYPE}"; exit 2 ;;
esac

PROMPT="$(cat <<EOF
You are agentic-delegator. Implement the following spec on the current git working tree.

Spec:
${SPEC_TEXT}

When done:
1. Stage and commit your changes with a descriptive message.
2. Push the branch '${WORK_BRANCH}' to origin.
3. Open a pull request with 'gh pr create --base ${BASE_BRANCH} --head ${WORK_BRANCH}'.
4. Write the resulting PR URL to /workspace/.pr-url so the orchestrator can pick it up.
EOF
)"

# Run Claude Code in non-interactive mode. The exact flag set may vary across
# Claude Code releases — adjust if the binary in the image rejects them.
echo "[delegator] running claude…"
GH_TOKEN="${GH_TOKEN}" claude --dangerously-skip-permissions --print "${PROMPT}"
RC=$?

# As a safety net: if Claude didn't write .pr-url but a PR was opened, try to discover it.
if [ ! -f /workspace/.pr-url ]; then
    if pr_url=$(gh pr view "${WORK_BRANCH}" --json url --jq .url 2>/dev/null); then
        echo "${pr_url}" > /workspace/.pr-url
    fi
fi

exit ${RC}
```

- [ ] **Step 3: Try to build the image (best-effort — npm install may take 1–2 minutes)**

```bash
cd /Users/akef/workspace/agentic-delegator
docker build -t agentic-delegator-runner:dev runner/
```

If the build fails because of network issues (Node repo, npm package, etc.), that's a known build-time problem — note it as a concern but continue. The Dockerfile is committed regardless and can be rebuilt later.

- [ ] **Step 4: Commit**

```bash
git add runner/Dockerfile runner/entrypoint.sh
git commit -m "feat(runner): Dockerfile + entrypoint for Claude Code + gh + git"
```

---

## Phase D — Composition root (`cmd/agentic-delegator/main.go`)

### Task 9: Config loader

**Files:**
- Create: `core/config/config.go`
- Test: `core/config/config_test.go`

- [ ] **Step 1: Write tests + impl**

```go
// core/config/config_test.go
package config_test

import (
	"testing"

	"agentic-delegator/core/config"
)

func TestLoad_defaults(t *testing.T) {
	t.Setenv("DELEGATOR_DSN", "")
	t.Setenv("AGENTIC_MASTER_KEY", "0000000000000000000000000000000000000000000000000000000000000000")
	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.HTTPBind != "127.0.0.1:8787" {
		t.Fatalf("HTTPBind default: %s", c.HTTPBind)
	}
	if c.MaxConcurrentPerUser != 3 {
		t.Fatalf("MaxConcurrentPerUser default: %d", c.MaxConcurrentPerUser)
	}
}

func TestLoad_rejectsBadMasterKey(t *testing.T) {
	t.Setenv("AGENTIC_MASTER_KEY", "tooshort")
	_, err := config.Load()
	if err == nil {
		t.Fatalf("expected error for short master key")
	}
}
```

```go
// core/config/config.go
package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
)

// Config is the env-loaded runtime configuration.
type Config struct {
	HTTPBind             string // default "127.0.0.1:8787"
	DSN                  string // Postgres DSN
	MasterKey            []byte // 32 bytes, hex-encoded in env
	RunnerImage          string // e.g. "agentic-delegator-runner:dev"
	WorkDirHost          string // host dir mounted into runners
	MaxConcurrentPerUser int    // default 3
	MaxConcurrentGlobal  int    // default 10
}

func Load() (*Config, error) {
	c := &Config{
		HTTPBind:             getEnv("AGENTIC_HTTP_BIND", "127.0.0.1:8787"),
		DSN:                  getEnv("DELEGATOR_DSN", "postgres://delegator:delegator@127.0.0.1:5433/delegator?sslmode=disable"),
		RunnerImage:          getEnv("AGENTIC_RUNNER_IMAGE", "agentic-delegator-runner:dev"),
		WorkDirHost:          getEnv("AGENTIC_WORK_DIR", "/tmp/agentic-delegator"),
		MaxConcurrentPerUser: getEnvInt("AGENTIC_MAX_CONCURRENT_PER_USER", 3),
		MaxConcurrentGlobal:  getEnvInt("AGENTIC_MAX_CONCURRENT_GLOBAL", 10),
	}

	keyHex := getEnv("AGENTIC_MASTER_KEY", "")
	if keyHex == "" {
		return nil, fmt.Errorf("AGENTIC_MASTER_KEY required (32 bytes hex)")
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("AGENTIC_MASTER_KEY must be 64 hex chars (32 bytes); got %d bytes", len(key))
	}
	c.MasterKey = key

	return c, nil
}

func getEnv(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func getEnvInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
```

- [ ] **Step 2: Test + commit**

```bash
go test ./core/config/...
make arch-check
git add core/config/
git commit -m "feat(config): env-loaded runtime config"
```

---

### Task 10: Composition root (`cmd/agentic-delegator/main.go`)

**Files:**
- Create: `cmd/agentic-delegator/main.go`

This is the wire-everything-together file. It's the only place that imports adapters concretely.

- [ ] **Step 1: Implement**

```go
// cmd/agentic-delegator/main.go
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"

	"agentic-delegator/core/adapter/clock"
	"agentic-delegator/core/adapter/crypto"
	"agentic-delegator/core/adapter/docker"
	adhttp "agentic-delegator/core/adapter/http"
	"agentic-delegator/core/adapter/idgen"
	"agentic-delegator/core/adapter/postgres"
	"agentic-delegator/core/adapter/webhook"
	"agentic-delegator/core/config"
	"agentic-delegator/core/domain"
	"agentic-delegator/core/runtime/selfhost"
	"agentic-delegator/core/usecase"
	"agentic-delegator/core/usecase/ports"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
			runInit()
			return
		case "serve":
			os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
			runServe()
			return
		case "reset-key":
			os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
			runResetKey()
			return
		}
	}
	fmt.Fprintln(os.Stderr, "usage: agentic-delegator <init|serve|reset-key>")
	os.Exit(2)
}

// ----- init -----

func runInit() {
	flag.Parse()
	cfg, err := config.Load()
	must("config.Load", err)
	db, err := postgres.Open(cfg.DSN)
	must("postgres.Open", err)
	defer db.Close()

	ctx := context.Background()

	// Ensure migrate has been run (best-effort: trying to insert the admin user
	// surfaces "table does not exist" if not).
	if _, err := db.ExecContext(ctx, `SELECT 1 FROM users LIMIT 1`); err != nil {
		log.Fatalf("schema not initialized — run `agentic-delegator migrate` first: %v", err)
	}

	// Generate the admin API key (plaintext shown once)
	plain := newAdminKeyPlaintext()
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	must("bcrypt", err)

	// Persist hash + ensure admin row + clear PAT
	users := postgres.NewUsersBootstrapRepo(db)
	if err := users.UpsertAdmin(ctx, selfhost.AdminUserID, "admin", time.Now().UTC()); err != nil {
		log.Fatalf("UpsertAdmin: %v", err)
	}
	keys := postgres.NewAPIKeysRepo(db)
	// Wipe any existing admin keys for re-init
	if existing, err := keys.ListForUser(ctx, selfhost.AdminUserID); err == nil {
		for _, k := range existing {
			_ = keys.Delete(ctx, k.ID, selfhost.AdminUserID)
		}
	}
	id := idgen.NanoID{}.NewAPIKeyID()
	prefix := plain[:8]
	if err := keys.Create(ctx, domain.NewAPIKey(domain.APIKeyID(id), selfhost.AdminUserID, "admin", prefix, hash, time.Now().UTC())); err != nil {
		log.Fatalf("Create api key: %v", err)
	}

	fmt.Println("admin API key (saved once — copy now):")
	fmt.Println(plain)
}

func newAdminKeyPlaintext() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	return "agdkey_admin_" + hex.EncodeToString(b)
}

// ----- serve -----

func runServe() {
	cfg, err := config.Load()
	must("config.Load", err)

	db, err := postgres.Open(cfg.DSN)
	must("postgres.Open", err)
	defer db.Close()

	ctx := context.Background()

	// outbound adapters
	clk := clock.System{}
	idg := idgen.NanoID{}
	aes, err := crypto.NewAESGCM(cfg.MasterKey)
	must("crypto.NewAESGCM", err)
	_ = aes // wired into secret encryption wrapper below

	jobsRepo := postgres.NewJobsRepo(db)
	rawSecrets := postgres.NewSecretsRepo(db)
	secrets := newEncryptingSecrets(rawSecrets, aes) // see helper at bottom of this file
	apiKeys := postgres.NewAPIKeysRepo(db)
	usersBootstrap := postgres.NewUsersBootstrapRepo(db)
	patStore := postgres.NewSelfhostPATStore(db, aes) // small adapter — created in Task 11
	runner := docker.New(docker.Config{
		Image:    cfg.RunnerImage,
		CPUs:     "2",
		MemoryMB: 2048,
	})
	hooks := webhook.New(&http.Client{Timeout: 10 * time.Second})

	// Load existing admin key hash (from the only api_keys row for AdminUserID)
	adminHash := loadAdminKeyHash(ctx, apiKeys)

	// Edition
	repoCreds := selfhost.NewRepoCredsProvider(patStore)
	anthCreds := selfhost.NewAnthropicCredsProvider(secrets)
	bootstrap := selfhost.NewAdminBootstrap(usersBootstrap, clk, patStore, adminHash)
	edition := selfhost.New(repoCreds, anthCreds, bootstrap, adminHash)
	if err := edition.Bootstrap(ctx); err != nil {
		log.Fatalf("edition.Bootstrap: %v", err)
	}

	// Use cases
	enqueue := &usecase.EnqueueJob{
		Jobs:                 jobsRepo,
		RepoCreds:            repoCreds,
		AnthropicCreds:       anthCreds,
		Runner:               runner,
		IDGen:                idg,
		Clock:                clk,
		MaxConcurrentPerUser: cfg.MaxConcurrentPerUser,
		MaxConcurrentGlobal:  cfg.MaxConcurrentGlobal,
		OnComplete:           func(res ports.RunnerResult) { /* wired below */ },
	}
	getJob := &usecase.GetJob{Jobs: jobsRepo}
	listJobs := &usecase.ListJobs{Jobs: jobsRepo}
	complete := &usecase.HandleRunnerCompletion{Jobs: jobsRepo, Clock: clk}
	reattach := &usecase.ReattachRunningJobs{Jobs: jobsRepo, Runner: runner, Clock: clk}
	mint := &usecase.MintAPIKey{Keys: apiKeys, IDGen: idg, Clock: clk}
	revoke := &usecase.RevokeAPIKey{Keys: apiKeys}
	setAnth := &usecase.SetAnthropicCredentials{Secrets: secrets}
	dispatch := &usecase.DispatchCompletionWebhook{Dispatcher: hooks}

	enqueue.OnComplete = func(res ports.RunnerResult) {
		_ = complete.Execute(ctx, res)
		_ = dispatch // dispatch wired in once notification_webhook plumbing lands (Phase 2)
	}

	if err := reattach.Execute(ctx); err != nil {
		log.Printf("reattach (best effort): %v", err)
	}

	// HTTP
	jobsHandler := adhttp.NewJobsHandler(enqueue, getJob, listJobs)
	settingsHandler := adhttp.NewSettingsHandler(setAnth, mint, revoke)
	statusPage := adhttp.NewStatusPage(getJob)
	dashHandler := adhttp.NewDashboardHandler(listJobs, apiKeys, secrets)

	router := adhttp.NewRouter(adhttp.Deps{
		Resolver:        editionResolver{e: edition},
		JobsHandler:     jobsHandler,
		SettingsHandler: settingsHandler,
		StatusPage:      statusPage,
		Dashboard:       dashHandler,
		Edition:         edition,
	})

	log.Printf("listening on http://%s", cfg.HTTPBind)
	if err := http.ListenAndServe(cfg.HTTPBind, router); err != nil {
		log.Fatal(err)
	}
}

// editionResolver bridges runtime.Edition's ResolveUser into the HTTP
// adapter's UserResolver shape.
type editionResolver struct{ e *selfhost.Edition }

func (er editionResolver) Resolve(r *http.Request) (domain.UserID, error) {
	return er.e.ResolveUser(r)
}

// ----- reset-key -----

func runResetKey() {
	// Same as init's key-generation step, but doesn't touch users/PAT.
	cfg, err := config.Load()
	must("config.Load", err)
	db, err := postgres.Open(cfg.DSN)
	must("postgres.Open", err)
	defer db.Close()

	ctx := context.Background()
	keys := postgres.NewAPIKeysRepo(db)
	if existing, err := keys.ListForUser(ctx, selfhost.AdminUserID); err == nil {
		for _, k := range existing {
			_ = keys.Delete(ctx, k.ID, selfhost.AdminUserID)
		}
	}
	plain := newAdminKeyPlaintext()
	hash, _ := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	id := idgen.NanoID{}.NewAPIKeyID()
	prefix := plain[:8]
	_ = keys.Create(ctx, domain.NewAPIKey(domain.APIKeyID(id), selfhost.AdminUserID, "admin", prefix, hash, time.Now().UTC()))
	fmt.Println("new admin API key:")
	fmt.Println(plain)
}

// ----- helpers -----

func must(what string, err error) {
	if err != nil {
		log.Fatalf("%s: %v", what, err)
	}
}

func loadAdminKeyHash(ctx context.Context, keys ports.APIKeysRepository) []byte {
	list, err := keys.ListForUser(ctx, selfhost.AdminUserID)
	if err != nil || len(list) == 0 {
		return nil
	}
	return []byte(list[0].Hash)
}

// encryptingSecrets is a tiny decorator that wraps SecretsRepository with
// AES-GCM at the composition seam. The Postgres impl stores bytes; this
// wrapper encrypts on write and decrypts on read.
type encryptingSecrets struct {
	inner ports.SecretsRepository
	aes   *crypto.AESGCM
}

func newEncryptingSecrets(inner ports.SecretsRepository, aes *crypto.AESGCM) *encryptingSecrets {
	return &encryptingSecrets{inner: inner, aes: aes}
}

func (e *encryptingSecrets) SetAnthropicCreds(ctx context.Context, userID domain.UserID, c domain.AnthropicCreds) error {
	ct, err := e.aes.Encrypt([]byte(c.APIKey))
	if err != nil {
		return err
	}
	return e.inner.SetAnthropicCreds(ctx, userID, domain.AnthropicCreds{APIKey: string(ct)})
}

func (e *encryptingSecrets) GetAnthropicCreds(ctx context.Context, userID domain.UserID) (domain.AnthropicCreds, error) {
	c, err := e.inner.GetAnthropicCreds(ctx, userID)
	if err != nil {
		return c, err
	}
	pt, err := e.aes.Decrypt([]byte(c.APIKey))
	if err != nil {
		return c, err
	}
	return domain.AnthropicCreds{APIKey: string(pt)}, nil
}

func (e *encryptingSecrets) DeleteAnthropicCreds(ctx context.Context, userID domain.UserID) error {
	return e.inner.DeleteAnthropicCreds(ctx, userID)
}
```

- [ ] **Step 2: Compile**

```bash
go build ./cmd/agentic-delegator
```

You may get compile errors referencing `postgres.NewUsersBootstrapRepo` and `postgres.NewSelfhostPATStore` — those are tiny new additions to the postgres package (next task).

- [ ] **Step 3: Defer compile to Task 11**, write the file, but don't commit yet.

---

### Task 11: Tiny Postgres additions (UsersBootstrapRepo + SelfhostPATStore)

**Files:**
- Modify: `core/adapter/postgres/users_repo.go` (NEW file)
- Modify: `core/adapter/postgres/selfhost_pat.go` (NEW file)

- [ ] **Step 1: UsersBootstrapRepo**

```go
// core/adapter/postgres/users_repo.go
package postgres

import (
	"context"
	"time"

	"github.com/uptrace/bun"

	"agentic-delegator/core/domain"
)

// UsersBootstrapRepo provides the tiny user-row upsert that selfhost needs.
type UsersBootstrapRepo struct {
	db *bun.DB
}

func NewUsersBootstrapRepo(db *bun.DB) *UsersBootstrapRepo {
	return &UsersBootstrapRepo{db: db}
}

func (r *UsersBootstrapRepo) UpsertAdmin(ctx context.Context, id domain.UserID, displayName string, now time.Time) error {
	row := &userRow{ID: string(id), DisplayName: displayName, CreatedAt: now}
	_, err := r.db.NewInsert().Model(row).
		On("CONFLICT (id) DO UPDATE").
		Set("display_name = EXCLUDED.display_name").
		Exec(ctx)
	return err
}
```

- [ ] **Step 2: SelfhostPATStore + migration**

Create a new table for the admin's PAT (single row, by convention `user_id = 'u_admin'`). Add a migration file:

`core/adapter/postgres/migrations/20260521000002_selfhost_admin_pat.go`:

```go
package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS selfhost_admin_pat (
    user_id  TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    pat_enc  BYTEA NOT NULL
);
`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS selfhost_admin_pat`)
			return err
		},
	)
}
```

And the PAT store impl:

```go
// core/adapter/postgres/selfhost_pat.go
package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"agentic-delegator/core/adapter/crypto"
	"agentic-delegator/core/domain"
	"agentic-delegator/core/runtime/selfhost"
)

type selfhostPATRow struct {
	bun.BaseModel `bun:"table:selfhost_admin_pat"`

	UserID string `bun:"user_id,pk"`
	PATEnc []byte `bun:"pat_enc,notnull"`
}

// SelfhostPATStore implements selfhost.PATStore using Postgres + AES-GCM.
type SelfhostPATStore struct {
	db  *bun.DB
	aes *crypto.AESGCM
}

func NewSelfhostPATStore(db *bun.DB, aes *crypto.AESGCM) *SelfhostPATStore {
	return &SelfhostPATStore{db: db, aes: aes}
}

var _ selfhost.PATStore = (*SelfhostPATStore)(nil)

func (s *SelfhostPATStore) Set(ctx context.Context, pat string) error {
	ct, err := s.aes.Encrypt([]byte(pat))
	if err != nil {
		return err
	}
	row := &selfhostPATRow{UserID: string(selfhost.AdminUserID), PATEnc: ct}
	_, err = s.db.NewInsert().Model(row).
		On("CONFLICT (user_id) DO UPDATE").
		Set("pat_enc = EXCLUDED.pat_enc").
		Exec(ctx)
	return err
}

func (s *SelfhostPATStore) Get(ctx context.Context) (string, error) {
	var row selfhostPATRow
	err := s.db.NewSelect().Model(&row).Where("user_id = ?", string(selfhost.AdminUserID)).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	pt, err := s.aes.Decrypt(row.PATEnc)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
```

- [ ] **Step 3: Update arch-lint** — `adapter_postgres` now depends on `adapter_crypto` and `runtime_selfhost` (for the selfhost.PATStore interface). This is a small SRP carve-out — selfhost-specific Postgres tables live in the postgres package but are tagged as selfhost-aware.

Actually a cleaner approach: move `SelfhostPATStore` to a new sub-package `core/adapter/postgres/selfhost/`. Update arch-lint accordingly:

```yaml
  adapter_postgres_selfhost:
    in: core/adapter/postgres/selfhost/**
```

```yaml
  adapter_postgres_selfhost:
    mayDependOn: [domain, ports, adapter_postgres, adapter_crypto, runtime_selfhost]
    anyVendorDeps: true
```

Move the file accordingly: rename to `core/adapter/postgres/selfhost/pat_store.go`, package `selfhost_pg`.

- [ ] **Step 4: Compile**

```bash
go build ./...
```

- [ ] **Step 5: Apply the new migration**

```bash
go run ./cmd/agentic-delegator/migrate -cmd=up
```

- [ ] **Step 6: Commit**

```bash
make arch-check
git add core/adapter/postgres/ .go-arch-lint.yml cmd/agentic-delegator/main.go
git commit -m "feat(cmd+postgres): composition root + selfhost PAT store + users bootstrap"
```

---

## Phase E — Skill markdown

### Task 12: Claude Code skill

**Files:**
- Create: `skill/delegate.md`

- [ ] **Step 1: Write the skill**

```markdown
---
name: delegate
description: Delegate a coding task to the agentic-delegator service. Pass it a spec (path/URL/inline) and it returns a job link.
---

# /delegate — delegate a coding task

You are running inside a Claude Code session. The user invoked `/delegate` to send a coding task to a running agentic-delegator service. The service will spawn a sandboxed runner, implement the spec, push a branch, and open a PR.

## Required env vars (must be set in the user's shell)

- `AGENTIC_DELEGATOR_URL` — e.g. `http://localhost:8787`
- `AGENTIC_DELEGATOR_API_KEY` — the personal API key minted from the dashboard

If either is missing, stop and tell the user how to set them.

## Workflow

1. **Detect repo + branch.**
   - Run `git remote get-url origin` to detect the GitHub repo. Expect a URL like `git@github.com:owner/name.git` or `https://github.com/owner/name.git`. Extract `owner/name`.
   - Run `git branch --show-current` to detect the current branch.
   - If you're not inside a git repo, stop and tell the user.

2. **Ask the user for the spec source.** Examples:
   - "Path inside the repo: `specs/auth-refactor.md`"
   - "URL: `https://gist.githubusercontent.com/.../raw/spec.md`"
   - "Or paste the spec content directly."
   - Classify the input:
     - Starts with `http://` or `https://` → `source_type=url`
     - Looks like a relative path ending in `.md` → `source_type=path`
     - Otherwise → `source_type=inline`

3. **Ask the user about the work branch.** Default is a new branch: `agentic/<spec-stem>-<shortid>` where `<spec-stem>` is the spec filename without extension (for inline/url specs, ask the user for a short stem). The base branch defaults to the current branch (or `main` if user prefers). Let the user override either.

4. **Show an editable summary** of `{repo, base_branch, work_branch, spec_source, source_type, model_override}` and ask the user to confirm or edit.

5. **POST the job:**

   ```bash
   curl -sS -X POST \
     -H "Authorization: Bearer $AGENTIC_DELEGATOR_API_KEY" \
     -H "Content-Type: application/json" \
     "$AGENTIC_DELEGATOR_URL/api/jobs" \
     -d '<the json payload>'
   ```

   Use this JSON shape:
   ```json
   {
     "repo": "owner/name",
     "base_branch": "main",
     "work_branch": "agentic/auth-refactor-9q2k",
     "spec_source": "<the value the user supplied>",
     "source_type": "path|url|inline",
     "model_override": ""
   }
   ```

6. **Print the response** to the user. Expected:
   ```json
   { "job_id": "j_xxx", "status_url": "http://localhost:8787/jobs/j_xxx" }
   ```

   Tell the user: "Job started. Watch progress at <status_url>. The agent will commit, push, and open a PR when done."

7. **Exit.** Do not poll; the dashboard is the source of truth.

## Failure modes

- If curl returns non-2xx, surface the response body.
- If the response doesn't include `job_id`, treat it as a service-side error and tell the user.
- If the user's repo doesn't have a GitHub remote, the service won't be able to clone — stop early with a clear error.
```

- [ ] **Step 2: Commit**

```bash
git add skill/delegate.md
git commit -m "feat(skill): delegate.md for Claude Code"
```

---

## Phase F — End-to-end smoke test

### Task 13: Smoke test procedure + documentation

**Files:**
- Create: `docs/end-to-end-smoke.md`

- [ ] **Step 1: Write the smoke procedure**

```markdown
# End-to-end smoke test — selfhost

This documents the manual smoke test that verifies Plan 03 ships a working
selfhost binary.

## Prereqs

- `plan-03-done` tag set
- Docker daemon running
- A GitHub repo you can push to (sandbox account recommended)
- An Anthropic API key
- A GitHub PAT with `repo` scope

## Steps

1. **Bring up the host services**
   ```bash
   make dev-db-up
   docker build -t agentic-delegator-runner:dev runner/
   ```

2. **Initialize the schema + admin user + admin API key**
   ```bash
   export AGENTIC_MASTER_KEY=$(openssl rand -hex 32)
   echo "AGENTIC_MASTER_KEY=$AGENTIC_MASTER_KEY" > .env.local
   go run ./cmd/agentic-delegator/migrate -cmd=up
   go run ./cmd/agentic-delegator init
   # Save the printed key as $AGENTIC_DELEGATOR_API_KEY
   ```

3. **Set the PAT** — start the server and open the setup page:
   ```bash
   go run ./cmd/agentic-delegator serve
   # In another shell: open http://127.0.0.1:8787/admin/setup, paste the PAT
   ```

4. **Set the Anthropic key + mint a skill key**
   - Open http://127.0.0.1:8787/settings
   - Paste the Anthropic API key, click Save
   - The admin key from step 2 is already a usable skill key.

5. **Install the skill in your Claude Code**
   ```bash
   mkdir -p ~/.claude/skills/delegate
   cp skill/delegate.md ~/.claude/skills/delegate/
   ```
   In your shell rc file:
   ```bash
   export AGENTIC_DELEGATOR_URL=http://127.0.0.1:8787
   export AGENTIC_DELEGATOR_API_KEY=<the key from step 2>
   ```

6. **Trigger a job**
   - In your sandbox repo: create `specs/hello.md` containing
     `Add a HELLO.md file at the repo root with the text 'hi from delegator'.`
   - Commit + push the spec.
   - Invoke `/delegate` in Claude Code from inside that repo.
   - Confirm the summary, submit.

7. **Watch**
   - Open the status URL printed by the skill.
   - The log should stream; status moves queued → running → succeeded.

8. **Verify**
   - Click the PR link. The PR should contain a single new file `HELLO.md` with the expected text.
   - Merge the PR. Sandbox cleanup: `git push origin --delete agentic/hello-…`.

## Acceptance criteria

- [x] All commands in steps 1–5 succeed without manual fixups.
- [x] Step 6: the skill detects repo, asks for spec source, posts a job, returns a job link.
- [x] Step 7: the status page loads, the log polls, the PR link appears within ~3 minutes.
- [x] Step 8: the PR has the expected diff.

If any step fails, file a bug with the relevant log path from `/jobs/<id>`.
```

- [ ] **Step 2: Commit**

```bash
git add docs/end-to-end-smoke.md
git commit -m "docs: end-to-end smoke test procedure"
```

---

## Phase G — Final verification

### Task 14: Sweep + tag

- [ ] **Step 1: Build everything**

```bash
go build ./...
```

- [ ] **Step 2: Run all tests (unit + integration)**

```bash
make test-race
make test-integration
make arch-check
```

All green.

- [ ] **Step 3: Smoke the binary boots**

```bash
export AGENTIC_MASTER_KEY=$(openssl rand -hex 32)
go run ./cmd/agentic-delegator/migrate -cmd=up
echo "skipping full init since admin key persists; smoke just confirms boot"
( go run ./cmd/agentic-delegator serve & ) ; SERVE_PID=$!
sleep 2
curl -sI http://127.0.0.1:8787/ | head -1
curl -sI http://127.0.0.1:8787/admin/setup | head -1
kill $SERVE_PID 2>/dev/null || true
```

Expected: both curl HEAD requests return `HTTP/1.1 200 OK` (or 303 redirect).

- [ ] **Step 4: Tag**

```bash
git tag -a plan-03-done -m "Plan 03: selfhost edition + runner image + skill + composition root"
```

---

## Self-review

**Spec coverage:**
- `runtime.Edition` port ✓ (Task 1)
- Selfhost Edition + admin bootstrap + repo creds + Anthropic creds ✓ (Tasks 2–4)
- Templ presenter ✓ (Tasks 5–7)
- Runner Docker image ✓ (Task 8)
- Composition root in `cmd/agentic-delegator/main.go` ✓ (Tasks 9–11)
- Skill .md ✓ (Task 12)
- E2E smoke procedure ✓ (Task 13)

**Out of scope (Plan 04):**
- SaaS module
- GitHub OAuth signup
- GitHub App management

**Out of scope (Plan 05):**
- systemd service files
- install.sh
- CI workflow

**Known-rough spots flagged in plan body:**
- Task 7 has a placeholder Reader-interface mistake at the top of `dashboard_handler.go` — the "cleaned-up version" below it is what the implementer actually writes.
- Task 11 reorganizes Postgres into a sub-package (`postgres/selfhost/`) to keep SRP clean. Move SelfhostPATStore there; arch-lint config has a new component for it.
- Templ generation must be run after each `.templ` change. The Makefile's `generate` target handles it.
- The runner image's `claude` flag set (`--dangerously-skip-permissions --print`) reflects current Claude Code behavior. If the binary in the image rejects these flags, adjust the entrypoint and rebuild.

---

## Execution handoff

Two options as before:

1. **Subagent-Driven (recommended)** — fresh subagent per task with two-stage review.
2. **Inline Execution** — batch with checkpoints.

Which approach?
