# Agentic Delegator — MVP Design (Clean Architecture, core + SaaS module)

## Context

Agentic Delegator is a service that lets developers fire off background coding tasks from inside any Claude Code session. The user signs up (SaaS) or admin-bootstraps (self-host), connects a repo (GitHub App on SaaS, GitHub App or PAT on self-host), supplies Anthropic credentials, generates a personal API key, and installs a small Claude Code skill. From any Claude Code session they run `/delegate`; the skill collects `{repo, branch, spec.md}` and POSTs it to the service. The service spins up a fresh container per job, clones the repo with short-lived credentials, runs Claude Code headless with the spec as input, and Claude Code itself commits, pushes, and opens the PR. The dev gets a job link and walks away — all execution lives on the server.

**Why this exists.** Long-running agentic work today blocks the dev's terminal or laptop. Delegator decouples agent execution from the dev's session so multiple specs can run in parallel on dedicated hardware.

**Deployment model — single codebase, two editions (GitLab CE/EE pattern).** The 80% identical between SaaS and self-host lives in the open-source **core**. The 20% SaaS-specific (multi-tenancy, signup, GitHub App management) lives in a separate **SaaS module** that plugs into core through a narrow port interface. The SaaS module is gated by a Go build tag so the OSS build never compiles it in. When the team is ready to private the SaaS code, the `saas/` directory moves to a separate repo cleanly with no refactor — the dependency boundary already exists by design.

- **OSS binary (selfhost):** `go build ./cmd/agentic-delegator` — core only.
- **SaaS binary:** `go build -tags=saas ./cmd/agentic-delegator-saas` — core + SaaS module.

**MVP boundary.** Both binaries from one repo. Same job execution. Same dashboard skeleton. Claude Code + GitHub only. Anthropic API key only. No mid-run Q&A (Claude Code runs `--dangerously-skip-permissions` in a container). Minimal jobs list + status page. Webhook completion notification + `.agentic-delegator.yml` in. Cancellation, retry/resume, billing, runner pool are Phase 2+.

## Architectural principles — Clean Architecture + SOLID

**Clean Architecture: dependencies point inward.**

Four concentric layers (innermost ↔ outermost):

1. **Domain (entities)** — `core/domain/`. Pure Go types and rules. Depends on nothing — not on Bun, not on chi, not on Docker, not even on `context`-flavored repository interfaces. A `Job` knows what statuses are valid; it doesn't know how it's stored. The domain layer is the part you'd port to a different language if you ever did.
2. **Use cases (application)** — `core/usecase/`. Application-specific business rules (`EnqueueJob`, `GetJob`, `HandleRunnerCompletion`, `ReattachRunningJobs`). Each use case orchestrates domain entities and calls out via **ports** (interfaces) defined in `core/usecase/ports/`. Knows about `context.Context` but nothing else from the outer world.
3. **Interface adapters** — `core/adapter/`. Concrete implementations of the ports plus translators between the outer world and use cases. Inbound: HTTP handlers, webhook receivers, CLI. Outbound: Postgres+Bun repositories, Docker runner adapter, AES-GCM secrets adapter, webhook HTTP client, system clock. Presenters (templ HTML) live here too.
4. **Frameworks & drivers** — outermost. The `runner/` Docker image, third-party libraries (Bun, chi, templ, OpenAPI codegen output), the OS, `cmd/` composition roots. Replaceable in principle.

**The dependency rule:** source code in an inner layer must not import from an outer layer. `domain` can be compiled by itself. `usecase` imports `domain`. `adapter` imports `usecase` (to implement ports or to call them). `cmd` imports everything to wire it.

**SOLID applied:**

