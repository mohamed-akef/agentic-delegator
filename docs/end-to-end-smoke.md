# End-to-end smoke test

This documents the manual smoke test that verifies a working build of the
single `agentic-delegator` binary end to end.

## Prereqs

- Docker daemon running
- A registered GitHub App (see [`saas-setup.md`](saas-setup.md)) with its env
  vars exported: `AGENTIC_GH_APP_ID`, `AGENTIC_GH_APP_PRIVATE_KEY`,
  `AGENTIC_GH_APP_SLUG`, `AGENTIC_GH_CLIENT_ID`, `AGENTIC_GH_CLIENT_SECRET`,
  `AGENTIC_GH_OAUTH_REDIRECT_URL`, `AGENTIC_GH_WEBHOOK_SECRET`
- A GitHub account you can sign in with and a repo you can push to (sandbox
  account recommended)
- An Anthropic API key

## Steps

1. **Bring up the host services**
   ```bash
   make dev-db-up
   docker build -t agentic-delegator-runner:dev runner/
   ```

2. **Initialize the schema**
   ```bash
   export AGENTIC_MASTER_KEY=$(openssl rand -hex 32)
   echo "AGENTIC_MASTER_KEY=$AGENTIC_MASTER_KEY" > .env.local
   go run ./cmd/agentic-delegator migrate up
   ```

3. **Start the server**
   ```bash
   go run ./cmd/agentic-delegator serve
   # listens on http://127.0.0.1:8787 by default
   ```

4. **Sign in with GitHub**
   - Open http://127.0.0.1:8787/login
   - Authorize the GitHub OAuth dance; you land on the dashboard.

5. **Install the GitHub App**
   - From the dashboard, start the GitHub App install flow
     (`/auth/github-app/install`).
   - Select the sandbox repo to grant access to; you return via
     `/auth/github-app/callback`.

6. **Set the Anthropic key + mint a skill key**
   - Open http://127.0.0.1:8787/settings
   - Paste the Anthropic API key, click Save.
   - Mint a per-user API key and copy it.

7. **Install the skill in your Claude Code**
   ```bash
   mkdir -p ~/.claude/skills/delegate
   cp skill/delegate.md ~/.claude/skills/delegate/
   ```
   In your shell rc file:
   ```bash
   export AGENTIC_DELEGATOR_URL=http://127.0.0.1:8787
   export AGENTIC_DELEGATOR_API_KEY=<the per-user key from step 6>
   ```

8. **Trigger a job**
   - In your sandbox repo: create `specs/hello.md` containing
     `Add a HELLO.md file at the repo root with the text 'hi from delegator'.`
   - Commit + push the spec.
   - Invoke `/delegate` in Claude Code from inside that repo.
   - Confirm the summary, submit.

9. **Watch**
   - Open the status URL printed by the skill.
   - The log should stream; status moves queued → running → succeeded.

10. **Verify**
    - Click the PR link. The PR should contain a single new file `HELLO.md`
      with the expected text.
    - Merge the PR. Sandbox cleanup: `git push origin --delete agentic/hello-…`.

## Acceptance criteria

- [ ] All commands in steps 1–7 succeed without manual fixups.
- [ ] Step 8: the skill detects repo, asks for spec source, posts a job, returns a job link.
- [ ] Step 9: the status page loads, the log polls, the PR link appears within ~3 minutes.
- [ ] Step 10: the PR has the expected diff.

If any step fails, file a bug with the relevant log path from `/jobs/<id>`.
