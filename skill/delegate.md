---
name: delegate
description: Delegate work to the agentic-delegator service — either a coding task (spec → branch → PR) or a question about the repo (answer written to docs/answers/<slug>.md → PR).
---

# /delegate — delegate a task or question

You are running inside a Claude Code session. The user invoked `/delegate` to hand work off to a running agentic-delegator service. The service spawns a sandboxed runner, runs Claude Code on the spec, pushes a branch, and opens a PR.

The skill handles two modes:

- **`task` mode** — user wants code changes. The spec describes the change; the runner implements it.
- **`question` mode** — user asked a question about the repo. The skill wraps the question in an inline spec that tells the runner to write the answer to `docs/answers/<slug>.md`, commit, and PR. Same transport, same endpoint, same shape — just a synthesized spec.

If the user's input is phrased as a question ("what is…", "how does…", "give me your opinion of…", "explain…", "why…") or they explicitly say "answer this", use **question mode**. Otherwise use **task mode**. When unsure, ask the user once.

## Required env vars (must be set in the user's shell)

- `AGENTIC_DELEGATOR_URL` — e.g. `http://localhost:8787`
- `AGENTIC_DELEGATOR_API_KEY` — the personal API key minted from the dashboard

If either is missing, stop and tell the user how to set them.

## Workflow

1. **Detect repo + branch.**
   - Run `git remote get-url origin` to detect the GitHub repo. Expect a URL like `git@github.com:owner/name.git` or `https://github.com/owner/name.git`. Extract `owner/name`.
   - Run `git branch --show-current` to detect the current branch.
   - If you're not inside a git repo, stop and tell the user.

2. **Gather the spec — branch on mode.**

   **Task mode.** Ask the user for the spec source. Examples:
   - "Path inside the repo: `specs/auth-refactor.md`"
   - "URL: `https://gist.githubusercontent.com/.../raw/spec.md`"
   - "Or paste the spec content directly."

   Classify the input:
   - Starts with `http://` or `https://` → `source_type=url`
   - Looks like a relative path ending in `.md` → `source_type=path`
   - Otherwise → `source_type=inline`

   **Question mode.** Take the user's question verbatim. Pick a short kebab-case slug from the question (e.g. "what is your opinion of this project" → `project-opinion`). Synthesize an inline spec of this shape and use it as `spec_source` with `source_type=inline`:

   ```
   # Answer a question about this repository

   The user asked the following question. Investigate the repository as needed, then write a clear, well-structured answer.

   Write the answer to `docs/answers/<slug>.md`. Create the `docs/answers/` directory if it does not exist. Include the question verbatim at the top of the file under a `## Question` heading, followed by `## Answer`.

   Do not modify any other files. Commit with message `docs: answer "<question, truncated to ~60 chars>"`. Push and open a PR titled the same.

   ## Question

   <the user's question, verbatim>
   ```

   Substitute `<slug>` and `<the user's question>` literally before sending.

3. **Decide the work branch.**
   - Task mode default: `agentic/<spec-stem>-<shortid>` where `<spec-stem>` is the spec filename without extension (for inline/url, ask the user for a short stem).
   - Question mode default: `agentic/answer-<slug>-<shortid>`.
   - Base branch defaults to the current branch (or `main` if the user prefers). Let the user override either.

4. **Show an editable summary** of `{repo, base_branch, work_branch, spec_source, source_type, model_override}` and ask the user to confirm or edit.

5. **POST the job:**

   ```bash
   curl -sS -X POST \
     -H "Authorization: Bearer $AGENTIC_DELEGATOR_API_KEY" \
     -H "Content-Type: application/json" \
     "$AGENTIC_DELEGATOR_URL/api/jobs" \
     -d '<the json payload>'
   ```

   Use this JSON shape:
   ```json
   {
     "repo": "owner/name",
     "base_branch": "main",
     "work_branch": "agentic/auth-refactor-9q2k",
     "spec_source": "<the value the user supplied>",
     "source_type": "path|url|inline",
     "model_override": ""
   }
   ```

6. **Print the response** to the user. Expected:
   ```json
   { "job_id": "j_xxx", "status_url": "http://localhost:8787/jobs/j_xxx" }
   ```

   Tell the user: "Job started. Watch progress at <status_url>. The agent will commit, push, and open a PR when done."

7. **Exit.** Do not poll; the dashboard is the source of truth.

## Failure modes

- If curl returns non-2xx, surface the response body.
- If the response doesn't include `job_id`, treat it as a service-side error and tell the user.
- If the user's repo doesn't have a GitHub remote, the service won't be able to clone — stop early with a clear error.