- **SRP** — each adapter file has one reason to change: `adapter/postgres/jobs_repo.go` only changes when the Postgres jobs schema or queries change. The HTTP handler for jobs only changes when the HTTP contract or input validation changes.
- **OCP** — use cases are closed to modification, open to extension via new adapters. A new edition, a new database, a new runner backend means new adapter files, not edits to use cases.
- **LSP** — every adapter implementing a port must be a true substitute. Test doubles (in-memory repos, fake runner) live next to the real adapters and pass the same conformance tests.
- **ISP** — ports are narrow. There is no "GodRepository." We have `JobsRepository`, `SecretsRepository`, `APIKeysRepo`, `RepoCredentialsProvider`, `AnthropicCredentialsProvider`, `RunnerService`, `WebhookDispatcher`, `Clock`. Use cases depend only on the ports they need.
- **DIP** — use cases depend on `ports.JobsRepository` (an interface), not on `bun.DB`. Composition roots in `cmd/` wire concrete adapters into use cases. Dependency direction is forced inward.

**The `Edition` is itself a port.** SaaS and selfhost are interchangeable implementations of one interface. Core never imports either; `cmd/agentic-delegator/main.go` picks `selfhost.New()`, `cmd/agentic-delegator-saas/main.go` picks `saas.New()`. This makes Clean Architecture and the build-tag-gated SaaS split the same architectural mechanism rather than two layered patterns.

## Tech stack (locked)

| Layer (Clean Arch) | Tool | Notes |
|---|---|---|
| Language | **Go** | Single static binary, good concurrency |
| Inbound adapter (HTTP) | **chi** | Tiny stdlib-compatible router |
| Outbound adapter (DB) | **Postgres** + **Bun** ORM | Postgres in both editions. Bun is SQL-first, fast, modest magic |
| Migrations | **Bun migrate** | Go-based migrations |
| Inbound adapter (HTML presenter) | **templ** | Type-safe templates, compile-time errors |
| Dashboard interactivity | **HTMX** | Server-rendered partials, HTMX swaps; no SPA |
| Inbound adapter (API contract) | **OpenAPI 3.1** + `oapi-codegen` | `api/openapi.yaml` is the source of truth; generates handler interfaces, types, Go client |
| Dev | **Air** | Hot reload |
| GitHub App (SaaS adapter) | `go-github` + `ghinstallation` | Standard combo |
| TLS | **Caddy** | Auto Let's Encrypt for SaaS |
| Runner | **Docker** (`claude` + `gh` + `git`) | One container per job, `--rm` |

## High-level architecture

### Codebase structure (Clean Architecture layout, module boundary preserved)

