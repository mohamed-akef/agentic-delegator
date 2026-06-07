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

## Prerequisite (BLOCKING — applies to both parts): WorkDirHost must be outside `PrivateTmp`

> **This is a pre-existing production bug, not a tidy-up.** Re-ranked from the draft's footnote: under the shipped unit, jobs cannot currently complete end-to-end (artifact round-trip is broken — see below). It must land **before** either part and be verified against a real `PrivateTmp` deployment.

[deploy/saas/agentic-delegator-saas.service](../../../deploy/saas/agentic-delegator-saas.service) sets `PrivateTmp=true`, and [config.go](../../../core/config/config.go) defaults `WorkDirHost` to `/tmp/agentic-delegator`. The orchestrator shells out to the host `docker` CLI; **dockerd performs bind mounts from the host mount namespace**, which cannot see the service's `PrivateTmp`-isolated `/tmp`. So `-v /tmp/agentic-delegator/<job>:/workspace` resolves to the *host's* `/tmp/...` — a different directory from the one the orchestrator's Go process (in the service's private-`/tmp` namespace) created and later reads.

**Consequence (code-trace; confirm on a live host):** the container writes `.pr-url` / `.notification-webhook` into the host-`/tmp` mount, but the orchestrator reads them back from its *private*-`/tmp` `jobDir` ([runner.go](../../../core/adapter/docker/runner.go) L148-149) — a different path — so **every job under the shipped unit reports no PR URL and the artifact round-trip silently fails**. Dev/CI/integration tests use `t.TempDir()` *without* the systemd unit, which is why this has not surfaced. The new secrets mount would inherit the identical bug.

Fix as a hard precondition:

