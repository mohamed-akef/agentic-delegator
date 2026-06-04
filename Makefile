.PHONY: build test test-race lint arch-check generate dev migrate dev-db-up dev-db-down test-integration clean css tailwindcss-install

GO := go
GOFLAGS :=

# Standalone Tailwind CLI binary. Pinned for reproducibility; bump when needed.
TAILWIND_VERSION := v3.4.13
TAILWIND_BIN := bin/tailwindcss
# OS/arch detection: defaults to linux-x64; override via TAILWIND_OS_ARCH on
# other platforms (e.g. "macos-arm64", "windows-x64.exe", "linux-arm64").
TAILWIND_OS_ARCH ?= linux-x64
TAILWIND_URL := https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-$(TAILWIND_OS_ARCH)

build:
	$(GO) build -o bin/agentic-delegator ./cmd/agentic-delegator

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

lint:
	$(GO) vet ./...

arch-check:
	$(GO) run github.com/fe3dback/go-arch-lint@v1.15.0 check --project-path .

generate:
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.7.0 -config api/codegen.yaml api/openapi.yaml
	go run github.com/a-h/templ/cmd/templ@v0.3.819 generate

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
	go run ./cmd/agentic-delegator migrate up

test-integration:
	$(GO) test -tags=integration ./...

clean:
	rm -rf bin/ tmp/

# Download the standalone tailwindcss binary into bin/tailwindcss. No Node
# required. Override TAILWIND_OS_ARCH for non-linux-x64 platforms; see the
# Tailwind releases page for asset names.
tailwindcss-install:
	@mkdir -p bin
	@if [ ! -x $(TAILWIND_BIN) ]; then \
		echo "downloading tailwindcss $(TAILWIND_VERSION) ($(TAILWIND_OS_ARCH))..."; \
		curl -fsSL -o $(TAILWIND_BIN) $(TAILWIND_URL); \
		chmod +x $(TAILWIND_BIN); \
	else \
		echo "tailwindcss already at $(TAILWIND_BIN) (delete to re-download)"; \
	fi

# Compile web/input.css into the embedded core/presenter/static/css/app.css.
# Requires `make tailwindcss-install` once. Re-run whenever templ class usage
# changes; commit the regenerated CSS alongside the templ changes.
css: tailwindcss-install
	$(TAILWIND_BIN) -i web/input.css -o core/presenter/static/css/app.css --minify