```
agentic-delegator/                                 # one git repo today
├── go.mod
├── api/openapi.yaml                               # outermost: contract for /api
│
├── cmd/                                           # composition roots (frameworks & drivers)
│   ├── agentic-delegator/main.go                  # OSS binary; wires selfhost edition
│   └── agentic-delegator-saas/main.go             # SaaS binary; //go:build saas; wires saas edition
│
├── core/                                          # ↓↓↓ OSS, always compiled ↓↓↓
│   ├── domain/                                    # LAYER 1 — entities, zero deps
│   │   ├── job.go                                 # Job, JobStatus (enum), JobID
│   │   ├── user.go                                # User, UserID
│   │   ├── credentials.go                         # GitCreds, AnthropicCreds (value objects)
│   │   ├── spec.go                                # SpecSource, SourceType
│   │   ├── api_key.go                             # APIKey, APIKeyHash (value objects)
│   │   └── errors.go                              # Domain errors (ErrNotFound, ErrConflict, ErrForbidden)
│   │
│   ├── usecase/                                   # LAYER 2 — application logic
│   │   ├── ports/                                 # interfaces (DIP)
│   │   │   ├── jobs_repo.go                       # JobsRepository
│   │   │   ├── secrets_repo.go                    # SecretsRepository
│   │   │   ├── api_keys_repo.go                   # APIKeysRepository
│   │   │   ├── runner_service.go                  # RunnerService
│   │   │   ├── repo_creds_provider.go             # RepoCredentialsProvider (selfhost/saas implement)
│   │   │   ├── anthropic_creds_provider.go        # AnthropicCredentialsProvider
│   │   │   ├── webhook_dispatcher.go              # WebhookDispatcher
│   │   │   ├── id_generator.go                    # IDGenerator (for job IDs, API keys)
│   │   │   └── clock.go                           # Clock
│   │   ├── enqueue_job.go                         # use case
│   │   ├── get_job.go
│   │   ├── list_jobs.go
│   │   ├── handle_runner_completion.go            # called by RunnerService on container exit
│   │   ├── reattach_running_jobs.go               # startup recovery use case
│   │   ├── mint_api_key.go
│   │   ├── revoke_api_key.go
│   │   ├── set_anthropic_credentials.go
│   │   └── dispatch_completion_webhook.go
│   │
│   ├── adapter/                                   # LAYER 3 — interface adapters
│   │   ├── http/                                  # inbound web
│   │   │   ├── router.go                          # chi router; mounts core routes; calls Edition.RegisterRoutes
│   │   │   ├── jobs_handler.go                    # uses EnqueueJob, GetJob, ListJobs
│   │   │   ├── settings_handler.go                # uses SetAnthropicCredentials, MintAPIKey, RevokeAPIKey
│   │   │   ├── status_page.go                     # renders templ pages (presenter)
│   │   │   ├── middleware/auth.go                 # bearer + session middleware, delegates to Edition.ResolveUser
│   │   │   └── gen/                               # oapi-codegen output: server.go, types.go, client.go
│   │   ├── postgres/                              # outbound DB
│   │   │   ├── db.go                              # *bun.DB wiring
│   │   │   ├── models.go                          # Bun-annotated row types
│   │   │   ├── jobs_repo.go                       # implements ports.JobsRepository
│   │   │   ├── secrets_repo.go                    # implements ports.SecretsRepository
│   │   │   ├── api_keys_repo.go                   # implements ports.APIKeysRepository
│   │   │   └── migrations/                        # core migrations (Bun migrate)
│   │   ├── docker/                                # outbound runner
│   │   │   └── runner.go                          # implements ports.RunnerService via docker CLI / SDK
│   │   ├── crypto/                                # outbound secrets
│   │   │   └── aesgcm.go                          # used by secrets repo to encrypt/decrypt
│   │   ├── webhook/                               # outbound webhook fan-out
│   │   │   └── http_webhook.go                    # implements ports.WebhookDispatcher
│   │   ├── idgen/                                 # outbound ID generation
│   │   │   └── nanoid.go                          # implements ports.IDGenerator
│   │   └── clock/
│   │       └── system_clock.go                    # implements ports.Clock
│   │
│   ├── runtime/                                   # Edition port + selfhost adapter
│   │   ├── edition.go                             # Edition interface (a port)
│   │   └── selfhost/
│   │       ├── edition.go                         # implements Edition
│   │       ├── repo_creds.go                      # implements ports.RepoCredentialsProvider (admin PAT)
│   │       ├── anthropic_creds.go                 # implements ports.AnthropicCredentialsProvider
│   │       └── admin_setup.go                     # /admin/setup route registration
│   │
│   ├── presenter/                                 # LAYER 3 (presenter side of adapters)
│   │   └── templ/                                 # templ files compiled to Go
│   │       ├── layouts/shell.templ
│   │       ├── pages/{landing,dashboard,status,settings}.templ
│   │       └── partials/{joblist,log_tail,onboarding}.templ
│   │
│   ├── config/                                    # adapter — loads env into typed config struct
│   │   └── config.go
│   │
│   └── testutil/                                  # in-memory adapter implementations for tests (LSP)
│       ├── fake_jobs_repo.go
│       ├── fake_runner_service.go
│       └── …
│
├── saas/                                          # ↓↓↓ SaaS module, build-tag gated ↓↓↓
│   ├── edition.go                                 # //go:build saas — implements core/runtime.Edition
│   ├── signup/
│   │   ├── github_oauth.go                        # /login, /auth/github/callback
│   │   └── sessions.go                            # cookie sessions
│   ├── ghapp/
│   │   ├── app_jwt.go                             # App JWT signing
│   │   ├── install.go                             # /auth/github-app/install, /auth/github-app/callback
│   │   ├── repo_creds.go                          # implements ports.RepoCredentialsProvider (installation tokens)
│   │   └── webhooks.go                            # /webhooks/github (HMAC verified)
│   ├── tenancy/
│   │   └── resolver.go                            # ResolveUser implementation; isolation guards
│   ├── store/                                     # SaaS-only outbound DB adapter
│   │   ├── models.go                              # Bun models: identities, installations, sessions
│   │   ├── identities_repo.go
│   │   ├── installations_repo.go
│   │   ├── sessions_repo.go
│   │   └── migrations/                            # SaaS-only migrations
│   └── presenter/templ/
│       └── partials/{signup_cta,app_install_banner}.templ
│
├── runner/                                        # frameworks & drivers — Docker image
│   ├── Dockerfile
│   └── entrypoint.sh
│
├── skill/delegate.md                              # the Claude Code skill (same for both editions)
│
├── deploy/
│   ├── selfhost/{agentic-delegator.service,docker-compose.postgres.yml,install.sh}
│   └── saas/{agentic-delegator-saas.service,docker-compose.postgres.yml,Caddyfile.example}
│
├── docker-compose.dev.yml
├── .air.toml
├── Makefile
├── README.md
└── LICENSE
```

