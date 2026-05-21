//go:build saas

// saas/store/identities_repo.go
package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"agentic-delegator/core/domain"
)

type IdentitiesRepo struct {
	db *bun.DB
}

func NewIdentitiesRepo(db *bun.DB) *IdentitiesRepo { return &IdentitiesRepo{db: db} }

type GitHubIdentity struct {
	UserID      domain.UserID
	GitHubID    int64
	GitHubLogin string
	Email       string
}

func (r *IdentitiesRepo) Upsert(ctx context.Context, id GitHubIdentity) error {
	row := &identityRow{
		UserID:      string(id.UserID),
		GitHubID:    id.GitHubID,
		GitHubLogin: id.GitHubLogin,
		Email:       id.Email,
	}
	_, err := r.db.NewInsert().Model(row).
		On("CONFLICT (github_id) DO UPDATE").
		Set("github_login = EXCLUDED.github_login").
		Set("email = EXCLUDED.email").
		Exec(ctx)
	return err
}

func (r *IdentitiesRepo) ByGitHubID(ctx context.Context, ghID int64) (*GitHubIdentity, error) {
	var row identityRow
	err := r.db.NewSelect().Model(&row).Where("github_id = ?", ghID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &GitHubIdentity{
		UserID:      domain.UserID(row.UserID),
		GitHubID:    row.GitHubID,
		GitHubLogin: row.GitHubLogin,
		Email:       row.Email,
	}, nil
}
