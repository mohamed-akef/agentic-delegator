# CMA Migration Evaluation — agentic-delegator → Claude Managed Agents

**Date:** 2026-06-09
**Status:** Evaluation (go/no-go decision memo)
**Author:** Akef + Claude

## Why this evaluation exists

External (multi-tenant) developers are uneasy about granting agentic-delegator
access to their **GitHub and Claude accounts**. The GitHub half is already
well-handled (GitHub App, per-repo scoped, ~50-minute installation tokens, never
in the clone URL). The Claude half is weaker: the tenant hands over a long-lived
Anthropic API key, and the runner's own
[secret-isolation spec](2026-06-06-runner-egress-and-secret-isolation-design.md)
admits **residual #1 — in-container read + exfil over public egress — is not
closed** (only a future Layer-2 SNI proxy closes it).

This memo evaluates re-platforming the **execution backend** onto Anthropic
**Claude Managed Agents (CMA, cloud)**, whose design makes *neither* credential
reachable from inside the sandbox — which is the deepest available answer to the
developer concern.

---

## 1. Recommendation — Conditional GO

Re-platform the Docker-runner backend onto **CMA cloud + operator-paid
inference**, gated on a small time-boxed PoC (§10). Keep the entire product
surface (`/delegate` skill, HTTP API, status UI, GitHub App/OAuth, API keys);
swap only what runs the job.

**Why GO:** CMA is purpose-built for exactly this loop (clone → run Claude →
push → PR). With CMA's git proxy + operator-side inference, **neither the GitHub
token nor the Claude credential ever enters the sandbox** — closing the residual
your isolation spec left open — and it lets you *delete* a large amount of
hand-maintained machinery (egress firewall, the whole secret-isolation
workstream, the no-queue-drain bug, in-process supervision). See §4.

**Proceed if:**
- operator-paid inference is acceptable — it is effectively *required* under CMA (§6); **and**
- the PoC (§10) confirms (a) event-stream → existing status UI, and (b) the out-of-band PR-open path.

**Do not migrate the affected tenant segment if:**
- per-tenant BYO-Claude-key billing is mandatory — that is incompatible with being the CMA operator (§6); **or**
- target tenants (banking/fintech) contractually forbid their private repo running on Anthropic-hosted containers and you won't run CMA self-hosted sandboxes (which dilutes the core benefit — §9).

**One line:** adopt **CMA cloud + operator-pays** for the general tenant base;
keep the current Docker runner (or CMA self-hosted) as the path for tenants who
can't put their repo on Anthropic infra.

---

## 2. What's being evaluated (and what is not)

**In scope — replace the execution backend only:**
`core/adapter/docker/*`, `runner/entrypoint.sh` + the runner image, the AES-GCM
secrets store + mount, the egress firewall (`deploy/firewall/*`), and the
in-process supervision goroutine. CMA becomes the thing `EnqueueJob` calls
instead of `Runner.Start()`; the orchestrator drives a CMA session and projects
its event stream into the same `jobs` table + status page you have today
(strangler — §8).

**Out of scope:** billing/plans/teams (still Phase 2+), UI redesign, and the
`/delegate` request contract — all unchanged.

---

## 3. Capability mapping (agentic-delegator → CMA)

Legend: ✅ strong / ✅✅ strong **and** a security or reliability upgrade /
⚠️ friction (achievable, not 1:1).

