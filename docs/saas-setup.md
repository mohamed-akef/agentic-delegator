# Deployment — setup guide

This is the canonical setup guide for running agentic-delegator. The service
authenticates users via GitHub OAuth, accesses their repos via a GitHub App
installation, and runs Claude Code on a sandboxed runner.

## 1. Register a GitHub App

Go to https://github.com/settings/apps/new (or your org's App settings) and create a new App:

- **Name:** Agentic Delegator (or your service name)
- **Homepage URL:** `https://<your-domain>`
- **Callback URL:** `https://<your-domain>/auth/github-app/callback`
  Add a second callback for OAuth: `https://<your-domain>/auth/github/callback`
- **Webhook URL:** `https://<your-domain>/webhooks/github`
- **Webhook secret:** generate a random 32-byte hex (`openssl rand -hex 32`) and save it
- **Permissions:**
  - Contents: Read & write
  - Pull requests: Read & write
  - Metadata: Read
- **Subscribe to events:** Installation, Installation repositories
- **Where can this GitHub App be installed?** Any account

After creation, capture:
- App ID
- Client ID
- Client secret (generate from the App page)
- Private key (Generate a private key — download the PEM)
- App "slug" (the URL-friendly name, e.g. `agentic-delegator`)
- The webhook secret you generated above

## 2. Configure env vars on the host

```bash
export AGENTIC_HTTP_BIND=127.0.0.1:8787
export DELEGATOR_DSN=postgres://...
export AGENTIC_MASTER_KEY=$(openssl rand -hex 32)
export AGENTIC_RUNNER_IMAGE=agentic-delegator-runner:dev

export AGENTIC_GH_APP_ID=12345
export AGENTIC_GH_APP_SLUG=agentic-delegator
export AGENTIC_GH_APP_PRIVATE_KEY="$(cat /etc/agentic-delegator/gh-app.pem)"
export AGENTIC_GH_CLIENT_ID=Iv1.xxxxxxx
export AGENTIC_GH_CLIENT_SECRET=xxxxxxxxxxxxxxxx
export AGENTIC_GH_OAUTH_REDIRECT_URL=https://<your-domain>/auth/github/callback
export AGENTIC_GH_WEBHOOK_SECRET=<the hex string from above>
```

## 3. Migrate the database

```bash
go run ./cmd/agentic-delegator migrate up
# or, from a built binary: bin/agentic-delegator migrate up
```

This applies the single consolidated initial migration.

## 4. Run the binary

```bash
go run ./cmd/agentic-delegator serve
# or, from a built binary (make build): bin/agentic-delegator serve
```

Reverse-proxy via Caddy to `https://<your-domain>`.

## 5. Smoke

- Visit `https://<your-domain>` → click "Sign in with GitHub"
- Authorize the OAuth dance
- On the dashboard, click "Install our GitHub App"
- Select a sandbox repo to grant access to
- Open `/settings` and paste an Anthropic API key
- Mint a personal API key for the skill
- Install `skill/delegate.md` locally + set env vars
- `/delegate` in Claude Code → confirm → wait for PR

## How auth and isolation work

| Concern | How it works |
|---|---|
| Auth | GitHub OAuth sign-in + per-user API keys minted from `/settings` |
| Repo access | GitHub App installation tokens (per user, per repo) |
| Data isolation | Strict per-user scoping; every request resolves to a `UserID` via session cookie or bearer key |
| Persistence | Postgres (users, jobs, api_keys, user_secrets, identities, installations, sessions) |
