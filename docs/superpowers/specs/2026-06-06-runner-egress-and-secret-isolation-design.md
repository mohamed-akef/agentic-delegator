# Runner egress restriction (Layer 1) + secret isolation — design

Date: 2026-06-06

## Goal

Two complementary hardening changes to the Docker runner ([core/adapter/docker/runner.go](../../../core/adapter/docker/runner.go) + [runner/entrypoint.sh](../../../runner/entrypoint.sh)):

1. **Egress Layer 1** — block runner-container egress to private / link-local / cloud-metadata ranges (RFC1918 `10/8`, `172.16/12`, `192.168/16`; `169.254.0.0/16` incl. `169.254.169.254`; IPv6 ULA/link-local) while **allowing the public internet**, so `go mod download` / `npm install` / `pip install` and `SPEC_TYPE=url` fetches keep working. Implemented as host firewall rules in Docker's `DOCKER-USER` chain, keyed on a dedicated runner bridge network. A strict github+anthropic SNI allowlist is **not** the goal — that is Layer 2, out of scope.

2. **Secret isolation** — deliver the GitHub token and Anthropic key to the container via a **read-only bind-mounted file dir** instead of `-e` env vars, and authenticate git via an on-demand `GIT_ASKPASS` helper so the token never lands in the clone URL or `.git/config`. The Anthropic key is exported only into the `claude` exec environment.

## Why

Threat model: **one job per container**, so cross-job in-container exposure is not the concern. The concerns are:

- **(a) Host-level visibility** — secrets are passed today as `-e GH_TOKEN=…` / `-e ANTHROPIC_API_KEY=…` (visible via `docker inspect`), and the GH token is embedded in the clone URL (`https://x-access-token:TOKEN@github.com/...`), which **persists into `.git/config` on host disk** inside the bind-mounted workspace.
- **(b) SSRF / metadata exfiltration** — a malicious spec or repo build script can reach `169.254.169.254` (cloud metadata → IAM creds) and internal RFC1918 services.