| agentic-delegator today | CMA primitive | Fit |
|---|---|---|
| Sandboxed runner: clone repo, run Claude headless, isolation | Cloud **environment** + **session**; `github_repository` resource clones the repo; `agent_toolset_20260401` (bash/read/write/edit/grep/glob) does the work | ✅✅ This is literally CMA's purpose |
| GitHub install token mounted into the container | `github_repository.authorization_token` injected by Anthropic-side **git proxy** *after* egress; never in the sandbox | ✅✅ Closes residual #1 (GitHub) |
| Anthropic key mounted + exported to `claude` | Inference runs on the **operator org key** (the session is created by *your* client); no Claude key in the sandbox | ✅✅ Closes residual #1 (Claude) — forces operator-pays (§6) |
| `git push` of the work branch | git proxy handles `git push` from inside the session | ✅ |
| `gh pr create` | **(a)** orchestrator opens the PR via the GitHub REST API reusing your existing App-install-token path; agent writes PR title/body to `/mnt/session/outputs/` — *recommended*. **(b)** in-session GitHub MCP server `create_pull_request` + per-tenant OAuth vault | ✅ (a) reuses proven code · ⚠️ (b) needs MCP + vault + token-type check (§5) |
| Branch continue-or-create (fetch/checkout from base) | agent does it via `bash`/`git`; `checkout:{branch}` on the resource sets the base | ✅ |
| Spec `inline` | kickoff `user.message` (or `user.define_outcome` for a rubric-graded loop) | ✅ |
| Spec `path` (file in repo) | agent `read`s the file after clone | ✅ |
| Spec `url` | agent `web_fetch`, or orchestrator fetches and injects into the message | ✅ |
| `.agentic-delegator.yml` → `model` | model lives on the **agent** config (versioned), not per-session → pre-create one agent per supported model; orchestrator reads the yml and picks at session-create | ⚠️ Friction |
| `.yml` → `system_prompt_append` | system lives on the agent; fold the per-repo append into the **kickoff message** (or a mid-session `role:"system"` message) | ⚠️ Friction |
| `.yml` → `max_turns` | no direct equivalent; use `output_config.task_budget` (beta) or outcome `max_iterations` | ⚠️ Friction |
| `.yml` → `allowed_tools` | `agent_toolset` per-tool enable/disable on the agent, or `sessions.update` tools override (session must be idle) | ⚠️ Friction |
| `.yml` → `notification_webhook` | orchestrator fires it on completion (as today), driven by a CMA webhook (`session.status_idled`/`terminated`) or the SSE stream | ✅ |
| `--dangerously-skip-permissions` (unattended) | permission policy `always_allow` (the default) | ✅ |
| `.pr-url` / `.notification-webhook` file artifacts | session outputs under `/mnt/session/outputs/` → `files.list(scope_id)` / download, or parse the final `agent.message` | ✅ (different mechanism; ~1–3s index lag on `files.list`) |
| Per-job logs + status-page tail | CMA **event stream** (SSE) → project into your `jobs` row + log store; status page unchanged | ✅✅ Durable + reconnectable |
| Job states `queued/running/succeeded/failed/cancelled` | map to the session lifecycle (`rescheduling/running/idle/terminated`) + your own DB row | ✅ |
| **No queue drain** — capped jobs sit in `queued` forever (no worker ever starts them) | CMA server-manages scheduling; lean on org RPM (300 creates/min) + simple admission control instead of the buggy inline cap | ✅✅ Bug removed |
| In-process supervision goroutine (lost on restart; orphans marked failed) | sessions are durable + server-side; reconnect via stream-consolidation | ✅✅ Fragility removed |
| Cancel → `docker kill` | `user.interrupt` + `sessions.archive`/`delete` | ✅ |
| Egress firewall (DOCKER-USER) + secret-isolation spec | `limited` networking + `allowed_hosts` — the **SNI-allowlist (Layer-2) you deferred as out-of-scope**, for free | ✅✅ Deleted *and* upgraded |
| Hardcoded 2 CPU / 2 GB, single runner image, no versioning | CMA-managed container; no image to build, pin, or roll | ✅ Deleted |
| AES-GCM secrets store + `AGENTIC_MASTER_KEY` | under operator-pays, the tenant Anthropic key disappears; the GitHub token is still minted per job by you but never enters the sandbox | ✅ Mostly deleted |

---

## 4. What CMA lets you delete (the win, concretely)

