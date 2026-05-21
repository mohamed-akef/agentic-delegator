// core/adapter/postgres/users_repo.go
package postgres

import (
	"context"
	"time"

	"github.com/uptrace/bun"

	"agentic-delegator/core/domain"
)

// UsersBootstrapRepo provides the tiny user-row upsert that selfhost needs.
type UsersBootstrapRepo struct {
	db *bun.DB
}

func NewUsersBootstrapRepo(db *bun.DB) *UsersBootstrapRepo {
	return &UsersBootstrapRepo{db: db}
}

func (r *UsersBootstrapRepo) UpsertAdmin(ctx context.Context, id domain.UserID, displayName string, now time.Time) error {
	row := &userRow{ID: string(id), DisplayName: displayName, CreatedAt: now}
	_, err := r.db.NewInsert().Model(row).
		On("CONFLICT (id) DO UPDATE").
		Set("display_name = EXCLUDED.display_name").
		Exec(ctx)
	return err
}
