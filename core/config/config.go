// core/config/config.go
package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
)

// Config is the env-loaded runtime configuration.
type Config struct {
	HTTPBind             string // default "127.0.0.1:8787"
	DSN                  string // Postgres DSN
	MasterKey            []byte // 32 bytes, hex-encoded in env
	RunnerImage          string // e.g. "agentic-delegator-runner:dev"
	WorkDirHost          string // host dir mounted into runners
	MaxConcurrentPerUser int    // default 3
	MaxConcurrentGlobal  int    // default 10
}

func Load() (*Config, error) {
	c := &Config{
		HTTPBind:             getEnv("AGENTIC_HTTP_BIND", "127.0.0.1:8787"),
		DSN:                  getEnv("DELEGATOR_DSN", "postgres://delegator:delegator@127.0.0.1:5433/delegator?sslmode=disable"),
		RunnerImage:          getEnv("AGENTIC_RUNNER_IMAGE", "agentic-delegator-runner:dev"),
		WorkDirHost:          getEnv("AGENTIC_WORK_DIR", "/tmp/agentic-delegator"),
		MaxConcurrentPerUser: getEnvInt("AGENTIC_MAX_CONCURRENT_PER_USER", 3),
		MaxConcurrentGlobal:  getEnvInt("AGENTIC_MAX_CONCURRENT_GLOBAL", 10),
	}

	keyHex := getEnv("AGENTIC_MASTER_KEY", "")
	if keyHex == "" {
		return nil, fmt.Errorf("AGENTIC_MASTER_KEY required (32 bytes hex)")
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("AGENTIC_MASTER_KEY must be 64 hex chars (32 bytes); got %d bytes", len(key))
	}
	c.MasterKey = key

	return c, nil
}

func getEnv(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func getEnvInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
