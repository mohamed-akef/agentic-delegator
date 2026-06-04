# SaaS-Only Collapse Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collapse the dual-edition (selfhost + SaaS) codebase into a single SaaS-only application — one binary, no build tags, no `Edition` abstraction — preserving all current SaaS behaviour.

**Architecture:** Clean Architecture is retained (domain → usecase → adapter → cmd, dependency rule enforced by go-arch-lint). The `runtime.Edition` port and the `saas/` build-tagged module are removed; SaaS code moves into the normal adapter layer. Composition happens in a single `cmd/agentic-delegator/main.go`.

**Tech Stack:** Go, chi, Postgres + Bun, templ + HTMX, go-github + ghinstallation, oapi-codegen, go-arch-lint.

---

## Nature of this work (read first)

This is a **behaviour-preserving refactor**, not new feature work. The standard red-green TDD loop doesn't apply to relocating code. The safety net is instead:

- The existing test suite (relocated packages carry their existing tests).
- `go build ./...` compiling the single binary.
- `make arch-check` enforcing the dependency rule.

Each task's verification runs those. **Frequent commits**: one per task.

Two judgment calls that deviate from the design doc, made to minimise churn/risk:
- `saas/ghapp` → `core/adapter/ghapp` keeping **package name `ghapp`** (the design said `github`; that name clashes with the imported `go-github` package `github` and would force renaming every call site). Directory and package both `ghapp`.
- `saas/store` repositories merge into the **existing `postgres` package** (`core/adapter/postgres`) rather than a new package. Verified: no symbol collisions.

`saas/signup` + `saas/tenancy` merge into one new `auth` package at `core/adapter/http/auth` (as the design specified).

---

## File structure (target)

Created:
- `core/adapter/credentials/anthropic.go` — secrets-backed Anthropic creds provider (package `credentials`)
- `core/adapter/ghapp/` — GitHub App: `app_jwt.go`, `repo_creds.go`, `webhooks.go`, `install.go`, `doc.go` (+ tests) (package `ghapp`)
- `core/adapter/http/auth/` — `sessions.go`, `github_oauth.go`, `session_middleware.go`, `resolver.go`, `doc.go` (+ tests) (package `auth`)
- `core/adapter/postgres/installations_repo.go`, `sessions_repo.go`, `identities_repo.go`, `saas_models.go` (package `postgres`)
- `core/adapter/postgres/migrations/20260603000001_initial.go` — single consolidated migration

Modified:
- `cmd/agentic-delegator/main.go` — fully replaced: single binary, `serve` + `migrate` subcommands
- `core/config/config.go` — adds GitHub App / OAuth config
- `core/adapter/http/router.go` — `EditionRouteMounter`→`RouteMounter`, `Deps.Edition`→`Deps.Routes`
- `core/adapter/http/middleware.go` — comment only
- `core/adapter/http/dashboard_handler.go` — drop `editionName`
- `core/adapter/http/dashboard_handler_test.go` — drop `"selfhost"` arg
- `core/presenter/templ/pages/landing.templ` (+ regenerated `landing_templ.go`) — drop `editionName`
- `core/domain/credentials.go`, `core/domain/user.go`, `core/usecase/ports/repo_creds_provider.go`, `core/adapter/postgres/users_repo.go` — selfhost-mention comment cleanup
- `.go-arch-lint.yml` — components/deps updated
- `Makefile` — single binary targets
- `README.md`, `docs/saas-setup.md`, `docs/end-to-end-smoke.md`, `skill/delegate.md`, `docs/design/2026-05-21-mvp-design.md`

Deleted:
- `core/runtime/` (entire)
- `core/adapter/postgres/selfhost/` (entire)
- `core/adapter/postgres/migrations/20260521000001_initial.go`, `20260521000002_selfhost_admin_pat.go`
- `saas/` (entire)
- `cmd/agentic-delegator-saas/` (entire)
- `cmd/agentic-delegator/migrate/` (entire — folded into the `migrate` subcommand)
- `deploy/selfhost/` (entire)

---

## Task 1: Add relocated packages as untagged copies (additive, tree stays green)

Copy the SaaS/shared code into its new Clean-Architecture homes **without** build tags. The old `saas/` copies are build-tag-gated and the old selfhost code is untouched, so the default build keeps working and nothing conflicts. No deletions in this task.

