// core/adapter/postgres/saas_models.go
package postgres

import (
	"time"

	"github.com/uptrace/bun"
)

type identityRow struct {
	bun.BaseModel `bun:"table:saas_github_identities,alias:gi"`

	UserID      string `bun:"user_id,pk"`
	GitHubID    int64  `bun:"github_id,unique,notnull"`
	GitHubLogin string `bun:"github_login,notnull"`
	Email       string `bun:"email,notnull,default:''"`
}

type installationRow struct {
	bun.BaseModel `bun:"table:saas_github_installations,alias:gh"`

	InstallationID int64     `bun:"installation_id,pk"`
	UserID         string    `bun:"user_id,notnull"`
	AccountLogin   string    `bun:"account_login,notnull"`
	Repos          []byte    `bun:"repos,notnull"` // JSONB stored as bytes, encoded on read/write
	CreatedAt      time.Time `bun:"created_at,notnull,default:current_timestamp"`
}

type sessionRow struct {
	bun.BaseModel `bun:"table:saas_sessions,alias:ss"`

	ID        []byte    `bun:"id,pk"`
	UserID    string    `bun:"user_id,notnull"`
	ExpiresAt time.Time `bun:"expires_at,notnull"`
}
