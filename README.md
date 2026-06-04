# agentic-delegator

Self-hostable + SaaS background coding agent that delegates implementation tasks to Claude Code.

## What it does

Invoke the `/delegate` skill from any Claude Code session, hand it a spec, and the service:

1. Clones your repo on a sandboxed runner container
2. Runs Claude Code on the spec
3. Commits, pushes, and opens a PR

You watch the status page; the agent does the work in the background.

## Editions

| Edition | Binary | Auth | Use case |
|---|---|---|---|
| **Selfhost (OSS)** | `agentic-delegator` | Single admin API key | Run on your own infra; everything works offline-ish |
| **SaaS** | `agentic-delegator-saas` (built with `-tags=saas`) | GitHub OAuth signup + per-user API keys | Multi-tenant deployment under your own domain |

Same codebase. SaaS-specific code is in `saas/` behind a `//go:build saas` tag — easily extracted to a private repo later.

## Architecture

Clean Architecture with strict layer boundaries enforced by [go-arch-lint](https://github.com/fe3dback/go-arch-lint):

- `core/domain/` — entities (zero deps)
- `core/usecase/` — application logic + port interfaces
- `core/adapter/` — Postgres+Bun, Docker runner, chi HTTP, AES-GCM, webhook, idgen, clock
- `core/presenter/templ/` — templ + HTMX UI
- `core/runtime/` — `Edition` port + `selfhost` impl
- `saas/` — SaaS edition (build-tag gated)
- `cmd/` — composition roots

See [`docs/design/2026-05-21-mvp-design.md`](docs/design/2026-05-21-mvp-design.md) for the full design.

## Quickstart — selfhost

```bash
curl -fsSL https://<your-releases>/install.sh | sudo bash
# Open http://127.0.0.1:8787/admin/setup → paste GitHub PAT
# Open /settings → paste Anthropic API key
# Save the printed admin API key
```

Then in your Claude Code shell:

```bash
export AGENTIC_DELEGATOR_URL=http://127.0.0.1:8787
export AGENTIC_DELEGATOR_API_KEY=<admin-key>
# Install the skill
mkdir -p ~/.claude/skills/delegate
cp skill/delegate.md ~/.claude/skills/delegate/
# In any repo:
# claude → /delegate → confirm → wait for PR
```

Full end-to-end: [`docs/end-to-end-smoke.md`](docs/end-to-end-smoke.md).

## Quickstart — SaaS

See [`docs/saas-setup.md`](docs/saas-setup.md) (requires a registered GitHub App).

## Development

```bash
make dev-db-up                    # local Postgres on host port 5433
make generate                     # oapi-codegen + templ generate
make test                         # unit tests
make test-integration             # against the local Postgres + Docker
make arch-check                   # dependency-rule enforcement
```

## License

See [LICENSE](LICENSE).
