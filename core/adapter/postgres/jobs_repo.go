// core/adapter/postgres/jobs_repo.go
package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type JobsRepo struct {
	db *bun.DB
}

func NewJobsRepo(db *bun.DB) *JobsRepo { return &JobsRepo{db: db} }

var _ ports.JobsRepository = (*JobsRepo)(nil)

func (r *JobsRepo) Create(ctx context.Context, j *domain.Job) error {
	row := fromJob(j)
	_, err := r.db.NewInsert().Model(row).Exec(ctx)
	return err
}

func (r *JobsRepo) Get(ctx context.Context, id domain.JobID) (*domain.Job, error) {
	var row jobRow
	err := r.db.NewSelect().Model(&row).Where("id = ?", string(id)).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toJob(&row), nil
}

func (r *JobsRepo) GetForUser(ctx context.Context, id domain.JobID, userID domain.UserID) (*domain.Job, error) {
	j, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if j.UserID != userID {
		return nil, domain.ErrNotFound
	}
	return j, nil
}

func (r *JobsRepo) ListForUser(ctx context.Context, userID domain.UserID, limit int) ([]*domain.Job, error) {
	var rows []jobRow
	q := r.db.NewSelect().Model(&rows).Where("user_id = ?", string(userID)).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]*domain.Job, len(rows))
	for i := range rows {
		out[i] = toJob(&rows[i])
	}
	return out, nil
}

func (r *JobsRepo) ListByStatus(ctx context.Context, status domain.JobStatus) ([]*domain.Job, error) {
	var rows []jobRow
	if err := r.db.NewSelect().Model(&rows).Where("status = ?", string(status)).Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]*domain.Job, len(rows))
	for i := range rows {
		out[i] = toJob(&rows[i])
	}
	return out, nil
}

func (r *JobsRepo) Update(ctx context.Context, j *domain.Job) error {
	row := fromJob(j)
	res, err := r.db.NewUpdate().Model(row).WherePK().Exec(ctx)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *JobsRepo) CountActiveForUser(ctx context.Context, userID domain.UserID) (int, error) {
	n, err := r.db.NewSelect().Model((*jobRow)(nil)).
		Where("user_id = ?", string(userID)).
		Where("status IN (?, ?)", string(domain.JobStatusQueued), string(domain.JobStatusRunning)).
		Count(ctx)
	return n, err
}

func (r *JobsRepo) CountActiveGlobal(ctx context.Context) (int, error) {
	n, err := r.db.NewSelect().Model((*jobRow)(nil)).
		Where("status IN (?, ?)", string(domain.JobStatusQueued), string(domain.JobStatusRunning)).
		Count(ctx)
	return n, err
}
