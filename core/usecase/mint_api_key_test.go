// core/usecase/mint_api_key_test.go
package usecase_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

func TestMintAPIKey_returnsPlaintextOnce(t *testing.T) {
	ctx := context.Background()
	clock := testutil.NewFakeClock(time.Unix(1000, 0))
	keys := testutil.NewFakeAPIKeysRepo()

	uc := &usecase.MintAPIKey{
		Keys:  keys,
		IDGen: &testutil.FakeIDGenerator{},
		Clock: clock,
	}

	out, err := uc.Execute(ctx, usecase.MintAPIKeyInput{UserID: "u_1", Name: "laptop"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Plaintext == "" {
		t.Fatalf("plaintext should be returned exactly once")
	}
	if !strings.HasPrefix(out.Plaintext, out.Key.Prefix) {
		t.Fatalf("prefix should match the start of the plaintext")
	}
	if out.Key.UserID != "u_1" || out.Key.Name != "laptop" {
		t.Fatalf("fields not set on stored key")
	}

	stored, _ := keys.GetByPrefix(ctx, out.Key.Prefix)
	if len(stored) != 1 || stored[0].ID != out.Key.ID {
		t.Fatalf("key not stored under its prefix")
	}
}

func TestMintAPIKey_rejectsEmptyName(t *testing.T) {
	uc := &usecase.MintAPIKey{
		Keys:  testutil.NewFakeAPIKeysRepo(),
		IDGen: &testutil.FakeIDGenerator{},
		Clock: testutil.NewFakeClock(time.Unix(1000, 0)),
	}
	_, err := uc.Execute(context.Background(), usecase.MintAPIKeyInput{UserID: "u_1", Name: ""})
	if err == nil {
		t.Fatalf("expected error on empty name")
	}
	if !errorsIs(err, domain.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

// errorsIs is a tiny wrapper used across usecase tests to keep imports tight.
func errorsIs(err, target error) bool {
	return err == target
}
