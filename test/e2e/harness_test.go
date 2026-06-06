// test/e2e/harness_test.go
//
// In-process, full-cycle end-to-end harness. It assembles the real chi router
// and real use cases exactly as cmd/agentic-delegator/main.go's runServe does,
// but injects in-memory fakes for every external boundary (Postgres, GitHub,
// Docker runner, Anthropic) plus a fake GitHub HTTP transport. Drives the whole
// delegation workflow over HTTP via httptest.Server.
//
// arch-lint excludes *_test.go, so this package may import every adapter.
// If runServe's wiring changes, mirror it here.
//
// Faithfully wired (real cross-component links exercised):
//   - session cookies (auth.Sessions over an in-memory store)
//   - Anthropic creds: set via HTTP -> fake secrets repo -> real
//     credentials.AnthropicCredsProvider -> read back during enqueue
//   - API keys: minted via HTTP -> keyhash bcrypt-on-write -> resolver bcrypt
//     bearer auth (the path the /delegate skill uses)
//   - enqueue -> EnqueueJob -> fake runner; runner exit -> OnComplete ->
//     HandleRunnerCompletion
//
// Faked-not-exercised (require live GitHub; covered by ghapp unit tests):
//   - repo git creds (real provider mints an installation token against GitHub)
//   - the GitHub-App install *callback* (AppClient + the install handler use a
//     non-injectable http client + JWT signing); the install *redirect* is
//     exercised, and an installation row is seeded to represent the grant.
package e2e

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"agentic-delegator/core/adapter/credentials"
	"agentic-delegator/core/adapter/ghapp"
	adhttp "agentic-delegator/core/adapter/http"
	"agentic-delegator/core/adapter/http/auth"
	"agentic-delegator/core/adapter/idgen"
	"agentic-delegator/core/adapter/keyhash"
	"agentic-delegator/core/adapter/postgres"
	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
	"agentic-delegator/core/usecase"
	"agentic-delegator/core/usecase/ports"
)

const testGitHubLogin = "tester"

// harness owns a running httptest.Server and the fakes behind it.
type harness struct {
	t       *testing.T
	server  *httptest.Server
	browser *http.Client // carries the session cookie; does not follow redirects
	cli     *http.Client // no cookies; used for bearer-token (skill) calls

	jobs     *testutil.FakeJobsRepo
	secrets  *testutil.FakeSecretsRepo
	runner   *testutil.FakeRunnerService
	clk      *testutil.FakeClock
	installs *fakeInstallationsRepo
}

type harnessOpts struct {
	maxPerUser int
	maxGlobal  int
}

