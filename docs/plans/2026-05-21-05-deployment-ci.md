# Plan 05 — Deployment + CI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Ship the binaries + image + deployment artifacts. After this plan: a self-hoster can install agentic-delegator from a release binary with a one-line install script; a SaaS operator can deploy with a systemd unit + Caddyfile; CI builds + tests on every PR and cuts release artifacts on tag push.

**Scope:** systemd unit files (selfhost + saas), install.sh, Caddyfile examples, GitHub Actions CI workflow, release workflow, README/CONTRIBUTING basics.

**Prerequisites:** `plan-04-done` tag set; both binaries build cleanly.

---

## File structure produced by this plan

```
agentic-delegator/
├── deploy/
│   ├── selfhost/
│   │   ├── agentic-delegator.service        # systemd unit
│   │   ├── docker-compose.postgres.yml      # Postgres bring-up (single-tenant)
│   │   └── install.sh                       # one-liner installer
│   └── saas/
│       ├── agentic-delegator-saas.service   # systemd unit
│       ├── docker-compose.postgres.yml      # Postgres bring-up
│       └── Caddyfile.example                # reverse proxy + auto-TLS
├── .github/
│   └── workflows/
│       ├── ci.yml                           # tests + lint + build on PR
│       └── release.yml                      # cut tagged binaries
├── README.md                                # rewritten with project overview + quickstart
└── CONTRIBUTING.md                          # dev setup + arch + plans/specs pointer
```

---

## Phase A — Deploy artifacts

### Task 1: Selfhost systemd unit + docker-compose

**Files:**
- Create: `deploy/selfhost/agentic-delegator.service`
- Create: `deploy/selfhost/docker-compose.postgres.yml`

- [ ] **Step 1: systemd unit**

```ini
# deploy/selfhost/agentic-delegator.service
[Unit]
Description=Agentic Delegator (selfhost)
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
User=agentic-delegator
Group=agentic-delegator
EnvironmentFile=/etc/agentic-delegator/env
ExecStart=/usr/local/bin/agentic-delegator serve
Restart=on-failure
RestartSec=5
StateDirectory=agentic-delegator
LogsDirectory=agentic-delegator
WorkingDirectory=/var/lib/agentic-delegator

# Hardening
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

- [ ] **Step 2: docker-compose for Postgres**

```yaml
# deploy/selfhost/docker-compose.postgres.yml
services:
  postgres:
    image: postgres:16-alpine
    container_name: agentic-delegator-postgres
    restart: unless-stopped
    environment:
      POSTGRES_USER: delegator
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?POSTGRES_PASSWORD required}
      POSTGRES_DB: delegator
    ports:
      - "127.0.0.1:5432:5432"
    volumes:
      - delegator-pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U delegator -d delegator"]
      interval: 10s
      timeout: 3s
      retries: 5

volumes:
  delegator-pgdata:
```

- [ ] **Step 3: Commit**

```bash
git add deploy/selfhost/agentic-delegator.service deploy/selfhost/docker-compose.postgres.yml
git commit -m "feat(deploy/selfhost): systemd unit + Postgres docker-compose"
```

---

### Task 2: Selfhost install.sh

**Files:**
- Create: `deploy/selfhost/install.sh`

- [ ] **Step 1: Write the installer**

```bash
#!/usr/bin/env bash
# deploy/selfhost/install.sh
# One-liner installer for agentic-delegator (selfhost edition).
# Usage: curl -fsSL https://<your-releases>/install.sh | sudo bash
set -euo pipefail

VERSION="${AGENTIC_VERSION:-latest}"
REPO="${AGENTIC_REPO:-<owner>/agentic-delegator}"
PREFIX="${AGENTIC_PREFIX:-/usr/local}"
USER_NAME="agentic-delegator"
HOME_DIR="/var/lib/agentic-delegator"
CONFIG_DIR="/etc/agentic-delegator"

require() { command -v "$1" >/dev/null 2>&1 || { echo "missing: $1" >&2; exit 1; }; }
require curl
require tar
require docker

if [ "$EUID" -ne 0 ]; then
  echo "must be run as root (use sudo)" >&2
  exit 1