**Files:**
- Create: `core/adapter/credentials/anthropic.go`
- Create: `core/adapter/ghapp/{app_jwt.go,repo_creds.go,webhooks.go,install.go,doc.go}` + tests
- Create: `core/adapter/http/auth/{sessions.go,github_oauth.go,session_middleware.go,resolver.go,doc.go}` + tests
- Create: `core/adapter/postgres/{installations_repo.go,sessions_repo.go,identities_repo.go,saas_models.go}`
- Modify: `.go-arch-lint.yml` (add new components only)

- [ ] **Step 1: Create `core/adapter/credentials/anthropic.go`**

```go
// core/adapter/credentials/anthropic.go
package credentials

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

// AnthropicCredsProvider yields a user's Anthropic credential from the secrets
// store. It implements ports.AnthropicCredentialsProvider.
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

- [ ] **Step 2: Copy the ghapp package**

Copy the non-test source files, stripping the `//go:build saas` line from each:

```bash
mkdir -p core/adapter/ghapp
for f in app_jwt.go repo_creds.go webhooks.go install.go doc.go; do
  sed '/^\/\/go:build saas$/d' "saas/ghapp/$f" | sed '/^$/{/./!d;}' > "core/adapter/ghapp/$f"
done
# copy tests too (strip build tag)
for f in app_jwt_test.go repo_creds_test.go webhooks_test.go; do
  sed '/^\/\/go:build saas$/d' "saas/ghapp/$f" > "core/adapter/ghapp/$f"
done
```

Then open each copied file and remove any now-stale leading blank line and fix the path comment (e.g. `// saas/ghapp/app_jwt.go` → `// core/adapter/ghapp/app_jwt.go`). Package stays `ghapp` / `ghapp_test`. No import changes needed (ghapp imports only domain, ports, and vendor — verified it does not import `core/runtime` or `core/adapter/crypto`).

- [ ] **Step 3: Create the merged `auth` package**

Create `core/adapter/http/auth/` and copy in the bodies of `saas/signup/{sessions.go,github_oauth.go,session_middleware.go,doc.go}` and `saas/tenancy/resolver.go`, making these edits to each:
- Remove the `//go:build saas` line.
- Change the package declaration to `package auth`.
- Fix the path comment.
- In `resolver.go`, the `SessionResolver` interface and `Resolver` type now live in the same package as `Sessions` — no import change needed; `*auth.Sessions` satisfies `auth.SessionResolver`.

Write `core/adapter/http/auth/doc.go`:

```go
// Package auth implements GitHub OAuth sign-in, cookie sessions, and request
// authentication (session cookie or bearer API key -> UserID).
package auth
```

Copy the tests `saas/signup/{github_oauth_test.go,sessions_test.go}` and `saas/tenancy/resolver_test.go` into the package, stripping build tags and renaming their package declarations (`signup_test` / `tenancy_test`) to `auth_test`. If the compiler reports a duplicate identifier across the merged external test files, rename the colliding helper in one file.

- [ ] **Step 4: Copy the store repos into the `postgres` package**

Copy `saas/store/{installations_repo.go,sessions_repo.go,identities_repo.go,models.go}` into `core/adapter/postgres/` as `installations_repo.go`, `sessions_repo.go`, `identities_repo.go`, `saas_models.go`. For each: remove the `//go:build saas` line, change `package store` to `package postgres`, fix the path comment. No symbol collisions exist (verified). The unexported row types (`identityRow`, `installationRow`, `sessionRow`) and exported types (`Installation`, `GitHubIdentity`, `InstallationsRepo`, `SessionsRepo`, `IdentitiesRepo`) move verbatim.

- [ ] **Step 5: Add new arch-lint components (additive)**

In `.go-arch-lint.yml`, under `components:` add:

```yaml
  adapter_credentials:
    in: core/adapter/credentials/**
  adapter_ghapp:
    in: core/adapter/ghapp/**
  adapter_auth:
    in: core/adapter/http/auth/**
```

Under `deps:` add:

```yaml
  adapter_credentials:
    mayDependOn: [domain, ports]
    anyVendorDeps: true
  adapter_ghapp:
    mayDependOn: [domain, ports, usecase]
    anyVendorDeps: true
  adapter_auth:
    mayDependOn: [domain, ports, usecase]
    anyVendorDeps: true
```

Leave all existing components (including `runtime*`, `saas_*`, `adapter_postgres_selfhost`) in place for now.

- [ ] **Step 6: Verify build, tests, arch-check**

