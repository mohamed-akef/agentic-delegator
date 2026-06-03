# SaaS-Only Collapse — Design

**Date:** 2026-06-03
**Status:** Approved (brainstorming)
**Author:** Akef + Claude

## Context

`agentic-delegator` is a background coding-agent service: from any Claude Code
session a developer runs `/delegate`, hands it a spec, and the service clones
the repo on a sandboxed runner, runs Claude Code headless, and opens a PR.

It was built as a **single codebase with two editions** (GitLab CE/EE pattern):

- **selfhost (OSS)** — single admin API key, admin-setup bootstrap, GitHub PAT.
- **SaaS** — GitHub OAuth signup, GitHub App for repo access, per-user API keys,
  multi-tenancy.

The variability is expressed through a `runtime.Edition` port; the SaaS-specific
code lives in `saas/` behind a `//go:build saas` tag, with dual binaries and
dual deploy stacks.

## Decision

**Shift the product to SaaS-only.** Pure simplification — same features as
today's SaaS edition, no new product surface (no billing/plans/teams added).
Collapse all edition machinery into a single SaaS application.

Decisions taken during brainstorming:

1. **Scope:** Simplify/collapse only. No new SaaS capabilities.
2. **Boundary:** Full merge — delete the `Edition` port, merge `saas/` into
   `core/`, drop all `//go:build saas` tags, single binary, single deploy.
   Abandons the "extract SaaS to a private repo" option.
3. **Migrations:** Nothing is deployed. Rewrite the migration history into one
   clean initial migration in a single table.

## Goals

- One application, one binary, no build tags.
- Keep the Clean Architecture layering (domain → usecase → adapter → cmd) and
  the dependency rule intact.
- Preserve all current SaaS behaviour: GitHub OAuth login, GitHub App repo
  access, per-user Anthropic credentials, per-user API keys, jobs +
  status/dashboard UI, runner, webhook completion.
- Remove every selfhost-only concept: admin API key, admin-setup, PAT store,
  the editions abstraction.

## Non-Goals

- No billing, plans, teams/orgs, usage metering, or onboarding redesign.
- No change to job execution, the runner image, or the `/delegate` skill's
  request contract (only its auth/setup instructions change).
- No UI redesign beyond removing edition-conditional bits.

## Plan

### 1. Delete the edition machinery

The `Edition` port exists only to switch selfhost vs SaaS. With one target it is
pure indirection. Remove:

- `core/runtime/` in its entirety — the `Edition` port (`core/runtime/edition.go`)
  and the whole `core/runtime/selfhost/` implementation (`edition.go`,
  `admin_setup.go`, `repo_creds.go`). **Exception:** `anthropic_creds.go` is a
  generic secrets-backed provider already reused by SaaS — it is relocated, not
  deleted (see §3).
- In `core/adapter/http`: the `EditionRouteMounter` interface, `Deps.Edition`,
  and the `editionResolver` shim. Edition routes (`/login`,
  `/auth/github/callback`, `/auth/github-app/install`,
  `/auth/github-app/callback`, `/webhooks/github`) are registered directly in
  `NewRouter`.
- The `editionName string` parameter on `NewDashboardHandler` and its use in
  `core/presenter/templ/pages/landing.templ`. The app is always SaaS.

### 2. Delete selfhost-only auth & storage

- Single-admin API key / admin-setup / PAT flow:
  `core/adapter/postgres/selfhost/pat_store.go` and the
  `20260521000002_selfhost_admin_pat.go` migration.
- `cmd/agentic-delegator-saas/` and the **old** selfhost
  `cmd/agentic-delegator/main.go` (with its `init` / `reset-key` admin
  subcommands) and `cmd/agentic-delegator/migrate/` if redundant after the
  single-binary `migrate` subcommand lands.
- `deploy/selfhost/` (install.sh, systemd unit, Postgres compose).

**Kept** (this is the SaaS auth model): per-user API keys —
`core/domain/api_key.go`, `usecase.MintAPIKey` / `usecase.RevokeAPIKey`,
`core/adapter/postgres/api_keys_repo.go`.

