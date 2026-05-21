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

type SecretsRepo struct {
	db *bun.DB
}

func NewSecretsRepo(db *bun.DB) *SecretsRepo { return &SecretsRepo{db: db} }

var _ ports.SecretsRepository = (*SecretsRepo)(nil)

func (r *SecretsRepo) SetAnthropicCreds(ctx context.Context, userID domain.UserID, c domain.AnthropicCreds) error {
	row := &userSecretRow{
		UserID:          string(userID),
		AnthropicKeyEnc: []byte(c.APIKey),
		UpdatedAt:       time.Now().UTC(),
	}
	_, err := r.db.NewInsert().Model(row).
		On("CONFLICT (user_id) DO UPDATE").
		Set("anthropic_key_enc = EXCLUDED.anthropic_key_enc").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

func (r *SecretsRepo) GetAnthropicCreds(ctx context.Context, userID domain.UserID) (domain.AnthropicCreds, error) {
	var row userSecretRow
	err := r.db.NewSelect().Model(&row).Where("user_id = ?", string(userID)).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AnthropicCreds{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.AnthropicCreds{}, err
	}
	return domain.AnthropicCreds{APIKey: string(row.AnthropicKeyEnc)}, nil
}

func (r *SecretsRepo) DeleteAnthropicCreds(ctx context.Context, userID domain.UserID) error {
	res, err := r.db.NewDelete().Model((*userSecretRow)(nil)).Where("user_id = ?", string(userID)).Exec(ctx)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