func newHarness(t *testing.T, opts harnessOpts) *harness {
	t.Helper()
	ctx := context.Background()
	clk := testutil.NewFakeClock(time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC))
	idg := &idgen.NanoID{} // real id generator: realistic opaque ids

	jobs := testutil.NewFakeJobsRepo()
	secrets := testutil.NewFakeSecretsRepo()
	apiKeys := keyhash.New(testutil.NewFakeAPIKeysRepo()) // same write-path as prod
	runner := testutil.NewFakeRunnerService()

	identities := newFakeIdentitiesRepo()
	users := &fakeUsersBootstrap{}
	installs := newFakeInstallationsRepo()
	sessionStore := newFakeSessionStore()
	sessions := auth.NewSessions(sessionStore, false)

	// Real Anthropic creds provider over the fake secrets repo — closes the
	// set-key -> read-key-at-enqueue loop.
	anthCreds := credentials.NewAnthropicCredsProvider(secrets)
	// Repo creds are faked: the real provider mints a GitHub installation token.
	repoCreds := testutil.NewFakeRepoCredsProvider(domain.GitCreds{Token: "ghs_faketoken"})

	githubClient := &http.Client{Transport: fakeGitHubTransport{}}
	oauth := auth.NewOAuth(
		auth.OAuthConfig{
			ClientID:     "Iv1.testclientid",
			ClientSecret: "testsecret",
			RedirectURL:  "http://example.test/auth/github/callback",
		},
		sessions, identities, users, idg, clk, githubClient,
	)
	appClient := ghapp.NewAppClient(ghapp.AppCreds{}) // unused on exercised paths
	installHandler := ghapp.NewInstallHandler("test-app", sessions, installs, appClient)
	webhookHandler := ghapp.NewWebhookHandler([]byte("whsecret"), installs)

	resolver := auth.NewResolver(sessions, apiKeys)

	enqueue := &usecase.EnqueueJob{
		Jobs:                 jobs,
		RepoCreds:            repoCreds,
		AnthropicCreds:       anthCreds,
		Runner:               runner,
		IDGen:                idg,
		Clock:                clk,
		MaxConcurrentPerUser: opts.maxPerUser,
		MaxConcurrentGlobal:  opts.maxGlobal,
	}
	getJob := &usecase.GetJob{Jobs: jobs}
	listJobs := &usecase.ListJobs{Jobs: jobs}
	complete := &usecase.HandleRunnerCompletion{Jobs: jobs, Clock: clk}
	mint := &usecase.MintAPIKey{Keys: apiKeys, IDGen: idg, Clock: clk}
	revoke := &usecase.RevokeAPIKey{Keys: apiKeys}
	setAnth := &usecase.SetAnthropicCredentials{Secrets: secrets}

	enqueue.OnComplete = func(res ports.RunnerResult) { _ = complete.Execute(ctx, res) }

	cancelJob := &usecase.CancelJob{Jobs: jobs, Runner: runner, Clock: clk}
	jobsHandler := adhttp.NewJobsHandler(enqueue, getJob, listJobs, cancelJob)
	settingsHandler := adhttp.NewSettingsHandler(setAnth, mint, revoke)
	statusPage := adhttp.NewStatusPage(getJob)
	dashHandler := adhttp.NewDashboardHandler(listJobs, apiKeys, secrets, resolver)

	router := adhttp.NewRouter(adhttp.Deps{
		Resolver:        resolver,
		JobsHandler:     jobsHandler,
		SettingsHandler: settingsHandler,
		StatusPage:      statusPage,
		Dashboard:       dashHandler,
		Routes:          routeMounter{oauth: oauth, install: installHandler, webhook: webhookHandler},
	})

	server := httptest.NewServer(router)
	jar, _ := cookiejar.New(nil)
	noRedirect := func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	h := &harness{
		t:        t,
		server:   server,
		browser:  &http.Client{Jar: jar, CheckRedirect: noRedirect},
		cli:      &http.Client{CheckRedirect: noRedirect},
		jobs:     jobs,
		secrets:  secrets,
		runner:   runner,
		clk:      clk,
		installs: installs,
	}
	t.Cleanup(server.Close)
	return h
}

// routeMounter mirrors the one in cmd/agentic-delegator/main.go.
type routeMounter struct {
	oauth   *auth.OAuth
	install *ghapp.InstallHandler
	webhook *ghapp.WebhookHandler
}

func (m routeMounter) RegisterRoutes(r chi.Router) {
	r.Get("/login", m.oauth.Login)
	r.Get("/auth/github/callback", m.oauth.Callback)
	r.Get("/auth/github-app/install", m.install.Install)
	r.Get("/auth/github-app/callback", m.install.Callback)
	r.Post("/webhooks/github", m.webhook.Handle)
}

// ---- HTTP helpers ----

