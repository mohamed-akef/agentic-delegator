//go:build saas

// saas/ghapp/app_jwt.go
package ghapp

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
)

// AppCreds holds the GitHub App credentials needed to mint installation tokens.
type AppCreds struct {
	AppID         int64
	PrivateKeyPEM []byte
}

// AppClient mints short-lived GitHub App installation tokens.
type AppClient struct {
	creds AppCreds
}

// NewAppClient returns an AppClient for the given App credentials.
func NewAppClient(creds AppCreds) *AppClient {
	return &AppClient{creds: creds}
}

// InstallationToken returns a short-lived token (~1h TTL) for the given installation.
// The ghinstallation library caches tokens internally; calling this repeatedly is cheap.
func (a *AppClient) InstallationToken(ctx context.Context, installationID int64) (string, time.Time, error) {
	tr, err := ghinstallation.New(http.DefaultTransport, a.creds.AppID, installationID, a.creds.PrivateKeyPEM)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("ghinstallation.New: %w", err)
	}
	tok, err := tr.Token(ctx)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("Token: %w", err)
	}
	// ghinstallation v2 doesn't expose expiry directly; assume ~50min to be safe.
	exp := time.Now().Add(50 * time.Minute).UTC()
	return tok, exp, nil
}
