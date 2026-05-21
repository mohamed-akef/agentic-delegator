// core/usecase/revoke_api_key_test.go
package usecase_test

import (
	"context"
	"testing"
	"time"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

func TestRevokeAPIKey_ok(t *testing.T) {
	ctx := context.Background()
	keys := testutil.NewFakeAPIKeysRepo()
	k := domain.NewAPIKey("k_1", "u_1", "laptop", "agdkey_a", []byte("h"), time.Unix(1000, 0))
	_ = keys.Create(ctx, k)

	uc := &usecase.RevokeAPIKey{Keys: keys}
	if err := uc.Execute(ctx, usecase.RevokeAPIKeyInput{ID: "k_1", UserID: "u_1"}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	list, _ := keys.ListForUser(ctx, "u_1")
	if len(list) != 0 {
		t.Fatalf("expected 0 keys after revoke, got %d", len(list))
	}
}

func TestRevokeAPIKey_otherUserCannot(t *testing.T) {
	ctx := context.Background()
	keys := testutil.NewFakeAPIKeysRepo()
	k := domain.NewAPIKey("k_1", "u_1", "laptop", "agdkey_a", []byte("h"), time.Unix(1000, 0))
	_ = keys.Create(ctx, k)

	uc := &usecase.RevokeAPIKey{Keys: keys}
	err := uc.Execute(ctx, usecase.RevokeAPIKeyInput{ID: "k_1", UserID: "u_2"})
	if err == nil {
		t.Fatalf("expected error on cross-user revoke")
	}
}