func (h *harness) get(client *http.Client, path string, bearer string) *http.Response {
	h.t.Helper()
	req, err := http.NewRequest(http.MethodGet, h.server.URL+path, nil)
	if err != nil {
		h.t.Fatalf("new request: %v", err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := client.Do(req)
	if err != nil {
		h.t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func (h *harness) postJSON(client *http.Client, path string, bearer string, body any) *http.Response {
	h.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			h.t.Fatalf("encode body: %v", err)
		}
	}
	req, err := http.NewRequest(http.MethodPost, h.server.URL+path, &buf)
	if err != nil {
		h.t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := client.Do(req)
	if err != nil {
		h.t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, into any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}

// ---- workflow step helpers ----

// login completes the OAuth callback (the redirect target of /login) and
// returns once the browser client holds a session cookie.
func (h *harness) login() {
	h.t.Helper()
	// /login sets the agdoauthstate cookie (CSRF defense) and redirects to GitHub.
	lr := h.get(h.browser, "/login", "")
	lr.Body.Close()
	state := h.cookieValue("agdoauthstate")
	if state == "" {
		h.t.Fatal("/login did not set the oauth state cookie")
	}
	resp := h.get(h.browser, "/auth/github/callback?code=testcode&state="+state, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		h.t.Fatalf("oauth callback: want 302, got %d (%s)", resp.StatusCode, readBody(h.t, resp))
	}
	if !h.hasSessionCookie() {
		h.t.Fatal("oauth callback did not set a session cookie")
	}
}

// cookieValue returns the named cookie's value from the browser jar, or "".
func (h *harness) cookieValue(name string) string {
	if h.browser.Jar == nil {
		return ""
	}
	su, _ := http.NewRequest(http.MethodGet, h.server.URL, nil)
	for _, c := range h.browser.Jar.Cookies(su.URL) {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func (h *harness) hasSessionCookie() bool {
	u := h.server.URL
	parsed := h.browser.Jar
	if parsed == nil {
		return false
	}
	su, _ := http.NewRequest(http.MethodGet, u, nil)
	for _, c := range parsed.Cookies(su.URL) {
		if c.Name == "agdsess" {
			return true
		}
	}
	return false
}

func (h *harness) setAnthropicKey(key string) {
	h.t.Helper()
	resp := h.postJSON(h.browser, "/settings/anthropic", "", map[string]string{"api_key": key})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		h.t.Fatalf("set anthropic: want 204, got %d (%s)", resp.StatusCode, readBody(h.t, resp))
	}
}

// mintAPIKey mints a per-user key via the session and returns the plaintext.
func (h *harness) mintAPIKey(name string) string {
	h.t.Helper()
	resp := h.postJSON(h.browser, "/settings/api-keys", "", map[string]string{"name": name})
	if resp.StatusCode != http.StatusOK {
		h.t.Fatalf("mint key: want 200, got %d (%s)", resp.StatusCode, readBody(h.t, resp))
	}
	var out struct {
		Plaintext string `json:"plaintext"`
	}
	decodeJSON(h.t, resp, &out)
	if out.Plaintext == "" {
		h.t.Fatal("mint key returned empty plaintext")
	}
	return out.Plaintext
}

type enqueueResult struct {
	JobID     string `json:"job_id"`
	StatusURL string `json:"status_url"`
}

func (h *harness) enqueue(bearer string, body map[string]string) (*http.Response, enqueueResult) {
	h.t.Helper()
	resp := h.postJSON(h.cli, "/api/jobs", bearer, body)
	if resp.StatusCode != http.StatusOK {
		return resp, enqueueResult{}
	}
	var out enqueueResult
	decodeJSON(h.t, resp, &out)
	return resp, out
}

// completeRunner drives the most recently started container to exit.
func (h *harness) completeRunner(exitCode int, prURL, errMsg string) {
	h.t.Helper()
	id, spec, ok := h.runner.LastStarted()
	if !ok {
		h.t.Fatal("no container has been started")
	}
	h.runner.Complete(id, ports.RunnerResult{
		JobID:    spec.JobID,
		ExitCode: exitCode,
		PRURL:    prURL,
		Error:    errMsg,
	})
}

func (h *harness) getJob(bearer, id string) (*http.Response, *domain.Job) {
	h.t.Helper()
	resp := h.get(h.cli, "/api/jobs/"+id, bearer)
	if resp.StatusCode != http.StatusOK {
		return resp, nil
	}
	var j domain.Job
	decodeJSON(h.t, resp, &j)
	return resp, &j
}

// seedInstallation records a completed GitHub-App installation, standing in
// for the install callback (which needs live GitHub). enqueue uses the fake
// repo-creds provider regardless of this row.
func (h *harness) seedInstallation() {
	h.t.Helper()
	err := h.installs.Upsert(context.Background(), postgres.Installation{
		InstallationID: 1,
		AccountLogin:   testGitHubLogin,
		Repos:          []string{"tester/sandbox"},
		CreatedAt:      h.clk.Now(),
	})
	if err != nil {
		h.t.Fatalf("seed installation: %v", err)
	}
	if h.installs.count() != 1 {
		h.t.Fatal("installation not recorded")
	}
}

// validSpecBody returns a well-formed enqueue request body.
func validSpecBody() map[string]string {
	return map[string]string{
		"repo":        "tester/sandbox",
		"base_branch": "main",
		"work_branch": "agentic/hello",
		"spec_source": "Add a HELLO.md file at the repo root.",
		"source_type": string(domain.SourceTypeInline),
	}
}

// ---- fake GitHub HTTP transport (OAuth token + /user) ----

type fakeGitHubTransport struct{}

func (fakeGitHubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	mk := func(code int, body string) *http.Response {
		return &http.Response{
			StatusCode: code,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Request:    req,
		}
	}
	switch {
	case req.URL.Host == "github.com" && req.URL.Path == "/login/oauth/access_token":
		return mk(200, `{"access_token":"gho_testtoken","token_type":"bearer"}`), nil
	case req.URL.Host == "api.github.com" && req.URL.Path == "/user":
		return mk(200, `{"id":424242,"login":"tester","email":"tester@example.com"}`), nil
	default:
		return mk(404, `{"message":"unexpected github call: `+req.URL.String()+`"}`), nil
	}
}

// ---- in-memory stores for the auth/ghapp boundary ----

type fakeSessionStore struct {
	mu sync.Mutex
	m  map[string]domain.UserID
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{m: map[string]domain.UserID{}}
}

func (s *fakeSessionStore) Create(_ context.Context, id []byte, userID domain.UserID, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[hex.EncodeToString(id)] = userID
	return nil
}

func (s *fakeSessionStore) ResolveUser(_ context.Context, id []byte) (domain.UserID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	uid, ok := s.m[hex.EncodeToString(id)]
	if !ok {
		return "", domain.ErrNotFound
	}
	return uid, nil
}

func (s *fakeSessionStore) Delete(_ context.Context, id []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, hex.EncodeToString(id))
	return nil
}

type fakeIdentitiesRepo struct {
	mu sync.Mutex
	m  map[int64]postgres.GitHubIdentity
}

func newFakeIdentitiesRepo() *fakeIdentitiesRepo {
	return &fakeIdentitiesRepo{m: map[int64]postgres.GitHubIdentity{}}
}

func (r *fakeIdentitiesRepo) Upsert(_ context.Context, id postgres.GitHubIdentity) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[id.GitHubID] = id
	return nil
}

func (r *fakeIdentitiesRepo) ByGitHubID(_ context.Context, ghID int64) (*postgres.GitHubIdentity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.m[ghID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	clone := id
	return &clone, nil
}

type fakeUsersBootstrap struct {
	mu sync.Mutex
	n  int
}

func (u *fakeUsersBootstrap) UpsertAdmin(_ context.Context, _ domain.UserID, _ string, _ time.Time) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.n++
	return nil
}

type fakeInstallationsRepo struct {
	mu sync.Mutex
	m  map[int64]postgres.Installation
}

func newFakeInstallationsRepo() *fakeInstallationsRepo {
	return &fakeInstallationsRepo{m: map[int64]postgres.Installation{}}
}

func (r *fakeInstallationsRepo) Upsert(_ context.Context, i postgres.Installation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[i.InstallationID] = i
	return nil
}

func (r *fakeInstallationsRepo) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, id)
	return nil
}

func (r *fakeInstallationsRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.m)
}
