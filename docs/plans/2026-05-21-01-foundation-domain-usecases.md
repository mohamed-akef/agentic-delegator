# Plan 01 — Foundation, Domain, Use Cases Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the repo scaffold and build the Clean-Architecture inner layers (domain + use cases + ports + test doubles). After this plan, `go test ./...` is green, `make arch-check` passes, and we have all the application logic ready for real adapters in Plan 02. No HTTP, no Postgres, no Docker yet.

**Architecture:** Strict Clean Architecture inner layers. `core/domain` has zero internal imports. `core/usecase` depends only on `domain` + `core/usecase/ports`. `core/testutil` provides in-memory fakes that satisfy the ports (LSP). Architectural dependency rule is mechanically enforced by `go-arch-lint`.

**Tech Stack:** Go 1.22, `github.com/fe3dback/go-arch-lint` for architectural checks, standard library only for everything else in this plan. No third-party runtime dependencies yet — those land in Plan 02 with the adapters.

**Spec reference:** [`docs/design/2026-05-21-mvp-design.md`](../design/2026-05-21-mvp-design.md), sections "Architectural principles," "High-level architecture → Codebase structure," "Data model → Domain entities," "Component boundaries."

---

## File structure produced by this plan

```
agentic-delegator/
├── go.mod                                              # module agentic-delegator, go 1.22
├── Makefile                                            # generate, build, test, arch-check, lint
├── .gitignore                                          # (existing, plus /bin /tmp)
├── arch-lint.yml                                       # go-arch-lint config
├── api/
│   └── openapi.yaml                                    # empty stub; flesh out in Plan 02
├── core/
│   ├── domain/
│   │   ├── errors.go         + errors_test.go
│   │   ├── user.go           + user_test.go
│   │   ├── api_key.go        + api_key_test.go
│   │   ├── credentials.go    + credentials_test.go
│   │   ├── spec.go           + spec_test.go
│   │   └── job.go            + job_test.go
│   ├── usecase/
│   │   ├── ports/
│   │   │   ├── clock.go
│   │   │   ├── id_generator.go
│   │   │   ├── jobs_repo.go
│   │   │   ├── secrets_repo.go
│   │   │   ├── api_keys_repo.go
│   │   │   ├── runner_service.go
│   │   │   ├── repo_creds_provider.go
│   │   │   ├── anthropic_creds_provider.go
│   │   │   └── webhook_dispatcher.go
│   │   ├── mint_api_key.go              + _test.go
│   │   ├── revoke_api_key.go            + _test.go
│   │   ├── set_anthropic_credentials.go + _test.go
│   │   ├── enqueue_job.go               + _test.go
│   │   ├── get_job.go                   + _test.go
│   │   ├── list_jobs.go                 + _test.go
│   │   ├── handle_runner_completion.go  + _test.go
│   │   ├── reattach_running_jobs.go     + _test.go
│   │   └── dispatch_completion_webhook.go + _test.go
│   └── testutil/
│       ├── fake_clock.go
│       ├── fake_id_generator.go
│       ├── fake_jobs_repo.go
│       ├── fake_secrets_repo.go
│       ├── fake_api_keys_repo.go
│       ├── fake_runner_service.go
│       ├── fake_repo_creds_provider.go
│       ├── fake_anthropic_creds_provider.go
│       └── fake_webhook_dispatcher.go
```

