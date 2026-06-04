package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type APIKeysRepo struct {
	db *bun.DB
}

func NewAPIKeysRepo(db *bun.DB) *APIKeysRepo { return &APIKeysRepo{db: db} }

var _ ports.APIKeysRepository = (*APIKeysRepo)(nil)

func (r *APIKeysRepo) Create(ctx context.Context, k *domain.APIKey) error {
	row := &apiKeyRow{
		ID:         string(k.ID),
		UserID:     string(k.UserID),
		Name:       k.Name,
		Prefix:     k.Prefix,
		Hash:       []byte(k.Hash),
		LastUsedAt: k.LastUsedAt,
		CreatedAt:  k.CreatedAt,
	}
	_, err := r.db.NewInsert().Model(row).Exec(ctx)
	return err
}

func (r *APIKeysRepo) GetByPrefix(ctx context.Context, prefix string) ([]*domain.APIKey, error) {
	var rows []apiKeyRow
	if err := r.db.NewSelect().Model(&rows).Where("key_prefix = ?", prefix).Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]*domain.APIKey, len(rows))
	for i := range rows {
		out[i] = toAPIKey(&rows[i])
	}
	return out, nil
}

func (r *APIKeysRepo) ListForUser(ctx context.Context, userID domain.UserID) ([]*domain.APIKey, error) {
	var rows []apiKeyRow
	if err := r.db.NewSelect().Model(&rows).Where("user_id = ?", string(userID)).Order("created_at DESC").Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]*domain.APIKey, len(rows))
	for i := range rows {
		out[i] = toAPIKey(&rows[i])
	}
	return out, nil
}

func (r *APIKeysRepo) Delete(ctx context.Context, id domain.APIKeyID, userID domain.UserID) error {
	res, err := r.db.NewDelete().Model((*apiKeyRow)(nil)).
		Where("id = ?", string(id)).
		Where("user_id = ?", string(userID)).
		Exec(ctx)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *APIKeysRepo) RecordUsed(ctx context.Context, id domain.APIKeyID, at time.Time) error {
	res, err := r.db.NewUpdate().Model((*apiKeyRow)(nil)).
		Set("last_used_at = ?", at).
		Where("id = ?", string(id)).
		Exec(ctx)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Compile-time assertion that we map sql.ErrNoRows correctly above (used in other repos)
var _ = errors.Is(sql.ErrNoRows, sql.ErrNoRows)
