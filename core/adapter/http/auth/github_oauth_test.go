// core/adapter/http/auth/github_oauth_test.go
package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agentic-delegator/core/adapter/http/auth"
	"agentic-delegator/core/adapter/postgres"
	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
)

type fakeIdentitiesRepo struct {
	byID map[int64]*postgres.GitHubIdentity
}

func newFakeIdentities() *fakeIdentitiesRepo {
	return &fakeIdentitiesRepo{byID: map[int64]*postgres.GitHubIdentity{}}
}
func (f *fakeIdentitiesRepo) Upsert(_ context.Context, id postgres.GitHubIdentity) error {
	cpy := id
	f.byID[id.GitHubID] = &cpy
	return nil
}
func (f *fakeIdentitiesRepo) ByGitHubID(_ context.Context, ghID int64) (*postgres.GitHubIdentity, error) {
	if v, ok := f.byID[ghID]; ok {
		return v, nil
	}
	return nil, domain.ErrNotFound
}

type fakeUsers struct{ created map[domain.UserID]bool }

func newFakeUsers() *fakeUsers { return &fakeUsers{created: map[domain.UserID]bool{}} }
func (f *fakeUsers) UpsertAdmin(_ context.Context, id domain.UserID, _ string, _ time.Time) error {
	f.created[id] = true
	return nil
}

func TestOAuth_callback_newUser(t *testing.T) {
	// Stub GitHub OAuth + user APIs
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/access_token"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"tok_x"}`))
		case strings.HasSuffix(r.URL.Path, "/user"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":12345,"login":"alice","email":"a@example"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer gh.Close()

	// Sub the OAuth.httpClient transport to redirect github.com calls to our stub.
	transport := &replaceHostTransport{stubURL: gh.URL}
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}

	sessionsStore := &fakeSessionStore{}
	sessions := auth.NewSessions(sessionsStore, false)
	identities := newFakeIdentities()
	users := newFakeUsers()
	idg := &testutil.FakeIDGenerator{}
	clk := testutil.NewFakeClock(time.Unix(1000, 0))

	oauth := auth.NewOAuth(
		auth.OAuthConfig{ClientID: "cid", ClientSecret: "csec", RedirectURL: "https://x/auth/github/callback"},
		sessions, identities, users, idg, clk, client,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=abc&state=xx", nil)
	// The callback now requires the state query param to match the state cookie
	// set at /login (CSRF defense).
	req.AddCookie(&http.Cookie{Name: "agdoauthstate", Value: "xx"})
	oauth.Callback(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/dashboard" {
		t.Fatalf("redirect mismatch: %s", loc)
	}
	if len(users.created) != 1 {
		t.Fatalf("expected 1 user created, got %d", len(users.created))
	}
	if len(identities.byID) != 1 {
		t.Fatalf("expected 1 identity, got %d", len(identities.byID))
	}
	if sessionsStore.uid == "" {
		t.Fatalf("session not created")
	}
}

func TestOAuth_callback_rejectsBadState(t *testing.T) {
	sessions := auth.NewSessions(&fakeSessionStore{}, false)
	oauth := auth.NewOAuth(
		auth.OAuthConfig{ClientID: "cid", ClientSecret: "csec", RedirectURL: "https://x/auth/github/callback"},
		sessions, newFakeIdentities(), newFakeUsers(), &testutil.FakeIDGenerator{}, testutil.NewFakeClock(time.Unix(1000, 0)), nil,
	)

	cases := []struct {
		name   string
		query  string
		cookie string // "" = no cookie
	}{
		{"no cookie", "?code=abc&state=xx", ""},
		{"mismatch", "?code=abc&state=xx", "yy"},
		{"missing state param", "?code=abc", "xx"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/auth/github/callback"+tc.query, nil)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: "agdoauthstate", Value: tc.cookie})
			}
			oauth.Callback(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", rec.Code)
			}
		})
	}
}

// replaceHostTransport rewrites github.com/api.github.com to the stub server.
type replaceHostTransport struct {
	stubURL string
}

func (t *replaceHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// crude: ignore the real host, send all requests to stub, preserving the path
	clone := req.Clone(req.Context())
	u, _ := req.URL.Parse(t.stubURL + req.URL.Path)
	clone.URL = u
	clone.Host = u.Host
	return http.DefaultTransport.RoundTrip(clone)
}
