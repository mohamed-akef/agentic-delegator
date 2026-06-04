// core/adapter/keyhash/keyhash_test.go
package keyhash_test

import (
	"context"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"agentic-delegator/core/adapter/keyhash"
	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
)

// On Create, the wrapper must bcrypt the plaintext-bearing Hash so that the
// resolver's bcrypt.CompareHashAndPassword succeeds — and must NOT persist the
// plaintext. This is the seam that ties MintAPIKey (stores plaintext in Hash)
// to the resolver (bcrypt-compares Hash), which were previously disconnected.
func TestRepo_CreateBcryptsHash(t *testing.T) {
	ctx := context.Background()
	inner := testutil.NewFakeAPIKeysRepo()
	repo := keyhash.New(inner)

	plain := "agdkey_test_0123456789abcdef"
	k := domain.NewAPIKey("k_1", "u_1", "laptop", plain[:8], domain.APIKeyHash([]byte(plain)), time.Unix(1000, 0))

	if err := repo.Create(ctx, k); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetByPrefix(ctx, plain[:8])
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 key, got %d", len(got))
	}
	stored := []byte(got[0].Hash)

	if string(stored) == plain {
		t.Fatal("plaintext key was persisted; expected a bcrypt hash")
	}
	if err := bcrypt.CompareHashAndPassword(stored, []byte(plain)); err != nil {
		t.Fatalf("stored hash does not verify against plaintext: %v", err)
	}
}

// Reads must pass through untouched so the resolver and dashboard see the
// stored (already-hashed) rows.
func TestRepo_ListForUserPassesThrough(t *testing.T) {
	ctx := context.Background()
	inner := testutil.NewFakeAPIKeysRepo()
	repo := keyhash.New(inner)

	plain := "agdkey_test_aaaaaaaaaaaaaaaa"
	k := domain.NewAPIKey("k_1", "u_1", "laptop", plain[:8], domain.APIKeyHash([]byte(plain)), time.Unix(1000, 0))
	if err := repo.Create(ctx, k); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.ListForUser(ctx, "u_1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].ID != "k_1" {
		t.Fatalf("unexpected list result: %+v", got)
	}
}