Module path used throughout this plan: `agentic-delegator` (bare module name; we'll rename to a `github.com/<owner>/agentic-delegator` path before publishing).

---

## Phase A — Repo scaffold

### Task 1: Initialize Go module and directory skeleton

**Files:**
- Create: `go.mod`
- Create: `bin/.gitkeep`, `core/.gitkeep`, `api/.gitkeep`

- [ ] **Step 1: Initialize the Go module**

```bash
cd /Users/akef/workspace/agentic-delegator
go mod init agentic-delegator
```

Then edit `go.mod` to pin the Go version:

```
module agentic-delegator

go 1.22
```

- [ ] **Step 2: Create skeleton directories**

```bash
mkdir -p core/domain core/usecase/ports core/testutil api bin
touch core/.gitkeep core/domain/.gitkeep core/usecase/.gitkeep core/usecase/ports/.gitkeep core/testutil/.gitkeep api/.gitkeep bin/.gitkeep
```

- [ ] **Step 3: Add bin/ to .gitignore**

Append to `.gitignore` (right after the `*.dylib` line block, before `# Test binary`):

```
# Build output
/bin/
/tmp/
```

- [ ] **Step 4: Verify Go compiles an empty module**

```bash
go build ./...
```

Expected: no output, exit 0. (Nothing to build yet, but the module is valid.)

- [ ] **Step 5: Commit**

```bash
git add go.mod .gitignore core api bin
git commit -m "chore: initialize go module + clean-architecture skeleton"
```

---

### Task 2: Add Makefile with the canonical targets

**Files:**
- Create: `Makefile`

- [ ] **Step 1: Write the Makefile**

```make
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
```

- [ ] **Step 2: Verify each defined target parses**

```bash
make -n build test lint arch-check generate dev clean
```

Expected: each line prints the would-be command. No errors.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "chore: add Makefile with canonical targets"
```

---

### Task 3: Configure go-arch-lint for the dependency rule

**Files:**
- Create: `arch-lint.yml`

- [ ] **Step 1: Write the architecture rule config**

```yaml
# arch-lint.yml — enforces Clean Architecture dependency rule.
# See https://github.com/fe3dback/go-arch-lint for syntax.

version: 3
workdir: .
allow:
  depOnAnyVendor: false
  deepScan: true

components:
  domain:
    in: core/domain/**
  ports:
    in: core/usecase/ports/**
  usecase:
    in: core/usecase
  testutil:
    in: core/testutil/**

deps:
  domain:
    mayDependOn: []  # domain depends on NOTHING inside the repo

  ports:
    mayDependOn:
      - domain

  usecase:
    mayDependOn:
      - domain
      - ports

  testutil:
    mayDependOn:
      - domain
      - ports
```

- [ ] **Step 2: Run the arch-check with no Go files yet — should trivially pass**

```bash
make arch-check
```

Expected: tool downloads, prints "OK" or similar, exit 0. (There are no `.go` files in the listed components yet, so there's nothing to verify, which is fine.)

- [ ] **Step 3: Commit**

```bash
git add arch-lint.yml
git commit -m "chore: enforce Clean Architecture dependency rule with go-arch-lint"
```

---

### Task 4: Add an OpenAPI stub so later plans can reference it

**Files:**
- Create: `api/openapi.yaml`

- [ ] **Step 1: Write a minimal valid OpenAPI 3.1 file**

```yaml
openapi: 3.1.0
info:
  title: agentic-delegator
  version: 0.1.0
  description: Spec-driven background coding agent service. Endpoints fleshed out in plan 02.
servers:
  - url: http://localhost:8787
paths: {}
components: {}
```

- [ ] **Step 2: Commit**

```bash
git add api/openapi.yaml
git commit -m "chore: add OpenAPI 3.1 stub"
```

---

## Phase B — Domain layer (entities, zero deps)

### Task 5: Domain sentinel errors

**Files:**
- Create: `core/domain/errors.go`
- Test: `core/domain/errors_test.go`

- [ ] **Step 1: Write the failing test**

```go
// core/domain/errors_test.go
package domain_test

import (
	"errors"
	"testing"

	"agentic-delegator/core/domain"
)

func TestDomainErrors_distinct(t *testing.T) {
	all := []error{
		domain.ErrNotFound,
		domain.ErrConflict,
		domain.ErrForbidden,
		domain.ErrInvalidState,
		domain.ErrInvalidInput,
	}
	for i := range all {
		for j := range all {
			if i == j {
				continue
			}
			if errors.Is(all[i], all[j]) {
				t.Fatalf("errors at %d and %d compare equal but must be distinct", i, j)
			}
		}
	}
}

func TestDomainErrors_isAndAs(t *testing.T) {
	wrapped := errors.Join(domain.ErrNotFound, errors.New("user u_1"))
	if !errors.Is(wrapped, domain.ErrNotFound) {
		t.Fatalf("wrapped ErrNotFound should be detectable via errors.Is")
	}
}
```

- [ ] **Step 2: Run the test, verify it fails**

```bash
go test ./core/domain/...
```

Expected: compile error — `undefined: domain.ErrNotFound`.

- [ ] **Step 3: Write the implementation**

```go
// core/domain/errors.go
package domain

import "errors"

// Sentinel domain errors. Adapters and use cases should wrap these so callers
// can detect failure categories via errors.Is.
var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrForbidden    = errors.New("forbidden")
	ErrInvalidState = errors.New("invalid state transition")
	ErrInvalidInput = errors.New("invalid input")
)
```

- [ ] **Step 4: Run the tests, verify they pass**

```bash
go test ./core/domain/...
```

Expected: `ok  	agentic-delegator/core/domain`.

- [ ] **Step 5: Commit**

```bash
git add core/domain/errors.go core/domain/errors_test.go
git commit -m "feat(domain): add sentinel domain errors"
```

---

### Task 6: Domain User entity

**Files:**
- Create: `core/domain/user.go`
- Test: `core/domain/user_test.go`

- [ ] **Step 1: Write the failing test**

```go
// core/domain/user_test.go
package domain_test

import (
	"testing"
	"time"

	"agentic-delegator/core/domain"
)

func TestNewUser_setsFields(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	u := domain.NewUser("u_1", "Alice", now)
	if u.ID != "u_1" {
		t.Fatalf("id: want u_1, got %s", u.ID)
	}
	if u.DisplayName != "Alice" {
		t.Fatalf("name: want Alice, got %s", u.DisplayName)
	}
	if !u.CreatedAt.Equal(now) {
		t.Fatalf("created_at: want %v, got %v", now, u.CreatedAt)
	}
}
```

- [ ] **Step 2: Run, verify it fails**

```bash
go test ./core/domain/...
```

Expected: `undefined: domain.NewUser`, `undefined: domain.UserID`.

- [ ] **Step 3: Implement**

```go
// core/domain/user.go
package domain

import "time"

// UserID is the opaque identifier of a user account. In SaaS, this is a UUID
// generated at signup. In selfhost, there is exactly one user with a fixed ID.
type UserID string

type User struct {
	ID          UserID
	DisplayName string
	CreatedAt   time.Time
}

func NewUser(id UserID, displayName string, now time.Time) *User {
	return &User{ID: id, DisplayName: displayName, CreatedAt: now}
}
```

- [ ] **Step 4: Run, verify it passes**

```bash
go test ./core/domain/...
```

Expected: `ok  	agentic-delegator/core/domain`.

- [ ] **Step 5: Commit**

```bash
git add core/domain/user.go core/domain/user_test.go
git commit -m "feat(domain): add User entity + UserID"
```

---

### Task 7: Domain APIKey entity

**Files:**
- Create: `core/domain/api_key.go`
- Test: `core/domain/api_key_test.go`

- [ ] **Step 1: Write the failing test**

```go
// core/domain/api_key_test.go
package domain_test

import (
	"testing"
	"time"

	"agentic-delegator/core/domain"
)

func TestNewAPIKey_setsFields(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	k := domain.NewAPIKey("k_1", "u_1", "laptop", "agdkey_a", []byte("hash"), now)
	if k.ID != "k_1" || k.UserID != "u_1" || k.Name != "laptop" || k.Prefix != "agdkey_a" {
		t.Fatalf("fields not set correctly: %+v", k)
	}
	if string(k.Hash) != "hash" {
		t.Fatalf("hash not stored")
	}
	if !k.CreatedAt.Equal(now) {
		t.Fatalf("created_at not stored")
	}
	if k.LastUsedAt != nil {
		t.Fatalf("last_used_at should be nil at creation")
	}
}

func TestAPIKey_RecordUsed(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	later := now.Add(time.Hour)
	k := domain.NewAPIKey("k_1", "u_1", "laptop", "agdkey_a", []byte("hash"), now)
	k.RecordUsed(later)
	if k.LastUsedAt == nil || !k.LastUsedAt.Equal(later) {
		t.Fatalf("LastUsedAt not updated")
	}
}
```

- [ ] **Step 2: Run, verify it fails**

```bash
go test ./core/domain/...
```

Expected: `undefined: domain.NewAPIKey`, etc.

- [ ] **Step 3: Implement**

```go
// core/domain/api_key.go
package domain

import "time"

type APIKeyID string

// APIKeyHash is an opaque hash (typically bcrypt) of the plaintext key.
// The plaintext is never stored — only this hash and the prefix.
type APIKeyHash []byte

type APIKey struct {
	ID         APIKeyID
	UserID     UserID
	Name       string
	Prefix     string // first 8 chars of the plaintext key, kept for UI lookups
	Hash       APIKeyHash
	LastUsedAt *time.Time
	CreatedAt  time.Time
}

func NewAPIKey(id APIKeyID, userID UserID, name, prefix string, hash APIKeyHash, now time.Time) *APIKey {
	return &APIKey{
		ID:        id,
		UserID:    userID,
		Name:      name,
		Prefix:    prefix,
		Hash:      hash,
		CreatedAt: now,
	}
}

func (k *APIKey) RecordUsed(now time.Time) {
	t := now
	k.LastUsedAt = &t
}
```

- [ ] **Step 4: Run, verify it passes**

```bash
go test ./core/domain/...
```

- [ ] **Step 5: Commit**

```bash
git add core/domain/api_key.go core/domain/api_key_test.go
git commit -m "feat(domain): add APIKey entity"
```

---

### Task 8: Domain credential value objects

**Files:**
- Create: `core/domain/credentials.go`
- Test: `core/domain/credentials_test.go`

- [ ] **Step 1: Write the failing test**

```go
// core/domain/credentials_test.go
package domain_test

import (
	"testing"
	"time"

	"agentic-delegator/core/domain"
)

func TestGitCreds_Expired(t *testing.T) {
	now := time.Unix(1000, 0)
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"future", now.Add(time.Hour), false},
		{"past", now.Add(-time.Hour), true},
		{"equal", now, true},
		{"zero is never expired", time.Time{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := domain.GitCreds{Token: "t", ExpiresAt: tc.expiresAt}
			if got := c.Expired(now); got != tc.want {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestAnthropicCreds_zeroValue(t *testing.T) {
	var c domain.AnthropicCreds
	if c.APIKey != "" {
		t.Fatalf("zero value should have empty APIKey")
	}
}
```

- [ ] **Step 2: Run, verify it fails**

```bash
go test ./core/domain/...
```

Expected: `undefined: domain.GitCreds`, etc.

- [ ] **Step 3: Implement**

```go
// core/domain/credentials.go
package domain

import "time"

// GitCreds is the short-lived credential the runner uses to clone + push.
// In selfhost mode, this wraps a long-lived PAT (ExpiresAt zero).
// In SaaS mode, this is a freshly minted GitHub App installation token.
type GitCreds struct {
	Token     string
	ExpiresAt time.Time
}

// Expired reports whether the token is past its expiry. A zero ExpiresAt
// means "no expiry tracked" (used for PATs) and is treated as never expired.
func (c GitCreds) Expired(now time.Time) bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return !now.Before(c.ExpiresAt)
}

// AnthropicCreds wraps the credential used to authenticate Claude Code.
// MVP supports only an API key. Phase 2 may add an OAuth bearer.
type AnthropicCreds struct {
	APIKey string
}
```

- [ ] **Step 4: Run, verify it passes**

```bash
go test ./core/domain/...
```

- [ ] **Step 5: Commit**

```bash
git add core/domain/credentials.go core/domain/credentials_test.go
git commit -m "feat(domain): add GitCreds + AnthropicCreds value objects"
```

---

### Task 9: Domain SpecSource value object

**Files:**
- Create: `core/domain/spec.go`
- Test: `core/domain/spec_test.go`

- [ ] **Step 1: Write the failing test**

```go
// core/domain/spec_test.go
package domain_test

import (
	"testing"

	"agentic-delegator/core/domain"
)

func TestSpecSource_Valid(t *testing.T) {
	tests := []struct {
		name string
		s    domain.SpecSource
		want bool
	}{
		{"inline ok", domain.SpecSource{Type: domain.SourceTypeInline, Value: "hello"}, true},
		{"path ok", domain.SpecSource{Type: domain.SourceTypePath, Value: "specs/x.md"}, true},
		{"url ok", domain.SpecSource{Type: domain.SourceTypeURL, Value: "https://x/y.md"}, true},
		{"empty value", domain.SpecSource{Type: domain.SourceTypePath, Value: ""}, false},
		{"unknown type", domain.SpecSource{Type: "weird", Value: "x"}, false},
		{"zero value", domain.SpecSource{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.s.Valid(); got != tc.want {
				t.Fatalf("Valid: want %v, got %v", tc.want, got)
			}
		})
	}
}
```

- [ ] **Step 2: Run, verify it fails**

```bash
go test ./core/domain/...
```

- [ ] **Step 3: Implement**

```go
// core/domain/spec.go
package domain

// SourceType describes where a spec's content lives.
type SourceType string

const (
	SourceTypeInline SourceType = "inline" // Value is the spec text itself
	SourceTypePath   SourceType = "path"   // Value is a path inside the target repo
	SourceTypeURL    SourceType = "url"    // Value is an http(s) URL
)

// SpecSource is the value object passed to the runner. The skill classifies
// the user's input into one of the three types before submitting.
type SpecSource struct {
	Type  SourceType
	Value string
}

func (s SpecSource) Valid() bool {
	if s.Value == "" {
		return false
	}
	switch s.Type {
	case SourceTypeInline, SourceTypePath, SourceTypeURL:
		return true
	}
	return false
}
```

- [ ] **Step 4: Run, verify it passes**

```bash
go test ./core/domain/...
```

- [ ] **Step 5: Commit**

```bash
git add core/domain/spec.go core/domain/spec_test.go
git commit -m "feat(domain): add SpecSource value object"
```

---

### Task 10: Domain Job entity with status transitions

**Files:**
- Create: `core/domain/job.go`
- Test: `core/domain/job_test.go`

- [ ] **Step 1: Write the failing test**

```go
// core/domain/job_test.go
package domain_test

import (
	"errors"
	"testing"
	"time"

	"agentic-delegator/core/domain"
)

func newTestJob(t *testing.T, now time.Time) *domain.Job {
	t.Helper()
	return domain.NewJob(
		"j_1", "u_1",
		"owner/repo", "main", "agentic/x",
		domain.SpecSource{Type: domain.SourceTypePath, Value: "specs/x.md"},
		"",
		now,
	)
}

func TestNewJob_initialStateIsQueued(t *testing.T) {
	now := time.Unix(1000, 0)
	j := newTestJob(t, now)

	if j.Status != domain.JobStatusQueued {
		t.Fatalf("status: want queued, got %s", j.Status)
	}
	if j.ID != "j_1" || j.UserID != "u_1" {
		t.Fatalf("ids not set")
	}
	if j.StartedAt != nil || j.FinishedAt != nil {
		t.Fatalf("timestamps should be nil on creation")
	}
	if !j.CreatedAt.Equal(now) {
		t.Fatalf("created_at not set")
	}
	if j.IsTerminal() {
		t.Fatalf("queued is not terminal")
	}
}

func TestJob_MarkRunning(t *testing.T) {
	now := time.Unix(1000, 0)
	then := now.Add(10 * time.Second)
	j := newTestJob(t, now)

	if err := j.MarkRunning("ctr_a", then); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if j.Status != domain.JobStatusRunning {
		t.Fatalf("status: want running, got %s", j.Status)
	}
	if j.ContainerID != "ctr_a" {
		t.Fatalf("container id not set")
	}
	if j.StartedAt == nil || !j.StartedAt.Equal(then) {
		t.Fatalf("started_at not set correctly")
	}
}

func TestJob_MarkRunning_fromTerminalFails(t *testing.T) {
	now := time.Unix(1000, 0)
	j := newTestJob(t, now)
	_ = j.MarkRunning("ctr_a", now)
	_ = j.MarkSucceeded("https://example/pr/1", now)

	err := j.MarkRunning("ctr_b", now)
	if !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("want ErrInvalidState, got %v", err)
	}
}

func TestJob_MarkSucceeded(t *testing.T) {
	now := time.Unix(1000, 0)
	j := newTestJob(t, now)
	_ = j.MarkRunning("ctr_a", now)

	finished := now.Add(time.Minute)
	if err := j.MarkSucceeded("https://example/pr/1", finished); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if j.Status != domain.JobStatusSucceeded {
		t.Fatalf("status: want succeeded, got %s", j.Status)
	}
	if j.PRURL != "https://example/pr/1" {
		t.Fatalf("pr_url not set")
	}
	if j.FinishedAt == nil || !j.FinishedAt.Equal(finished) {
		t.Fatalf("finished_at not set")
	}
	if !j.IsTerminal() {
		t.Fatalf("succeeded is terminal")
	}
}

func TestJob_MarkSucceeded_fromQueuedFails(t *testing.T) {
	j := newTestJob(t, time.Unix(1000, 0))
	err := j.MarkSucceeded("https://example/pr/1", time.Unix(2000, 0))
	if !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("want ErrInvalidState, got %v", err)
	}
}

func TestJob_MarkFailed_fromQueuedOrRunning(t *testing.T) {
	now := time.Unix(1000, 0)

	t.Run("from queued", func(t *testing.T) {
		j := newTestJob(t, now)
		if err := j.MarkFailed("boom", now); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if j.Status != domain.JobStatusFailed || j.Error != "boom" {
			t.Fatalf("not marked failed correctly")
		}
	})

	t.Run("from running", func(t *testing.T) {
		j := newTestJob(t, now)
		_ = j.MarkRunning("ctr_a", now)
		if err := j.MarkFailed("boom", now); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if j.Status != domain.JobStatusFailed {
			t.Fatalf("not failed")
		}
	})

	t.Run("from terminal fails", func(t *testing.T) {
		j := newTestJob(t, now)
		_ = j.MarkRunning("ctr_a", now)
		_ = j.MarkSucceeded("https://x/pr/1", now)
		if err := j.MarkFailed("boom", now); !errors.Is(err, domain.ErrInvalidState) {
			t.Fatalf("want ErrInvalidState, got %v", err)
		}
	})
}

func TestJob_MarkCancelled(t *testing.T) {
	now := time.Unix(1000, 0)

	t.Run("from queued ok", func(t *testing.T) {
		j := newTestJob(t, now)
		if err := j.MarkCancelled(now); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if j.Status != domain.JobStatusCancelled {
			t.Fatalf("not cancelled")
		}
	})

	t.Run("from running ok", func(t *testing.T) {
		j := newTestJob(t, now)
		_ = j.MarkRunning("ctr_a", now)
		if err := j.MarkCancelled(now); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})

	t.Run("from succeeded fails", func(t *testing.T) {
		j := newTestJob(t, now)
		_ = j.MarkRunning("ctr_a", now)
		_ = j.MarkSucceeded("https://x/pr/1", now)
		err := j.MarkCancelled(now)
		if !errors.Is(err, domain.ErrInvalidState) {
			t.Fatalf("want ErrInvalidState, got %v", err)
		}
	})
}

func TestJob_IsTerminal(t *testing.T) {
	tests := map[domain.JobStatus]bool{
		domain.JobStatusQueued:    false,
		domain.JobStatusRunning:   false,
		domain.JobStatusSucceeded: true,
		domain.JobStatusFailed:    true,
		domain.JobStatusCancelled: true,
	}
	for status, want := range tests {
		j := newTestJob(t, time.Unix(1000, 0))
		j.Status = status
		if got := j.IsTerminal(); got != want {
			t.Fatalf("status %s: want %v, got %v", status, want, got)
		}
	}
}
```

- [ ] **Step 2: Run, verify it fails**

```bash
go test ./core/domain/...
```

Expected: many `undefined: domain.JobStatus*`, `undefined: domain.NewJob`.

- [ ] **Step 3: Implement**

```go
// core/domain/job.go
package domain

import "time"

type JobID string

type JobStatus string

const (
	JobStatusQueued    JobStatus = "queued"
	JobStatusRunning   JobStatus = "running"
	JobStatusSucceeded JobStatus = "succeeded"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

// Job is the central domain entity. It tracks the lifecycle of one
// agentic-delegator task from submission through completion.
type Job struct {
	ID            JobID
	UserID        UserID
	Status        JobStatus
	Repo          string
	BaseBranch    string
	WorkBranch    string
	Spec          SpecSource
	ModelOverride string
	ContainerID   string // populated when MarkRunning
	PRURL         string // populated on MarkSucceeded
	Error         string // populated on MarkFailed
	LogPath       string // filesystem path the runner streams stdout to
	CreatedAt     time.Time
	StartedAt     *time.Time
	FinishedAt    *time.Time
}

func NewJob(id JobID, userID UserID, repo, baseBranch, workBranch string, spec SpecSource, modelOverride string, now time.Time) *Job {
	return &Job{
		ID:            id,
		UserID:        userID,
		Status:        JobStatusQueued,
		Repo:          repo,
		BaseBranch:    baseBranch,
		WorkBranch:    workBranch,
		Spec:          spec,
		ModelOverride: modelOverride,
		CreatedAt:     now,
	}
}

func (j *Job) MarkRunning(containerID string, now time.Time) error {
	if j.Status != JobStatusQueued {
		return ErrInvalidState
	}
	t := now
	j.Status = JobStatusRunning
	j.ContainerID = containerID
	j.StartedAt = &t
	return nil
}

func (j *Job) MarkSucceeded(prURL string, now time.Time) error {
	if j.Status != JobStatusRunning {
		return ErrInvalidState
	}
	t := now
	j.Status = JobStatusSucceeded
	j.PRURL = prURL
	j.FinishedAt = &t
	return nil
}

func (j *Job) MarkFailed(reason string, now time.Time) error {
	if j.IsTerminal() {
		return ErrInvalidState
	}
	t := now
	j.Status = JobStatusFailed
	j.Error = reason
	j.FinishedAt = &t
	return nil
}

func (j *Job) MarkCancelled(now time.Time) error {
	if j.Status != JobStatusQueued && j.Status != JobStatusRunning {
		return ErrInvalidState
	}
	t := now
	j.Status = JobStatusCancelled
	j.FinishedAt = &t
	return nil
}

func (j *Job) IsTerminal() bool {
	switch j.Status {
	case JobStatusSucceeded, JobStatusFailed, JobStatusCancelled:
		return true
	}
	return false
}
```

- [ ] **Step 4: Run, verify it passes**

```bash
go test ./core/domain/...
```

Expected: `ok  	agentic-delegator/core/domain`.

- [ ] **Step 5: Run arch-check — domain must still depend on nothing inside the repo**

```bash
make arch-check
```

Expected: passes.

- [ ] **Step 6: Commit**

```bash
git add core/domain/job.go core/domain/job_test.go
git commit -m "feat(domain): add Job entity with status FSM"
```

---

## Phase C — Use case ports (interfaces only, no logic)

### Task 11: Clock + IDGenerator ports

**Files:**
- Create: `core/usecase/ports/clock.go`
- Create: `core/usecase/ports/id_generator.go`

- [ ] **Step 1: Write `clock.go`**

```go
// core/usecase/ports/clock.go
package ports

import "time"

// Clock is a port for time. Use cases must depend on this rather than
// time.Now() directly so tests can freeze and advance time.
type Clock interface {
	Now() time.Time
}
```

- [ ] **Step 2: Write `id_generator.go`**

```go
// core/usecase/ports/id_generator.go
package ports

// IDGenerator is a port for generating new opaque identifiers and key
// material. Tests use a deterministic fake.
type IDGenerator interface {
	NewJobID() string
	NewAPIKeyID() string
	NewUserID() string

	// NewAPIKeyPlaintext returns a freshly generated API key (plaintext) and
	// its prefix (typically the first 8 characters, used for fast lookup).
	NewAPIKeyPlaintext() (plain string, prefix string)
}
```

- [ ] **Step 3: Verify both compile**

```bash
go build ./core/usecase/ports/...
```

Expected: exit 0.

- [ ] **Step 4: Commit**

```bash
git add core/usecase/ports/clock.go core/usecase/ports/id_generator.go
git commit -m "feat(ports): add Clock + IDGenerator ports"
```

---

### Task 12: Repository ports (Jobs / Secrets / APIKeys)

**Files:**
- Create: `core/usecase/ports/jobs_repo.go`
- Create: `core/usecase/ports/secrets_repo.go`
- Create: `core/usecase/ports/api_keys_repo.go`

- [ ] **Step 1: Write `jobs_repo.go`**

```go
// core/usecase/ports/jobs_repo.go
package ports

import (
	"context"

	"agentic-delegator/core/domain"
)

// JobsRepository is the outbound port for persisting Jobs. Adapter
// implementations (postgres, in-memory fake) must scope every query by
// UserID for multi-tenant safety, except where explicitly noted.
type JobsRepository interface {
	Create(ctx context.Context, j *domain.Job) error

	// Get returns the job regardless of owner. Use only from trusted callers
	// (the runner completion path needs cross-user access).
	Get(ctx context.Context, id domain.JobID) (*domain.Job, error)

	// GetForUser returns the job iff it belongs to userID. Otherwise ErrNotFound.
	GetForUser(ctx context.Context, id domain.JobID, userID domain.UserID) (*domain.Job, error)

	// ListForUser returns the most recent `limit` jobs for userID. limit<=0 means no cap.
	ListForUser(ctx context.Context, userID domain.UserID, limit int) ([]*domain.Job, error)

	// ListByStatus returns jobs in the given status across all users.
	// Used at startup to reattach orphaned containers.
	ListByStatus(ctx context.Context, status domain.JobStatus) ([]*domain.Job, error)

	Update(ctx context.Context, j *domain.Job) error

	// CountActiveForUser returns jobs in queued+running for this user.
	CountActiveForUser(ctx context.Context, userID domain.UserID) (int, error)

	// CountActiveGlobal returns jobs in queued+running across all users.
	CountActiveGlobal(ctx context.Context) (int, error)
}
```

- [ ] **Step 2: Write `secrets_repo.go`**

```go
// core/usecase/ports/secrets_repo.go
package ports

import (
	"context"

	"agentic-delegator/core/domain"
)

// SecretsRepository stores per-user Anthropic credentials (encrypted at
// rest by the adapter). The interface returns plaintext value objects;
// encryption happens inside the adapter.
type SecretsRepository interface {
	SetAnthropicCreds(ctx context.Context, userID domain.UserID, creds domain.AnthropicCreds) error
	GetAnthropicCreds(ctx context.Context, userID domain.UserID) (domain.AnthropicCreds, error)
	DeleteAnthropicCreds(ctx context.Context, userID domain.UserID) error
}
```

- [ ] **Step 3: Write `api_keys_repo.go`**

```go
// core/usecase/ports/api_keys_repo.go
package ports

import (
	"context"
	"time"

	"agentic-delegator/core/domain"
)

type APIKeysRepository interface {
	Create(ctx context.Context, k *domain.APIKey) error

	// GetByPrefix returns all keys with this prefix (typically the first 8
	// chars of the plaintext). Caller bcrypt-checks each candidate's Hash
	// against the supplied plaintext to find a match.
	GetByPrefix(ctx context.Context, prefix string) ([]*domain.APIKey, error)

	ListForUser(ctx context.Context, userID domain.UserID) ([]*domain.APIKey, error)

	// Delete removes the key iff it belongs to userID. Otherwise ErrNotFound.
	Delete(ctx context.Context, id domain.APIKeyID, userID domain.UserID) error

	RecordUsed(ctx context.Context, id domain.APIKeyID, at time.Time) error
}
```

- [ ] **Step 4: Verify all compile**

```bash
go build ./core/usecase/ports/...
```

- [ ] **Step 5: Commit**

```bash
git add core/usecase/ports/jobs_repo.go core/usecase/ports/secrets_repo.go core/usecase/ports/api_keys_repo.go
git commit -m "feat(ports): add JobsRepository, SecretsRepository, APIKeysRepository"
```

---

### Task 13: Runner + Provider + Webhook ports

**Files:**
- Create: `core/usecase/ports/runner_service.go`
- Create: `core/usecase/ports/repo_creds_provider.go`
- Create: `core/usecase/ports/anthropic_creds_provider.go`
- Create: `core/usecase/ports/webhook_dispatcher.go`

- [ ] **Step 1: Write `runner_service.go`**

```go
// core/usecase/ports/runner_service.go
package ports

import (
	"context"

	"agentic-delegator/core/domain"
)

// RunnerStartSpec is everything needed to spawn one runner container.
type RunnerStartSpec struct {
	JobID      domain.JobID
	Repo       string
	BaseBranch string
	WorkBranch string
	Spec       domain.SpecSource
	GitCreds   domain.GitCreds
	Anthropic  domain.AnthropicCreds
	Model      string // empty = adapter default
	LogPath    string // path the runner streams stdout/stderr to
}

// RunnerResult is what the adapter reports when the container exits.
type RunnerResult struct {
	JobID    domain.JobID
	ExitCode int
	PRURL    string // empty if no PR opened
	Error    string // populated when ExitCode != 0
}

// RunnerService is the outbound port for spawning, supervising, and
// terminating runner containers.
type RunnerService interface {
	// Start spawns a container. The adapter wires its own completion path:
	// when the container exits, it must call onComplete with the result.
	// Returns the container ID once the container is started.
	Start(ctx context.Context, spec RunnerStartSpec, onComplete func(RunnerResult)) (containerID string, err error)

	// Inspect reports whether the container is still alive.
	Inspect(ctx context.Context, containerID string) (alive bool, err error)

	// Stop forcibly terminates a running container. Idempotent.
	Stop(ctx context.Context, containerID string) error
}
```

- [ ] **Step 2: Write `repo_creds_provider.go`**

```go
// core/usecase/ports/repo_creds_provider.go
package ports

import (
	"context"

	"agentic-delegator/core/domain"
)

// RepoCredentialsProvider returns short-lived git credentials for a user+repo.
// Edition-specific: selfhost returns the admin's PAT, SaaS mints a fresh
// GitHub App installation token.
type RepoCredentialsProvider interface {
	For(ctx context.Context, userID domain.UserID, repo string) (domain.GitCreds, error)
}
```

- [ ] **Step 3: Write `anthropic_creds_provider.go`**

```go
// core/usecase/ports/anthropic_creds_provider.go
package ports

import (
	"context"

	"agentic-delegator/core/domain"
)

// AnthropicCredentialsProvider returns the Anthropic credential to pass into
// the runner. Edition-specific: both editions today read from the
// SecretsRepository, but the abstraction lets us add OAuth-style sources later.
type AnthropicCredentialsProvider interface {
	For(ctx context.Context, userID domain.UserID) (domain.AnthropicCreds, error)
}
```

- [ ] **Step 4: Write `webhook_dispatcher.go`**

```go
// core/usecase/ports/webhook_dispatcher.go
package ports

import "context"

// WebhookDispatcher fires an outbound HTTP webhook with the given JSON body.
// MVP: no retry at this layer. Adapter implementations may log+swallow errors
// since callers cannot react usefully.
type WebhookDispatcher interface {
	Dispatch(ctx context.Context, url string, payload []byte) error
}
```

- [ ] **Step 5: Verify all compile**

```bash
go build ./core/usecase/ports/...
```

- [ ] **Step 6: Commit**

```bash
git add core/usecase/ports/runner_service.go core/usecase/ports/repo_creds_provider.go core/usecase/ports/anthropic_creds_provider.go core/usecase/ports/webhook_dispatcher.go
git commit -m "feat(ports): add RunnerService + credential providers + WebhookDispatcher"
```

---

## Phase D — Test utility fakes (in-memory adapters)

These are not production code; they are how use case tests stay fast (no Docker, no DB) while still going through the same ports the real adapters will satisfy in Plan 02.

### Task 14: FakeClock + FakeIDGenerator

**Files:**
- Create: `core/testutil/fake_clock.go`
- Create: `core/testutil/fake_id_generator.go`

- [ ] **Step 1: Write `fake_clock.go`**

```go
// core/testutil/fake_clock.go
package testutil

import (
	"sync"
	"time"
)

// FakeClock is a deterministic Clock for tests.
type FakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func NewFakeClock(t time.Time) *FakeClock {
	return &FakeClock{now: t}
}

func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func (c *FakeClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}
```

- [ ] **Step 2: Write `fake_id_generator.go`**

```go
// core/testutil/fake_id_generator.go
package testutil

import (
	"fmt"
	"sync/atomic"
)

// FakeIDGenerator hands out deterministic IDs of the form j_1, j_2, … so tests
// can assert against stable identifiers.
type FakeIDGenerator struct {
	jobCounter  uint64
	keyCounter  uint64
	userCounter uint64
}

func (g *FakeIDGenerator) NewJobID() string {
	return fmt.Sprintf("j_%d", atomic.AddUint64(&g.jobCounter, 1))
}

func (g *FakeIDGenerator) NewAPIKeyID() string {
	return fmt.Sprintf("k_%d", atomic.AddUint64(&g.keyCounter, 1))
}

func (g *FakeIDGenerator) NewUserID() string {
	return fmt.Sprintf("u_%d", atomic.AddUint64(&g.userCounter, 1))
}

// NewAPIKeyPlaintext returns a deterministic plaintext + its 8-char prefix.
func (g *FakeIDGenerator) NewAPIKeyPlaintext() (string, string) {
	n := atomic.AddUint64(&g.keyCounter, 1)
	plain := fmt.Sprintf("agdkey_test_%016d", n)
	return plain, plain[:8]
}
```

- [ ] **Step 3: Verify they compile**

```bash
go build ./core/testutil/...
```

- [ ] **Step 4: Commit**

```bash
git add core/testutil/fake_clock.go core/testutil/fake_id_generator.go
git commit -m "test: add FakeClock + FakeIDGenerator"
```

---

### Task 15: FakeJobsRepo (in-memory)

**Files:**
- Create: `core/testutil/fake_jobs_repo.go`

- [ ] **Step 1: Write the implementation**

```go
// core/testutil/fake_jobs_repo.go
package testutil

import (
	"context"
	"sort"
	"sync"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

// FakeJobsRepo is an in-memory ports.JobsRepository. Stores copies on write
// and returns copies on read to avoid aliasing bugs in tests.
type FakeJobsRepo struct {
	mu sync.Mutex
	m  map[domain.JobID]*domain.Job
}

func NewFakeJobsRepo() *FakeJobsRepo {
	return &FakeJobsRepo{m: map[domain.JobID]*domain.Job{}}
}

var _ ports.JobsRepository = (*FakeJobsRepo)(nil)

func (r *FakeJobsRepo) Create(ctx context.Context, j *domain.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.m[j.ID]; exists {
		return domain.ErrConflict
	}
	clone := *j
	r.m[j.ID] = &clone
	return nil
}

func (r *FakeJobsRepo) Get(ctx context.Context, id domain.JobID) (*domain.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.m[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	clone := *j
	return &clone, nil
}

func (r *FakeJobsRepo) GetForUser(ctx context.Context, id domain.JobID, userID domain.UserID) (*domain.Job, error) {
	j, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if j.UserID != userID {
		return nil, domain.ErrNotFound
	}
	return j, nil
}

func (r *FakeJobsRepo) ListForUser(ctx context.Context, userID domain.UserID, limit int) ([]*domain.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.Job
	for _, j := range r.m {
		if j.UserID == userID {
			clone := *j
			out = append(out, &clone)
		}
	}
	sort.Slice(out, func(i, k int) bool { return out[i].CreatedAt.After(out[k].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *FakeJobsRepo) ListByStatus(ctx context.Context, status domain.JobStatus) ([]*domain.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.Job
	for _, j := range r.m {
		if j.Status == status {
			clone := *j
			out = append(out, &clone)
		}
	}
	return out, nil
}

func (r *FakeJobsRepo) Update(ctx context.Context, j *domain.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.m[j.ID]; !ok {
		return domain.ErrNotFound
	}
	clone := *j
	r.m[j.ID] = &clone
	return nil
}

func (r *FakeJobsRepo) CountActiveForUser(ctx context.Context, userID domain.UserID) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, j := range r.m {
		if j.UserID == userID && (j.Status == domain.JobStatusQueued || j.Status == domain.JobStatusRunning) {
			n++
		}
	}
	return n, nil
}

func (r *FakeJobsRepo) CountActiveGlobal(ctx context.Context) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, j := range r.m {
		if j.Status == domain.JobStatusQueued || j.Status == domain.JobStatusRunning {
			n++
		}
	}
	return n, nil
}
```

- [ ] **Step 2: Verify it compiles + satisfies the port (the `var _` line above)**

```bash
go build ./core/testutil/...
```

Expected: exit 0. (If the interface check failed, you'd see a compile error here.)

- [ ] **Step 3: Commit**

```bash
git add core/testutil/fake_jobs_repo.go
git commit -m "test: add FakeJobsRepo"
```

---

### Task 16: FakeSecretsRepo + FakeAPIKeysRepo

**Files:**
- Create: `core/testutil/fake_secrets_repo.go`
- Create: `core/testutil/fake_api_keys_repo.go`

- [ ] **Step 1: Write `fake_secrets_repo.go`**

```go
// core/testutil/fake_secrets_repo.go
package testutil

import (
	"context"
	"sync"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type FakeSecretsRepo struct {
	mu sync.Mutex
	m  map[domain.UserID]domain.AnthropicCreds
}

func NewFakeSecretsRepo() *FakeSecretsRepo {
	return &FakeSecretsRepo{m: map[domain.UserID]domain.AnthropicCreds{}}
}

var _ ports.SecretsRepository = (*FakeSecretsRepo)(nil)

func (r *FakeSecretsRepo) SetAnthropicCreds(ctx context.Context, userID domain.UserID, c domain.AnthropicCreds) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[userID] = c
	return nil
}

func (r *FakeSecretsRepo) GetAnthropicCreds(ctx context.Context, userID domain.UserID) (domain.AnthropicCreds, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.m[userID]
	if !ok {
		return domain.AnthropicCreds{}, domain.ErrNotFound
	}
	return c, nil
}

func (r *FakeSecretsRepo) DeleteAnthropicCreds(ctx context.Context, userID domain.UserID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.m[userID]; !ok {
		return domain.ErrNotFound
	}
	delete(r.m, userID)
	return nil
}
```

- [ ] **Step 2: Write `fake_api_keys_repo.go`**

```go
// core/testutil/fake_api_keys_repo.go
package testutil

import (
	"context"
	"sync"
	"time"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type FakeAPIKeysRepo struct {
	mu sync.Mutex
	m  map[domain.APIKeyID]*domain.APIKey
}

func NewFakeAPIKeysRepo() *FakeAPIKeysRepo {
	return &FakeAPIKeysRepo{m: map[domain.APIKeyID]*domain.APIKey{}}
}

var _ ports.APIKeysRepository = (*FakeAPIKeysRepo)(nil)

func (r *FakeAPIKeysRepo) Create(ctx context.Context, k *domain.APIKey) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.m[k.ID]; ok {
		return domain.ErrConflict
	}
	clone := *k
	r.m[k.ID] = &clone
	return nil
}

func (r *FakeAPIKeysRepo) GetByPrefix(ctx context.Context, prefix string) ([]*domain.APIKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.APIKey
	for _, k := range r.m {
		if k.Prefix == prefix {
			clone := *k
			out = append(out, &clone)
		}
	}
	return out, nil
}

func (r *FakeAPIKeysRepo) ListForUser(ctx context.Context, userID domain.UserID) ([]*domain.APIKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.APIKey
	for _, k := range r.m {
		if k.UserID == userID {
			clone := *k
			out = append(out, &clone)
		}
	}
	return out, nil
}

func (r *FakeAPIKeysRepo) Delete(ctx context.Context, id domain.APIKeyID, userID domain.UserID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k, ok := r.m[id]
	if !ok || k.UserID != userID {
		return domain.ErrNotFound
	}
	delete(r.m, id)
	return nil
}

func (r *FakeAPIKeysRepo) RecordUsed(ctx context.Context, id domain.APIKeyID, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k, ok := r.m[id]
	if !ok {
		return domain.ErrNotFound
	}
	t := at
	k.LastUsedAt = &t
	return nil
}
```

- [ ] **Step 3: Verify both compile**

```bash
go build ./core/testutil/...
```

- [ ] **Step 4: Commit**

```bash
git add core/testutil/fake_secrets_repo.go core/testutil/fake_api_keys_repo.go
git commit -m "test: add FakeSecretsRepo + FakeAPIKeysRepo"
```

---

### Task 17: FakeRunnerService + FakeProviders + FakeWebhookDispatcher

**Files:**
- Create: `core/testutil/fake_runner_service.go`
- Create: `core/testutil/fake_repo_creds_provider.go`
- Create: `core/testutil/fake_anthropic_creds_provider.go`
- Create: `core/testutil/fake_webhook_dispatcher.go`

- [ ] **Step 1: Write `fake_runner_service.go`**

```go
// core/testutil/fake_runner_service.go
package testutil

import (
	"context"
	"fmt"
	"sync"

	"agentic-delegator/core/usecase/ports"
)

// FakeRunnerService records every Start call and exposes Complete to drive
// the onComplete callback from tests deterministically.
type FakeRunnerService struct {
	mu           sync.Mutex
	StartErr     error
	started      map[string]ports.RunnerStartSpec
	alive        map[string]bool
	callbacks    map[string]func(ports.RunnerResult)
	nextCounter  int
	StartedSpecs []ports.RunnerStartSpec // append-only history for assertions
}

func NewFakeRunnerService() *FakeRunnerService {
	return &FakeRunnerService{
		started:   map[string]ports.RunnerStartSpec{},
		alive:     map[string]bool{},
		callbacks: map[string]func(ports.RunnerResult){},
	}
}

var _ ports.RunnerService = (*FakeRunnerService)(nil)

func (r *FakeRunnerService) Start(ctx context.Context, spec ports.RunnerStartSpec, onComplete func(ports.RunnerResult)) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.StartErr != nil {
		return "", r.StartErr
	}
	r.nextCounter++
	id := fmt.Sprintf("ctr_%d", r.nextCounter)
	r.started[id] = spec
	r.alive[id] = true
	r.callbacks[id] = onComplete
	r.StartedSpecs = append(r.StartedSpecs, spec)
	return id, nil
}

func (r *FakeRunnerService) Inspect(ctx context.Context, containerID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.alive[containerID], nil
}

func (r *FakeRunnerService) Stop(ctx context.Context, containerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.alive, containerID)
	return nil
}

// Complete simulates the container with this ID exiting. The recorded
// onComplete callback is invoked synchronously.
func (r *FakeRunnerService) Complete(containerID string, result ports.RunnerResult) {
	r.mu.Lock()
	cb := r.callbacks[containerID]
	delete(r.alive, containerID)
	r.mu.Unlock()
	if cb != nil {
		cb(result)
	}
}
```

- [ ] **Step 2: Write `fake_repo_creds_provider.go`**

```go
// core/testutil/fake_repo_creds_provider.go
package testutil

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type FakeRepoCredsProvider struct {
	Creds domain.GitCreds
	Err   error
}

func NewFakeRepoCredsProvider(c domain.GitCreds) *FakeRepoCredsProvider {
	return &FakeRepoCredsProvider{Creds: c}
}

var _ ports.RepoCredentialsProvider = (*FakeRepoCredsProvider)(nil)

func (p *FakeRepoCredsProvider) For(ctx context.Context, userID domain.UserID, repo string) (domain.GitCreds, error) {
	if p.Err != nil {
		return domain.GitCreds{}, p.Err
	}
	return p.Creds, nil
}
```

- [ ] **Step 3: Write `fake_anthropic_creds_provider.go`**

```go
// core/testutil/fake_anthropic_creds_provider.go
package testutil

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type FakeAnthropicCredsProvider struct {
	Creds domain.AnthropicCreds
	Err   error
}

func NewFakeAnthropicCredsProvider(c domain.AnthropicCreds) *FakeAnthropicCredsProvider {
	return &FakeAnthropicCredsProvider{Creds: c}
}

var _ ports.AnthropicCredentialsProvider = (*FakeAnthropicCredsProvider)(nil)

func (p *FakeAnthropicCredsProvider) For(ctx context.Context, userID domain.UserID) (domain.AnthropicCreds, error) {
	if p.Err != nil {
		return domain.AnthropicCreds{}, p.Err
	}
	return p.Creds, nil
}
```

- [ ] **Step 4: Write `fake_webhook_dispatcher.go`**

```go
// core/testutil/fake_webhook_dispatcher.go
package testutil

import (
	"context"
	"sync"

	"agentic-delegator/core/usecase/ports"
)

type FakeWebhookCall struct {
	URL     string
	Payload []byte
}

type FakeWebhookDispatcher struct {
	mu    sync.Mutex
	Err   error
	Calls []FakeWebhookCall
}

func NewFakeWebhookDispatcher() *FakeWebhookDispatcher {
	return &FakeWebhookDispatcher{}
}

var _ ports.WebhookDispatcher = (*FakeWebhookDispatcher)(nil)

func (d *FakeWebhookDispatcher) Dispatch(ctx context.Context, url string, payload []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.Err != nil {
		return d.Err
	}
	cpy := make([]byte, len(payload))
	copy(cpy, payload)
	d.Calls = append(d.Calls, FakeWebhookCall{URL: url, Payload: cpy})
	return nil
}
```

- [ ] **Step 5: Verify all compile**

```bash
go build ./core/testutil/...
```

- [ ] **Step 6: Commit**

```bash
git add core/testutil/fake_runner_service.go core/testutil/fake_repo_creds_provider.go core/testutil/fake_anthropic_creds_provider.go core/testutil/fake_webhook_dispatcher.go
git commit -m "test: add FakeRunnerService + credential providers + webhook dispatcher"
```

---

## Phase E — Use cases (TDD)

Each use case is a struct with its dependencies as fields, and an `Execute(ctx, input) (*output, error)` method. The composition root in `cmd/*` (Plan 03) constructs each use case with concrete adapters. Tests construct them with the fakes from Phase D.

### Task 18: MintAPIKey use case

**Files:**
- Create: `core/usecase/mint_api_key.go`
- Test: `core/usecase/mint_api_key_test.go`

- [ ] **Step 1: Write the failing test**

```go
// core/usecase/mint_api_key_test.go
package usecase_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

func TestMintAPIKey_returnsPlaintextOnce(t *testing.T) {
	ctx := context.Background()
	clock := testutil.NewFakeClock(time.Unix(1000, 0))
	keys := testutil.NewFakeAPIKeysRepo()

	uc := &usecase.MintAPIKey{
		Keys:  keys,
		IDGen: &testutil.FakeIDGenerator{},
		Clock: clock,
	}

	out, err := uc.Execute(ctx, usecase.MintAPIKeyInput{UserID: "u_1", Name: "laptop"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Plaintext == "" {
		t.Fatalf("plaintext should be returned exactly once")
	}
	if !strings.HasPrefix(out.Plaintext, out.Key.Prefix) {
		t.Fatalf("prefix should match the start of the plaintext")
	}
	if out.Key.UserID != "u_1" || out.Key.Name != "laptop" {
		t.Fatalf("fields not set on stored key")
	}

	stored, _ := keys.GetByPrefix(ctx, out.Key.Prefix)
	if len(stored) != 1 || stored[0].ID != out.Key.ID {
		t.Fatalf("key not stored under its prefix")
	}
}

func TestMintAPIKey_rejectsEmptyName(t *testing.T) {
	uc := &usecase.MintAPIKey{
		Keys:  testutil.NewFakeAPIKeysRepo(),
		IDGen: &testutil.FakeIDGenerator{},
		Clock: testutil.NewFakeClock(time.Unix(1000, 0)),
	}
	_, err := uc.Execute(context.Background(), usecase.MintAPIKeyInput{UserID: "u_1", Name: ""})
	if err == nil {
		t.Fatalf("expected error on empty name")
	}
	if !errorsIs(err, domain.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

// errorsIs is a tiny wrapper used across usecase tests to keep imports tight.
func errorsIs(err, target error) bool {
	return err == target
}
```

- [ ] **Step 2: Run, verify it fails**

```bash
go test ./core/usecase/...
```

Expected: `undefined: usecase.MintAPIKey`.

- [ ] **Step 3: Implement**

```go
// core/usecase/mint_api_key.go
package usecase

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type MintAPIKey struct {
	Keys  ports.APIKeysRepository
	IDGen ports.IDGenerator
	Clock ports.Clock
}

type MintAPIKeyInput struct {
	UserID domain.UserID
	Name   string
}

type MintAPIKeyOutput struct {
	Key       *domain.APIKey
	Plaintext string // only returned once — caller must show user and discard
}

func (uc *MintAPIKey) Execute(ctx context.Context, in MintAPIKeyInput) (*MintAPIKeyOutput, error) {
	if in.UserID == "" || in.Name == "" {
		return nil, domain.ErrInvalidInput
	}

	plain, prefix := uc.IDGen.NewAPIKeyPlaintext()
	// For MVP, store the plaintext directly as the "hash" — bcrypt happens in
	// the Postgres adapter where the real KDF lives (Plan 02 wires it). Tests
	// see a deterministic "hash" so they can assert.
	hash := domain.APIKeyHash([]byte(plain))

	k := domain.NewAPIKey(
		domain.APIKeyID(uc.IDGen.NewAPIKeyID()),
		in.UserID, in.Name, prefix, hash,
		uc.Clock.Now(),
	)
	if err := uc.Keys.Create(ctx, k); err != nil {
		return nil, err
	}
	return &MintAPIKeyOutput{Key: k, Plaintext: plain}, nil
}
```

> **Implementation note:** the real bcrypt step lives in the Postgres adapter (Plan 02). The use case stays adapter-agnostic about KDF choice. The adapter accepts a `domain.APIKey` and re-hashes `Hash` if it looks like plaintext. We'll revisit this seam in Plan 02 Task "API key hashing in Postgres adapter."

- [ ] **Step 4: Run, verify it passes**

```bash
go test ./core/usecase/...
```

- [ ] **Step 5: Commit**

```bash
git add core/usecase/mint_api_key.go core/usecase/mint_api_key_test.go
git commit -m "feat(usecase): mint API key"
```

---

### Task 19: RevokeAPIKey use case

**Files:**
- Create: `core/usecase/revoke_api_key.go`
- Test: `core/usecase/revoke_api_key_test.go`

- [ ] **Step 1: Write the failing test**

```go
// core/usecase/revoke_api_key_test.go
package usecase_test

import (
	"context"
	"testing"
	"time"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

func TestRevokeAPIKey_ok(t *testing.T) {
	ctx := context.Background()
	keys := testutil.NewFakeAPIKeysRepo()
	k := domain.NewAPIKey("k_1", "u_1", "laptop", "agdkey_a", []byte("h"), time.Unix(1000, 0))
	_ = keys.Create(ctx, k)

	uc := &usecase.RevokeAPIKey{Keys: keys}
	if err := uc.Execute(ctx, usecase.RevokeAPIKeyInput{ID: "k_1", UserID: "u_1"}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	list, _ := keys.ListForUser(ctx, "u_1")
	if len(list) != 0 {
		t.Fatalf("expected 0 keys after revoke, got %d", len(list))
	}
}

func TestRevokeAPIKey_otherUserCannot(t *testing.T) {
	ctx := context.Background()
	keys := testutil.NewFakeAPIKeysRepo()
	k := domain.NewAPIKey("k_1", "u_1", "laptop", "agdkey_a", []byte("h"), time.Unix(1000, 0))
	_ = keys.Create(ctx, k)

	uc := &usecase.RevokeAPIKey{Keys: keys}
	err := uc.Execute(ctx, usecase.RevokeAPIKeyInput{ID: "k_1", UserID: "u_2"})
	if err == nil {
		t.Fatalf("expected error on cross-user revoke")
	}
}
```

- [ ] **Step 2: Run, verify it fails**

- [ ] **Step 3: Implement**

```go
// core/usecase/revoke_api_key.go
package usecase

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type RevokeAPIKey struct {
	Keys ports.APIKeysRepository
}

type RevokeAPIKeyInput struct {
	ID     domain.APIKeyID
	UserID domain.UserID
}

func (uc *RevokeAPIKey) Execute(ctx context.Context, in RevokeAPIKeyInput) error {
	if in.ID == "" || in.UserID == "" {
		return domain.ErrInvalidInput
	}
	return uc.Keys.Delete(ctx, in.ID, in.UserID)
}
```

- [ ] **Step 4: Run, verify it passes**

```bash
go test ./core/usecase/...
```

- [ ] **Step 5: Commit**

```bash
git add core/usecase/revoke_api_key.go core/usecase/revoke_api_key_test.go
git commit -m "feat(usecase): revoke API key"
```

---

### Task 20: SetAnthropicCredentials use case

**Files:**
- Create: `core/usecase/set_anthropic_credentials.go`
- Test: `core/usecase/set_anthropic_credentials_test.go`

- [ ] **Step 1: Write the failing test**

```go
// core/usecase/set_anthropic_credentials_test.go
package usecase_test

import (
	"context"
	"testing"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

func TestSetAnthropicCredentials_ok(t *testing.T) {
	ctx := context.Background()
	secrets := testutil.NewFakeSecretsRepo()
	uc := &usecase.SetAnthropicCredentials{Secrets: secrets}

	if err := uc.Execute(ctx, usecase.SetAnthropicCredentialsInput{UserID: "u_1", APIKey: "sk-ant-xxx"}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	got, _ := secrets.GetAnthropicCreds(ctx, "u_1")
	if got.APIKey != "sk-ant-xxx" {
		t.Fatalf("stored creds mismatch: %v", got)
	}
}

func TestSetAnthropicCredentials_rejectsEmpty(t *testing.T) {
	uc := &usecase.SetAnthropicCredentials{Secrets: testutil.NewFakeSecretsRepo()}
	err := uc.Execute(context.Background(), usecase.SetAnthropicCredentialsInput{UserID: "u_1", APIKey: ""})
	if err != domain.ErrInvalidInput {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}
```

- [ ] **Step 2: Run, verify it fails**

- [ ] **Step 3: Implement**

```go
// core/usecase/set_anthropic_credentials.go
package usecase

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type SetAnthropicCredentials struct {
	Secrets ports.SecretsRepository
}

type SetAnthropicCredentialsInput struct {
	UserID domain.UserID
	APIKey string
}

func (uc *SetAnthropicCredentials) Execute(ctx context.Context, in SetAnthropicCredentialsInput) error {
	if in.UserID == "" || in.APIKey == "" {
		return domain.ErrInvalidInput
	}
	return uc.Secrets.SetAnthropicCreds(ctx, in.UserID, domain.AnthropicCreds{APIKey: in.APIKey})
}
```

- [ ] **Step 4: Run, verify it passes**

- [ ] **Step 5: Commit**

```bash
git add core/usecase/set_anthropic_credentials.go core/usecase/set_anthropic_credentials_test.go
git commit -m "feat(usecase): set Anthropic credentials"
```

---

### Task 21: EnqueueJob use case (the central one)

**Files:**
- Create: `core/usecase/enqueue_job.go`
- Test: `core/usecase/enqueue_job_test.go`

- [ ] **Step 1: Write the failing test**

```go
// core/usecase/enqueue_job_test.go
package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

type enqueueDeps struct {
	clock   *testutil.FakeClock
	jobs    *testutil.FakeJobsRepo
	runner  *testutil.FakeRunnerService
	repoCp  *testutil.FakeRepoCredsProvider
	anth    *testutil.FakeAnthropicCredsProvider
	idgen   *testutil.FakeIDGenerator
}

func newEnqueueUC(t *testing.T) (*usecase.EnqueueJob, *enqueueDeps) {
	t.Helper()
	deps := &enqueueDeps{
		clock:  testutil.NewFakeClock(time.Unix(1000, 0)),
		jobs:   testutil.NewFakeJobsRepo(),
		runner: testutil.NewFakeRunnerService(),
		repoCp: testutil.NewFakeRepoCredsProvider(domain.GitCreds{Token: "git-token"}),
		anth:   testutil.NewFakeAnthropicCredsProvider(domain.AnthropicCreds{APIKey: "sk-ant"}),
		idgen:  &testutil.FakeIDGenerator{},
	}
	uc := &usecase.EnqueueJob{
		Jobs:                 deps.jobs,
		RepoCreds:            deps.repoCp,
		AnthropicCreds:       deps.anth,
		Runner:               deps.runner,
		IDGen:                deps.idgen,
		Clock:                deps.clock,
		MaxConcurrentPerUser: 2,
		MaxConcurrentGlobal:  4,
	}
	return uc, deps
}

func validInput() usecase.EnqueueJobInput {
	return usecase.EnqueueJobInput{
		UserID:     "u_1",
		Repo:       "owner/repo",
		BaseBranch: "main",
		WorkBranch: "agentic/x",
		Spec:       domain.SpecSource{Type: domain.SourceTypePath, Value: "specs/x.md"},
		LogPath:    "/tmp/j_1.log",
	}
}

func TestEnqueueJob_happyPath(t *testing.T) {
	ctx := context.Background()
	uc, deps := newEnqueueUC(t)

	out, err := uc.Execute(ctx, validInput())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Status != domain.JobStatusRunning {
		t.Fatalf("status: want running, got %s", out.Status)
	}

	saved, _ := deps.jobs.Get(ctx, out.JobID)
	if saved.ContainerID == "" {
		t.Fatalf("container id not set")
	}
	if len(deps.runner.StartedSpecs) != 1 {
		t.Fatalf("expected 1 runner start, got %d", len(deps.runner.StartedSpecs))
	}
	if deps.runner.StartedSpecs[0].GitCreds.Token != "git-token" {
		t.Fatalf("git creds not threaded through to runner")
	}
}

func TestEnqueueJob_invalidInput(t *testing.T) {
	uc, _ := newEnqueueUC(t)
	_, err := uc.Execute(context.Background(), usecase.EnqueueJobInput{})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

func TestEnqueueJob_concurrencyCapPerUser_staysQueued(t *testing.T) {
	ctx := context.Background()
	uc, deps := newEnqueueUC(t)
	uc.MaxConcurrentPerUser = 1

	// Pre-populate one active job for the same user.
	preexisting := domain.NewJob("j_pre", "u_1", "owner/repo", "main", "agentic/pre", domain.SpecSource{Type: domain.SourceTypePath, Value: "specs/pre.md"}, "", time.Unix(500, 0))
	_ = preexisting.MarkRunning("ctr_pre", time.Unix(600, 0))
	_ = deps.jobs.Create(ctx, preexisting)

	out, err := uc.Execute(ctx, validInput())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Status != domain.JobStatusQueued {
		t.Fatalf("want queued (cap hit), got %s", out.Status)
	}
	if len(deps.runner.StartedSpecs) != 0 {
		t.Fatalf("runner should not have been called when capped")
	}
}

func TestEnqueueJob_repoCredsErrorPropagates(t *testing.T) {
	uc, deps := newEnqueueUC(t)
	deps.repoCp.Err = errors.New("github unreachable")

	_, err := uc.Execute(context.Background(), validInput())
	if err == nil {
		t.Fatalf("expected error from RepoCreds provider")
	}
}

func TestEnqueueJob_runnerStartErrorMarksFailed(t *testing.T) {
	ctx := context.Background()
	uc, deps := newEnqueueUC(t)
	deps.runner.StartErr = errors.New("docker down")

	out, err := uc.Execute(ctx, validInput())
	if err == nil {
		t.Fatalf("expected error from runner")
	}
	// The job should have been persisted as failed.
	if out != nil {
		t.Fatalf("output should be nil on failure")
	}
	jobs, _ := deps.jobs.ListByStatus(ctx, domain.JobStatusFailed)
	if len(jobs) != 1 {
		t.Fatalf("want 1 failed job, got %d", len(jobs))
	}
	if jobs[0].Error == "" {
		t.Fatalf("error reason not recorded")
	}
}
```

- [ ] **Step 2: Run, verify it fails**

```bash
go test ./core/usecase/...
```

- [ ] **Step 3: Implement**

```go
// core/usecase/enqueue_job.go
package usecase

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type EnqueueJob struct {
	Jobs           ports.JobsRepository
	RepoCreds      ports.RepoCredentialsProvider
	AnthropicCreds ports.AnthropicCredentialsProvider
	Runner         ports.RunnerService
	IDGen          ports.IDGenerator
	Clock          ports.Clock

	MaxConcurrentPerUser int // 0 = unlimited
	MaxConcurrentGlobal  int // 0 = unlimited

	// OnComplete is invoked by the runner adapter when a container exits.
	// Plan 03 wires this to the HandleRunnerCompletion use case. For tests,
	// the field can stay nil.
	OnComplete func(ports.RunnerResult)
}

type EnqueueJobInput struct {
	UserID        domain.UserID
	Repo          string
	BaseBranch    string
	WorkBranch    string
	Spec          domain.SpecSource
	ModelOverride string
	LogPath       string
}

type EnqueueJobOutput struct {
	JobID  domain.JobID
	Status domain.JobStatus
}

func (uc *EnqueueJob) Execute(ctx context.Context, in EnqueueJobInput) (*EnqueueJobOutput, error) {
	if in.UserID == "" || in.Repo == "" || in.BaseBranch == "" || in.WorkBranch == "" || !in.Spec.Valid() || in.LogPath == "" {
		return nil, domain.ErrInvalidInput
	}

	now := uc.Clock.Now()
	job := domain.NewJob(
		domain.JobID(uc.IDGen.NewJobID()),
		in.UserID,
		in.Repo, in.BaseBranch, in.WorkBranch,
		in.Spec, in.ModelOverride, now,
	)
	job.LogPath = in.LogPath

	if err := uc.Jobs.Create(ctx, job); err != nil {
		return nil, err
	}

	// Concurrency caps: if exceeded, leave queued and return without starting.
	if uc.MaxConcurrentPerUser > 0 {
		n, err := uc.Jobs.CountActiveForUser(ctx, in.UserID)
		if err != nil {
			return nil, err
		}
		if n > uc.MaxConcurrentPerUser {
			return &EnqueueJobOutput{JobID: job.ID, Status: job.Status}, nil
		}
	}
	if uc.MaxConcurrentGlobal > 0 {
		n, err := uc.Jobs.CountActiveGlobal(ctx)
		if err != nil {
			return nil, err
		}
		if n > uc.MaxConcurrentGlobal {
			return &EnqueueJobOutput{JobID: job.ID, Status: job.Status}, nil
		}
	}

	gitCreds, err := uc.RepoCreds.For(ctx, in.UserID, in.Repo)
	if err != nil {
		return nil, err
	}
	anth, err := uc.AnthropicCreds.For(ctx, in.UserID)
	if err != nil {
		return nil, err
	}

	spec := ports.RunnerStartSpec{
		JobID:      job.ID,
		Repo:       in.Repo,
		BaseBranch: in.BaseBranch,
		WorkBranch: in.WorkBranch,
		Spec:       in.Spec,
		GitCreds:   gitCreds,
		Anthropic:  anth,
		Model:      in.ModelOverride,
		LogPath:    in.LogPath,
	}

	containerID, startErr := uc.Runner.Start(ctx, spec, uc.OnComplete)
	if startErr != nil {
		_ = job.MarkFailed(startErr.Error(), uc.Clock.Now())
		_ = uc.Jobs.Update(ctx, job)
		return nil, startErr
	}

	if err := job.MarkRunning(containerID, uc.Clock.Now()); err != nil {
		return nil, err
	}
	if err := uc.Jobs.Update(ctx, job); err != nil {
		return nil, err
	}
	return &EnqueueJobOutput{JobID: job.ID, Status: job.Status}, nil
}
```

- [ ] **Step 4: Run, verify it passes**

```bash
go test ./core/usecase/...
```

- [ ] **Step 5: Commit**

```bash
git add core/usecase/enqueue_job.go core/usecase/enqueue_job_test.go
git commit -m "feat(usecase): enqueue job"
```

---

### Task 22: GetJob use case

**Files:**
- Create: `core/usecase/get_job.go`
- Test: `core/usecase/get_job_test.go`

- [ ] **Step 1: Write the failing test**

```go
// core/usecase/get_job_test.go
package usecase_test

import (
	"context"
	"testing"
	"time"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

func TestGetJob_ownerCanRead(t *testing.T) {
	ctx := context.Background()
	jobs := testutil.NewFakeJobsRepo()
	j := domain.NewJob("j_1", "u_1", "o/r", "main", "agentic/x", domain.SpecSource{Type: domain.SourceTypePath, Value: "specs/x.md"}, "", time.Unix(1000, 0))
	_ = jobs.Create(ctx, j)

	uc := &usecase.GetJob{Jobs: jobs}
	got, err := uc.Execute(ctx, usecase.GetJobInput{JobID: "j_1", UserID: "u_1"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got.ID != "j_1" {
		t.Fatalf("wrong job returned")
	}
}

func TestGetJob_nonOwnerSees404(t *testing.T) {
	ctx := context.Background()
	jobs := testutil.NewFakeJobsRepo()
	j := domain.NewJob("j_1", "u_1", "o/r", "main", "agentic/x", domain.SpecSource{Type: domain.SourceTypePath, Value: "specs/x.md"}, "", time.Unix(1000, 0))
	_ = jobs.Create(ctx, j)

	uc := &usecase.GetJob{Jobs: jobs}
	_, err := uc.Execute(ctx, usecase.GetJobInput{JobID: "j_1", UserID: "u_2"})
	if err != domain.ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run, verify it fails**

- [ ] **Step 3: Implement**

```go
// core/usecase/get_job.go
package usecase

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type GetJob struct {
	Jobs ports.JobsRepository
}

type GetJobInput struct {
	JobID  domain.JobID
	UserID domain.UserID
}

func (uc *GetJob) Execute(ctx context.Context, in GetJobInput) (*domain.Job, error) {
	if in.JobID == "" || in.UserID == "" {
		return nil, domain.ErrInvalidInput
	}
	return uc.Jobs.GetForUser(ctx, in.JobID, in.UserID)
}
```

- [ ] **Step 4: Run, verify it passes**

- [ ] **Step 5: Commit**

```bash
git add core/usecase/get_job.go core/usecase/get_job_test.go
git commit -m "feat(usecase): get job with owner check"
```

---

### Task 23: ListJobs use case

**Files:**
- Create: `core/usecase/list_jobs.go`
- Test: `core/usecase/list_jobs_test.go`

- [ ] **Step 1: Write the failing test**

```go
// core/usecase/list_jobs_test.go
package usecase_test

import (
	"context"
	"testing"
	"time"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

func TestListJobs_returnsOnlyOwner(t *testing.T) {
	ctx := context.Background()
	jobs := testutil.NewFakeJobsRepo()
	_ = jobs.Create(ctx, domain.NewJob("j_1", "u_1", "o/r", "main", "agentic/a", domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "", time.Unix(1000, 0)))
	_ = jobs.Create(ctx, domain.NewJob("j_2", "u_2", "o/r", "main", "agentic/b", domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "", time.Unix(2000, 0)))
	_ = jobs.Create(ctx, domain.NewJob("j_3", "u_1", "o/r", "main", "agentic/c", domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "", time.Unix(3000, 0)))

	uc := &usecase.ListJobs{Jobs: jobs}
	got, err := uc.Execute(ctx, usecase.ListJobsInput{UserID: "u_1", Limit: 50})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 jobs for u_1, got %d", len(got))
	}
	// Most recent first.
	if got[0].ID != "j_3" {
		t.Fatalf("want j_3 first, got %s", got[0].ID)
	}
}
```

- [ ] **Step 2: Run, verify it fails**

- [ ] **Step 3: Implement**

```go
// core/usecase/list_jobs.go
package usecase

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type ListJobs struct {
	Jobs ports.JobsRepository
}

type ListJobsInput struct {
	UserID domain.UserID
	Limit  int // 0 = default 50
}

func (uc *ListJobs) Execute(ctx context.Context, in ListJobsInput) ([]*domain.Job, error) {
	if in.UserID == "" {
		return nil, domain.ErrInvalidInput
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 50
	}
	return uc.Jobs.ListForUser(ctx, in.UserID, limit)
}
```

- [ ] **Step 4: Run, verify it passes**

- [ ] **Step 5: Commit**

```bash
git add core/usecase/list_jobs.go core/usecase/list_jobs_test.go
git commit -m "feat(usecase): list jobs scoped to owner"
```

---

### Task 24: HandleRunnerCompletion use case

**Files:**
- Create: `core/usecase/handle_runner_completion.go`
- Test: `core/usecase/handle_runner_completion_test.go`

- [ ] **Step 1: Write the failing test**

```go
// core/usecase/handle_runner_completion_test.go
package usecase_test

import (
	"context"
	"testing"
	"time"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
	"agentic-delegator/core/usecase/ports"
)

func setupRunningJob(t *testing.T) (*testutil.FakeJobsRepo, *testutil.FakeClock) {
	t.Helper()
	clock := testutil.NewFakeClock(time.Unix(1000, 0))
	jobs := testutil.NewFakeJobsRepo()
	j := domain.NewJob("j_1", "u_1", "o/r", "main", "agentic/x", domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "", clock.Now())
	_ = j.MarkRunning("ctr_a", clock.Now())
	_ = jobs.Create(context.Background(), j)
	return jobs, clock
}

func TestHandleRunnerCompletion_success(t *testing.T) {
	ctx := context.Background()
	jobs, clock := setupRunningJob(t)
	clock.Advance(30 * time.Second)

	uc := &usecase.HandleRunnerCompletion{Jobs: jobs, Clock: clock}
	err := uc.Execute(ctx, ports.RunnerResult{JobID: "j_1", ExitCode: 0, PRURL: "https://example/pr/1"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	saved, _ := jobs.Get(ctx, "j_1")
	if saved.Status != domain.JobStatusSucceeded {
		t.Fatalf("want succeeded, got %s", saved.Status)
	}
	if saved.PRURL != "https://example/pr/1" {
		t.Fatalf("pr_url not set")
	}
}

func TestHandleRunnerCompletion_failure(t *testing.T) {
	ctx := context.Background()
	jobs, clock := setupRunningJob(t)
	clock.Advance(30 * time.Second)

	uc := &usecase.HandleRunnerCompletion{Jobs: jobs, Clock: clock}
	err := uc.Execute(ctx, ports.RunnerResult{JobID: "j_1", ExitCode: 2, Error: "compilation failed"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	saved, _ := jobs.Get(ctx, "j_1")
	if saved.Status != domain.JobStatusFailed {
		t.Fatalf("want failed, got %s", saved.Status)
	}
	if saved.Error != "compilation failed" {
		t.Fatalf("error not recorded")
	}
}

func TestHandleRunnerCompletion_unknownJob(t *testing.T) {
	uc := &usecase.HandleRunnerCompletion{
		Jobs:  testutil.NewFakeJobsRepo(),
		Clock: testutil.NewFakeClock(time.Unix(1000, 0)),
	}
	err := uc.Execute(context.Background(), ports.RunnerResult{JobID: "j_nope", ExitCode: 0})
	if err != domain.ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run, verify it fails**

- [ ] **Step 3: Implement**

```go
// core/usecase/handle_runner_completion.go
package usecase

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type HandleRunnerCompletion struct {
	Jobs  ports.JobsRepository
	Clock ports.Clock
}

func (uc *HandleRunnerCompletion) Execute(ctx context.Context, res ports.RunnerResult) error {
	if res.JobID == "" {
		return domain.ErrInvalidInput
	}

	job, err := uc.Jobs.Get(ctx, res.JobID)
	if err != nil {
		return err
	}

	now := uc.Clock.Now()
	if res.ExitCode == 0 {
		if err := job.MarkSucceeded(res.PRURL, now); err != nil {
			return err
		}
	} else {
		reason := res.Error
		if reason == "" {
			reason = "runner exited with non-zero code"
		}
		if err := job.MarkFailed(reason, now); err != nil {
			return err
		}
	}
	return uc.Jobs.Update(ctx, job)
}
```

- [ ] **Step 4: Run, verify it passes**

- [ ] **Step 5: Commit**

```bash
git add core/usecase/handle_runner_completion.go core/usecase/handle_runner_completion_test.go
git commit -m "feat(usecase): handle runner completion"
```

---

### Task 25: ReattachRunningJobs use case (startup recovery)

**Files:**
- Create: `core/usecase/reattach_running_jobs.go`
- Test: `core/usecase/reattach_running_jobs_test.go`

- [ ] **Step 1: Write the failing test**

```go
// core/usecase/reattach_running_jobs_test.go
package usecase_test

import (
	"context"
	"testing"
	"time"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

func TestReattachRunningJobs_aliveStays(t *testing.T) {
	ctx := context.Background()
	jobs := testutil.NewFakeJobsRepo()
	runner := testutil.NewFakeRunnerService()
	clock := testutil.NewFakeClock(time.Unix(1000, 0))

	j := domain.NewJob("j_1", "u_1", "o/r", "main", "agentic/x", domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "", clock.Now())
	_ = j.MarkRunning("ctr_alive", clock.Now())
	_ = jobs.Create(ctx, j)

	// Mark the container alive in the runner without going through Start.
	runner.SetAlive("ctr_alive", true)

	uc := &usecase.ReattachRunningJobs{Jobs: jobs, Runner: runner, Clock: clock}
	if err := uc.Execute(ctx); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	got, _ := jobs.Get(ctx, "j_1")
	if got.Status != domain.JobStatusRunning {
		t.Fatalf("alive container's job should stay running, got %s", got.Status)
	}
}

func TestReattachRunningJobs_deadGetsMarkedFailed(t *testing.T) {
	ctx := context.Background()
	jobs := testutil.NewFakeJobsRepo()
	runner := testutil.NewFakeRunnerService()
	clock := testutil.NewFakeClock(time.Unix(1000, 0))

	j := domain.NewJob("j_2", "u_1", "o/r", "main", "agentic/x", domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "", clock.Now())
	_ = j.MarkRunning("ctr_dead", clock.Now())
	_ = jobs.Create(ctx, j)
	// Container is not alive in the runner.

	uc := &usecase.ReattachRunningJobs{Jobs: jobs, Runner: runner, Clock: clock}
	if err := uc.Execute(ctx); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	got, _ := jobs.Get(ctx, "j_2")
	if got.Status != domain.JobStatusFailed {
		t.Fatalf("dead container's job should be failed, got %s", got.Status)
	}
	if got.Error == "" {
		t.Fatalf("error reason should be set")
	}
}
```

> Note: this test references `runner.SetAlive`. Add it in Step 3 below as a small addition to FakeRunnerService.

- [ ] **Step 2: Run, verify it fails**

```bash
go test ./core/usecase/...
```

- [ ] **Step 3: Augment FakeRunnerService**

Add to `core/testutil/fake_runner_service.go` (at the bottom of the file):

```go
// SetAlive forces an entry into the alive map. Used by tests that don't go
// through Start (e.g., startup recovery tests).
func (r *FakeRunnerService) SetAlive(containerID string, alive bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.alive[containerID] = alive
}
```

- [ ] **Step 4: Implement the use case**

```go
// core/usecase/reattach_running_jobs.go
package usecase

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type ReattachRunningJobs struct {
	Jobs   ports.JobsRepository
	Runner ports.RunnerService
	Clock  ports.Clock
}

func (uc *ReattachRunningJobs) Execute(ctx context.Context) error {
	running, err := uc.Jobs.ListByStatus(ctx, domain.JobStatusRunning)
	if err != nil {
		return err
	}
	now := uc.Clock.Now()
	for _, j := range running {
		alive, err := uc.Runner.Inspect(ctx, j.ContainerID)
		if err != nil {
			return err
		}
		if alive {
			continue
		}
		_ = j.MarkFailed("api restarted while runner container gone", now)
		if err := uc.Jobs.Update(ctx, j); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 5: Run, verify it passes**

```bash
go test ./core/usecase/...
```

- [ ] **Step 6: Commit**

```bash
git add core/usecase/reattach_running_jobs.go core/usecase/reattach_running_jobs_test.go core/testutil/fake_runner_service.go
git commit -m "feat(usecase): reattach running jobs at startup"
```

---

### Task 26: DispatchCompletionWebhook use case

**Files:**
- Create: `core/usecase/dispatch_completion_webhook.go`
- Test: `core/usecase/dispatch_completion_webhook_test.go`

- [ ] **Step 1: Write the failing test**

```go
// core/usecase/dispatch_completion_webhook_test.go
package usecase_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

func TestDispatchCompletionWebhook_postsExpectedPayload(t *testing.T) {
	ctx := context.Background()
	disp := testutil.NewFakeWebhookDispatcher()

	uc := &usecase.DispatchCompletionWebhook{Dispatcher: disp}

	j := domain.NewJob("j_1", "u_1", "o/r", "main", "agentic/x", domain.SpecSource{Type: domain.SourceTypePath, Value: "x.md"}, "", time.Unix(1000, 0))
	_ = j.MarkRunning("ctr_a", time.Unix(1100, 0))
	_ = j.MarkSucceeded("https://example/pr/1", time.Unix(1200, 0))

	err := uc.Execute(ctx, usecase.DispatchCompletionWebhookInput{
		URL:     "https://hook.example/dest",
		Job:     j,
		LogTail: "build ok",
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(disp.Calls) != 1 {
		t.Fatalf("want 1 dispatch, got %d", len(disp.Calls))
	}
	var got map[string]any
	if err := json.Unmarshal(disp.Calls[0].Payload, &got); err != nil {
		t.Fatalf("payload not valid json: %v", err)
	}
	if got["event"] != "job.completed" {
		t.Fatalf("event field wrong: %v", got["event"])
	}
	if got["log_tail"] != "build ok" {
		t.Fatalf("log_tail not threaded through: %v", got["log_tail"])
	}
}

func TestDispatchCompletionWebhook_skippedOnEmptyURL(t *testing.T) {
	disp := testutil.NewFakeWebhookDispatcher()
	uc := &usecase.DispatchCompletionWebhook{Dispatcher: disp}
	err := uc.Execute(context.Background(), usecase.DispatchCompletionWebhookInput{URL: ""})
	if err != nil {
		t.Fatalf("empty URL should be a no-op, got %v", err)
	}
	if len(disp.Calls) != 0 {
		t.Fatalf("expected no dispatches for empty URL")
	}
}
```

- [ ] **Step 2: Run, verify it fails**

- [ ] **Step 3: Implement**

```go
// core/usecase/dispatch_completion_webhook.go
package usecase

import (
	"context"
	"encoding/json"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type DispatchCompletionWebhook struct {
	Dispatcher ports.WebhookDispatcher
}

type DispatchCompletionWebhookInput struct {
	URL     string
	Job     *domain.Job
	LogTail string
}

func (uc *DispatchCompletionWebhook) Execute(ctx context.Context, in DispatchCompletionWebhookInput) error {
	if in.URL == "" {
		return nil
	}
	if in.Job == nil {
		return domain.ErrInvalidInput
	}
	payload := map[string]any{
		"event":    "job.completed",
		"job":      in.Job,
		"log_tail": in.LogTail,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return uc.Dispatcher.Dispatch(ctx, in.URL, body)
}
```

- [ ] **Step 4: Run, verify it passes**

- [ ] **Step 5: Commit**

```bash
git add core/usecase/dispatch_completion_webhook.go core/usecase/dispatch_completion_webhook_test.go
git commit -m "feat(usecase): dispatch completion webhook"
```

---

## Phase F — Final verification

### Task 27: Full test + arch-check sweep, tag plan-01 done

- [ ] **Step 1: Run the full test suite, race-enabled**

```bash
make test-race
```

Expected: all packages green, no race detector warnings.

- [ ] **Step 2: Run vet**

```bash
make lint
```

Expected: no output, exit 0.

- [ ] **Step 3: Run the architectural check**

```bash
make arch-check
```

Expected: the tool reports the four components (`domain`, `ports`, `usecase`, `testutil`) and confirms no rule violations. If it fails, the failure message names the offending file + the disallowed dependency.

- [ ] **Step 4: Verify domain has no internal imports**

```bash
go list -deps -test ./core/domain/... | grep -E '^agentic-delegator/' | grep -v '^agentic-delegator/core/domain'
```

Expected: empty output. (Anything other than `agentic-delegator/core/domain` means the dependency rule is broken.)

- [ ] **Step 5: Verify usecase only depends on domain + ports**

```bash
go list -deps ./core/usecase/... | grep -E '^agentic-delegator/' | grep -vE '^agentic-delegator/core/(domain|usecase)'
```

Expected: empty output.

- [ ] **Step 6: Tag the milestone**

```bash
git tag -a plan-01-done -m "Plan 01: foundation + domain + use cases (in-memory only)"
```

- [ ] **Step 7: Final commit (release notes)**

```bash
git add docs/plans/2026-05-21-01-foundation-domain-usecases.md
git commit -m "docs: plan 01 complete"
```

(If the plan file is already committed from earlier, this commit may be empty — skip it.)

---

## Self-review (done by the plan author, not the engineer)

**Spec coverage:**
- `core/domain/*` entities ✓ (Tasks 5–10)
- Pluggable ports ✓ (Tasks 11–13)
- Use cases listed in spec (EnqueueJob, GetJob, ListJobs, HandleRunnerCompletion, ReattachRunningJobs, MintAPIKey, RevokeAPIKey, SetAnthropicCredentials, DispatchCompletionWebhook) ✓ (Tasks 18–26)
- Clean Architecture dependency rule enforced ✓ (Tasks 3, 27)
- In-memory fakes for tests ✓ (Tasks 14–17)
- `Edition` port: deferred to Plan 03 (where the selfhost edition is wired). Acceptable — the port is part of the `runtime` package, not the inner layers, so it doesn't block Plan 01.

**Placeholder scan:** no TBD/TODO/elided code in any task.

**Type consistency:**
- `JobID`, `UserID`, `APIKeyID` used consistently as `type X string`.
- `JobStatus` constants used the same way in every test.
- `EnqueueJobInput`, `MintAPIKeyInput`, `GetJobInput`, etc. are all `*Input` named after their use case.

**Out-of-scope items intentionally deferred:**
- Real Postgres adapter → Plan 02
- Real Docker runner adapter → Plan 02
- HTTP routes + OpenAPI handlers → Plan 02
- AES-GCM secrets adapter → Plan 02
- `runtime.Edition` interface + selfhost implementation → Plan 03
- Composition root (`cmd/*`) → Plan 03

---

## Execution

Plan complete and saved to [`docs/plans/2026-05-21-01-foundation-domain-usecases.md`](docs/plans/2026-05-21-01-foundation-domain-usecases.md). Two execution options:

1. **Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration via `superpowers:subagent-driven-development`.
2. **Inline Execution** — execute tasks in this session using `superpowers:executing-plans`, with batch checkpoints.

Which approach?