When the SaaS module goes private later: `saas/` becomes a separate repo with its own `go.mod`; new repo `require`s the open-source core at a pinned version. The dependency direction is already correct — no refactor.

### Runtime architecture

```
       [ TLS via Caddy (SaaS) / optional in selfhost ]
                     |
         [ agentic-delegator(-saas) binary :8787 ]
              |                     |
   [ docker daemon on host ]    [ Postgres ]
              |
   [ runner container per job ]  ×N (per-user + global caps)
```

| Component | Self-host edition | SaaS edition |
|---|---|---|
| Delegator skill | Same `.md` | Same `.md` |
| HTTP service | `agentic-delegator` (core only) | `agentic-delegator-saas` (core + saas module) |
| User model | Single admin | GitHub OAuth signup, multi-user |
| Repo auth | PAT (or self-managed GH App) | Centrally registered GH App, fresh installation tokens per job |
| Storage | Postgres | Postgres |
| Runners | Docker on host | Same |
| Status page | `/jobs/{id}`, localhost or admin session | `/jobs/{id}`, session cookie, isolated by `user_id` |

### Pluggable port: `Edition`

`core/runtime/edition.go`:

```go
package runtime

// Edition is the port that selfhost and saas implement.
// Core never imports either implementation; cmd/* wires the right one.
type Edition interface {
    Name() string  // "selfhost" | "saas"

    RegisterRoutes(r chi.Router)

    ResolveUser(r *http.Request) (domain.UserID, error)

    RepoCredentialsProvider() ports.RepoCredentialsProvider
    AnthropicCredentialsProvider() ports.AnthropicCredentialsProvider

    DashboardPartials(ctx context.Context, userID domain.UserID) []templ.Component
}
```

The Edition itself hands core the credential-provider ports — so use cases never know whether the credentials came from "admin PAT" or "minted installation token." They get a `domain.GitCreds`.

## MVP user journey

### SaaS edition

1. **Sign up.** `https://<your-domain>` → "Sign in with GitHub." OAuth → `users` + `saas_github_identities` rows.
2. **Onboarding** (HTMX, partials swap as steps complete):
   - Install the GitHub App on chosen repos → `saas_github_installations`.
   - Paste Anthropic API key → `user_secrets` (AES-GCM encrypted).
   - Generate personal API key → shown once, bcrypt stored.
   - Install the skill → copy a one-liner; set env vars.
3. **Each invocation.** `/delegate` → skill collects `{repo, branch, spec}` → confirms → `POST /api/jobs` (bearer) → HTTP adapter calls `EnqueueJob` use case → use case validates + persists via `JobsRepository` port → returns job link.
4. **Server side.** `EnqueueJob` schedules the job; `RunnerService` adapter (Docker) spawns a container with creds from the credential providers (Edition-supplied); container clones, runs Claude Code, pushes, opens PR. On exit, the runner adapter calls `HandleRunnerCompletion` use case → updates job → dispatches webhook.

