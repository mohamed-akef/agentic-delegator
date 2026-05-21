// core/adapter/http/settings_handler_test.go
package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	adhttp "agentic-delegator/core/adapter/http"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
)

func newSettingsRouter(t *testing.T) (http.Handler, *testutil.FakeSecretsRepo, *testutil.FakeAPIKeysRepo) {
	secrets := testutil.NewFakeSecretsRepo()
	keys := testutil.NewFakeAPIKeysRepo()
	setA := &usecase.SetAnthropicCredentials{Secrets: secrets}
	mint := &usecase.MintAPIKey{Keys: keys, IDGen: &testutil.FakeIDGenerator{}, Clock: testutil.NewFakeClock(time.Unix(1000, 0))}
	rev := &usecase.RevokeAPIKey{Keys: keys}

	r := adhttp.NewRouter(adhttp.Deps{
		Resolver:        stubResolver{uid: "u_1"},
		JobsHandler:     adhttp.NewJobsHandler(nil, nil, nil), // not exercised
		SettingsHandler: adhttp.NewSettingsHandler(setA, mint, rev),
	})
	return r, secrets, keys
}

func TestSetAnthropic_204(t *testing.T) {
	r, secrets, _ := newSettingsRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/settings/anthropic", bytes.NewBufferString(`{"api_key":"sk-1"}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
	got, _ := secrets.GetAnthropicCreds(context.Background(), "u_1")
	if got.APIKey != "sk-1" {
		t.Fatalf("not stored")
	}
}

func TestMintAPIKey_returnsPlaintextOnce(t *testing.T) {
	r, _, keys := newSettingsRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/settings/api-keys", bytes.NewBufferString(`{"name":"laptop"}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["plaintext"] == "" {
		t.Fatalf("plaintext missing")
	}
	list, _ := keys.ListForUser(context.Background(), "u_1")
	if len(list) != 1 {
		t.Fatalf("want 1 key, got %d", len(list))
	}
}
