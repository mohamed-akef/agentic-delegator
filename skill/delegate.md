---
name: delegate
description: Delegate a coding task to the agentic-delegator service. Pass it a spec (path/URL/inline) and it returns a job link.
---

# /delegate — delegate a coding task

You are running inside a Claude Code session. The user invoked `/delegate` to send a coding task to a running agentic-delegator service. The service will spawn a sandboxed runner, implement the spec, push a branch, and open a PR.

## Required env vars (must be set in the user's shell)

- `AGENTIC_DELEGATOR_URL` — e.g. `http://localhost:8787`
- `AGENTIC_DELEGATOR_API_KEY` — the personal API key minted from the dashboard

If either is missing, stop and tell the user how to set them.

## Workflow

1. **Detect repo + branch.**
   - Run `git remote get-url origin` to detect the GitHub repo. Expect a URL like `git@github.com:owner/name.git` or `https://github.com/owner/name.git`. Extract `owner/name`.
   - Run `git branch --show-current` to detect the current branch.
   - If you're not inside a git repo, stop and tell the user.

2. **Ask the user for the spec source.** Examples:
   - "Path inside the repo: `specs/auth-refactor.md`"
   - "URL: `https://gist.githubusercontent.com/.../raw/spec.md`"
   - "Or paste the spec content directly."
   - Classify the input:
     - Starts with `http://` or `https://` → `source_type=url`
     - Looks like a relative path ending in `.md` → `source_type=path`
     - Otherwise → `source_type=inline`

3. **Ask the user about the work branch.** Default is a new branch: `agentic/<spec-stem>-<shortid>` where `<spec-stem>` is the spec filename without extension (for inline/url specs, ask the user for a short stem). The base branch defaults to the current branch (or `main` if user prefers). Let the user override either.

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
