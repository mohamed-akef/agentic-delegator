//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"agentic-delegator/core/adapter/postgres"
	"agentic-delegator/core/domain"
)

func TestSecretsRepo_setGetDelete(t *testing.T) {
	db, _ := postgres.Open(testDSN(t))
	defer db.Close()
	// seed user
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, `INSERT INTO users (id, display_name) VALUES ('u_sec', 'sec') ON CONFLICT DO NOTHING`)
	_, _ = db.ExecContext(ctx, `DELETE FROM user_secrets WHERE user_id = 'u_sec'`)

	r := postgres.NewSecretsRepo(db)
	if err := r.SetAnthropicCreds(ctx, "u_sec", domain.AnthropicCreds{APIKey: "sk-1"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := r.GetAnthropicCreds(ctx, "u_sec")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.APIKey != "sk-1" {
		t.Fatalf("mismatch: %s", got.APIKey)
	}
	// update (upsert)
	_ = r.SetAnthropicCreds(ctx, "u_sec", domain.AnthropicCreds{APIKey: "sk-2"})
	got, _ = r.GetAnthropicCreds(ctx, "u_sec")
	if got.APIKey != "sk-2" {
		t.Fatalf("upsert failed: %s", got.APIKey)
	}
	if err := r.DeleteAnthropicCreds(ctx, "u_sec"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := r.GetAnthropicCreds(ctx, "u_sec"); err != domain.ErrNotFound {
		t.Fatalf("post-delete get: want ErrNotFound, got %v", err)
	}
}
