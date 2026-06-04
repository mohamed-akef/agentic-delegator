// core/adapter/postgres/installations_repo.go
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/uptrace/bun"

	"agentic-delegator/core/domain"
)

type InstallationsRepo struct {
	db *bun.DB
}

func NewInstallationsRepo(db *bun.DB) *InstallationsRepo { return &InstallationsRepo{db: db} }

type Installation struct {
	InstallationID int64
	UserID         domain.UserID
	AccountLogin   string
	Repos          []string // empty = all repos
	CreatedAt      time.Time
}

func (r *InstallationsRepo) Upsert(ctx context.Context, i Installation) error {
	repos, err := json.Marshal(i.Repos)
	if err != nil {
		return err
	}
	row := &installationRow{
		InstallationID: i.InstallationID,
		UserID:         string(i.UserID),
		AccountLogin:   i.AccountLogin,
		Repos:          repos,
		CreatedAt:      i.CreatedAt,
	}
	_, err = r.db.NewInsert().Model(row).
		On("CONFLICT (installation_id) DO UPDATE").
		Set("user_id = EXCLUDED.user_id").
		Set("account_login = EXCLUDED.account_login").
		Set("repos = EXCLUDED.repos").
		Exec(ctx)
	return err
}

func (r *InstallationsRepo) ByUserAndRepo(ctx context.Context, userID domain.UserID, repo string) (*Installation, error) {
	var rows []installationRow
	if err := r.db.NewSelect().Model(&rows).Where("user_id = ?", string(userID)).Scan(ctx); err != nil {
		return nil, err
	}
	for _, row := range rows {
		var repos []string
		_ = json.Unmarshal(row.Repos, &repos)
		// Empty repos slice = all repos. Otherwise check membership.
		if len(repos) == 0 || contains(repos, repo) {
			return &Installation{
				InstallationID: row.InstallationID,
				UserID:         domain.UserID(row.UserID),
				AccountLogin:   row.AccountLogin,
				Repos:          repos,
				CreatedAt:      row.CreatedAt,
			}, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *InstallationsRepo) ByInstallationID(ctx context.Context, id int64) (*Installation, error) {
	var row installationRow
	err := r.db.NewSelect().Model(&row).Where("installation_id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var repos []string
	_ = json.Unmarshal(row.Repos, &repos)
	return &Installation{
		InstallationID: row.InstallationID,
		UserID:         domain.UserID(row.UserID),
		AccountLogin:   row.AccountLogin,
		Repos:          repos,
		CreatedAt:      row.CreatedAt,
	}, nil
}

func (r *InstallationsRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.NewDelete().Model((*installationRow)(nil)).Where("installation_id = ?", id).Exec(ctx)
	return err
}

func contains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}
