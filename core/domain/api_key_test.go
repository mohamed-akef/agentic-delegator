// core/domain/api_key_test.go
package domain_test

import (
	"testing"
	"time"

	"agentic-delegator/core/domain"
)

func TestNewAPIKey_setsFields(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	k := domain.NewAPIKey("k_1", "u_1", "laptop", "agdkey_a", []byte("hash"), now)
	if k.ID != "k_1" || k.UserID != "u_1" || k.Name != "laptop" || k.Prefix != "agdkey_a" {
		t.Fatalf("fields not set correctly: %+v", k)
	}
	if string(k.Hash) != "hash" {
		t.Fatalf("hash not stored")
	}
	if !k.CreatedAt.Equal(now) {
		t.Fatalf("created_at not stored")
	}
	if k.LastUsedAt != nil {
		t.Fatalf("last_used_at should be nil at creation")
	}
}

func TestAPIKey_RecordUsed(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	later := now.Add(time.Hour)
	k := domain.NewAPIKey("k_1", "u_1", "laptop", "agdkey_a", []byte("hash"), now)
	k.RecordUsed(later)
	if k.LastUsedAt == nil || !k.LastUsedAt.Equal(later) {
		t.Fatalf("LastUsedAt not updated")
	}
}
