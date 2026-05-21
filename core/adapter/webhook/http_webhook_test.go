// core/adapter/webhook/http_webhook_test.go
package webhook_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agentic-delegator/core/adapter/webhook"
	"agentic-delegator/core/usecase/ports"
)

func TestHTTPWebhook_satisfiesPort(t *testing.T) {
	var _ ports.WebhookDispatcher = webhook.New(http.DefaultClient)
}

func TestHTTPWebhook_postsJSON(t *testing.T) {
	var gotBody []byte
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w := webhook.New(&http.Client{Timeout: 2 * time.Second})
	payload := []byte(`{"event":"job.completed"}`)
	if err := w.Dispatch(context.Background(), srv.URL, payload); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !bytes.Equal(gotBody, payload) {
		t.Fatalf("body mismatch: got %q want %q", gotBody, payload)
	}
	if gotContentType != "application/json" {
		t.Fatalf("content-type mismatch: %q", gotContentType)
	}
}

func TestHTTPWebhook_returnsErrorOn5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	w := webhook.New(&http.Client{Timeout: 2 * time.Second})
	err := w.Dispatch(context.Background(), srv.URL, []byte(`{}`))
	if err == nil {
		t.Fatalf("expected error on 5xx response")
	}
}

func TestHTTPWebhook_returnsErrorOnDialFailure(t *testing.T) {
	w := webhook.New(&http.Client{Timeout: 200 * time.Millisecond})
	err := w.Dispatch(context.Background(), "http://127.0.0.1:1", []byte(`{}`))
	if err == nil {
		t.Fatalf("expected dial error")
	}
	// verify we got an error (just checking it's non-nil is sufficient)
	_ = errors.Is(err, context.DeadlineExceeded) // might or might not be deadline, but error is present
}
