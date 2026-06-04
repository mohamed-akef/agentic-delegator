// core/testutil/fake_anthropic_creds_provider.go
package testutil

import (
	"context"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

type FakeAnthropicCredsProvider struct {
	Creds domain.AnthropicCreds
	Err   error
}

func NewFakeAnthropicCredsProvider(c domain.AnthropicCreds) *FakeAnthropicCredsProvider {
	return &FakeAnthropicCredsProvider{Creds: c}
}

var _ ports.AnthropicCredentialsProvider = (*FakeAnthropicCredsProvider)(nil)

func (p *FakeAnthropicCredsProvider) For(ctx context.Context, userID domain.UserID) (domain.AnthropicCreds, error) {
	if p.Err != nil {
		return domain.AnthropicCreds{}, p.Err
	}
	return p.Creds, nil
}
