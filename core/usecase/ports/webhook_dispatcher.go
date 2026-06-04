// core/usecase/ports/webhook_dispatcher.go
package ports

import "context"

// WebhookDispatcher fires an outbound HTTP webhook with the given JSON body.
// MVP: no retry at this layer. Adapter implementations may log+swallow errors
// since callers cannot react usefully.
type WebhookDispatcher interface {
	Dispatch(ctx context.Context, url string, payload []byte) error
}
