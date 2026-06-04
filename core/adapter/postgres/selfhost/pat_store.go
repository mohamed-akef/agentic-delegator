// core/adapter/postgres/selfhost/pat_store.go
package selfhost_pg

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"agentic-delegator/core/adapter/crypto"
	"agentic-delegator/core/domain"
	"agentic-delegator/core/runtime/selfhost"
)

type selfhostPATRow struct {
	bun.BaseModel `bun:"table:selfhost_admin_pat"`

	UserID string `bun:"user_id,pk"`
	PATEnc []byte `bun:"pat_enc,notnull"`
}

// SelfhostPATStore implements selfhost.PATStore using Postgres + AES-GCM.
type SelfhostPATStore struct {
	db  *bun.DB
	aes *crypto.AESGCM
}

func NewSelfhostPATStore(db *bun.DB, aes *crypto.AESGCM) *SelfhostPATStore {
	return &SelfhostPATStore{db: db, aes: aes}
}

var _ selfhost.PATStore = (*SelfhostPATStore)(nil)

func (s *SelfhostPATStore) Set(ctx context.Context, pat string) error {
	ct, err := s.aes.Encrypt([]byte(pat))
	if err != nil {
		return err
	}
	row := &selfhostPATRow{UserID: string(selfhost.AdminUserID), PATEnc: ct}
	_, err = s.db.NewInsert().Model(row).
		On("CONFLICT (user_id) DO UPDATE").
		Set("pat_enc = EXCLUDED.pat_enc").
		Exec(ctx)
	return err
}

func (s *SelfhostPATStore) Get(ctx context.Context) (string, error) {
	var row selfhostPATRow
	err := s.db.NewSelect().Model(&row).Where("user_id = ?", string(selfhost.AdminUserID)).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	pt, err := s.aes.Decrypt(row.PATEnc)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
