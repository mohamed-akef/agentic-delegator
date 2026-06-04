# End-to-end smoke test — selfhost

This documents the manual smoke test that verifies Plan 03 ships a working
selfhost binary.

## Prereqs

- `plan-03-done` tag set
- Docker daemon running
- A GitHub repo you can push to (sandbox account recommended)
- An Anthropic API key
- A GitHub PAT with `repo` scope

## Steps

1. **Bring up the host services**
   ```bash
   make dev-db-up
   docker build -t agentic-delegator-runner:dev runner/
   ```

2. **Initialize the schema + admin user + admin API key**
   ```bash
   export AGENTIC_MASTER_KEY=$(openssl rand -hex 32)
   echo "AGENTIC_MASTER_KEY=$AGENTIC_MASTER_KEY" > .env.local
   go run ./cmd/agentic-delegator/migrate -cmd=up
   go run ./cmd/agentic-delegator init
   # Save the printed key as $AGENTIC_DELEGATOR_API_KEY
   ```

3. **Set the PAT** — start the server and open the setup page:
   ```bash
   go run ./cmd/agentic-delegator serve
   # In another shell: open http://127.0.0.1:8787/admin/setup, paste the PAT
   ```

4. **Set the Anthropic key + mint a skill key**
   - Open http://127.0.0.1:8787/settings
   - Paste the Anthropic API key, click Save
   - The admin key from step 2 is already a usable skill key.

5. **Install the skill in your Claude Code**
   ```bash
   mkdir -p ~/.claude/skills/delegate
   cp skill/delegate.md ~/.claude/skills/delegate/
   ```
   In your shell rc file:
   ```bash
   export AGENTIC_DELEGATOR_URL=http://127.0.0.1:8787
   export AGENTIC_DELEGATOR_API_KEY=<the key from step 2>
   ```

6. **Trigger a job**
   - In your sandbox repo: create `specs/hello.md` containing
     `Add a HELLO.md file at the repo root with the text 'hi from delegator'.`
   - Commit + push the spec.
   - Invoke `/delegate` in Claude Code from inside that repo.
   - Confirm the summary, submit.

7. **Watch**
   - Open the status URL printed by the skill.
   - The log should stream; status moves queued → running → succeeded.

8. **Verify**
   - Click the PR link. The PR should contain a single new file `HELLO.md` with the expected text.
   - Merge the PR. Sandbox cleanup: `git push origin --delete agentic/hello-…`.

## Acceptance criteria

- [x] All commands in steps 1–5 succeed without manual fixups.
- [x] Step 6: the skill detects repo, asks for spec source, posts a job, returns a job link.
- [x] Step 7: the status page loads, the log polls, the PR link appears within ~3 minutes.
- [x] Step 8: the PR has the expected diff.

If any step fails, file a bug with the relevant log path from `/jobs/<id>`.