Run:
```bash
go build ./... && go test ./... && make arch-check
```
Expected: all pass. (The new packages compile and their tests run; the old build-tagged `saas/` copies are excluded from the default build, so there's no duplication conflict.)

- [ ] **Step 7: Commit**

```bash
git add core/adapter/credentials core/adapter/ghapp core/adapter/http/auth \
        core/adapter/postgres/installations_repo.go core/adapter/postgres/sessions_repo.go \
        core/adapter/postgres/identities_repo.go core/adapter/postgres/saas_models.go .go-arch-lint.yml
git commit -m "refactor: relocate SaaS+shared code into core adapter layer (untagged copies)"
```

---

## Task 2: Cutover — single binary, remove Edition, delete old trees

This is one atomic change: the `Edition` port, the router seam, and both `cmd` entrypoints are interdependent and cannot be split without a broken build. Make all edits, then verify once at the end.

**Files:** see per-step. Deletions happen in Step 11.

- [ ] **Step 1: Replace `core/config/config.go`**

Full replacement (adds GitHub App / OAuth fields; GH values are non-fatal so `migrate` works without them):

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

	// GitHub App + OAuth (required for `serve`; optional for `migrate`).
	GHAppID            int64
	GHAppPrivateKey    []byte
	GHAppSlug          string
	GHClientID         string
	GHClientSecret     string
	GHOAuthRedirectURL string
	GHWebhookSecret    []byte
}

func Load() (*Config, error) {
	c := &Config{
		HTTPBind:             getEnv("AGENTIC_HTTP_BIND", "127.0.0.1:8787"),
		DSN:                  getEnv("DELEGATOR_DSN", "postgres://delegator:delegator@127.0.0.1:5433/delegator?sslmode=disable"),
		RunnerImage:          getEnv("AGENTIC_RUNNER_IMAGE", "agentic-delegator-runner:dev"),
		WorkDirHost:          getEnv("AGENTIC_WORK_DIR", "/tmp/agentic-delegator"),
		MaxConcurrentPerUser: getEnvInt("AGENTIC_MAX_CONCURRENT_PER_USER", 3),
		MaxConcurrentGlobal:  getEnvInt("AGENTIC_MAX_CONCURRENT_GLOBAL", 10),

		GHAppID:            getEnvInt64("AGENTIC_GH_APP_ID", 0),
		GHAppPrivateKey:    []byte(getEnv("AGENTIC_GH_APP_PRIVATE_KEY", "")),
		GHAppSlug:          getEnv("AGENTIC_GH_APP_SLUG", ""),
		GHClientID:         getEnv("AGENTIC_GH_CLIENT_ID", ""),
		GHClientSecret:     getEnv("AGENTIC_GH_CLIENT_SECRET", ""),
		GHOAuthRedirectURL: getEnv("AGENTIC_GH_OAUTH_REDIRECT_URL", ""),
		GHWebhookSecret:    []byte(getEnv("AGENTIC_GH_WEBHOOK_SECRET", "")),
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

func getEnvInt64(name string, def int64) int64 {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}
```

- [ ] **Step 2: Edit `core/adapter/http/router.go`**

Rename the edition seam to a neutral route mounter. Replace the `EditionRouteMounter` type and the `Deps.Edition` field and its use:

Old:
```go
// EditionRouteMounter is the slice of the runtime.Edition interface the
// router needs. Defined here so router doesn't import core/runtime.
type EditionRouteMounter interface {
	RegisterRoutes(r chi.Router)
}
```
New:
```go
// RouteMounter lets the composition root mount auth/webhook routes without the
// http adapter importing the auth/ghapp adapters (preserves the SRP boundary).
type RouteMounter interface {
	RegisterRoutes(r chi.Router)
}
```

Old:
```go
	Dashboard       *DashboardHandler
	Edition         EditionRouteMounter // calls Edition.RegisterRoutes
}
```
New:
```go
	Dashboard       *DashboardHandler
	Routes          RouteMounter // mounts /login, /auth/*, /webhooks/github
}
```

Old:
```go
	// edition-specific routes (selfhost: /admin/setup; saas: /login, etc.)
	if deps.Edition != nil {
		deps.Edition.RegisterRoutes(r)
	}
```
New:
```go
	// auth + GitHub-App routes (/login, /auth/github/callback,
	// /auth/github-app/*, /webhooks/github)
	if deps.Routes != nil {
		deps.Routes.RegisterRoutes(r)
	}
```

- [ ] **Step 3: Edit `core/adapter/http/middleware.go` comment**

Old:
```go
// UserResolver looks up a user from an HTTP request. Editions implement this;
// for selfhost it's the admin, for SaaS it's a session or bearer-key lookup.
```
New:
```go
// UserResolver looks up a user from an HTTP request via session cookie or
// bearer API key.
```

- [ ] **Step 4: Edit `core/adapter/http/dashboard_handler.go`**

Remove the `editionName` field and constructor param, and the arg to `pages.Landing`.

Old struct + constructor:
```go
type DashboardHandler struct {
	list        *usecase.ListJobs
	keys        ports.APIKeysRepository
	secrets     ports.SecretsRepository
	editionName string
	resolver    UserResolver
}