### 3. Relocate shared & SaaS code into `core/`

Move the `saas/` packages and the one mislabeled "selfhost" provider into the
Clean Architecture layers. No build tags anywhere afterward.

| From | To | What it is |
|---|---|---|
| `core/runtime/selfhost/anthropic_creds.go` | `core/adapter/credentials/anthropic.go` | Generic secrets-backed Anthropic creds provider (already reused by SaaS) |
| `saas/ghapp/` | `core/adapter/github/` | GitHub App: app JWT, install handler, webhooks, repo-creds provider |
| `saas/signup/` + `saas/tenancy/` | `core/adapter/http/auth/` | GitHub OAuth login/callback, session store + middleware, the `UserResolver` (session cookie or bearer API key → UserID) |
| `saas/store/` | `core/adapter/postgres/` | identities, installations, sessions repos + models |
| `saas/store/migrations/` | folded into core migrations | see §5 |

Tests move with their code (`ghapp/*_test.go`, `signup/*_test.go`,
`tenancy/*_test.go`). After the moves, delete the now-empty `saas/` tree and
`core/runtime/`.

### 4. Single binary & wiring

One `cmd/agentic-delegator/main.go` (no build tag) with subcommands `serve` and
`migrate`. It is today's SaaS `main.go` minus the `editionResolver` shim: it
wires the GitHub App client, OAuth, sessions, the tenancy `Resolver` (now the
concrete `UserResolver`), and the use cases directly. The `encryptingSecrets`
decorator stays at this composition seam.

### 5. Consolidate migrations (clean rewrite)

Nothing is deployed, so collapse the three migrations (core initial + selfhost
admin PAT + saas initial) into **one** `20260603000001_initial.go` under
`core/adapter/postgres/migrations/`. Drop the selfhost `admin_pat` table and the
dual-table (`bun_saas_migrations`) hack. One migrator, one `bun_migrations`
table. The schema is the union of the current core + saas tables, minus
`admin_pat`.

### 6. Config, docs, deploy, skill

- **Config:** fold the GitHub App / OAuth env reads (currently raw `os.Getenv`
  in the saas `main.go`: `AGENTIC_GH_APP_ID`, `AGENTIC_GH_APP_PRIVATE_KEY`,
  `AGENTIC_GH_CLIENT_ID`, `AGENTIC_GH_CLIENT_SECRET`, `AGENTIC_GH_OAUTH_REDIRECT_URL`,
  `AGENTIC_GH_APP_SLUG`, `AGENTIC_GH_WEBHOOK_SECRET`) into `core/config.Config`
  so the single binary has one config surface.
- **README / docs:** drop the editions table and selfhost quickstart; SaaS is
  *the* product. Rewrite `docs/saas-setup.md` as the canonical setup doc, add a
  short addendum to `docs/design/2026-05-21-mvp-design.md` noting the collapse,
  update `docs/end-to-end-smoke.md`.
- **`skill/delegate.md`** + the skill quickstart: point at the SaaS
  signup/login flow (no admin key). The `/delegate` request contract is
  unchanged.
- **arch-lint** (`.go-arch-lint.yml` if present) + **Makefile:** drop the
  dual-binary / `-tags=saas` targets; update component boundaries for the new
  `core/adapter/github` and `core/adapter/http/auth` packages.

### 7. Verification

- `make test` and `make test-integration` green. Relocated packages keep their
  existing tests.
- `make arch-check` passes against the updated rules.
- `go build ./cmd/agentic-delegator` produces the single binary; a `-tags=saas`
  build no longer exists.

## Risks / Notes

- The relocation touches many imports across `cmd` and `adapter`; mechanical but
  broad. Done package-by-package with the compiler as the guide.
- arch-lint rules must be updated in lockstep with the moves or `arch-check`
  fails mid-way; treat the rule update as part of each relocation step.
- `landing.templ` and `dashboard_handler.go` require regenerating templ output
  (`make generate`) after removing the edition param.
