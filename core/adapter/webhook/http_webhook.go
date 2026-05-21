// core/adapter/webhook/http_webhook.go
package webhook

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
)

// HTTPWebhook dispatches webhooks via an http.Client. No retry — MVP fire-and-forget.
type HTTPWebhook struct {
	client *http.Client
}

func New(client *http.Client) *HTTPWebhook {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPWebhook{client: client}
}

func (w *HTTPWebhook) Dispatch(ctx context.Context, url string, payload []byte) error {
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
		return fmt.Errorf("webhook: %s returned %d: %s", url, resp.StatusCode, string(body))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}
