// core/domain/user_test.go
package domain_test

import (
	"testing"
	"time"

	"agentic-delegator/core/domain"
)

func TestNewUser_setsFields(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	u := domain.NewUser("u_1", "Alice", now)
	if u.ID != "u_1" {
		t.Fatalf("id: want u_1, got %s", u.ID)
	}
	if u.DisplayName != "Alice" {
		t.Fatalf("name: want Alice, got %s", u.DisplayName)
	}
	if !u.CreatedAt.Equal(now) {
		t.Fatalf("created_at: want %v, got %v", now, u.CreatedAt)
	}
}
