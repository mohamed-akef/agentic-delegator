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