### Self-host edition

1. **Operator installs** OSS binary + Postgres.
2. `agentic-delegator init` creates the admin user + admin key. Operator visits `/admin/setup` (route mounted only by selfhost edition); pastes PAT + Anthropic key; mints skill API key.
3. **Each invocation.** Same `/delegate` flow. Selfhost edition's `RepoCredentialsProvider` returns the admin's PAT; everything else identical.

## Component boundaries (in CA terms)

**Delegator skill** — outermost driver. Bash + curl. Knows only the HTTP contract (`POST /api/jobs`).

**HTTP inbound adapter** (`core/adapter/http`) — translates HTTP → use case calls. The OpenAPI-generated handler interface lives here; implementations call use cases and present templ pages or JSON.

**Use cases** (`core/usecase`) — single-responsibility application logic. Examples:
- `EnqueueJob`: validate input, fetch credentials via providers, persist a queued `Job`, ask `RunnerService` to start it if under cap.
- `HandleRunnerCompletion`: parse exit code + `/workspace/.pr-url`, update `Job` status, dispatch webhook.
- `ReattachRunningJobs` (startup): for each `running` job, ask `RunnerService.Inspect`; if alive, attach log tailing; else mark `failed`.

**Domain entities** (`core/domain`) — `Job` knows its valid statuses, what counts as terminal, how to transition. `APIKey` knows its prefix layout. Pure, testable.

**Outbound adapters** (`core/adapter/postgres`, `core/adapter/docker`, etc.) — each implements one port. New persistence backend = new adapter file. Tests against use cases use the in-memory fakes in `core/testutil`.

**SaaS module** (`saas/*`) — implements the `Edition` port and supplies its own credential-provider implementations. Registers extra HTTP routes. Has its own outbound adapter for SaaS-only tables.

## Data model

**Domain entities** (`core/domain/`, pure):
- `Job{ID, UserID, Status, Repo, BaseBranch, WorkBranch, SpecSource, SourceType, ModelOverride, ContainerID, PRURL, Error, LogPath, CreatedAt, StartedAt, FinishedAt}` with `JobStatus` enum and transition methods (`MarkRunning`, `MarkSucceeded(prURL)`, `MarkFailed(reason)`).
- `User{ID, DisplayName, CreatedAt}`.
- `APIKey{ID, UserID, Name, KeyPrefix, KeyHash, LastUsedAt, CreatedAt}`.
- `GitCreds{Token, ExpiresAt}` value object.

**Postgres adapter models** (`core/adapter/postgres/models.go`, Bun-annotated):
```go
type jobRow struct {
    bun.BaseModel  `bun:"table:jobs"`
    ID             string    `bun:"id,pk"`
    UserID         string    `bun:"user_id,type:uuid,notnull"`
    Status         string    `bun:"status,notnull"`
    Repo           string    `bun:"repo,notnull"`
    BaseBranch     string    `bun:"base_branch,notnull"`
    WorkBranch     string    `bun:"work_branch,notnull"`
    SpecSource     string    `bun:"spec_source,notnull"`
    SourceType     string    `bun:"source_type,notnull"`
    ModelOverride  string    `bun:"model_override"`
    ContainerID    string    `bun:"container_id"`
    PRURL          string    `bun:"pr_url"`
    Error          string    `bun:"error"`
    LogPath        string    `bun:"log_path,notnull"`
    CreatedAt      time.Time `bun:"created_at,notnull,default:now()"`
    StartedAt      *time.Time `bun:"started_at"`
    FinishedAt     *time.Time `bun:"finished_at"`
}
```
The Postgres adapter translates `jobRow ↔ domain.Job` so the domain stays free of Bun tags.

**SaaS-only adapter models** (`saas/store/models.go`, `//go:build saas`): identities, installations, sessions.

Logs always on filesystem (`Job.LogPath`), never in DB.

## API contract (OpenAPI 3.1)