This work closes the `docker inspect` + clone-URL/`.git/config` persistence vectors of (a), and the private-range portion of (b). It does **not** close in-container read + exfiltrate over **public** egress — see [Residual exposure](#residual-exposure-honest). Both changes ship in one image+orchestrator release because the runner contract changes; see [Backward compatibility](#backward-compatibility).

The two parts are independent and can land in separate commits/PRs; Part 1 is backward-compatible on its own.

---

## Prerequisite (applies to both parts): WorkDirHost must be outside `PrivateTmp`

[deploy/saas/agentic-delegator-saas.service](../../../deploy/saas/agentic-delegator-saas.service) sets `PrivateTmp=true`, and [config.go](../../../core/config/config.go) defaults `WorkDirHost` to `/tmp/agentic-delegator`. The orchestrator shells out to the host `docker` CLI; **dockerd performs bind mounts from the host mount namespace**, which cannot see the service's `PrivateTmp`-isolated `/tmp`. So `-v /tmp/agentic-delegator/<job>:/workspace` mounts the *host's* `/tmp/...` (empty), not the service's private one.

This means the **existing workspace mount is already misconfigured** under the shipped unit; the new secrets mount would inherit the same bug. Fix as a prerequisite:

- Set `AGENTIC_WORK_DIR=/var/lib/agentic-delegator/work` in `/etc/agentic-delegator/env` (the unit's `EnvironmentFile`). `/var/lib/agentic-delegator` is already the unit's `StateDirectory` + `WorkingDirectory`, is a real host path dockerd can see, and survives `ProtectSystem=full`.
- Document that `WorkDirHost` must live outside any `PrivateTmp` namespace. No code change required beyond the env default discussion in Decisions.

---

## Part 1 — Egress restriction (Layer 1)

### Approach

One **shared** user-defined bridge network for all runners, with a **pinned subnet** and **pinned bridge name**, plus static `DOCKER-USER` DROP rules keyed on that subnet. Rejected alternatives: `docker0` (shared with all workloads); per-job networks (privileged `iptables` mutation on the hot path; rule-leak risk; Docker recycles subnets so a stale rule could mis-target a later job).

Because the rules match `-s 172.31.255.0/24`, only runner containers are affected; other Docker workloads are untouched. Rules are written **once** by the installer; the orchestrator hot path only appends `--network`/`--dns` flags and never runs `iptables`.

### Network creation (installer, once)

```
docker network create \
  --driver bridge \
  --subnet 172.31.255.0/24 \
  --gateway 172.31.255.1 \
  --opt com.docker.network.bridge.name=br-runner \
  --opt com.docker.network.bridge.enable_ip_masquerade=true \
  runner-net
```

- Pinned `--subnet` → the rule `-s 172.31.255.0/24` is stable.
- Pinned `com.docker.network.bridge.name=br-runner` → stable (Docker otherwise auto-names bridges `br-<12hex>`).
- `172.31/16` avoids `docker0`'s default `172.17/16`. Do **not** enable IPv6 (no `--ipv6`) so there's no v6 egress path to police.

### DOCKER-USER rules (filter table)

`DOCKER-USER` is jumped from `FORWARD` before Docker's own chains, evaluated top-to-bottom. Use **DROP, not REJECT** — black-holing (curl times out, exit 28) is the desired SSRF posture; REJECT leaks "host is filtering."

```
# 0) Allow established/related return traffic for runner containers.
#    Defensive hygiene only — with the destination-scoped DROPs below, return
#    traffic (-s public -d 172.31.255.0/24) matches no DROP anyway. This rule
#    becomes load-bearing if a broader/default-deny rule is ever added (Layer 2).
iptables -I DOCKER-USER 1 -s 172.31.255.0/24 -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN

# 1) DROP egress from the runner subnet to private + link-local + metadata ranges.
iptables -A DOCKER-USER -s 172.31.255.0/24 -d 10.0.0.0/8     -j DROP
iptables -A DOCKER-USER -s 172.31.255.0/24 -d 172.16.0.0/12  -j DROP
iptables -A DOCKER-USER -s 172.31.255.0/24 -d 192.168.0.0/16 -j DROP
iptables -A DOCKER-USER -s 172.31.255.0/24 -d 169.254.0.0/16 -j DROP   # cloud metadata
# 100.64.0.0/10 (CGNAT) is DROPPED OPTIONALLY — commented OFF by default because
# some carrier-NAT / cloud setups use it for legitimate endpoints. Opt-in only.
# iptables -A DOCKER-USER -s 172.31.255.0/24 -d 100.64.0.0/10 -j DROP
```

Notes:

- The runner subnet is inside `172.16/12`, so that DROP also blocks **runner-to-runner** and **runner-to-gateway** (free L3 isolation between concurrent jobs; closes the gateway as SSRF surface). Intentional — do **not** add a `-d 172.31.255.0/24 -j RETURN` exception by default.
- **No loopback rule.** The container's `127.0.0.11` embedded-DNS DNAT lives in the *container* netns, never in `DOCKER-USER` (host FORWARD); `169.254/16` is link-local, not loopback. Dropping it does not touch DNS.
- **iptables vs nft:** on Debian 12 / Ubuntu 24.04 `iptables` is the nft shim, which Docker uses by default through Docker 28 and remains default on 29 (`docker info` → `Firewall Backend: iptables`). The installer MUST assert `docker info --format '{{.FirewallBackend}}'` is `iptables` (or `iptables -L DOCKER-USER` exists) and refuse/warn loudly otherwise — the experimental Docker 29+ `nftables` backend has **no** `DOCKER-USER` chain and these rules would silently no-op.

### DNS

Blocking RFC1918 does **not** break in-container DNS: the embedded resolver forwards cache-miss queries from the **host** netns (the forwarded query does not carry the container source IP), so `-s <runner subnet>` rules don't match it. The one real break case is an operator passing `--dns <RFC1918-ip>` — the container would then query that resolver over the bridge and the DROP kills DNS.

**Mitigation:** when the runner network is in use, pin public resolvers (`--dns 1.1.1.1 --dns 1.0.0.1`). This is belt-and-suspenders for the default forwarding case and a real fix for the explicit-RFC1918-`--dns` case. **Crucially, only inject `--dns` when `Network != ""`** — in dev/local/air-gapped runs with no egress network, forcing `1.1.1.1` would break DNS that would otherwise resolve via the host's internal resolver.

### Persistence

- **Daemon restart / network recreate:** Docker recreates the `DOCKER-USER` chain + `FORWARD→DOCKER-USER` jump if absent but does **not** flush rule contents — DROPs survive `systemctl restart docker`. The chain must exist before rules are added (install order: start docker, then apply rules).
- **Host reboot:** re-apply via a systemd oneshot ordered `After=docker.service` (preferred over `netfilter-persistent`, which has boot-ordering hazards where restore runs before `DOCKER-USER` exists).

Keep all custom rules in `DOCKER-USER` (never edit `FORWARD` or set `--iptables=false`). Make the installer idempotent (`iptables -C … || iptables -A …`).

### File-by-file changes (Part 1)

**[core/adapter/docker/runner.go](../../../core/adapter/docker/runner.go)** — `Config` (lines 18–28) gains:

```go
Network string   // runner bridge network; "" => no --network (egress filtering off)
DNS     []string // public resolvers; only emitted when Network != ""
```

Arg construction must be extracted into a **pure, unit-testable** function (REQUIRED, not optional — the current inline build in `Start` does real `os.MkdirAll`/`exec`, so the arg assertions in the test plan are otherwise unrealizable):

```go
func buildRunArgs(cfg Config, spec ports.RunnerStartSpec, jobDir, secretsDir string) []string
```

It appends `--network cfg.Network` and a `--dns` per `cfg.DNS` entry **only when `cfg.Network != ""`**, placed before `cfg.Image` (Image must remain the last non-`EntryOverride` arg). Update the comment block (lines 74–77) to state egress to private ranges is blocked at the host `DOCKER-USER` layer; public egress is intentionally allowed.

**Startup preflight (resolves the default contradiction):** the binary default for `AGENTIC_RUNNER_NETWORK` is **empty** (see Decisions #1), so dev/local/CI work unchanged. When it is **set** (production via the deploy env), `runServe` performs a one-time `docker network inspect <name>` and **fails fast with a clear error** if absent — silently running unfiltered after an operator opted into filtering is a security footgun. (Implemented as a small check in [cmd/agentic-delegator/main.go](../../../cmd/agentic-delegator/main.go) `runServe`, before serving.)

**[core/config/config.go](../../../core/config/config.go)** — new fields:

```go
RunnerNetwork string   // AGENTIC_RUNNER_NETWORK, default ""  (empty = filtering off)
RunnerDNS     []string // AGENTIC_RUNNER_DNS, comma-separated, default "1.1.1.1,1.0.0.1"
```

`RunnerDNS` parsed by comma-split with empty-trim. Neither added to `ValidateForServe` (empty network is a valid disabled state; the installer, not the binary, owns network/firewall creation).

**[cmd/agentic-delegator/main.go](../../../cmd/agentic-delegator/main.go)** — pass `Network: cfg.RunnerNetwork, DNS: cfg.RunnerDNS` into `docker.New(docker.Config{…})`; add the network-inspect preflight described above. No firewall creation in the binary (it stays unprivileged).

### New deploy artifacts (Part 1)

Under a **new `deploy/firewall/` directory**:

- **`setup-network.sh`** — idempotent root script: create `runner-net` if absent (pinned subnet/bridge/masquerade), assert the docker firewall backend is `iptables`, then apply the `DOCKER-USER` rules idempotently. Hardcodes `SUBNET=172.31.255.0/24`, `BR=br-runner`. Includes `--down` to remove rules (and optionally the network).
- **`egress-filter.rules`** — canonical rule list (the DROP CIDRs + leading RETURN, CGNAT commented off, plus a commented IPv6 mirror for operators who enable v6). Documentation + sourced by the script.
- **`runner-egress-firewall.service`** — systemd oneshot, `After=docker.service Requires=docker.service`, `Type=oneshot RemainAfterExit=yes`, `ExecStart=/usr/local/sbin/runner-egress-firewall.sh`, `WantedBy=multi-user.target`.

**[deploy/saas/agentic-delegator-saas.service](../../../deploy/saas/agentic-delegator-saas.service)** — add ordering so the network+rules exist before any job runs (no firewall logic here; the unit runs unprivileged `User=agentic-delegator`):

```ini
After=network-online.target docker.service runner-egress-firewall.service
Wants=network-online.target runner-egress-firewall.service
```

**[.github/workflows/release.yml](../../../.github/workflows/release.yml)** — the release `build` job copies exactly three deploy files into the tarball (lines 37–39). The new SaaS unit now `Wants=`/`After=` a unit and scripts that must ship with it. **Add** `deploy/firewall/*` to that `cp` block (e.g. `mkdir -p dist/firewall && cp deploy/firewall/* dist/firewall/`), or the released tarball references a firewall unit it never shipped.

---

## Part 2 — Secret isolation (file delivery + git credential helper)

### Approach

- **Delivery:** read-only bind-mount of a per-job host secrets dir (`-v <secretsDir>:/run/delegator-secrets:ro`), a **sibling** of `jobDir` under the same `0700` `WorkDirHost` parent. The orchestrator writes two files before `docker run`. tmpfs rejected for delivery (`--mount type=tmpfs` starts empty, can't be pre-populated by the host).
- **Artifacts unchanged:** `.pr-url` / `.notification-webhook` keep round-tripping on the existing read-write `/workspace` mount. Secrets in (`:ro`), artifacts out (`rw`), two distinct host paths.
- **git auth:** clone over a clean `https://github.com/owner/repo.git` URL; supply the token via a transient `GIT_ASKPASS` that `cat`s the mounted file on demand, with `credential.helper=""` so nothing caches. Token never enters the URL, `.git/config`, a persistent env var, or a credential store.
- **gh CLI:** `gh auth login --git-protocol https --hostname github.com --with-token < /run/delegator-secrets/gh-token`. `GH_TOKEN`/`GITHUB_TOKEN` env must be **absent** (when set they take precedence and disable gh's stored-cred path).
- **Anthropic key:** exported only into the claude exec env: `ANTHROPIC_API_KEY="$(cat …/anthropic-key)" claude …`.

### Permission model (load-bearing)

The container is uid 0 but with `--cap-drop=ALL` has **no `CAP_DAC_OVERRIDE`**, so the DAC check gives it no special treatment and resolves owner → group → other by first match.

- **Root orchestrator** (files `0:0`, container uid 0): owner class → `0400` readable.
- **Non-root orchestrator** (the shipped SaaS unit runs as the system user `agentic-delegator`, *any* non-root uid; files owned by it, container uid 0 differs, no shared group): falls to **other** class → `0400`/`0600`/`0640` are **unreadable** by the container.

**Modes:** secrets **dir** `0700`, secret **files** `0644`, each set with an explicit `os.Chmod` after create (`os.MkdirAll`/`os.WriteFile` are umask-subject — mirror the existing `jobDir` chmod dance). `0644` is uid-agnostic (owner-read for root, other-read for any non-root orchestrator). The world-read bit on the leaf is safe because host exposure is gated by the `0700` parent (other host users can't traverse it); the container reaches the leaf directly via the bind mount, which is why it needs other-read. (chown-to-`0:0` rejected — needs root/`CAP_CHOWN`.)

### File-by-file changes (Part 2)

**[core/adapter/docker/runner.go](../../../core/adapter/docker/runner.go)**

- `Start`: after `jobDir`, create the sibling secrets dir and write both files:
  ```go
  secretsDir := filepath.Join(r.cfg.WorkDirHost, string(spec.JobID)+".secrets")
  os.MkdirAll(secretsDir, 0o700); os.Chmod(secretsDir, 0o700)
  // writeSecret: os.WriteFile(p, []byte(val), 0o644) then os.Chmod(p, 0o644); raw, no newline
  writeSecret("gh-token", spec.GitCreds.Token)
  writeSecret("anthropic-key", spec.Anthropic.APIKey)
  ```
- Arg slice: **remove** `-e GH_TOKEN=…` and `-e ANTHROPIC_API_KEY=…` (keep `JOB_ID`, `REPO`, `BASE_BRANCH`, `WORK_BRANCH`, `MODEL_OVERRIDE`, `SPEC_TYPE`, `SPEC_VALUE`). Add `-v secretsDir:/run/delegator-secrets:ro` alongside the workspace mount.
- **Cleanup parity:** every error-return path in `Start` after `secretsDir` is created must `os.RemoveAll(secretsDir)` (and, fixing a pre-existing leak, `os.RemoveAll(jobDir)`) because `supervise` never runs if `docker run` fails. Thread `secretsDir` through the `supervise(containerID, jobDir, logPath, jobID, …)` signature exactly as `jobDir` is threaded, and add `os.RemoveAll(secretsDir)` next to the existing `os.RemoveAll(jobDir)` (line 155) so the timeout-kill path also cleans it. No `shred` (false assurance on CoW/SSD; tokens are short-lived).

**Orphan sweep (handles the reattach leak):** [reattach_running_jobs.go](../../../core/usecase/reattach_running_jobs.go) `continue`s past still-alive jobs, so for a job spanning an orchestrator restart, `supervise` never re-runs and its `secretsDir`/`jobDir` are never cleaned — leaving plaintext tokens on disk past the intended lifetime. Add a **best-effort startup sweep** in `runServe` (after reattach): list `WorkDirHost/*.secrets`, and `os.RemoveAll` any whose job ID is not currently in `running` status. This bounds orphaned-secret lifetime to "until next restart of a completed job's host" at worst. The [Residual exposure](#residual-exposure-honest) section qualifies the remaining window for jobs still genuinely running across the restart.

**[runner/entrypoint.sh](../../../runner/entrypoint.sh)** (runs under `set -euo pipefail`)

- Remove the `: "${GH_TOKEN:?}"` / `: "${ANTHROPIC_API_KEY:?}"` guards (lines 8–9). Add a fail-fast preflight:
  ```bash
  SECRETS_DIR=/run/delegator-secrets
  GH_TOKEN_FILE="$SECRETS_DIR/gh-token"; ANTHROPIC_KEY_FILE="$SECRETS_DIR/anthropic-key"
  [ -r "$GH_TOKEN_FILE" ] && [ -r "$ANTHROPIC_KEY_FILE" ] || { echo "missing secrets mount"; exit 3; }
  ```
- Install askpass, disable credential storage, clone clean URL (replaces the token-in-URL clone):
  ```bash
  cat > /tmp/askpass.sh <<EOF
  #!/usr/bin/env bash
  case "\$1" in *Username*) printf '%s' "x-access-token";; *Password*) printf '%s' "\$(cat $GH_TOKEN_FILE)";; esac
  EOF
  chmod 0700 /tmp/askpass.sh; export GIT_ASKPASS=/tmp/askpass.sh
  git config --global credential.helper ""
  git clone "https://github.com/${REPO}.git" repo
  ```
  `GIT_ASKPASS` is inherited by the subsequent fetch/checkout/push.
- Authenticate gh from the file, and **fail fast on its exit status** (it validates the token against `api.github.com` before writing `~/.config/gh/hosts.yml`; a 401 here would otherwise leave every later `gh pr create` silently unauthenticated):
  ```bash
  gh auth login --git-protocol https --hostname github.com --with-token < "$GH_TOKEN_FILE" \
    || { echo "gh auth failed"; exit 4; }
  ```
  This is a hard network dependency on `api.github.com` (public → allowed by Layer 1) and must run after the firewall is up.
- Drop the `GH_TOKEN="${GH_TOKEN}"` wrapper on the claude line; export the key only there: `ANTHROPIC_API_KEY="$(cat "$ANTHROPIC_KEY_FILE")" claude "${CLAUDE_ARGS[@]}" "$PROMPT"`.

The rest of the entrypoint (per-repo `.agentic-delegator.yml` parsing, `SPEC_TYPE` resolution, `CLAUDE_ARGS`, `.pr-url` safety net) is unchanged.

**[runner/Dockerfile](../../../runner/Dockerfile)** — no new packages (`bash`, `git`, `gh`, `curl`, `ca-certificates` already present; the askpass is generated at runtime). No `git-credential-store` installed — we deliberately keep zero on-disk credential state.

**Ports / DB:** `RunnerStartSpec` is unchanged (`spec.GitCreds`, `spec.Anthropic` still carry the secrets) — only *delivery* changes. No usecase/port edits, no migration. Clean-Architecture boundaries unaffected.

### Residual exposure (honest)

**Closed:** `docker inspect` visibility of both secrets; token persistence to the clone URL / `.git/config`.

**Not eliminated — secrets still touch host disk:** Part 2 *moves* the bytes from the clone URL to the `0644` `secretsDir` files (host disk, for the job's duration) and gh's `~/.config/gh`. It does not make the host disk secret-free. State this plainly; don't claim "closes (a) entirely."

**Remaining:**

1. **In-container read + exfiltrate.** Container-root code (malicious spec / repo build script) can read `/run/delegator-secrets/*` and gh's store and POST them to a **public** endpoint — Layer 1 allows public egress. The mount defends only against host-level `docker inspect`/disk persistence. Includes a trivial variant: `SPEC_TYPE=path` with `SPEC_VALUE=/run/delegator-secrets/gh-token` reads the secret straight into the prompt ([entrypoint.sh](../../../runner/entrypoint.sh) `cat`s the path with no validation). Consider constraining `SPEC_TYPE=path` to repo-relative paths resolved after clone; at minimum it's listed here.
2. **Anthropic key** in the `claude` process `environ` inside the container (gone from `docker inspect` — that's the win).
3. **Log leakage** — `supervise` captures `docker logs` to `logPath` (0600) and a `LogTail` into the webhook; if git/gh/claude echo the token it lands there. Redaction is out of scope.
4. **Orphaned secretsDir across an orchestrator restart** for a job still genuinely running (the startup sweep only reaps non-running jobs). Bounded by the host-tmpfs option below.

The real mitigation for #1 is the out-of-scope Layer-2 SNI proxy, plus short-lived least-scope tokens (`GitCreds.ExpiresAt` already supports short installation-token TTLs — keep them short).

---

## Config / env knobs

| Env var | Default | Purpose |
|---|---|---|
| `AGENTIC_RUNNER_NETWORK` | `""` (empty) | Bridge network attached to each runner (`--network`). Empty = no `--network`, egress filtering off (dev/local/CI). Production deploy env sets `runner-net`; binary fails fast at startup if set-but-absent. |
| `AGENTIC_RUNNER_DNS` | `1.1.1.1,1.0.0.1` | Comma-separated public resolvers. Emitted as `--dns` **only when** `AGENTIC_RUNNER_NETWORK` is non-empty. |
| `AGENTIC_WORK_DIR` | (deploy) `/var/lib/agentic-delegator/work` | Host dir for per-job workspaces + secrets dirs. **Must be outside any `PrivateTmp` namespace** (see Prerequisite). |

Secrets-dir path is derived (`WorkDirHost/<jobID>.secrets`), not an env knob. Firewall subnet/bridge are operator constants in `deploy/firewall/`.

---

## Backward compatibility

**Runner image and orchestrator deploy together** (Part 2 couples the contract):

- New orchestrator + old image → old entrypoint's `: "${GH_TOKEN:?}"` finds no env → fails fast.
- Old orchestrator + new image → new entrypoint's secrets-mount preflight fails (`exit 3`).

Recommend embedding a contract-version label on the image and a clear entrypoint message ("secrets dir contract vN expected; is your orchestrator new enough?") so a lockstep mismatch is diagnosable. `RunnerStartSpec`/ports/DB unchanged → no migration. **Part 1 is independently deployable and backward-compatible**: with `AGENTIC_RUNNER_NETWORK` empty (the default), the runner behaves exactly as today.

---

## Testing plan

**Unit (Go, no docker)**
- `buildRunArgs`: asserts (a) no `-e GH_TOKEN=`/`-e ANTHROPIC_API_KEY=`, (b) contains `-v <secretsDir>:/run/delegator-secrets:ro`, (c) `--network runner-net` + `--dns 1.1.1.1`/`1.0.0.1` when `Network` set, (d) **no** `--network`/`--dns` when `Network == ""`, (e) `Image` is last before `EntryOverride`.
- secrets write: after a faked `Start`, `*.secrets` dir is `0700`, files `0644`, content matches; removed on the `docker run` error path.
- `config.go`: `AGENTIC_RUNNER_NETWORK`/`AGENTIC_RUNNER_DNS` defaults + overrides (comma split, empty handling).
- startup sweep: orphaned `*.secrets` dir for a non-running job is removed; one for a running job is kept.

**Integration ([core/adapter/docker/runner_test.go](../../../core/adapter/docker/runner_test.go), `//go:build integration`)**
- Existing `TestDockerRunner_helloWorldExitsZero` keeps passing unchanged (never consumed the secret env; doesn't assert args). Add an assertion that `secretsDir` is gone after completion.
- **New egress test:** run a curl-capable probe as the container's **main process** (e.g. `Image: curlimages/curl` or busybox `wget`), attached via `Config.Network: "runner-net"`. Assert `res.ExitCode != 0` (timeout) for `http://169.254.169.254/` and `http://10.0.0.1/`, and `== 0` for `https://github.com`. **Gate on `docker network inspect runner-net` succeeding** (skip cleanly in CI without the installer).

**E2E ([test/e2e/harness_test.go](../../../test/e2e/harness_test.go))** — uses `FakeRunnerService`; no real docker, no arg validation → **no change required**. The "mirror runServe wiring" header note doesn't apply (the harness builds `EnqueueJob` with a fake runner, not `docker.New`).

**Manual verification** (from a container on `runner-net`):
```bash
docker run --rm --network runner-net curlimages/curl -sS -m 10 -o /dev/null -w "gh=%{http_code}\n" https://github.com   # expect 200/3xx
docker run --rm --network runner-net curlimages/curl sh -c 'curl -sS -m 5 http://169.254.169.254/; echo exit=$?'        # expect timeout exit=28
docker run --rm --network runner-net alpine nslookup github.com                                                          # DNS works
iptables -L DOCKER-USER -n -v --line-numbers                                                                             # RETURN at line 1, DROPs below
# Secrets: run one real job, then:
docker inspect <ctr> | grep -i token   # nothing
# inside the clone: git config --get remote.origin.url  → clean https URL, no token; .git/config has no token
```
DROP (timeout, exit 28) vs REJECT (instant refused) matters — a `200` to metadata means the DROP is missing or below a `RETURN`.

---

## Out of scope / future

- **Egress Layer 2 (SNI-allowlist proxy).** Layer 1 is L3/IP-based and cannot stop exfiltration to an attacker-controlled **public** host. Layer 2 routes runner egress through a forward proxy allowlisting destinations by SNI (github.com, api.anthropic.com, registries) and denying the rest — the only thing that closes residual #1. Explicit opt-in future; operators must not over-trust Layer 1 as exfil prevention.
- **IPv6 egress filtering.** Default disables IPv6 on `runner-net`. If enabled, mirror every DROP in `ip6tables` for `fc00::/7` + `fe80::/10` (documented in `egress-filter.rules`, not wired).
- **Known incompatibility:** internally-hosted (RFC1918) `SPEC_TYPE=url` spec URLs will be blocked by design. Document it.
- **Log/webhook secret redaction** of `docker logs` / `LogTail`.
- **Host tmpfs for `WorkDirHost`** so secret bytes never hit persistent disk (ops/systemd choice; bounds residual #4).

---

## Decisions (reviewer, please confirm)

1. **`AGENTIC_RUNNER_NETWORK` defaults to empty** (dev/local/CI unaffected; production deploy env sets `runner-net`), and `runServe` **fails fast** if it's set but the network is absent (rather than silently running unfiltered). `--dns` only emitted when the network is set. *(Resolves the draft's default-vs-backward-compat contradiction.)*
2. **WorkDirHost moved out of `PrivateTmp`** to `/var/lib/agentic-delegator/work` via the deploy env — fixes a pre-existing workspace-mount bug and is required for the secrets mount. Confirm the path.
3. **Secret delivery = read-only bind-mounted sibling dir** (`<jobID>.secrets`); files `0644`, dir `0700`; no tmpfs, no copy-in; no `shred`.
4. **git auth = inline `GIT_ASKPASS` + `credential.helper=""`** (no helper file, no `git-credential-store`); **gh auth = `gh auth login --with-token`** with fail-fast on its exit.
5. **Firewall = new `deploy/firewall/` dir + dedicated root systemd oneshot**, packaged into the release tarball (release.yml `cp` block updated); the unprivileged SaaS unit only orders after it.
6. **DROP not REJECT; no loopback rule; DROP covers the runner subnet itself** (gateway intentionally unreachable); **CGNAT `100.64/10` DROP off by default** (opt-in).
7. **Reattach leak handled by a startup orphan sweep** of non-running jobs' `*.secrets` dirs; jobs still running across a restart retain their secrets dir until completion/next sweep (documented residual).
8. **Minimum CLI versions** assumed: `gh auth login --with-token` (stdin) and the current `claude` flag set. Confirm a minimum-version requirement is acceptable.
