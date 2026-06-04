// core/domain/credentials.go
package domain

import "time"

// GitCreds is the short-lived credential the runner uses to clone + push.
// May wrap a short-lived installation token (ExpiresAt set) or a long-lived token (ExpiresAt zero).
type GitCreds struct {
	Token     string
	ExpiresAt time.Time
}

// Expired reports whether the token is past its expiry. A zero ExpiresAt
// means "no expiry tracked" (used for PATs) and is treated as never expired.
func (c GitCreds) Expired(now time.Time) bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return !now.Before(c.ExpiresAt)
}

// AnthropicCreds wraps the credential used to authenticate Claude Code.
// MVP supports only an API key. Phase 2 may add an OAuth bearer.
type AnthropicCreds struct {
	APIKey string
}