`api/openapi.yaml` is the source of truth for `/api`. `oapi-codegen` produces:
- handler interface (implemented by `core/adapter/http`)
- request/response types
- a Go client used in tests and a future-CLI

| Method | Path | Auth | Edition |
|---|---|---|---|
| `POST` | `/api/jobs` | bearer | core |
| `GET` | `/api/jobs/{id}` | bearer | core |
| `GET` | `/api/jobs` | bearer | core |
| `GET` | `/dashboard` | Edition.ResolveUser | core |
| `GET` | `/jobs/{id}` | Edition.ResolveUser | core |
| `GET` | `/jobs/{id}/log` | Edition.ResolveUser | core (HTMX partial) |
| `POST` | `/settings/anthropic` | Edition.ResolveUser | core |
| `POST` | `/settings/api-keys` | Edition.ResolveUser | core |
| `DELETE` | `/settings/api-keys/{id}` | Edition.ResolveUser | core |
| `GET` | `/admin/setup` | bootstrap | selfhost only |
| `POST` | `/admin/pat` | admin session | selfhost only |
| `GET` | `/` | none | saas only |
| `GET` | `/login` | none | saas only |
| `GET` | `/auth/github/callback` | none | saas only |
| `GET` | `/auth/github-app/install` | session | saas only |
| `GET` | `/auth/github-app/callback` | session | saas only |
| `POST` | `/webhooks/github` | HMAC | saas only |

`POST /api/jobs` payload:
```json
{ "repo": "owner/name", "base_branch": "main", "work_branch": "agentic/auth-9q2k", "spec_source": "specs/auth.md", "source_type": "path", "model_override": "claude-opus-4-7" }
```
Response: `{ "job_id": "j_8x2K9q", "status_url": "https://<host>/jobs/j_8x2K9q" }`.

## Per-repo config

`.agentic-delegator.yml` at the target repo root:
```yaml
model: claude-opus-4-7
max_turns: 50
system_prompt_append: |
  Use go modules. Run `go test ./...` before declaring done.
allowed_tools: ["Bash", "Edit", "Read", "Write", "Grep"]
notification_webhook: https://hooks.slack.com/...
```

## Security & secrets

- **At rest:** Anthropic key, GH App private key (SaaS), selfhost PAT → AES-GCM via the `crypto/aesgcm` adapter. Master key from `AGENTIC_MASTER_KEY` env at boot (host secret manager or `systemd LoadCredential=`).
- **In transit:** TLS at Caddy.
- **API keys (personal):** bcrypt-hashed, shown once, prefix kept plaintext for UI.
- **Tenant isolation (SaaS):** all repo queries go through use cases that take `UserID` resolved by `Edition.ResolveUser`; the Postgres adapter scopes every query by `user_id`. Repo + installation pairing re-validated per request.
- **GH App tokens (SaaS):** fresh installation token per job, 1-hour TTL, never persisted.
- **Webhook signatures:** HMAC-verified before any state change.
- **Runner isolation:** `--rm`, non-privileged, `--cpus=2 --memory=2g`. Egress restrictions deferred to Phase 2.
- **Secrets in runner:** env vars at spawn time; destroyed with the container.

## Caveats + known unknowns

- **Anthropic OAuth feasibility** — defer to Phase 2 after a spike.
- **PR URL detection** — runner writes `/workspace/.pr-url`; service reads after exit. Log-tail regex as fallback.
- **Spec source classification** — `http(s)://` → URL; `\.md` path pattern → path; else inline.
- **Service crash recovery** — `ReattachRunningJobs` use case at startup.
- **Webhook payload** — `{"event":"job.completed","job":{…},"log_tail":"…"}`. No retry in MVP.
- **GH App registration (SaaS)** — one-time setup; credentials in env at boot.
- **Module split timing** — when SaaS goes private, `saas/` becomes its own repo. Already module-clean.

## Deployment

### Self-host

