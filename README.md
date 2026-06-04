# agentic-delegator

SaaS background coding agent that delegates implementation tasks to Claude Code.

## What it does

Invoke the `/delegate` skill from any Claude Code session, hand it a spec, and the service:

1. Clones your repo on a sandboxed runner container
2. Runs Claude Code on the spec
3. Commits, pushes, and opens a PR

You watch the status page; the agent does the work in the background.

## Architecture

Clean Architecture with strict layer boundaries enforced by [go-arch-lint](https://github.com/fe3dback/go-arch-lint):

- `core/domain/` — entities (zero deps)
- `core/usecase/` — application logic + port interfaces
- `core/adapter/` — Postgres+Bun, Docker runner, chi HTTP, AES-GCM, webhook, idgen, clock
- `core/presenter/templ/` — templ + HTMX UI
- `core/adapter/ghapp/` — GitHub App (JWT, installation tokens, install flow, webhooks)
- `core/adapter/http/auth/` — GitHub OAuth, cookie sessions, request resolver (session or bearer key → user)
- `core/adapter/credentials/` — secrets-backed Anthropic credentials provider
- `cmd/agentic-delegator/` — the single composition root (`serve` + `migrate` subcommands)

See [`docs/design/2026-05-21-mvp-design.md`](docs/design/2026-05-21-mvp-design.md) for the full design.

## Quickstart

See [`docs/saas-setup.md`](docs/saas-setup.md) (requires a registered GitHub App).

Full end-to-end walkthrough: [`docs/end-to-end-smoke.md`](docs/end-to-end-smoke.md).

## Development

```bash
make dev-db-up                    # local Postgres on host port 5433
make generate                     # oapi-codegen + templ generate
make build                        # build bin/agentic-delegator
make migrate                      # apply migrations (go run ./cmd/agentic-delegator migrate up)
make test                         # unit tests
make test-integration             # against the local Postgres + Docker
make arch-check                   # dependency-rule enforcement
```

## License

See [LICENSE](LICENSE).
