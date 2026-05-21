.PHONY: build build-saas test test-race lint arch-check generate dev migrate migrate-saas dev-db-up dev-db-down test-integration clean

GO := go
GOFLAGS :=

build:
	$(GO) build -o bin/agentic-delegator ./cmd/agentic-delegator

build-saas:
	$(GO) build -tags=saas -o bin/agentic-delegator-saas ./cmd/agentic-delegator-saas

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

lint:
	$(GO) vet ./...

arch-check:
	$(GO) run github.com/fe3dback/go-arch-lint@v1.15.0 check --project-path .

generate:
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.4.1 -config api/codegen.yaml api/openapi.yaml

dev-db-up:
	docker compose -f docker-compose.dev.yml up -d
	@echo "waiting for postgres..."
	@until docker exec agentic-delegator-postgres-dev pg_isready -U delegator >/dev/null 2>&1; do sleep 1; done
	@echo "postgres ready"

dev-db-down:
	docker compose -f docker-compose.dev.yml down

dev:
	@echo "Plan 03 will wire Air here."

migrate:
	go run ./cmd/agentic-delegator/migrate up

migrate-saas:
	@echo "Plan 04 will wire SaaS-only migrations here."

test-integration:
	$(GO) test -tags=integration ./...

clean:
	rm -rf bin/ tmp/
