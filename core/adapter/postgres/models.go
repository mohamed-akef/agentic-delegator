// core/adapter/postgres/models.go
package postgres

import (
	"time"

	"github.com/uptrace/bun"

	"agentic-delegator/core/domain"
)

type userRow struct {
	bun.BaseModel `bun:"table:users,alias:u"`

	ID          string    `bun:"id,pk"`
	DisplayName string    `bun:"display_name,notnull,default:''"`
	CreatedAt   time.Time `bun:"created_at,notnull,default:current_timestamp"`
}

type userSecretRow struct {
	bun.BaseModel `bun:"table:user_secrets,alias:us"`

	UserID          string    `bun:"user_id,pk"`
	AnthropicKeyEnc []byte    `bun:"anthropic_key_enc,notnull"`
	UpdatedAt       time.Time `bun:"updated_at,notnull,default:current_timestamp"`
}

type apiKeyRow struct {
	bun.BaseModel `bun:"table:api_keys,alias:ak"`

	ID         string     `bun:"id,pk"`
	UserID     string     `bun:"user_id,notnull"`
	Name       string     `bun:"name,notnull"`
	Prefix     string     `bun:"key_prefix,notnull"`
	Hash       []byte     `bun:"key_hash,notnull"`
	LastUsedAt *time.Time `bun:"last_used_at"`
	CreatedAt  time.Time  `bun:"created_at,notnull,default:current_timestamp"`
}

type jobRow struct {
	bun.BaseModel `bun:"table:jobs,alias:j"`

	ID            string     `bun:"id,pk"`
	UserID        string     `bun:"user_id,notnull"`
	Status        string     `bun:"status,notnull"`
	Repo          string     `bun:"repo,notnull"`
	BaseBranch    string     `bun:"base_branch,notnull"`
	WorkBranch    string     `bun:"work_branch,notnull"`
	SpecSource    string     `bun:"spec_source,notnull"`
	SourceType    string     `bun:"source_type,notnull"`
	ModelOverride string     `bun:"model_override,notnull,default:''"`
	ContainerID   string     `bun:"container_id,notnull,default:''"`
	PRURL         string     `bun:"pr_url,notnull,default:''"`
	Error         string     `bun:"error,notnull,default:''"`
	LogPath       string     `bun:"log_path,notnull"`
	CreatedAt     time.Time  `bun:"created_at,notnull,default:current_timestamp"`
	StartedAt     *time.Time `bun:"started_at"`
	FinishedAt    *time.Time `bun:"finished_at"`
}

// --- mapping helpers ---

func toJob(r *jobRow) *domain.Job {
	return &domain.Job{
		ID:            domain.JobID(r.ID),
		UserID:        domain.UserID(r.UserID),
		Status:        domain.JobStatus(r.Status),
		Repo:          r.Repo,
		BaseBranch:    r.BaseBranch,
		WorkBranch:    r.WorkBranch,
		Spec:          domain.SpecSource{Type: domain.SourceType(r.SourceType), Value: r.SpecSource},
		ModelOverride: r.ModelOverride,
		ContainerID:   r.ContainerID,
		PRURL:         r.PRURL,
		Error:         r.Error,
		LogPath:       r.LogPath,
		CreatedAt:     r.CreatedAt,
		StartedAt:     r.StartedAt,
		FinishedAt:    r.FinishedAt,
	}
}

func fromJob(j *domain.Job) *jobRow {
	return &jobRow{
		ID:            string(j.ID),
		UserID:        string(j.UserID),
		Status:        string(j.Status),
		Repo:          j.Repo,
		BaseBranch:    j.BaseBranch,
		WorkBranch:    j.WorkBranch,
		SpecSource:    j.Spec.Value,
		SourceType:    string(j.Spec.Type),
		ModelOverride: j.ModelOverride,
		ContainerID:   j.ContainerID,
		PRURL:         j.PRURL,
		Error:         j.Error,
		LogPath:       j.LogPath,
		CreatedAt:     j.CreatedAt,
		StartedAt:     j.StartedAt,
		FinishedAt:    j.FinishedAt,
	}
}

func toAPIKey(r *apiKeyRow) *domain.APIKey {
	return &domain.APIKey{
		ID:         domain.APIKeyID(r.ID),
		UserID:     domain.UserID(r.UserID),
		Name:       r.Name,
		Prefix:     r.Prefix,
		Hash:       domain.APIKeyHash(r.Hash),
		LastUsedAt: r.LastUsedAt,
		CreatedAt:  r.CreatedAt,
	}
}
