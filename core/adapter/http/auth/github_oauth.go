// core/adapter/http/auth/github_oauth.go
package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"agentic-delegator/core/adapter/postgres"
	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

// IdentitiesRepo is the postgres interface we depend on.
type IdentitiesRepo interface {
	Upsert(ctx context.Context, id postgres.GitHubIdentity) error
	ByGitHubID(ctx context.Context, ghID int64) (*postgres.GitHubIdentity, error)
}

// UsersBootstrap is the slice of postgres.UsersBootstrapRepo we need —
// adds the User row at signup time.
type UsersBootstrap interface {
	UpsertAdmin(ctx context.Context, id domain.UserID, displayName string, now time.Time) error
}

type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string // e.g. https://your-domain/auth/github/callback
	CookieSecure bool   // set true behind TLS so the state cookie is HTTPS-only
}

type OAuth struct {
	cfg        OAuthConfig
	sessions   *Sessions
	identities IdentitiesRepo
	users      UsersBootstrap
	idgen      ports.IDGenerator
	clock      ports.Clock
	httpClient *http.Client
}

func NewOAuth(
	cfg OAuthConfig,
	sessions *Sessions,
	identities IdentitiesRepo,
	users UsersBootstrap,
	idgen ports.IDGenerator,
	clock ports.Clock,
	client *http.Client,
) *OAuth {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &OAuth{
		cfg:        cfg,
		sessions:   sessions,
		identities: identities,
		users:      users,
		idgen:      idgen,
		clock:      clock,
		httpClient: client,
	}
}

const oauthStateCookie = "agdoauthstate"

// Login handles GET /login — redirects to GitHub OAuth.
func (o *OAuth) Login(w http.ResponseWriter, r *http.Request) {
	// CSRF defense: bind a random state to a short-lived cookie and echo it to
	// GitHub. The callback rejects any request whose state doesn't match.
	state := o.idgen.NewJobID()
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    state,
		MaxAge:   600, // 10 minutes
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   o.cfg.CookieSecure,
		Path:     "/",
	})
	u := url.URL{Scheme: "https", Host: "github.com", Path: "/login/oauth/authorize"}
	q := u.Query()
	q.Set("client_id", o.cfg.ClientID)
	q.Set("redirect_uri", o.cfg.RedirectURL)
	q.Set("scope", "read:user user:email")
	q.Set("state", state)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// Callback handles GET /auth/github/callback.
func (o *OAuth) Callback(w http.ResponseWriter, r *http.Request) {
	// CSRF defense: the state echoed back by GitHub must match the cookie we set
	// at /login. Constant-time compare and require both to be present.
	stateCookie, err := r.Cookie(oauthStateCookie)
	gotState := r.URL.Query().Get("state")
	if err != nil || gotState == "" || subtle.ConstantTimeCompare([]byte(stateCookie.Value), []byte(gotState)) != 1 {
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}
	// State is single-use: clear the cookie regardless of outcome.
	http.SetCookie(w, &http.Cookie{Name: oauthStateCookie, Value: "", MaxAge: -1, Path: "/"})

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	tok, err := o.exchangeCode(r.Context(), code)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	user, err := o.fetchUser(r.Context(), tok)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	// Look up existing identity OR create a new user row.
	id, err := o.identities.ByGitHubID(r.Context(), user.ID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var uid domain.UserID
	if id != nil {
		uid = id.UserID
	} else {
		uid = domain.UserID(o.idgen.NewUserID())
		if err := o.users.UpsertAdmin(r.Context(), uid, user.Login, o.clock.Now()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := o.identities.Upsert(r.Context(), postgres.GitHubIdentity{
		UserID:      uid,
		GitHubID:    user.ID,
		GitHubLogin: user.Login,
		Email:       user.Email,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Session-fixation defense: drop any session attached to the inbound request
	// before minting a fresh one for the authenticated user.
	_ = o.sessions.Logout(r.Context(), w, r)
	if err := o.sessions.Login(r.Context(), w, uid); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

type ghUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Email string `json:"email"`
}

func (o *OAuth) exchangeCode(ctx context.Context, code string) (string, error) {
	body := url.Values{}
	body.Set("client_id", o.cfg.ClientID)
	body.Set("client_secret", o.cfg.ClientSecret)
	body.Set("code", code)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://github.com/login/oauth/access_token",
		strings.NewReader(body.Encode()))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := o.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var parsed struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}
	if parsed.Error != "" {
		return "", fmt.Errorf("github oauth: %s: %s", parsed.Error, parsed.ErrorDesc)
	}
	return parsed.AccessToken, nil
}

func (o *OAuth) fetchUser(ctx context.Context, token string) (*ghUser, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github api %d: %s", resp.StatusCode, string(body))
	}
	var u ghUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}