```bash
sudo apt install docker.io
docker compose -f /etc/agentic-delegator/docker-compose.postgres.yml up -d
curl -fsSL https://<releases>/agentic-delegator | sudo tar -xz -C /usr/local/bin
agentic-delegator migrate
agentic-delegator init
sudo cp deploy/selfhost/agentic-delegator.service /etc/systemd/system/
sudo systemctl enable --now agentic-delegator
```

### SaaS

```bash
sudo apt install docker.io caddy
docker compose -f /etc/agentic-delegator/docker-compose.postgres.yml up -d
# Register GH App on github.com; put creds + master key in /etc/agentic-delegator.env
curl -fsSL https://<releases>/agentic-delegator-saas | sudo tar -xz -C /usr/local/bin
agentic-delegator-saas migrate
sudo cp deploy/saas/agentic-delegator-saas.service /etc/systemd/system/
sudo systemctl enable --now agentic-delegator-saas
echo "<your-domain> { reverse_proxy 127.0.0.1:8787 }" | sudo tee /etc/caddy/Caddyfile
sudo systemctl restart caddy
```

### Local dev

```bash
docker compose -f docker-compose.dev.yml up -d
make generate     # templ generate + oapi-codegen
make dev          # Air watches and rebuilds
```

## Phasing — what's IN MVP, what's OUT

**MVP (this design):**
- Both binaries from one repo (core + saas, build-tag gated)
- Clean-Architecture layered codebase (domain / usecase / adapter / cmd) from day 1
- SaaS: GitHub OAuth signup, GH App, per-user secrets, personal API keys, jobs + status
- Selfhost: admin bootstrap, PAT, same job flow
- Webhook completion + `.agentic-delegator.yml`
- Per-user + global concurrency caps
- Postgres + Bun + chi + templ + HTMX + OpenAPI + Air

**Phase 2:**
- Job cancellation (new use case + UI)
- Better dashboard
- Anthropic OAuth after spike
- Container resource limits + egress restrictions + audit log
- Slack/email notification adapters
- SaaS module split to private repo

**Phase 3:**
- Mid-run Q&A channel (new use case + dashboard partial)
- Retry / resume (new use case)
- Billing (SaaS-only adapter + use cases)
- Org-level accounts

**Phase 4:**
- New runner adapters (Codex, others — same `RunnerService` port)
- GitLab / Bitbucket adapters (new `RepoCredentialsProvider` impls)
- K8s deployment shape

## Verification — how to test the MVP end-to-end

**Architectural checks (compile + lint time):**
1. `go build ./cmd/agentic-delegator` (no `-tags`) succeeds; `go list -deps ./cmd/agentic-delegator | grep -E '/saas/'` returns empty.
2. `go build -tags=saas ./cmd/agentic-delegator-saas` succeeds.
3. **Dependency-rule lint.** A `make arch-check` step using e.g. `go-arch-lint` enforces: `domain` imports nothing inside the repo; `usecase` imports only `domain` + `usecase/ports`; `adapter/*` may import `usecase` and `domain` but not other `adapter/*` siblings (except composition wiring in `cmd/`).
4. `make generate` is idempotent.

**Use-case-level tests (fast, no Docker, no DB):**
5. `core/usecase` tests use `core/testutil` in-memory fake adapters. `EnqueueJob`, `HandleRunnerCompletion`, `ReattachRunningJobs`, etc., all have unit tests with fakes.

**Adapter-level tests:**
6. `core/adapter/postgres` tests run against a throwaway Postgres container (one shared per test package), assert query correctness + migration shape.
7. `core/adapter/docker` tests run against a real Docker daemon, spinning a no-op image to validate spawn/log-tail/exit-detection.

**SaaS smoke (against a staging deploy):**
8. Staging VPS up, GH App registered, Caddy + Postgres healthy.
9. Sign in with GitHub. Install GH App on a sandbox repo. Paste Anthropic key. Generate API key. Install skill on dev laptop.
10. `/delegate` against the sandbox repo with `specs/hello.md`. Confirm job_id + status URL.
11. Open status URL: metadata + HTMX-polled log tail + PR link.
12. Verify PR contents.
13. Failure path: impossible spec → `failed`, no PR.
14. Concurrency: two parallel jobs respect caps.
15. Tenant isolation: account 2 cannot read account 1's `/api/jobs/{id}`.
16. Webhook: configured `notification_webhook` fires on completion.