- `core/adapter/docker/` (runner, `buildRunArgs`, supervision goroutine) and the `runner/` image + `entrypoint.sh` + `Dockerfile`.
- The **entire** [runner egress + secret-isolation workstream](2026-06-06-runner-egress-and-secret-isolation-design.md): DOCKER-USER rules, the secrets-dir `0711`/`0644` permission dance, the `WorkDirHost`/`PrivateTmp` production bug, the orphan sweep, `deploy/firewall/*`, the network-inspect preflight.
- The **no-queue-drain** gap and the `ReattachRunningJobs` orphan-marking fragility (both surfaced in the capability inventory).
- Under operator-pays: the AES-GCM Anthropic-key store (`user_secrets.anthropic_key_enc`), `POST /settings/anthropic`, and `AGENTIC_MASTER_KEY` plumbing.

This is the real payoff: a large, security-sensitive surface you maintain by hand
moves to Anthropic, and the Layer-2 control you couldn't justify building arrives
as configuration.

---

## 5. Gaps & blockers (with severity)

1. **PR creation — NOT a blocker.** Recommended path: CMA's git proxy pushes the
   branch; the agent writes PR title/body to `/mnt/session/outputs/`; **your
   orchestrator opens the PR via the GitHub REST API using the App installation
   token you already mint** (`core/adapter/ghapp`). This reuses proven code and
   avoids the GitHub MCP server entirely. The MCP path (option b) carries a real
   unknown — hosted MCP servers expect **OAuth bearer** tokens, and it is *not*
   established that the GitHub MCP server accepts an App **installation** token —
   so only adopt it if you specifically want in-session PR creation, and make
   that token-type question the explicit PoC gate if so.
2. **Per-repo customization (`model` / `system_prompt_append` / `max_turns` /
   `allowed_tools`) — friction, not a blocker.** Achievable orchestrator-side:
   one pre-created agent per supported model; fold the append into the kickoff
   message; `task_budget`/`max_iterations` for turn bounds; tools via
   `agent_toolset` config or `sessions.update`.
3. **Status streaming — the main PoC item.** You must consume the SSE event
   stream and project it into your existing `jobs` row + log tail (with the
   reconnect/consolidation + correct idle-break handling the CMA client patterns
   document). Low risk, but it is net-new code.
4. **Output retrieval** — `.pr-url` becomes a session-output file or a
   final-message parse; mind the ~1–3s `files.list` index lag. Minor.
5. **Concurrency** — drop your buggy in-app caps; rely on CMA org RPM plus, if
   needed, a thin admission gate. Net positive.

---

## 6. Billing & trust — operator-pays vs BYO-key (you asked to evaluate both)

**The trust win is conditional on operator-pays.** "No credential in the
sandbox" holds *because* inference runs under **your** org key (so no tenant
Claude key exists to leak) **and** the git proxy keeps the GitHub token out. The
two are one decision, not two.

**BYO-key-under-CMA is effectively incompatible** with being the SaaS operator:
to bill inference to the tenant you would have to create the session inside the
*tenant's* Anthropic org with the tenant's key — you cannot operate their org,
and you lose central control of the run. So "evaluate both" resolves cleanly:
**operator-pays is the only viable CMA billing model; if BYO-key is mandatory,
CMA is the wrong tool** (stay on the Docker runner and pursue the incremental
hardening path instead).

**If operator-pays, per-tenant Anthropic Workspaces are a *requirement*, not an
option.** Each tenant's sessions run in their own Workspace with a **spend cap**.
This gives you (a) cost attribution for metering/pricing and (b) protection from
a malicious or runaway tenant turning *your* inference budget into a billing-DoS.

**Trade-off summary:**

