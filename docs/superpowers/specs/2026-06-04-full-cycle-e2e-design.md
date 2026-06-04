# Full-cycle end-to-end test — design

Date: 2026-06-04

## Goal

One automated test suite that drives the entire delegation workflow through the
real HTTP router and real use cases, faking only the external world (GitHub,
Docker, Anthropic). It runs under plain `go test ./...` with no Docker, Postgres,
or credentials, and proves the whole cycle works as a unit — not just the parts
the existing unit tests already cover in isolation.

## Why

The repo has thorough per-unit tests (domain, usecases, adapters) and Postgres/
Docker integration tests behind the `integration` build tag. The only thing
exercising the *whole* cycle is the manual smoke test in
`docs/end-to-end-smoke.md`. Nothing automated verifies that login → install →
set key → mint → enqueue → runner completes → status/PR observed actually wires
together. This suite fills that gap.

## Fidelity

In-process, all external boundaries faked:

- repos: existing `core/testutil` in-memory fakes
- runner: `testutil.FakeRunnerService` (its `Complete` driver simulates a
  container exit deterministically)
- clock / ids: `testutil.FakeClock`, `testutil.FakeIDGenerator`
- GitHub: a fake `http.RoundTripper` injected into the OAuth client (and the
  GitHub-App client where a call must be faked); where a GitHub interaction
  cannot be cleanly faked through the transport, the harness seeds the relevant
  repo directly and documents why

## Location

New package `test/e2e/`, containing **only** `*_test.go` files:

- arch-lint excludes `^.*_test\.go$`, so the harness may import every adapter
  without violating the Clean Architecture dependency rule
- `go test ./...` picks it up automatically; no build tag

## Components

### `harness_test.go`

Assembles the real `chi` router the same way `cmd/agentic-delegator/main.go`'s
`runServe` does, but with the fakes above and a fake GitHub transport, wrapped in
an `httptest.Server`. A header comment points back to `runServe` to flag drift.

Ergonomic helpers used by the scenario tests:

- `login()` → completes the OAuth dance, returns the session cookie
- `installApp()` → completes the GitHub-App install, records the installation
- `setAnthropicKey(key)` → `POST /settings/anthropic` with the session
- `mintAPIKey()` → `POST /settings/api-keys`, returns the plaintext key
- `enqueue(key, body)` → `POST /api/jobs` with a bearer key
- `completeRunner(exitCode, prURL)` → drives `FakeRunnerService.Complete`
- `getJob(key, id)`, `listJobs(key)` → bearer reads
- `statusPage(id)` → renders the HTML status page

### `fullcycle_test.go` — happy path

End-to-end, asserted at each hop:

1. `GET /login` → 302 to GitHub; `GET /auth/github/callback?code=…` → session
   cookie set, user created.
2. `GET /auth/github-app/install` → install; `…/callback` → installation
   recorded.
3. `POST /settings/anthropic` (session) → key stored;
   `POST /settings/api-keys` → plaintext key returned.
4. `POST /api/jobs` (bearer) → job `running`; runner started with the expected
   `RunnerStartSpec` (repo, branches, spec, git + anthropic creds).
5. `completeRunner(0, prURL)` → `OnComplete` → `HandleRunnerCompletion` → job
   `succeeded` with the PR URL.
6. `GET /api/jobs/{id}` (bearer) → `succeeded` + PR URL; `GET /api/jobs` lists
   it; status page renders the PR link.

### `failure_test.go` — key failure modes

- `POST /api/jobs` with no / invalid auth → 401.
- Enqueue with missing Anthropic creds → error surfaced; job not left `running`.
- Concurrency cap (`MaxConcurrentPerUser = 1`) → 2nd job stays `queued`, runner
  not started for it.
- Runner completes with non-zero exit → job `failed` with the reason.

## Method

Built test-first: each helper and assertion is written against the real
handlers, watched fail, then made to pass. The suite must be green under
`go test ./...`, `go vet ./...`, and `make arch-check`.

## Out of scope

- Every-endpoint error matrix, all auth permutations
- Job-completion webhook (note: `DispatchCompletionWebhook` is constructed but
  not wired in `main.go` today; the GitHub-App webhook *is* wired but is not part
  of this cycle)
- Webhook retry/failure, reattach-on-restart
- Real Postgres SQL / migrations (covered by the `integration` suite)

## Acceptance

- `test/e2e` suite passes under `go test ./...`.
- Happy-path test traverses both the session and bearer auth paths in one run.
- All four failure-mode tests pass.
- `go vet ./...` and `make arch-check` stay green.
