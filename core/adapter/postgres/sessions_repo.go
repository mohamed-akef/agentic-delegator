// core/adapter/postgres/sessions_repo.go
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"

	"agentic-delegator/core/domain"
)

type SessionsRepo struct {
	db *bun.DB
}

func NewSessionsRepo(db *bun.DB) *SessionsRepo { return &SessionsRepo{db: db} }

func (r *SessionsRepo) Create(ctx context.Context, id []byte, userID domain.UserID, expires time.Time) error {
	row := &sessionRow{ID: id, UserID: string(userID), ExpiresAt: expires}
	_, err := r.db.NewInsert().Model(row).Exec(ctx)
	return err
}

func (r *SessionsRepo) ResolveUser(ctx context.Context, id []byte) (domain.UserID, error) {
	var row sessionRow
	err := r.db.NewSelect().Model(&row).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if time.Now().UTC().After(row.ExpiresAt) {
		return "", domain.ErrNotFound
	}
	return domain.UserID(row.UserID), nil
}

func (r *SessionsRepo) Delete(ctx context.Context, id []byte) error {
	_, err := r.db.NewDelete().Model((*sessionRow)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}
