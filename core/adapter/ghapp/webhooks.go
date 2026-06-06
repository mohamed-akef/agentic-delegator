// core/adapter/ghapp/webhooks.go
package ghapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

// replayWindow is how long a delivery ID is remembered to reject replays.
const replayWindow = 10 * time.Minute

// WebhookHandler handles HMAC-verified GitHub App webhook events.
type WebhookHandler struct {
	secret        []byte
	installations InstallationsWriter

	mu   sync.Mutex
	seen map[string]time.Time // X-GitHub-Delivery -> first-seen time
}

// NewWebhookHandler returns a WebhookHandler that validates webhook signatures
// using secret and dispatches installation events to installs.
func NewWebhookHandler(secret []byte, installs InstallationsWriter) *WebhookHandler {
	return &WebhookHandler{secret: secret, installations: installs, seen: map[string]time.Time{}}
}

// alreadyDelivered records the delivery ID and reports whether it was seen
// within the replay window. Empty IDs are never treated as replays.
func (h *WebhookHandler) alreadyDelivered(id string, now time.Time) bool {
	if id == "" {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for k, t := range h.seen {
		if now.Sub(t) > replayWindow {
			delete(h.seen, k)
		}
	}
	if _, dup := h.seen[id]; dup {
		return true
	}
	h.seen[id] = now
	return false
}

// Handle is POST /webhooks/github.
func (h *WebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !h.verify(r, body) {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}

	// Replay protection: GitHub assigns each delivery a unique ID. Acting on the
	// same one twice (e.g. a captured "installation deleted" event replayed) is
	// rejected as already-processed.
	if h.alreadyDelivered(r.Header.Get("X-GitHub-Delivery"), time.Now()) {
		w.WriteHeader(http.StatusOK)
		return
	}

	event := r.Header.Get("X-GitHub-Event")
	switch event {
	case "installation":
		var payload struct {
			Action       string `json:"action"`
			Installation struct {
				ID int64 `json:"id"`
			} `json:"installation"`
		}
		_ = json.Unmarshal(body, &payload)
		if payload.Action == "deleted" {
			_ = h.installations.Delete(r.Context(), payload.Installation.ID)
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (h *WebhookHandler) verify(r *http.Request, body []byte) bool {
	sig := r.Header.Get("X-Hub-Signature-256")
	if len(sig) < 7 || sig[:7] != "sha256=" {
		return false
	}
	want, err := hex.DecodeString(sig[7:])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, h.secret)
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), want)
}
