// core/adapter/postgres/migrations/20260603000001_initial.go
package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		// up
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    display_name  TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_secrets (
    user_id              TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    anthropic_key_enc    BYTEA NOT NULL,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS api_keys (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    key_prefix    TEXT NOT NULL,
    key_hash      BYTEA NOT NULL,
    last_used_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys(key_prefix);

CREATE TABLE IF NOT EXISTS jobs (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status          TEXT NOT NULL,
    repo            TEXT NOT NULL,
    base_branch     TEXT NOT NULL,
    work_branch     TEXT NOT NULL,
    spec_source     TEXT NOT NULL,
    source_type     TEXT NOT NULL,
    model_override  TEXT NOT NULL DEFAULT '',
    container_id    TEXT NOT NULL DEFAULT '',
    pr_url          TEXT NOT NULL DEFAULT '',
    error           TEXT NOT NULL DEFAULT '',
    log_path        TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_jobs_user_status ON jobs(user_id, status);
CREATE INDEX IF NOT EXISTS idx_jobs_created ON jobs(created_at DESC);

CREATE TABLE IF NOT EXISTS saas_github_identities (
    user_id       TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    github_id     BIGINT UNIQUE NOT NULL,
    github_login  TEXT NOT NULL,
    email         TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS saas_github_installations (
    installation_id  BIGINT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_login    TEXT NOT NULL,
    repos            JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_saas_install_user ON saas_github_installations(user_id);

CREATE TABLE IF NOT EXISTS saas_sessions (
    id          BYTEA PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_saas_sessions_user ON saas_sessions(user_id);
`)
			return err
		},
		// down
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
DROP TABLE IF EXISTS saas_sessions;
DROP TABLE IF EXISTS saas_github_installations;
DROP TABLE IF EXISTS saas_github_identities;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS user_secrets;
DROP TABLE IF EXISTS users;
`)
			return err
		},
	)
}