- Set `AGENTIC_WORK_DIR=/var/lib/agentic-delegator/work` in `/etc/agentic-delegator/env` (the unit's `EnvironmentFile`). `/var/lib/agentic-delegator` is already the unit's `StateDirectory` + `WorkingDirectory`, is a real host path dockerd can see, and survives `ProtectSystem=full`.
- **`LogDir` rides on `WorkDirHost`.** [config.go](../../../core/config/config.go) defaults `LogDir = WorkDirHost + "/logs"` (`AGENTIC_LOG_DIR` overrides), so relocating `AGENTIC_WORK_DIR` co-relocates logs. Intended, but call it out, and scope the orphan sweep strictly to `*.secrets` children so the `logs/` sibling can never match (it does not — verified).
- **Ensure `WorkDirHost` itself is `0700`.** `New()` does `os.MkdirAll(cfg.WorkDirHost, 0o700)` ([runner.go](../../../core/adapter/docker/runner.go) L48), which is umask-safe — *but `MkdirAll` does not tighten a pre-existing directory*. If an operator pre-creates `AGENTIC_WORK_DIR` world-traversable, the `0644` secret leaves (Part 2) become host-readable. Add an explicit `os.Chmod(cfg.WorkDirHost, 0o700)` in `New()` (mirroring the `jobDir` dance) so the host-exposure gate holds regardless of how the dir was created.
- Document that `WorkDirHost` must live outside any `PrivateTmp` namespace. The relocation is env-only; no orchestrator code change beyond the `Chmod` above.

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
- **iptables vs nft:** on Debian 12 / Ubuntu 24.04 `iptables` is the nft shim, which Docker uses by default through Docker 28 and remains default on 29 (`docker info` → `Firewall Backend: iptables`). The installer MUST verify the backend is `iptables` and refuse/warn loudly otherwise — the experimental Docker 29+ `nftables` backend has **no** `DOCKER-USER` chain and these rules would silently no-op. **Use the right probe:** `docker info --format '{{.FirewallBackend}}'` renders the *struct* — `{iptables []}` on Docker 29.x (verified on a 29.5.2 host) — so a `[ "$(…)" = iptables ]` equality check falsely fails on every modern host. Either template the sub-field, `docker info --format '{{.FirewallBackend.Driver}}'` (→ bare `iptables`), or — preferred, since it is what actually load-bears — probe that the chain exists: `iptables -L -n DOCKER-USER >/dev/null 2>&1`.

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

> **Single source of truth for the script name:** the worker script is `runner-egress-firewall.sh` everywhere — repo path `deploy/firewall/runner-egress-firewall.sh`, install target `/usr/local/sbin/runner-egress-firewall.sh`, unit `ExecStart`. (The earlier draft called it `setup-network.sh` in one place and `runner-egress-firewall.sh` in the `ExecStart` — that basename mismatch would have produced `status=203/EXEC`. One name only.)

Under a **new `deploy/firewall/` directory**:

- **`runner-egress-firewall.sh`** — idempotent root script: create `runner-net` if absent (pinned subnet/bridge/masquerade), verify the docker firewall backend is `iptables` (via `iptables -L -n DOCKER-USER` existence probe, **not** a `{{.FirewallBackend}}` string-equality — see the iptables-vs-nft note above), then apply the `DOCKER-USER` rules idempotently. Hardcodes `SUBNET=172.31.255.0/24`, `BR=br-runner`. Supports `up` (default) / `down` (remove rules, optionally the network).
- **`egress-filter.rules`** — canonical rule list (the DROP CIDRs + leading RETURN, CGNAT commented off, plus a commented IPv6 mirror for operators who enable v6). Documentation + sourced by the script.
- **`runner-egress-firewall.service`** — systemd oneshot, `After=docker.service Requires=docker.service`, `Type=oneshot RemainAfterExit=yes`, `ExecStart=/usr/local/sbin/runner-egress-firewall.sh up`, `WantedBy=multi-user.target`.

**[deploy/saas/agentic-delegator-saas.service](../../../deploy/saas/agentic-delegator-saas.service)** — the current unit has `After=network-online.target docker.service` / `Wants=network-online.target`. **Replace** those two lines so the network+rules exist before any job runs (no firewall logic here; the unit stays unprivileged `User=agentic-delegator`):

```ini
After=network-online.target docker.service runner-egress-firewall.service
Wants=network-online.target runner-egress-firewall.service
```

Note `Wants=` is a **weak** dependency: if `runner-egress-firewall.service` is missing/disabled, the SaaS unit still starts — **with egress filtering silently absent**. That fail-open is caught only by the `runServe` set-but-absent preflight (when `AGENTIC_RUNNER_NETWORK` is set). Document that enabling the firewall unit is mandatory in production.

**Install path — `deploy/saas/install.sh` (NEW, required; shipping files ≠ installing them).** The repo currently has **no installer**: `deploy/` holds only `saas/` (3 files), and [release.yml](../../../.github/workflows/release.yml) just `cp`s those three into a tarball that an operator untars by hand — nothing copies units into `/etc/systemd/system`, runs `daemon-reload`/`enable`, installs the `/usr/local/sbin/runner-egress-firewall.sh` `ExecStart` target, or runs the script as root. So merely packaging `deploy/firewall/*` leaves the new oneshot uninstalled and the `ExecStart` path absent. Add `deploy/saas/install.sh` (run as root) that:
1. installs the binary to `/usr/local/bin/`, the SaaS unit + `runner-egress-firewall.service` to `/etc/systemd/system/`, and `runner-egress-firewall.sh` to `/usr/local/sbin/` (chmod `0755`);
2. runs `systemctl daemon-reload && systemctl enable --now runner-egress-firewall.service` (which creates the network + rules), then `enable --now` the SaaS unit;
3. is idempotent (safe to re-run on upgrade).
   *(Alternative if an installer is out of scope: document each manual step explicitly in `docs/saas-setup.md`. Either way the gap must be closed — “the installer” cannot be left undefined.)*

**[.github/workflows/release.yml](../../../.github/workflows/release.yml)** — the release `build` job copies exactly three SaaS files into the tarball (`deploy/saas/{agentic-delegator-saas.service,Caddyfile.example,docker-compose.postgres.yml}` at lines 37–39) and tars. **Add** `deploy/firewall/*` **and** `deploy/saas/install.sh` to that block (e.g. `mkdir -p dist/firewall && cp deploy/firewall/* dist/firewall/ && cp deploy/saas/install.sh dist/`), or the tarball references units/scripts it never shipped.

**[docs/saas-setup.md](../../../docs/saas-setup.md)** — the canonical setup guide today runs the binary directly with no systemd install and enumerates env vars by shell `export`. Update it for: the three new env vars (`AGENTIC_RUNNER_NETWORK`, `AGENTIC_RUNNER_DNS`, `AGENTIC_WORK_DIR`/`AGENTIC_LOG_DIR`), the `EnvironmentFile=/etc/agentic-delegator/env` the unit already references (no sample exists in-repo — add one), and the firewall install/enable step.

---

## Part 2 — Secret isolation (file delivery + git credential helper)

### Approach

- **Delivery:** read-only bind-mount of a per-job host secrets dir (`-v <secretsDir>:/run/delegator-secrets:ro`), a **sibling** of `jobDir` under the same `0700` `WorkDirHost` parent (secrets **dir `0711`**, **files `0644`** — see [Permission model](#permission-model-load-bearing--corrected); the `0700` mode in the draft is a blocker). The orchestrator writes two files before `docker run`. tmpfs rejected for delivery (`--mount type=tmpfs` starts empty, can't be pre-populated by the host).
- **Artifacts unchanged:** `.pr-url` / `.notification-webhook` keep round-tripping on the existing read-write `/workspace` mount. Secrets in (`:ro`), artifacts out (`rw`), two distinct host paths.
- **git auth:** clone over a clean `https://github.com/owner/repo.git` URL; supply the token via a transient `GIT_ASKPASS` that `cat`s the mounted file on demand, with `credential.helper=""` so nothing caches. Token never enters the URL, `.git/config`, a persistent env var, or a credential store.
- **gh CLI:** `gh auth login --git-protocol https --hostname github.com --with-token < /run/delegator-secrets/gh-token`. `GH_TOKEN`/`GITHUB_TOKEN` env must be **absent** (when set they take precedence and disable gh's stored-cred path).
- **Anthropic key:** exported only into the claude exec env: `ANTHROPIC_API_KEY="$(cat …/anthropic-key)" claude …`.

### Permission model (load-bearing — CORRECTED)

The container is uid 0 but with `--cap-drop=ALL` has **no `CAP_DAC_OVERRIDE` and no `CAP_DAC_READ_SEARCH`** (verified: `CapEff=0000000000000000`), so the DAC check gives it no special treatment and resolves owner → group → other by first match. The shipped SaaS unit runs as the non-root system user `agentic-delegator`, and the container runs as image-default **uid 0** (`runner.go` passes no `--user`; the Dockerfile has no `USER`). So **the container is in the `other` DAC class** relative to orchestrator-owned files.

Opening `/run/delegator-secrets/gh-token` requires **two** permissions, not one:

1. **SEARCH (execute) on the mounted directory** `/run/delegator-secrets`. A bind/volume mount does **not** bypass this — the mount point's inode *is* the host secrets dir, and a child cannot be resolved without `+x` on it. → the dir must grant `other` the execute bit.
2. **READ on the leaf file.** → the file must grant `other` the read bit.

> **The draft's `0700` secrets dir is a blocker.** With dir `0700`, `other` gets no execute bit, the container cannot traverse into it, the `0644` file is unreachable, and the entrypoint preflight `[ -r … ]` fails → **every production job exits 3** under the shipped (non-root) unit. Empirically reproduced on a native-fs Docker volume (Docker 29.5.2, uid-0 + `--cap-drop=ALL`): `0700` dir → `cat gh-token` = *Permission denied*; a `0000` file in the same `0700` dir is denied identically (proving it's the *directory's* search bit, not the leaf mode). The draft contradicts its own cited precedent — [runner.go](../../../core/adapter/docker/runner.go) L58/69 chmod the workspace `jobDir` to **`0777`** for exactly this reason (comment L61-68: the uid-0/no-caps container must use the bind mount regardless of uid), with host exposure gated by the `0700` `WorkDirHost` parent. *(Note: macOS Docker Desktop's virtiofs fileshare silently ignores guest DAC and makes `0700` falsely "work" — perm tests MUST run on a native Linux fs / named volume.)*

**Corrected modes:** secrets **dir `0711`** (search-only — the container opens the known filenames `gh-token`/`anthropic-key` and never needs to `ls`), secret **files `0644`**, each set with an explicit `os.Chmod` after create (`os.MkdirAll`/`os.WriteFile` are umask-subject — mirror the `jobDir` chmod dance, but to `0711`/`0644`). Verified on native ext4: dir `0711` → the uid-0/no-caps container `cat`s the leaf (rc=0) but cannot `ls` (rc=1).

**Host exposure is gated by the `0700` `WorkDirHost` parent, not the secrets-dir mode.** Other host users can't traverse the `0700` parent, so the world-read `0644` leaves + the `0711` secrets dir introduce no new host exposure — *provided `WorkDirHost` stays `0700`* (see the Prerequisite's explicit-`Chmod` note; `MkdirAll` won't tighten a pre-existing loose dir). (chown-to-`0:0` rejected — needs root/`CAP_CHOWN`.)

### File-by-file changes (Part 2)

**[core/adapter/docker/runner.go](../../../core/adapter/docker/runner.go)**

- `Start`: after `jobDir`, create the sibling secrets dir and write both files:
  ```go
  secretsDir := filepath.Join(r.cfg.WorkDirHost, string(spec.JobID)+".secrets")
  os.MkdirAll(secretsDir, 0o711); os.Chmod(secretsDir, 0o711) // 0711, NOT 0700 — see Permission model
  // writeSecret: os.WriteFile(p, []byte(val), 0o644) then os.Chmod(p, 0o644); raw, no newline
  writeSecret("gh-token", spec.GitCreds.Token)
  writeSecret("anthropic-key", spec.Anthropic.APIKey)
  ```
- Arg slice: **remove** `-e GH_TOKEN=…` and `-e ANTHROPIC_API_KEY=…` (keep `JOB_ID`, `REPO`, `BASE_BRANCH`, `WORK_BRANCH`, `MODEL_OVERRIDE`, `SPEC_TYPE`, `SPEC_VALUE`). Add `-v secretsDir:/run/delegator-secrets:ro` alongside the workspace mount.
- **Cleanup parity:** every error-return path in `Start` after `secretsDir` is created must `os.RemoveAll(secretsDir)` (and, fixing a pre-existing leak, `os.RemoveAll(jobDir)`) because `supervise` never runs if `docker run` fails. Thread `secretsDir` through the `supervise(containerID, jobDir, logPath, jobID, …)` signature exactly as `jobDir` is threaded, and add `os.RemoveAll(secretsDir)` next to the existing `os.RemoveAll(jobDir)` (line 155) so the timeout-kill path also cleans it. No `shred` (false assurance on CoW/SSD; tokens are short-lived).
  - **Cancellation is already covered** (don't add a separate path): `CancelJob` → `Runner.Stop` → `docker kill` ([cancel_job.go](../../../core/usecase/cancel_job.go) L36, [runner.go](../../../core/adapter/docker/runner.go) L186) unblocks the `docker wait` in the still-running `supervise` goroutine, which falls through to the same line-155 `RemoveAll`. Cancel-across-restart is reaped by the orphan sweep (a cancelled job is non-running). Stated so a reviewer needn't re-trace `Stop → wait → RemoveAll`.

**Orphan sweep (handles the reattach leak):** [reattach_running_jobs.go](../../../core/usecase/reattach_running_jobs.go) `continue`s past still-alive jobs, so for a job spanning an orchestrator restart, `supervise` never re-runs and its `secretsDir`/`jobDir` are never cleaned — leaving plaintext tokens on disk past the intended lifetime. Add a **best-effort startup sweep** in `runServe` (after reattach): glob `WorkDirHost/*.secrets` — which matches only the per-job secrets dirs, never the `logs/` sibling under the same parent (verified) — and `os.RemoveAll` any whose job ID is not currently in `running` status (use `Jobs.ListByStatus(running)` to build the keep-set). This bounds orphaned-secret lifetime to "until next restart of a completed job's host" at worst. The [Residual exposure](#residual-exposure-honest) section qualifies the remaining window for jobs still genuinely running across the restart.

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
- Drop the `GH_TOKEN="${GH_TOKEN}"` wrapper on the claude line; export the key only there: `ANTHROPIC_API_KEY="$(cat "$ANTHROPIC_KEY_FILE")" claude "${CLAUDE_ARGS[@]}" "$PROMPT"`. The agent's own `gh pr create` / `git push` **and** the safety-net `gh pr view` ([entrypoint.sh](../../../runner/entrypoint.sh) L90) remain authenticated via the `gh auth login` store + the exported `GIT_ASKPASS`; the `gh auth` fail-fast (`exit 4`) is what guarantees the store exists before `claude` runs. (Closing the reasoning loop: removing the env wrapper does not de-auth the agent.)
- **Constrain `SPEC_TYPE=path` (IN SCOPE — closes a one-line self-bypass of this very change; see [Residual exposure](#residual-exposure-honest) #1).** Today `path) SPEC_TEXT="$(cat "${SPEC_VALUE}")"` cats an arbitrary, caller-controlled absolute path with no validation ([entrypoint.sh](../../../runner/entrypoint.sh) L55), and `SPEC_VALUE` flows unsanitized from the HTTP body ([jobs_handler.go](../../../core/adapter/http/jobs_handler.go) L57 → [enqueue_job.go](../../../core/usecase/enqueue_job.go) L52, which checks only `Spec.Valid()`). Once Part 2 mounts secrets at the fixed path `/run/delegator-secrets/{gh-token,anthropic-key}`, **any job submitter** can set `SPEC_TYPE=path`, `SPEC_VALUE=/run/delegator-secrets/gh-token` and read the token straight into the prompt the model commits/PRs. Enforce the contract [spec.go](../../../core/domain/spec.go) L9 already documents but never checks ("a path inside the target repo"): in the entrypoint, **after clone**, resolve `path` relative to the repo root and reject escape — `realpath`/`filepath.EvalSymlinks` then assert the cleaned path is under `/workspace/repo` (rejects absolute paths, `..`, symlink escape). Optionally also reject absolute `SPEC_TYPE=path` values in `SpecSource.Valid()` as defense-in-depth.

The rest of the entrypoint (per-repo `.agentic-delegator.yml` parsing, `CLAUDE_ARGS`, `.pr-url` safety net) is unchanged.

**[runner/Dockerfile](../../../runner/Dockerfile)** — no new packages: `git`, `gh`, `curl`, `ca-certificates` are explicitly installed; `bash` is present transitively via the `debian:12-slim` base (the runtime-generated askpass's `#!/usr/bin/env bash` depends on that base — pin `bash` in the `apt-get install` line if you want it explicit, defensive against a future minimal-base swap). The askpass is generated at runtime; no `git-credential-store` installed — we deliberately keep zero on-disk credential state. (The `gh-token` file has **three** readers — `gh auth login`, the `GIT_ASKPASS` helper, and the constrained `SPEC_TYPE=path` `cat` — all satisfied by the `0711` dir / `0644` file model.)

**Ports / DB:** `RunnerStartSpec` is unchanged (`spec.GitCreds`, `spec.Anthropic` still carry the secrets) — only *delivery* changes. No usecase/port edits, no migration. Clean-Architecture boundaries unaffected.

### Residual exposure (honest)

**Closed:** `docker inspect` visibility of both secrets; token persistence to the clone URL / `.git/config`.

**Not eliminated — secrets still touch host disk:** Part 2 *moves* the bytes from the clone URL to the `0644` `secretsDir` files (host disk, for the job's duration) and gh's `~/.config/gh`. It does not make the host disk secret-free. State this plainly; don't claim "closes (a) entirely."

**Remaining:**

1. **In-container read + exfiltrate.** Container-root code (malicious spec / repo build script) can read `/run/delegator-secrets/*` and gh's store and POST them to a **public** endpoint — Layer 1 allows public egress. The mount defends only against host-level `docker inspect`/disk persistence. The general case is closed only by the out-of-scope Layer-2 SNI proxy. *(The previously-listed trivial variant — `SPEC_TYPE=path` reading the mounted secret straight into the prompt — is now **fixed in-scope**: see the `SPEC_TYPE=path` containment item in [Part 2 file-by-file](#file-by-file-changes-part-2). Leaving it a mere residual would ship a one-line self-bypass of the isolation.)*
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
| `AGENTIC_WORK_DIR` | (deploy) `/var/lib/agentic-delegator/work` | Host dir for per-job workspaces + secrets dirs. **Must be outside any `PrivateTmp` namespace** (see Prerequisite). Must itself be `0700` (host-exposure gate). |
| `AGENTIC_LOG_DIR` | `${WorkDirHost}/logs` | Per-job log dir ([config.go](../../../core/config/config.go) L73-74). **Co-relocates with `AGENTIC_WORK_DIR`** unless set explicitly — listed so operators aren't surprised when moving `WorkDirHost` also moves logs. |

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
- secrets write: after a faked `Start`, `*.secrets` dir is `0711`, files `0644`, content matches; removed on the `docker run` error path.
- `config.go`: `AGENTIC_RUNNER_NETWORK`/`AGENTIC_RUNNER_DNS` defaults + overrides (comma split, empty handling).
- startup sweep: orphaned `*.secrets` dir for a non-running job is removed; one for a running job is kept.

**Integration ([core/adapter/docker/runner_test.go](../../../core/adapter/docker/runner_test.go), `//go:build integration`)**
- Existing `TestDockerRunner_helloWorldExitsZero` keeps passing unchanged (never consumed the secret env; doesn't assert args). Add an assertion that `secretsDir` is gone after completion.
- **New egress test:** run a **self-bounding** curl probe as the container's **main process** — e.g. `Image: curlimages/curl` with `EntryOverride: ["-sS","-m","5", "<url>"]` (or busybox `wget -T 5`) — and set a short `Config.MaxJobDuration` (e.g. 30s) as a backstop. Assert exit **28** (curl timeout) for `http://169.254.169.254/` and `http://10.0.0.1/`, and exit `0` for `https://github.com`. Assert the **specific** code, not `!= 0`: a DROP-blackholed probe with no `-m` hangs up to `MaxJobDuration` (default 30m), and a bare `!= 0` would also pass on the timeout-kill (exit 124) and hide a missing rule. **Gate on `docker network inspect runner-net` succeeding** (skip cleanly in CI without the installer).
- **New secrets-perms regression test (for the blocker):** create a `*.secrets` dir + `0644` files **owned by a non-root uid** on a **native fs** — a Docker named volume, **not** a macOS-host bind mount (Docker Desktop's virtiofs ignores guest DAC and yields a false pass). Assert a uid-0 `--cap-drop=ALL` container **can** `cat` the leaf with the dir at `0711` and **cannot** at `0700`.

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

## Decisions

> Most of these are **resolved positions** (post-review), not open questions. The two genuinely needing a call are tagged **OPEN**.

1. **`AGENTIC_RUNNER_NETWORK` defaults to empty** (dev/local/CI unaffected; production deploy env sets `runner-net`), and `runServe` **fails fast** if it's set but the network is absent (rather than silently running unfiltered). `--dns` only emitted when the network is set. *(Resolves the draft's default-vs-backward-compat contradiction.)*
2. **WorkDirHost moved out of `PrivateTmp`** to `/var/lib/agentic-delegator/work` via the deploy env — this is a **prod-blocking precondition**, not a tidy-up: under the shipped unit the artifact round-trip is currently broken (see [Prerequisite](#prerequisite-blocking--applies-to-both-parts-workdirhost-must-be-outside-privatetmp)). Lands first; `New()` also gains an explicit `os.Chmod(WorkDirHost, 0o700)`. **OPEN:** confirm the path `/var/lib/agentic-delegator/work`.
3. **Secret delivery = read-only bind-mounted sibling dir** (`<jobID>.secrets`); files `0644`, **dir `0711`** (search-only — *the draft's `0700` is a verified blocker that makes the secrets unreadable by the non-root runner*); host exposure gated by the `0700` `WorkDirHost` parent; no tmpfs, no copy-in, no `shred`.
4. **git auth = inline `GIT_ASKPASS` + `credential.helper=""`** (no helper file, no `git-credential-store`); **gh auth = `gh auth login --with-token`** with fail-fast on its exit.
5. **Firewall = new `deploy/firewall/` dir + dedicated root systemd oneshot**, with a **new `deploy/saas/install.sh`** (or documented manual steps) that actually installs/enables the unit + script — shipping files in the tarball is not installing them. One script name throughout (`runner-egress-firewall.sh`); backend check via the `iptables -L DOCKER-USER` probe, not `{{.FirewallBackend}}` string-equality. The unprivileged SaaS unit only orders after the oneshot (weak `Wants=`, so enabling it is mandatory in prod).
6. **DROP not REJECT; no loopback rule; DROP covers the runner subnet itself** (gateway intentionally unreachable); **CGNAT `100.64/10` DROP off by default** (opt-in).
7. **Reattach leak handled by a startup orphan sweep** of non-running jobs' `*.secrets` dirs (glob scoped so the `logs/` sibling never matches); jobs still running across a restart retain their secrets dir until completion/next sweep (documented residual).
8. **`SPEC_TYPE=path` containment is in-scope** (not a deferred residual): resolve repo-relative after clone and reject escape, closing the one-line read of the just-mounted secret.
9. **Minimum CLI versions** assumed: `gh auth login --with-token` (stdin) and the current `claude` flag set. **OPEN:** confirm a minimum-version requirement is acceptable.
