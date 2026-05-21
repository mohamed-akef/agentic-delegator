//go:build saas

// saas/ghapp/webhooks_test.go
package ghapp_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentic-delegator/saas/ghapp"
	"agentic-delegator/saas/store"
)

type fakeInstalls struct {
	deleted  []int64
	upserted []store.Installation
}

func (f *fakeInstalls) Upsert(_ context.Context, i store.Installation) error {
	f.upserted = append(f.upserted, i)
	return nil
}
func (f *fakeInstalls) Delete(_ context.Context, id int64) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func TestWebhook_rejectsBadSig(t *testing.T) {
	w := ghapp.NewWebhookHandler([]byte("secret"), &fakeInstalls{})
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewBufferString(`{}`))
	req.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")
	rec := httptest.NewRecorder()
	w.Handle(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: %d", rec.Code)
	}
}

func TestWebhook_installationDeleted(t *testing.T) {
	body := []byte(`{"action":"deleted","installation":{"id":42}}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	installs := &fakeInstalls{}
	h := ghapp.NewWebhookHandler([]byte("secret"), installs)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "installation")
	req.Header.Set("X-Hub-Signature-256", sig)
	rec := httptest.NewRecorder()
	h.Handle(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
	if len(installs.deleted) != 1 || installs.deleted[0] != 42 {
		t.Fatalf("expected installation 42 deleted, got %+v", installs.deleted)
	}
}
