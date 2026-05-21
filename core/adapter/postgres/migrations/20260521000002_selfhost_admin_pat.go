package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS selfhost_admin_pat (
    user_id  TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    pat_enc  BYTEA NOT NULL
);
`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS selfhost_admin_pat`)
			return err
		},
	)
}
