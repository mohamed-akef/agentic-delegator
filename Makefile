.PHONY: build build-saas test test-race lint arch-check generate dev migrate migrate-saas clean

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
	$(GO) run github.com/fe3dback/go-arch-lint@v1.11.4 check --project-path .

generate:
	@echo "(plan 02 will wire templ + oapi-codegen here)"

dev:
	@echo "(plan 03 will wire air here)"

migrate:
	@echo "(plan 02 will wire bun migrate here)"

migrate-saas:
	@echo "(plan 02 will wire saas bun migrate here)"

clean:
	rm -rf bin/ tmp/