**Selfhost smoke (separate machine):**
17. `docker compose up -d postgres` → `agentic-delegator migrate` → `agentic-delegator init` → `/admin/setup` → paste PAT + Anthropic key → mint skill API key. `/delegate` flow → PR opened. Same outcome.

## Critical files (for the implementation plan)

To be created in `/Users/akef/workspace/agentic-delegator`:

**Repo root + tooling:**
- `go.mod`, `go.sum`
- `Makefile` — `generate`, `build`, `build-saas`, `test`, `arch-check`, `dev`, `migrate`, `migrate-saas`, `lint`
- `.air.toml`, `docker-compose.dev.yml`, `api/openapi.yaml`
- `arch-lint.yml` (or equivalent) — enforces the Clean Architecture dependency rule
- `README.md`, `LICENSE`

**Composition roots:**
- `cmd/agentic-delegator/main.go` — wires `selfhost.New()` + concrete adapters into use cases
- `cmd/agentic-delegator-saas/main.go` — `//go:build saas`; wires `saas.New()` instead

**Core — domain:**
- `core/domain/{job,user,credentials,spec,api_key,errors}.go`

**Core — use cases + ports:**
- `core/usecase/ports/{jobs_repo,secrets_repo,api_keys_repo,runner_service,repo_creds_provider,anthropic_creds_provider,webhook_dispatcher,id_generator,clock}.go`
- `core/usecase/{enqueue_job,get_job,list_jobs,handle_runner_completion,reattach_running_jobs,mint_api_key,revoke_api_key,set_anthropic_credentials,dispatch_completion_webhook}.go`

**Core — adapters:**
- `core/adapter/http/{router,jobs_handler,settings_handler,status_page}.go` + `middleware/auth.go` + `gen/{server,types,client}.go`
- `core/adapter/postgres/{db,models,jobs_repo,secrets_repo,api_keys_repo}.go` + `migrations/`
- `core/adapter/docker/runner.go`
- `core/adapter/crypto/aesgcm.go`
- `core/adapter/webhook/http_webhook.go`
- `core/adapter/idgen/nanoid.go`
- `core/adapter/clock/system_clock.go`

**Core — presenter + edition + config + tests:**
- `core/presenter/templ/{layouts,pages,partials}/*.templ`
- `core/runtime/edition.go` + `core/runtime/selfhost/{edition,repo_creds,anthropic_creds,admin_setup}.go`
- `core/config/config.go`
- `core/testutil/{fake_jobs_repo,fake_runner_service,…}.go`

**SaaS module (build tag `//go:build saas`):**
- `saas/edition.go`
- `saas/signup/{github_oauth,sessions}.go`
- `saas/ghapp/{app_jwt,install,repo_creds,webhooks}.go`
- `saas/tenancy/resolver.go`
- `saas/store/{models,identities_repo,installations_repo,sessions_repo}.go` + `migrations/`
- `saas/presenter/templ/partials/*.templ`

**Runner + skill + deploy:**
- `runner/Dockerfile`, `runner/entrypoint.sh`
- `skill/delegate.md`
- `deploy/selfhost/*`, `deploy/saas/*`

**Key dependencies (`go.mod`):**
- `github.com/go-chi/chi/v5`
- `github.com/uptrace/bun` + `bun/dialect/pgdialect` + `bun/driver/pgdriver` + `bun/migrate`
- `github.com/a-h/templ`
- `github.com/oapi-codegen/oapi-codegen/v2`
- `github.com/google/go-github/v60` + `github.com/bradleyfalzon/ghinstallation/v2` (saas only)
- `golang.org/x/crypto/bcrypt`
- `github.com/fgrosse/go-arch-lint` (or `github.com/loov/dotc` / hand-rolled go-list checker) for architectural enforcement