| | Operator-pays (CMA-native) | BYO-key (today) |
|---|---|---|
| "Give me your Claude account" ask | **Gone** | Present (the concern) |
| Credential in sandbox | **No** (git proxy + org-key inference) | Yes (residual #1 open) |
| Who pays inference | You (COGS) — meter + price it | Tenant, directly |
| Abuse exposure | Bounded by per-Workspace spend cap | Tenant's own bill |
| CMA-compatible | **Yes** | No (can't operate tenant's org) |

---

## 7. Rough cost model

- **Per job:** Opus inference tokens + CMA cloud-container runtime overhead
  (repos are cached → warm starts). No separate per-key billing.
- **Org limits:** 300 session-creates/min, 600 other ops/min; model inference
  draws org ITPM/OTPM. Comfortably above current scale (global concurrency cap
  today is 10).
- **Metering:** per-tenant Workspace usage → your billing; per-Workspace spend
  caps bound worst case.
- **Net delta vs today:** you stop paying for runner VMs + egress infra and the
  engineering time on the isolation workstream; you start paying Anthropic for
  inference you previously passed through to tenants. That delta is a **pricing
  exercise, not a blocker** — and it's the cost of removing the credential ask.

---

## 8. Migration shape (strangler — keep the front, swap the backend)

- **Phase 0 — PoC (§10).**
- **Phase 1 — new backend behind the existing port.** Add a `CMARunner`
  implementing `ports.RunnerService` (or a sibling port) so `EnqueueJob` is
  unchanged; feature-flag it per tenant; keep the Docker runner as fallback.
- **Phase 2 — orchestrator drives CMA.** Pre-create agents (one per supported
  model) once. Per job: create session with the `github_repository` resource in
  the tenant's Workspace → stream events into `jobs` + log store → on completion,
  read the PR title/body output and open the PR via your REST path → fire the
  notification webhook. Add `session_id` + `backend` columns to `jobs`.
- **Phase 3 — migrate + retire.** Move tenants over; delete the Docker runner,
  egress firewall, and secrets store for migrated tenants. Retain the
  Docker/self-hosted path *only* for repo-residency-sensitive tenants (§9).

Keeping the `RunnerService` port abstraction is also the lock-in hedge (§9).

---

## 9. Risks & lock-in

- **Repo on Anthropic infra — the banking ceiling (elevated, not a footnote).**
  The tenants most nervous about access (banks/fintech) are precisely the ones
  most likely to refuse their private repo running on Anthropic-hosted
  containers. CMA **self-hosted sandboxes** are the fallback — but they
  **dilute the core benefit**: you clone with the token back in *your* infra, so
  credential-isolation-from-the-sandbox becomes your responsibility again. State
  plainly: cloud CMA's trust win is for tenants who accept Anthropic-hosted
  execution; the rest keep the Docker path.
- **Vendor lock-in.** Execution moves from portable Docker to Anthropic-managed.
  Mitigate by keeping the `RunnerService` port so Docker stays a swappable
  backend.
- **Beta surface.** CMA ships under `managed-agents-2026-04-01`; pin beta headers
  and track migration notes. Acceptable for a greenfield product.
- **Billing-DoS** if per-Workspace spend caps aren't enforced — mitigated by
  making Workspaces + caps a requirement (§6).
- **PR token-type unknown** *only* if you choose the in-session MCP path — avoided
  by the orchestrator-side REST path (§5).

---

## 10. Decision criteria & PoC

**PoC (time-boxed ~3–5 days, one throwaway tenant + one test repo):**

1. Create a cloud environment + an agent (Opus 4.8, full agent toolset). Create a
   session with a `github_repository` resource for the test repo and a kickoff
   message implementing a trivial spec.
2. Confirm the agent edits, commits, and **pushes the branch via the git proxy**,
   and verify code in the session **cannot read** the GitHub token.
3. Confirm the **out-of-band PR open**: agent writes PR title/body to
   `/mnt/session/outputs/`; orchestrator reads it and opens the PR via the
   existing App-token REST path.
4. Confirm **status streaming**: consume the SSE stream and project it into a
   mock `jobs` row + log tail mirroring the current status page (with reconnect +
   correct idle-break handling).
5. Confirm `limited` networking + `allowed_hosts` blocks egress except
   GitHub / Anthropic / package registries.

**Proceed** if 1–5 pass **and** leadership accepts operator-pays.
**Defer / stay on Docker** if status streaming or output retrieval proves
unworkable, if operator-pays is rejected, or if the target tenant base
contractually rejects Anthropic-hosted execution.

**Decision owner:** Akef. **Inputs needed before commit:** operator-pays
sign-off (pricing), and a read on whether key prospective tenants accept
Anthropic-hosted repo execution.
