// core/adapter/ghapp/webhooks.go
package ghapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
)

// WebhookHandler handles HMAC-verified GitHub App webhook events.
type WebhookHandler struct {
	secret        []byte
	installations InstallationsWriter
}

// NewWebhookHandler returns a WebhookHandler that validates webhook signatures
// using secret and dispatches installation events to installs.
func NewWebhookHandler(secret []byte, installs InstallationsWriter) *WebhookHandler {
	return &WebhookHandler{secret: secret, installations: installs}
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
