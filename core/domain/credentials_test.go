// core/domain/credentials_test.go
package domain_test

import (
	"testing"
	"time"

	"agentic-delegator/core/domain"
)

func TestGitCreds_Expired(t *testing.T) {
	now := time.Unix(1000, 0)
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"future", now.Add(time.Hour), false},
		{"past", now.Add(-time.Hour), true},
		{"equal", now, true},
		{"zero is never expired", time.Time{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := domain.GitCreds{Token: "t", ExpiresAt: tc.expiresAt}
			if got := c.Expired(now); got != tc.want {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestAnthropicCreds_zeroValue(t *testing.T) {
	var c domain.AnthropicCreds
	if c.APIKey != "" {
		t.Fatalf("zero value should have empty APIKey")
	}
}
