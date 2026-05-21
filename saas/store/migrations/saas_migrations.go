//go:build saas

// saas/store/migrations/saas_migrations.go
package migrations

import (
	"context"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"

	// register core migrations too — saas binary still needs them
	_ "agentic-delegator/core/adapter/postgres/migrations"
)

var Migrations = migrate.NewMigrations()

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
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
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
DROP TABLE IF EXISTS saas_sessions;
DROP TABLE IF EXISTS saas_github_installations;
DROP TABLE IF EXISTS saas_github_identities;
`)
			return err
		},
	)
}
