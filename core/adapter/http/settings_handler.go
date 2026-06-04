// core/adapter/http/settings_handler.go
package http

import (
	"encoding/json"
	"net/http"

	"agentic-delegator/core/usecase"
)

// SettingsHandler handles settings routes (/settings/anthropic and /settings/api-keys).
type SettingsHandler struct {
	setAnthropic *usecase.SetAnthropicCredentials
	mint         *usecase.MintAPIKey
	revoke       *usecase.RevokeAPIKey
}

func NewSettingsHandler(setA *usecase.SetAnthropicCredentials, mint *usecase.MintAPIKey, revoke *usecase.RevokeAPIKey) *SettingsHandler {
	return &SettingsHandler{setAnthropic: setA, mint: mint, revoke: revoke}
}

// SetAnthropic handles POST /settings/anthropic.
func (h *SettingsHandler) SetAnthropic(w http.ResponseWriter, r *http.Request) {
	uid, _ := UserFromContext(r.Context())
	var body struct {
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.setAnthropic.Execute(r.Context(), usecase.SetAnthropicCredentialsInput{UserID: uid, APIKey: body.APIKey}); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MintAPIKey handles POST /settings/api-keys.
func (h *SettingsHandler) MintAPIKey(w http.ResponseWriter, r *http.Request) {
	uid, _ := UserFromContext(r.Context())
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	out, err := h.mint.Execute(r.Context(), usecase.MintAPIKeyInput{UserID: uid, Name: body.Name})
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"id":        string(out.Key.ID),
		"plaintext": out.Plaintext,
		"prefix":    out.Key.Prefix,
	})
}