func NewDashboardHandler(list *usecase.ListJobs, keys ports.APIKeysRepository, secrets ports.SecretsRepository, editionName string, resolver UserResolver) *DashboardHandler {
	return &DashboardHandler{list: list, keys: keys, secrets: secrets, editionName: editionName, resolver: resolver}
}
```
New:
```go
type DashboardHandler struct {
	list     *usecase.ListJobs
	keys     ports.APIKeysRepository
	secrets  ports.SecretsRepository
	resolver UserResolver
}

func NewDashboardHandler(list *usecase.ListJobs, keys ports.APIKeysRepository, secrets ports.SecretsRepository, resolver UserResolver) *DashboardHandler {
	return &DashboardHandler{list: list, keys: keys, secrets: secrets, resolver: resolver}
}
```

Old (in `Landing`):
```go
	_ = pages.Landing(h.editionName).Render(r.Context(), w)
```
New:
```go
	_ = pages.Landing().Render(r.Context(), w)
```

- [ ] **Step 5: Edit `core/presenter/templ/pages/landing.templ`**

Drop the param and collapse the conditional to the SaaS branch.

Old:
```
templ Landing(editionName string) {
```
New:
```
templ Landing() {
```

Old (the whole conditional block):
```
                if editionName == "saas" {
                    @ui.LinkButton("/login", "primary") {
                        Sign in with GitHub
                    }
                } else {
                    @ui.LinkButton("/admin/setup", "primary") {
                        First-time setup
                    }
                    @ui.LinkButton("/dashboard", "secondary") {
                        Open dashboard
                    }
                }
```
New:
```
                @ui.LinkButton("/login", "primary") {
                    Sign in with GitHub
                }
```

- [ ] **Step 6: Regenerate templ + codegen**

Run:
```bash
make generate
```
Expected: regenerates `core/adapter/http/gen` and `landing_templ.go` so `Landing()` now takes no args. (If `make generate` is unavailable in the environment, hand-edit `landing_templ.go`: change `func Landing(editionName string)` to `func Landing()` and replace the `if editionName == "saas"` rendering block with the single `/login` LinkButton output.)

- [ ] **Step 7: Fix `core/adapter/http/dashboard_handler_test.go`**

Remove the `"selfhost"` argument from the `NewDashboardHandler` call.

Old:
```go
	h := adhttp.NewDashboardHandler(
		&usecase.ListJobs{Jobs: jobs},
		testutil.NewFakeAPIKeysRepo(),
		testutil.NewFakeSecretsRepo(),
		"selfhost",
		nil, // landing redirect-if-authenticated not exercised in dashboard test
	)
```
New:
```go
	h := adhttp.NewDashboardHandler(
		&usecase.ListJobs{Jobs: jobs},
		testutil.NewFakeAPIKeysRepo(),
		testutil.NewFakeSecretsRepo(),
		nil, // landing redirect-if-authenticated not exercised in dashboard test
	)
```

- [ ] **Step 8: Consolidate migrations**

Delete the two old migration files and create one consolidated migration (union of core + saas tables, minus `selfhost_admin_pat`). Table names keep the `saas_` prefix to match the moved `saas_models.go` bun tags.

```bash
git rm core/adapter/postgres/migrations/20260521000001_initial.go \
       core/adapter/postgres/migrations/20260521000002_selfhost_admin_pat.go
```

Create `core/adapter/postgres/migrations/20260603000001_initial.go`:

```go
// core/adapter/postgres/migrations/20260603000001_initial.go
package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		// up
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    display_name  TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_secrets (
    user_id              TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    anthropic_key_enc    BYTEA NOT NULL,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS api_keys (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    key_prefix    TEXT NOT NULL,
    key_hash      BYTEA NOT NULL,
    last_used_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys(key_prefix);

CREATE TABLE IF NOT EXISTS jobs (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status          TEXT NOT NULL,
    repo            TEXT NOT NULL,
    base_branch     TEXT NOT NULL,
    work_branch     TEXT NOT NULL,
    spec_source     TEXT NOT NULL,
    source_type     TEXT NOT NULL,
    model_override  TEXT NOT NULL DEFAULT '',
    container_id    TEXT NOT NULL DEFAULT '',
    pr_url          TEXT NOT NULL DEFAULT '',
    error           TEXT NOT NULL DEFAULT '',
    log_path        TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_jobs_user_status ON jobs(user_id, status);
CREATE INDEX IF NOT EXISTS idx_jobs_created ON jobs(created_at DESC);

CREATE TABLE IF NOT EXISTS saas_github_identities (
    user_id       TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    github_id     BIGINT UNIQUE NOT NULL,
    github_login  TEXT NOT NULL,
    email         TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS saas_github_installations (
    installation_id  BIGINT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_login    TEXT NOT NULL,
    repos            JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_saas_install_user ON saas_github_installations(user_id);

CREATE TABLE IF NOT EXISTS saas_sessions (
    id          BYTEA PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_saas_sessions_user ON saas_sessions(user_id);
`)
			return err
		},
		// down
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
DROP TABLE IF EXISTS saas_sessions;
DROP TABLE IF EXISTS saas_github_installations;
DROP TABLE IF EXISTS saas_github_identities;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS user_secrets;
DROP TABLE IF EXISTS users;
`)
			return err
		},
	)
}
```

- [ ] **Step 9: Replace `cmd/agentic-delegator/main.go`**

Full replacement — single binary, `serve` + `migrate` subcommands, no edition shim. Write the file verbatim:

```go
// cmd/agentic-delegator/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"

	"agentic-delegator/core/adapter/clock"
	"agentic-delegator/core/adapter/credentials"
	adcrypto "agentic-delegator/core/adapter/crypto"
	"agentic-delegator/core/adapter/docker"
	"agentic-delegator/core/adapter/ghapp"
	adhttp "agentic-delegator/core/adapter/http"
	"agentic-delegator/core/adapter/http/auth"
	"agentic-delegator/core/adapter/idgen"
	"agentic-delegator/core/adapter/postgres"
	pgmig "agentic-delegator/core/adapter/postgres/migrations"
	"agentic-delegator/core/adapter/webhook"
	"agentic-delegator/core/config"
	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase"
	"agentic-delegator/core/usecase/ports"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: agentic-delegator <serve|migrate>")
	}
	cmdName := os.Args[1]

	cfg, err := config.Load()
	must("config", err)

	db, err := postgres.Open(cfg.DSN)
	must("db", err)
	defer db.Close()

	switch cmdName {
	case "migrate":
		runMigrate(db, os.Args[2:])
	case "serve":
		runServe(cfg, db)
	default:
		log.Fatalf("unknown cmd: %s", cmdName)
	}
}

func runServe(cfg *config.Config, db *bun.DB) {
	ctx := context.Background()
	clk := clock.System{}
	idg := idgen.NanoID{}
	aes, err := adcrypto.NewAESGCM(cfg.MasterKey)
	must("aes", err)

	jobsRepo := postgres.NewJobsRepo(db)
	rawSecrets := postgres.NewSecretsRepo(db)
	secrets := encryptingSecrets{inner: rawSecrets, aes: aes}
	apiKeys := postgres.NewAPIKeysRepo(db)
	usersBootstrap := postgres.NewUsersBootstrapRepo(db)

	runner := docker.New(docker.Config{Image: cfg.RunnerImage, CPUs: "2", MemoryMB: 2048})
	hooks := webhook.New(&http.Client{Timeout: 10 * time.Second})

	identitiesRepo := postgres.NewIdentitiesRepo(db)
	installationsRepo := postgres.NewInstallationsRepo(db)
	sessionsRepo := postgres.NewSessionsRepo(db)
	sessions := auth.NewSessions(sessionsRepo)

	appClient := ghapp.NewAppClient(ghapp.AppCreds{
		AppID:         cfg.GHAppID,
		PrivateKeyPEM: cfg.GHAppPrivateKey,
	})

	oauth := auth.NewOAuth(
		auth.OAuthConfig{
			ClientID:     cfg.GHClientID,
			ClientSecret: cfg.GHClientSecret,
			RedirectURL:  cfg.GHOAuthRedirectURL,
		},
		sessions, identitiesRepo, usersBootstrap, idg, clk, nil,
	)
	installHandler := ghapp.NewInstallHandler(cfg.GHAppSlug, sessions, installationsRepo, appClient)
	webhookHandler := ghapp.NewWebhookHandler(cfg.GHWebhookSecret, installationsRepo)

	repoCreds := ghapp.NewRepoCredsProvider(appClient, installationsRepo)
	anthCreds := credentials.NewAnthropicCredsProvider(&secrets)
	resolver := auth.NewResolver(sessions, apiKeys)

	enqueue := &usecase.EnqueueJob{
		Jobs:                 jobsRepo,
		RepoCreds:            repoCreds,
		AnthropicCreds:       anthCreds,
		Runner:               runner,
		IDGen:                idg,
		Clock:                clk,
		MaxConcurrentPerUser: cfg.MaxConcurrentPerUser,
		MaxConcurrentGlobal:  cfg.MaxConcurrentGlobal,
	}
	getJob := &usecase.GetJob{Jobs: jobsRepo}
	listJobs := &usecase.ListJobs{Jobs: jobsRepo}
	complete := &usecase.HandleRunnerCompletion{Jobs: jobsRepo, Clock: clk}
	reattach := &usecase.ReattachRunningJobs{Jobs: jobsRepo, Runner: runner, Clock: clk}
	mint := &usecase.MintAPIKey{Keys: apiKeys, IDGen: idg, Clock: clk}
	revoke := &usecase.RevokeAPIKey{Keys: apiKeys}
	setAnth := &usecase.SetAnthropicCredentials{Secrets: &secrets}
	_ = &usecase.DispatchCompletionWebhook{Dispatcher: hooks}

	enqueue.OnComplete = func(res ports.RunnerResult) { _ = complete.Execute(ctx, res) }
	_ = reattach.Execute(ctx)

	jobsHandler := adhttp.NewJobsHandler(enqueue, getJob, listJobs)
	settingsHandler := adhttp.NewSettingsHandler(setAnth, mint, revoke)
	statusPage := adhttp.NewStatusPage(getJob)
	dashHandler := adhttp.NewDashboardHandler(listJobs, apiKeys, &secrets, resolver)

	router := adhttp.NewRouter(adhttp.Deps{
		Resolver:        resolver,
		JobsHandler:     jobsHandler,
		SettingsHandler: settingsHandler,
		StatusPage:      statusPage,
		Dashboard:       dashHandler,
		Routes:          routeMounter{oauth: oauth, install: installHandler, webhook: webhookHandler},
	})

	log.Printf("listening on http://%s", cfg.HTTPBind)
	if err := http.ListenAndServe(cfg.HTTPBind, router); err != nil {
		log.Fatal(err)
	}
}

// routeMounter mounts auth + GitHub-App routes. Living in the composition root
// keeps the http adapter from importing the auth/ghapp adapters.
type routeMounter struct {
	oauth   *auth.OAuth
	install *ghapp.InstallHandler
	webhook *ghapp.WebhookHandler
}

func (m routeMounter) RegisterRoutes(r chi.Router) {
	r.Get("/login", m.oauth.Login)
	r.Get("/auth/github/callback", m.oauth.Callback)
	r.Get("/auth/github-app/install", m.install.Install)
	r.Get("/auth/github-app/callback", m.install.Callback)
	r.Post("/webhooks/github", m.webhook.Handle)
}

func runMigrate(db *bun.DB, args []string) {
	cmd := "up"
	if len(args) > 0 {
		cmd = args[0]
	}
	ctx := context.Background()
	m := migrate.NewMigrator(db, pgmig.Migrations)
	switch cmd {
	case "init":
		must("init", m.Init(ctx))
		fmt.Println("migration tables initialized")
	case "up":
		must("init", m.Init(ctx))
		group, err := m.Migrate(ctx)
		must("migrate", err)
		if group.IsZero() {
			fmt.Println("no new migrations")
		} else {
			fmt.Printf("applied: %s\n", group)
		}
	case "down":
		group, err := m.Rollback(ctx)
		must("rollback", err)
		fmt.Printf("rolled back: %s\n", group)
	case "status":
		ms, err := m.MigrationsWithStatus(ctx)
		must("status", err)
		for _, mm := range ms {
			fmt.Printf("%s  applied=%v\n", mm.Name, !mm.MigratedAt.IsZero())
		}
	default:
		log.Fatalf("unknown migrate cmd %q", cmd)
	}
}

func must(what string, err error) {
	if err != nil {
		log.Fatalf("%s: %v", what, err)
	}
}

// encryptingSecrets wraps SecretsRepository with AES-GCM at the composition
// seam: the Postgres impl stores bytes; this wrapper encrypts on write and
// decrypts on read.
type encryptingSecrets struct {
	inner ports.SecretsRepository
	aes   *adcrypto.AESGCM
}

var _ ports.SecretsRepository = (*encryptingSecrets)(nil)

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

- [ ] **Step 10: Clean up selfhost-mention doc comments**

Edit these comments (cosmetic; removes selfhost framing):

`core/domain/credentials.go:7` — old `// In selfhost mode, this wraps a long-lived PAT (ExpiresAt zero).` → new `// May wrap a short-lived installation token (ExpiresAt set) or a long-lived token (ExpiresAt zero).`

`core/domain/user.go:7` — old `// generated at signup. In selfhost, there is exactly one user with a fixed ID.` → new `// generated at signup.`

`core/usecase/ports/repo_creds_provider.go:11` — old `// Edition-specific: selfhost returns the admin's PAT, SaaS mints a fresh` → new `// Mints fresh short-lived git credentials for a user+repo via the GitHub App.` (adjust the following comment line if it continues the sentence; keep it coherent).

`core/adapter/postgres/users_repo.go:13` — old `// UsersBootstrapRepo provides the tiny user-row upsert that selfhost needs.` → new `// UsersBootstrapRepo provides the tiny user-row upsert used at signup.`

- [ ] **Step 11: Delete the old trees**

```bash
git rm -r core/runtime \
          core/adapter/postgres/selfhost \
          saas \
          cmd/agentic-delegator-saas \
          cmd/agentic-delegator/migrate \
          deploy/selfhost
```

- [ ] **Step 12: Update `.go-arch-lint.yml` — remove dead components/deps**

Remove these `components:` entries: `adapter_postgres_selfhost`, `runtime`, `runtime_selfhost`, `saas_root`, `saas_signup`, `saas_ghapp`, `saas_tenancy`, `saas_store`, `saas_presenter`.

Remove the matching `deps:` blocks for all of the above.

Keep the `adapter_credentials`, `adapter_ghapp`, `adapter_auth` components/deps added in Task 1. Confirm `adapter_http`'s `deps` remain `mayDependOn: [domain, ports, usecase, adapter_http_gen, presenter]` with `deepScan: false` (the `routeMounter` lives in `cmd`, so the http adapter still does not import auth/ghapp).

- [ ] **Step 13: Update `Makefile`**

Old:
```make
.PHONY: build build-saas test test-race lint arch-check generate dev migrate migrate-saas dev-db-up dev-db-down test-integration clean css tailwindcss-install
```
New:
```make
.PHONY: build test test-race lint arch-check generate dev migrate dev-db-up dev-db-down test-integration clean css tailwindcss-install
```

Old:
```make
build:
	$(GO) build -o bin/agentic-delegator ./cmd/agentic-delegator

build-saas:
	$(GO) build -tags=saas -o bin/agentic-delegator-saas ./cmd/agentic-delegator-saas
```
New:
```make
build:
	$(GO) build -o bin/agentic-delegator ./cmd/agentic-delegator
```

Old:
```make
migrate:
	go run ./cmd/agentic-delegator/migrate up

migrate-saas:
	@echo "Plan 04 will wire SaaS-only migrations here."
```
New:
```make
migrate:
	go run ./cmd/agentic-delegator migrate up
```

- [ ] **Step 14: Verify everything**

Run:
```bash
go build ./... && go test ./... && make arch-check
```
Expected: PASS. A single binary builds; the full suite (now including the relocated `ghapp`/`auth` tests, which no longer need a build tag) runs; arch-check passes against the trimmed rules.

Then confirm no stray references remain:
```bash
grep -rin "selfhost\|//go:build saas\|EditionRouteMounter\|core/runtime" --include="*.go" . ; echo "exit: $?"
```
Expected: no matches (grep exit 1). If anything prints, fix it and re-run Step 14.

- [ ] **Step 15: Commit**

```bash
git add -A
git commit -m "refactor: collapse to single SaaS binary; remove Edition + selfhost"
```

---

## Task 3: Docs, README, skill

Update prose to SaaS-only. No code.

**Files:** `README.md`, `docs/saas-setup.md`, `docs/end-to-end-smoke.md`, `skill/delegate.md`, `docs/design/2026-05-21-mvp-design.md`

- [ ] **Step 1: Rewrite `README.md`**

Replace the editions table and selfhost quickstart with a SaaS-only description. Concretely:
- Change the tagline line 3 from `Self-hostable + SaaS background coding agent...` to `SaaS background coding agent that delegates implementation tasks to Claude Code.`
- Delete the entire `## Editions` section (the table + the "Same codebase / SaaS-specific code in saas/" paragraph).
- In `## Architecture`, replace the `core/runtime/` and `saas/` bullets with the new layout: `core/adapter/ghapp` (GitHub App), `core/adapter/http/auth` (OAuth + sessions + resolver), `core/adapter/credentials` (Anthropic creds), and note `cmd/agentic-delegator` is the single composition root.
- Delete the `## Quickstart — selfhost` section.
- Rename `## Quickstart — SaaS` to `## Quickstart` and ensure it points to `docs/saas-setup.md`.
- In `## Development`, replace the `make test-integration` note if needed and drop any `-tags=saas` mentions (there are none in the current Development block, but verify).

- [ ] **Step 2: Make `docs/saas-setup.md` the canonical setup doc**

Read it and remove any framing that positions SaaS as one of two editions; it is now *the* product. Ensure the env vars it documents match `config.go`: `AGENTIC_GH_APP_ID`, `AGENTIC_GH_APP_PRIVATE_KEY`, `AGENTIC_GH_APP_SLUG`, `AGENTIC_GH_CLIENT_ID`, `AGENTIC_GH_CLIENT_SECRET`, `AGENTIC_GH_OAUTH_REDIRECT_URL`, `AGENTIC_GH_WEBHOOK_SECRET`, `AGENTIC_MASTER_KEY`, `DELEGATOR_DSN`. Update the run commands to `go run ./cmd/agentic-delegator migrate up` and `go run ./cmd/agentic-delegator serve` (or the built `bin/agentic-delegator`).

- [ ] **Step 3: Update `docs/end-to-end-smoke.md`**

Replace any selfhost flow (admin setup, `init`/`reset-key`, admin API key) with the SaaS flow: sign in with GitHub at `/login`, install the GitHub App, set the Anthropic key in `/settings`, mint a per-user API key, install the skill, run `/delegate`. Replace any `-tags=saas` / `agentic-delegator-saas` references with the single binary.

- [ ] **Step 4: Update `skill/delegate.md`**

Update only the auth/setup instructions: the skill authenticates with a per-user API key obtained from `/settings` after GitHub sign-in (no admin key, no `/admin/setup`). The `/delegate` request contract (`{repo, branch, spec.md}` POST) is unchanged — do not alter it. Verify `AGENTIC_DELEGATOR_URL` / `AGENTIC_DELEGATOR_API_KEY` env usage still matches.

- [ ] **Step 5: Add a collapse addendum to the design doc**

Append a short section to `docs/design/2026-05-21-mvp-design.md`:

```markdown
## Addendum (2026-06-04): SaaS-only collapse

The dual-edition model (selfhost OSS + SaaS) was dropped. The product is now
SaaS-only: one binary (`cmd/agentic-delegator`), no build tags, and no
`runtime.Edition` port. The former `saas/` packages moved into the adapter
layer (`core/adapter/ghapp`, `core/adapter/http/auth`, `core/adapter/postgres`)
and the secrets-backed Anthropic provider moved to `core/adapter/credentials`.
Migrations were consolidated into a single initial migration. See
`docs/superpowers/specs/2026-06-03-saas-only-collapse-design.md`.
```

- [ ] **Step 6: Verify no selfhost references remain in docs/skill**

Run:
```bash
grep -rin "self-host\|selfhost\|two editions\|-tags=saas\|agentic-delegator-saas\|admin/setup" README.md docs skill ; echo "exit: $?"
```
Expected: only the historical mention inside the design-doc body (pre-addendum context) may remain; anything in README/saas-setup/end-to-end-smoke/skill describing selfhost as a current option must be gone. Fix and re-run as needed.

- [ ] **Step 7: Commit**

```bash
git add README.md docs skill
git commit -m "docs: SaaS-only README, setup, smoke test, and skill"
```

---

## Self-Review notes

- **Spec coverage:** §1 edition machinery → Task 2 Steps 2–4,9,12; §2 selfhost auth/storage → Task 2 Steps 8,11; §3 relocations → Task 1; §4 single binary → Task 2 Step 9; §5 migrations → Task 2 Step 8; §6 config/docs/deploy/skill → Task 2 Steps 1,13 + Task 3; §7 verification → Task 2 Step 14, Task 1 Step 6.
- **Naming deviations** from the spec (`ghapp` vs `github`, store-into-`postgres`) are documented at the top with rationale; types/signatures used in `main.go` (`auth.NewOAuth`, `auth.NewResolver`, `auth.NewSessions`, `ghapp.NewAppClient`, `ghapp.AppCreds`, `ghapp.NewInstallHandler`, `ghapp.NewWebhookHandler`, `ghapp.NewRepoCredsProvider`, `postgres.New{Identities,Installations,Sessions}Repo`, `credentials.NewAnthropicCredsProvider`) match the relocated source verbatim.
- **Placeholder scan:** clean — Step 9 now contains a single complete `main.go`; no stubs or "TBD".
```
