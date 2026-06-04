// core/usecase/set_anthropic_credentials_test.go
package usecase_test

import (
	"context"
	"testing"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

func TestSetAnthropicCredentials_ok(t *testing.T) {
	ctx := context.Background()
	secrets := testutil.NewFakeSecretsRepo()
	uc := &usecase.SetAnthropicCredentials{Secrets: secrets}

	if err := uc.Execute(ctx, usecase.SetAnthropicCredentialsInput{UserID: "u_1", APIKey: "sk-ant-xxx"}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	got, _ := secrets.GetAnthropicCreds(ctx, "u_1")
	if got.APIKey != "sk-ant-xxx" {
		t.Fatalf("stored creds mismatch: %v", got)
	}
}

func TestSetAnthropicCredentials_rejectsEmpty(t *testing.T) {
	uc := &usecase.SetAnthropicCredentials{Secrets: testutil.NewFakeSecretsRepo()}
	err := uc.Execute(context.Background(), usecase.SetAnthropicCredentialsInput{UserID: "u_1", APIKey: ""})
	if err != domain.ErrInvalidInput {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}
