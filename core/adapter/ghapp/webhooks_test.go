// core/adapter/ghapp/webhooks_test.go
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

	"agentic-delegator/core/adapter/ghapp"
	"agentic-delegator/core/adapter/postgres"
)

type fakeInstalls struct {
	deleted  []int64
	upserted []postgres.Installation
}

func (f *fakeInstalls) Upsert(_ context.Context, i postgres.Installation) error {
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

func TestWebhook_rejectsReplay(t *testing.T) {
	body := []byte(`{"action":"deleted","installation":{"id":42}}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	installs := &fakeInstalls{}
	h := ghapp.NewWebhookHandler([]byte("secret"), installs)

	send := func() int {
		req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
		req.Header.Set("X-GitHub-Event", "installation")
		req.Header.Set("X-Hub-Signature-256", sig)
		req.Header.Set("X-GitHub-Delivery", "delivery-123")
		rec := httptest.NewRecorder()
		h.Handle(rec, req)
		return rec.Code
	}

	// Same delivery ID twice: both return 200, but the action runs only once.
	if c := send(); c != http.StatusOK {
		t.Fatalf("first delivery status: %d", c)
	}
	if c := send(); c != http.StatusOK {
		t.Fatalf("replayed delivery status: %d", c)
	}
	if len(installs.deleted) != 1 {
		t.Fatalf("replay should not re-run the action; deletes=%d", len(installs.deleted))
	}
}