fi

uname_s=$(uname -s | tr A-Z a-z)
uname_m=$(uname -m)
case "$uname_m" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "unsupported arch: $uname_m"; exit 2 ;;
esac

# 1. Create unprivileged user
if ! id "$USER_NAME" >/dev/null 2>&1; then
  useradd --system --home-dir "$HOME_DIR" --create-home --shell /usr/sbin/nologin "$USER_NAME"
fi
usermod -aG docker "$USER_NAME"

# 2. Download release tarball
url="https://github.com/${REPO}/releases/${VERSION}/download/agentic-delegator-${uname_s}-${ARCH}.tar.gz"
if [ "$VERSION" = "latest" ]; then
  url="https://github.com/${REPO}/releases/latest/download/agentic-delegator-${uname_s}-${ARCH}.tar.gz"
fi
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $url"
curl -fsSL "$url" | tar -xz -C "$tmp"

install -m 755 "$tmp/agentic-delegator" "$PREFIX/bin/agentic-delegator"

# 3. Config dir
mkdir -p "$CONFIG_DIR"
if [ ! -f "$CONFIG_DIR/env" ]; then
  master_key=$(openssl rand -hex 32)
  pg_password=$(openssl rand -hex 16)
  cat > "$CONFIG_DIR/env" <<EOF
# Generated by agentic-delegator install.sh
AGENTIC_HTTP_BIND=127.0.0.1:8787
AGENTIC_MASTER_KEY=${master_key}
DELEGATOR_DSN=postgres://delegator:${pg_password}@127.0.0.1:5432/delegator?sslmode=disable
AGENTIC_RUNNER_IMAGE=agentic-delegator-runner:dev
POSTGRES_PASSWORD=${pg_password}
EOF
  chmod 600 "$CONFIG_DIR/env"
  chown "$USER_NAME":"$USER_NAME" "$CONFIG_DIR/env"
fi

# 4. systemd unit
install -m 644 "$tmp/agentic-delegator.service" /etc/systemd/system/agentic-delegator.service
systemctl daemon-reload

# 5. Postgres
install -m 644 "$tmp/docker-compose.postgres.yml" "$CONFIG_DIR/docker-compose.postgres.yml"
( cd "$CONFIG_DIR" && set -a; . ./env; set +a; docker compose -f docker-compose.postgres.yml up -d )

# 6. Migrate + init
sudo -u "$USER_NAME" env $(grep -v '^#' "$CONFIG_DIR/env" | xargs) "$PREFIX/bin/agentic-delegator" migrate up
sudo -u "$USER_NAME" env $(grep -v '^#' "$CONFIG_DIR/env" | xargs) "$PREFIX/bin/agentic-delegator" init

# 7. Start
systemctl enable --now agentic-delegator

cat <<EOF

==========================================================
agentic-delegator installed.
Service: systemctl status agentic-delegator
Config:  $CONFIG_DIR/env
Logs:    journalctl -u agentic-delegator -f
URL:     http://127.0.0.1:8787 (consider Caddy in front)

Next:
  1) Open http://127.0.0.1:8787/admin/setup and paste your GitHub PAT.
  2) Open /settings and paste your Anthropic API key.
  3) Install skill/delegate.md into Claude Code.
==========================================================
EOF
```

- [ ] **Step 2: Make executable + commit**

```bash
chmod +x deploy/selfhost/install.sh
git add deploy/selfhost/install.sh
git commit -m "feat(deploy/selfhost): one-liner install.sh"
```

---

### Task 3: SaaS systemd unit + Caddyfile + docker-compose

**Files:**
- Create: `deploy/saas/agentic-delegator-saas.service`
- Create: `deploy/saas/docker-compose.postgres.yml`
- Create: `deploy/saas/Caddyfile.example`

- [ ] **Step 1: systemd unit**

```ini
# deploy/saas/agentic-delegator-saas.service
[Unit]
Description=Agentic Delegator (SaaS)
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
User=agentic-delegator
Group=agentic-delegator
EnvironmentFile=/etc/agentic-delegator/env
ExecStart=/usr/local/bin/agentic-delegator-saas serve
Restart=on-failure
RestartSec=5
StateDirectory=agentic-delegator
LogsDirectory=agentic-delegator
WorkingDirectory=/var/lib/agentic-delegator

NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

- [ ] **Step 2: docker-compose** (same shape as selfhost)

```yaml
# deploy/saas/docker-compose.postgres.yml
services:
  postgres:
    image: postgres:16-alpine
    container_name: agentic-delegator-saas-postgres
    restart: unless-stopped
    environment:
      POSTGRES_USER: delegator
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?POSTGRES_PASSWORD required}
      POSTGRES_DB: delegator
    ports:
      - "127.0.0.1:5432:5432"
    volumes:
      - delegator-saas-pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U delegator -d delegator"]
      interval: 10s
      timeout: 3s
      retries: 5

volumes:
  delegator-saas-pgdata:
```

- [ ] **Step 3: Caddyfile**

```
# deploy/saas/Caddyfile.example
# Replace <your-domain> with your actual domain; Caddy will auto-fetch TLS certs.

<your-domain> {
    reverse_proxy 127.0.0.1:8787

    # Long-running runner jobs can take many minutes — bump timeouts.
    request_body {
        max_size 10MB
    }

    encode gzip zstd

    log {
        output file /var/log/caddy/agentic-delegator-saas.log
    }
}
```

- [ ] **Step 4: Commit**

```bash
git add deploy/saas/
git commit -m "feat(deploy/saas): systemd unit + Caddyfile + Postgres compose"
```

---

## Phase B — GitHub Actions

### Task 4: CI workflow (PR-time)

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Workflow**

```yaml
# .github/workflows/ci.yml
name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16-alpine
        env:
          POSTGRES_USER: delegator
          POSTGRES_PASSWORD: delegator
          POSTGRES_DB: delegator
        ports:
          - 5433:5432
        options: >-
          --health-cmd "pg_isready -U delegator"
          --health-interval 5s
          --health-timeout 3s
          --health-retries 10
    env:
      DELEGATOR_TEST_DSN: postgres://delegator:delegator@127.0.0.1:5433/delegator?sslmode=disable
      AGENTIC_MASTER_KEY: 0000000000000000000000000000000000000000000000000000000000000000
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
          cache: true

      - name: go mod download
        run: go mod download

      - name: arch-check
        run: make arch-check

      - name: vet
        run: go vet ./...

      - name: unit tests (race)
        run: go test -race ./...

      - name: integration tests
        run: go test -tags=integration ./...

      - name: saas tests
        run: go test -tags=saas ./saas/...

      - name: OSS binary builds
        run: go build ./cmd/agentic-delegator

      - name: SaaS binary builds
        run: go build -tags=saas ./cmd/agentic-delegator-saas

      - name: module boundary check
        run: |
          set -e
          leak=$(go list -deps ./cmd/agentic-delegator | grep '/saas/' || true)
          if [ -n "$leak" ]; then
            echo "BAD: saas leaked into OSS binary"
            echo "$leak"
            exit 1
          fi
          echo "clean: no saas imports in OSS binary"
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: PR workflow (tests, lint, arch-check, module boundary)"
```

---

### Task 5: Release workflow (tag-time)

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Workflow**

