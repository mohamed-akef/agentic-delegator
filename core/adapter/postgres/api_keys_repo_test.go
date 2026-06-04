//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"agentic-delegator/core/adapter/postgres"
	"agentic-delegator/core/domain"
)

func TestAPIKeysRepo_createListGetByPrefixDelete(t *testing.T) {
	db, _ := postgres.Open(testDSN(t))
	defer db.Close()
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, `INSERT INTO users (id, display_name) VALUES ('u_k', 'k') ON CONFLICT DO NOTHING`)
	_, _ = db.ExecContext(ctx, `DELETE FROM api_keys WHERE user_id = 'u_k'`)

	r := postgres.NewAPIKeysRepo(db)
	k := domain.NewAPIKey("k_1", "u_k", "laptop", "agdkey_a", []byte("hashbytes"), time.Now().UTC())
	if err := r.Create(ctx, k); err != nil {
		t.Fatalf("create: %v", err)
	}

	list, _ := r.ListForUser(ctx, "u_k")
	if len(list) != 1 || list[0].ID != "k_1" {
		t.Fatalf("list mismatch: %+v", list)
	}

	byPrefix, _ := r.GetByPrefix(ctx, "agdkey_a")
	if len(byPrefix) != 1 || byPrefix[0].ID != "k_1" {
		t.Fatalf("byPrefix mismatch: %+v", byPrefix)
	}

	if err := r.RecordUsed(ctx, "k_1", time.Now().UTC()); err != nil {
		t.Fatalf("RecordUsed: %v", err)
	}
	list2, _ := r.ListForUser(ctx, "u_k")
	if list2[0].LastUsedAt == nil {
		t.Fatalf("LastUsedAt should be populated after RecordUsed")
	}

	if err := r.Delete(ctx, "k_1", "u_k"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := r.Delete(ctx, "k_1", "u_k"); err != domain.ErrNotFound {
		t.Fatalf("redelete: want ErrNotFound, got %v", err)
	}
}
