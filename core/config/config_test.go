// core/config/config_test.go
package config_test

import (
	"testing"

	"agentic-delegator/core/config"
)

func TestLoad_defaults(t *testing.T) {
	t.Setenv("DELEGATOR_DSN", "")
	t.Setenv("AGENTIC_MASTER_KEY", "0000000000000000000000000000000000000000000000000000000000000000")
	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.HTTPBind != "127.0.0.1:8787" {
		t.Fatalf("HTTPBind default: %s", c.HTTPBind)
	}
	if c.MaxConcurrentPerUser != 3 {
		t.Fatalf("MaxConcurrentPerUser default: %d", c.MaxConcurrentPerUser)
	}
}

func TestLoad_rejectsBadMasterKey(t *testing.T) {
	t.Setenv("AGENTIC_MASTER_KEY", "tooshort")
	_, err := config.Load()
	if err == nil {
		t.Fatalf("expected error for short master key")
	}
}

func TestValidateForServe(t *testing.T) {
	// A config with no GitHub settings must be rejected.
	bare := &config.Config{}
	if err := bare.ValidateForServe(); err == nil {
		t.Fatal("expected ValidateForServe to fail on empty config")
	}

	full := &config.Config{
		RunnerImage:        "img:dev",
		GHAppID:            123,
		GHAppPrivateKey:    []byte("key"),
		GHAppSlug:          "slug",
		GHClientID:         "cid",
		GHClientSecret:     "csec",
		GHOAuthRedirectURL: "https://x/cb",
		GHWebhookSecret:    []byte("whsec"),
	}
	if err := full.ValidateForServe(); err != nil {
		t.Fatalf("expected complete config to validate, got: %v", err)
	}
}
