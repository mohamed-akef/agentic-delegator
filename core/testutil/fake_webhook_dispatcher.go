// core/testutil/fake_webhook_dispatcher.go
package testutil

import (
	"context"
	"sync"

	"agentic-delegator/core/usecase/ports"
)

type FakeWebhookCall struct {
	URL     string
	Payload []byte
}

type FakeWebhookDispatcher struct {
	mu    sync.Mutex
	Err   error
	Calls []FakeWebhookCall
}

func NewFakeWebhookDispatcher() *FakeWebhookDispatcher {
	return &FakeWebhookDispatcher{}
}

var _ ports.WebhookDispatcher = (*FakeWebhookDispatcher)(nil)

func (d *FakeWebhookDispatcher) Dispatch(ctx context.Context, url string, payload []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.Err != nil {
		return d.Err
	}
	cpy := make([]byte, len(payload))
	copy(cpy, payload)
	d.Calls = append(d.Calls, FakeWebhookCall{URL: url, Payload: cpy})
	return nil
}
