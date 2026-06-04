// core/adapter/http/auth/resolver_test.go
package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"agentic-delegator/core/adapter/http/auth"
	"agentic-delegator/core/domain"
	"agentic-delegator/core/testutil"
)

type stubSessions struct {
	uid domain.UserID
	err error
}

func (s stubSessions) UserFromRequest(r *http.Request) (domain.UserID, error) { return s.uid, s.err }

func TestResolver_bearerMatchesByPrefix(t *testing.T) {
	keys := testutil.NewFakeAPIKeysRepo()
	plain := "agdkey_user1_0123456789abcdef"
	hash, _ := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	_ = keys.Create(context.Background(), domain.NewAPIKey("k_1", "u_a", "laptop", plain[:8], hash, time.Unix(1000, 0)))

	r := auth.NewResolver(stubSessions{err: http.ErrNoCookie}, keys)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+plain)

	uid, err := r.Resolve(req)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if uid != "u_a" {
		t.Fatalf("uid mismatch: %s", uid)
	}
}

func TestResolver_fallsBackToSession(t *testing.T) {
	r := auth.NewResolver(stubSessions{uid: "u_s"}, testutil.NewFakeAPIKeysRepo())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	uid, err := r.Resolve(req)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if uid != "u_s" {
		t.Fatalf("uid mismatch: %s", uid)
	}
}

func TestResolver_noAuthRejected(t *testing.T) {
	r := auth.NewResolver(stubSessions{err: http.ErrNoCookie}, testutil.NewFakeAPIKeysRepo())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, err := r.Resolve(req); err == nil {
		t.Fatalf("expected error")
	}
}
