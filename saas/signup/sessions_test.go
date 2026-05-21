//go:build saas

// saas/signup/sessions_test.go
package signup_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agentic-delegator/core/domain"
	"agentic-delegator/saas/signup"
)

type fakeSessionStore struct {
	uid     domain.UserID
	created [][]byte
}

func (f *fakeSessionStore) Create(ctx context.Context, id []byte, uid domain.UserID, exp time.Time) error {
	f.uid = uid
	f.created = append(f.created, id)
	return nil
}

func (f *fakeSessionStore) ResolveUser(ctx context.Context, id []byte) (domain.UserID, error) {
	if len(f.created) > 0 && bytesEq(f.created[0], id) {
		return f.uid, nil
	}
	return "", domain.ErrNotFound
}

func (f *fakeSessionStore) Delete(ctx context.Context, id []byte) error { return nil }

func bytesEq(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSessions_setAndRead(t *testing.T) {
	s := &fakeSessionStore{}
	mgr := signup.NewSessions(s)

	w := httptest.NewRecorder()
	if err := mgr.Login(context.Background(), w, "u_42"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	cookie := w.Result().Cookies()[0]
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(cookie)

	got, err := mgr.UserFromRequest(r)
	if err != nil {
		t.Fatalf("UserFromRequest: %v", err)
	}
	if got != "u_42" {
		t.Fatalf("uid mismatch: %s", got)
	}
}
