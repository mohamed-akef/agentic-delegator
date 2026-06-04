# Contributing to agentic-delegator

## Architecture rules (enforced)

The codebase follows Clean Architecture with strict dependency direction:

- `core/domain/` imports nothing from this repo
- `core/usecase/` imports only `domain` + `usecase/ports`
- `core/adapter/<X>/` may import `domain`, `ports`, `usecase`, but NOT other `adapter/*` siblings
- `cmd/agentic-delegator` is the only place that wires concrete adapters together

`make arch-check` enforces this. CI fails on violations.

## Adding a new feature

1. Open the design doc to see if the feature fits the existing port surface.
2. If it needs a new port: define the interface in `core/usecase/ports/`, implement it in an adapter, wire it in `cmd/`.
3. Add a use case in `core/usecase/` if the feature is application logic.
4. Add a templ partial in `core/presenter/templ/` if it's UI.
5. Write tests at the appropriate layer:
   - Domain entities → pure unit tests
   - Use cases → use `core/testutil` fakes
   - Adapters → integration tests with `//go:build integration` + real Postgres/Docker
6. Update `api/openapi.yaml` if you're changing the HTTP contract, then `make generate`.

## Build + run

```bash
make build                                    # build bin/agentic-delegator
go run ./cmd/agentic-delegator migrate up     # apply migrations
go run ./cmd/agentic-delegator serve          # run the server
```

## Plans + specs

Long-form changes go through brainstorming → spec → plan → execution. See `docs/plans/` for examples (the project itself was built using this workflow).

## Common make targets

- `make test` — unit tests (fast)
- `make test-race` — unit tests with race detector
- `make test-integration` — needs `make dev-db-up` + Docker daemon
- `make lint` — go vet
- `make arch-check` — Clean Architecture rule check
- `make generate` — re-run oapi-codegen + templ
- `make dev-db-up` / `dev-db-down` — local Postgres on host port 5433
- `make build` — build the binary
- `make migrate` — apply migrations