```yaml
# .github/workflows/release.yml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write
  packages: write

jobs:
  build:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        include:
          - { goos: linux,   goarch: amd64 }
          - { goos: linux,   goarch: arm64 }
          - { goos: darwin,  goarch: amd64 }
          - { goos: darwin,  goarch: arm64 }
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Build OSS binary
        env:
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
        run: |
          mkdir -p dist
          go build -o dist/agentic-delegator ./cmd/agentic-delegator
          cp deploy/selfhost/agentic-delegator.service dist/
          cp deploy/selfhost/docker-compose.postgres.yml dist/
          tar -czf "agentic-delegator-${{ matrix.goos }}-${{ matrix.goarch }}.tar.gz" -C dist .

      - name: Build SaaS binary
        env:
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
        run: |
          mkdir -p dist-saas
          go build -tags=saas -o dist-saas/agentic-delegator-saas ./cmd/agentic-delegator-saas
          cp deploy/saas/agentic-delegator-saas.service dist-saas/
          cp deploy/saas/Caddyfile.example dist-saas/
          cp deploy/saas/docker-compose.postgres.yml dist-saas/
          tar -czf "agentic-delegator-saas-${{ matrix.goos }}-${{ matrix.goarch }}.tar.gz" -C dist-saas .

      - uses: softprops/action-gh-release@v2
        with:
          files: |
            agentic-delegator-${{ matrix.goos }}-${{ matrix.goarch }}.tar.gz
            agentic-delegator-saas-${{ matrix.goos }}-${{ matrix.goarch }}.tar.gz

  runner-image:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - uses: actions/checkout@v4

      - name: Log in to ghcr.io
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build + push runner image
        uses: docker/build-push-action@v5
        with:
          context: runner
          push: true
          tags: |
            ghcr.io/${{ github.repository_owner }}/agentic-delegator-runner:${{ github.ref_name }}
            ghcr.io/${{ github.repository_owner }}/agentic-delegator-runner:latest
          platforms: linux/amd64,linux/arm64
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: release workflow (multi-arch binaries + runner image)"
```

---

## Phase C — Docs

### Task 6: README rewrite

**Files:**
- Modify: `README.md` (currently a one-line stub)

- [ ] **Step 1: Write a real README**

```markdown
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
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: rewrite README with project overview + quickstart"
```

---

### Task 7: CONTRIBUTING.md

**Files:**
- Create: `CONTRIBUTING.md`

- [ ] **Step 1: Write it**

```markdown
# Contributing to agentic-delegator

## Architecture rules (enforced)

The codebase follows Clean Architecture with strict dependency direction:

- `core/domain/` imports nothing from this repo
- `core/usecase/` imports only `domain` + `usecase/ports`
- `core/adapter/<X>/` may import `domain`, `ports`, `usecase`, but NOT other `adapter/*` siblings
- `saas/` imports gated by `//go:build saas`; OSS binary never includes it
- `cmd/` is the only place that wires concrete adapters together

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

## Build modes

```bash
go build ./cmd/agentic-delegator              # OSS (selfhost)
go build -tags=saas ./cmd/agentic-delegator-saas  # SaaS (includes saas/)
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
- `make build` / `make build-saas` — build the binaries
```

- [ ] **Step 2: Commit**

```bash
git add CONTRIBUTING.md
git commit -m "docs: add CONTRIBUTING.md (architecture rules + workflow)"
```

---

## Phase D — Final verification + tag

### Task 8: Final sweep

- [ ] **Step 1: Full verification**

```bash
make dev-db-up
make arch-check
make test-race
make test-integration
go test -tags=saas ./saas/... -count=1
go build ./cmd/agentic-delegator
go build -tags=saas ./cmd/agentic-delegator-saas

# module boundary
go list -deps ./cmd/agentic-delegator | grep '/saas/' && exit 1 || echo "clean"

# CI workflow lint
[ -f .github/workflows/ci.yml ] || exit 1
[ -f .github/workflows/release.yml ] || exit 1
[ -x deploy/selfhost/install.sh ] || exit 1
```

All must pass.

- [ ] **Step 2: Tag + close out the branch**

```bash
git tag -a plan-05-done -m "Plan 05: deployment artifacts + CI"
git tag -l
```

- [ ] **Step 3: Verify branch is healthy**

```bash
git log --oneline | wc -l       # commit count
git status                       # clean working tree
git log --oneline plan-04-done..HEAD | head
```

---

## Self-review

**Spec coverage:**
- systemd units (selfhost + saas) ✓
- docker-compose for Postgres in both deployments ✓
- install.sh ✓
- Caddyfile ✓
- CI workflow (test, lint, arch, module boundary) ✓
- Release workflow (multi-arch binaries + runner image) ✓
- README ✓
- CONTRIBUTING.md ✓

**Out of scope (Phase 2+):**
- Helm chart / K8s manifests
- Auto-update mechanism
- Telemetry / observability

---

## Execution

Subagent-driven as before.
