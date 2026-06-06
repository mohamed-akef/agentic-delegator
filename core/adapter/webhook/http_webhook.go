// core/adapter/webhook/http_webhook.go
package webhook

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// maxAttempts bounds total delivery tries (1 initial + retries).
const maxAttempts = 3

// HTTPWebhook dispatches webhooks via an http.Client with bounded retries.
type HTTPWebhook struct {
	client  *http.Client
	backoff time.Duration
}

func New(client *http.Client) *HTTPWebhook {
	return NewWithBackoff(client, time.Second)
}

// NewWithBackoff is New with an explicit base backoff between retry attempts.
func NewWithBackoff(client *http.Client, backoff time.Duration) *HTTPWebhook {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPWebhook{client: client, backoff: backoff}
}

func (w *HTTPWebhook) Dispatch(ctx context.Context, url string, payload []byte) error {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			// Exponential backoff, cancellable.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(w.backoff * time.Duration(1<<(attempt-2))):
			}
		}
		lastErr = w.attempt(ctx, url, payload)
		if lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("webhook: giving up after %d attempts: %w", maxAttempts, lastErr)
}

func (w *HTTPWebhook) attempt(ctx context.Context, url string, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
		return fmt.Errorf("%s returned %d: %s", url, resp.StatusCode, string(body))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}
