// core/runtime/selfhost/anthropic_creds_test.go
package selfhost_test

import (
	"context"
	"testing"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/runtime/selfhost"
	"agentic-delegator/core/testutil"
)

func TestAnthropicCreds_readsFromSecretsRepo(t *testing.T) {
	secrets := testutil.NewFakeSecretsRepo()
	_ = secrets.SetAnthropicCreds(context.Background(), "u_admin", domain.AnthropicCreds{APIKey: "sk-1"})
	p := selfhost.NewAnthropicCredsProvider(secrets)
	got, err := p.For(context.Background(), "u_admin")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got.APIKey != "sk-1" {
		t.Fatalf("api key mismatch: %s", got.APIKey)
	}
}
