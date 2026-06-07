// core/config/config.go
package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the env-loaded runtime configuration.
type Config struct {
	HTTPBind             string        // default "127.0.0.1:8787"
	DSN                  string        // Postgres DSN
	MasterKey            []byte        // 32 bytes, hex-encoded in env
	RunnerImage          string        // e.g. "agentic-delegator-runner:dev"
	RunnerNetwork        string        // AGENTIC_RUNNER_NETWORK; "" = no --network (egress filtering off)
	RunnerDNS            []string      // AGENTIC_RUNNER_DNS; public resolvers, emitted only when RunnerNetwork != ""
	WorkDirHost          string        // host dir mounted into runners
	LogDir               string        // private dir (0700) for per-job log files
	MaxConcurrentPerUser int           // default 3
	MaxConcurrentGlobal  int           // default 10
	MaxJobDuration       time.Duration // hard cap on a single container's run time
	CookieSecure         bool          // mark session/state cookies Secure (HTTPS-only)

	// DB connection pool tuning.
	DBMaxOpenConns int
	DBMaxIdleConns int

	// GitHub App + OAuth (required for `serve`; optional for `migrate`).
	GHAppID            int64
	GHAppPrivateKey    []byte
	GHAppSlug          string
	GHClientID         string
	GHClientSecret     string
	GHOAuthRedirectURL string
	GHWebhookSecret    []byte
}

func Load() (*Config, error) {
	c := &Config{
		HTTPBind:             getEnv("AGENTIC_HTTP_BIND", "127.0.0.1:8787"),
		DSN:                  getEnv("DELEGATOR_DSN", "postgres://delegator:delegator@127.0.0.1:5433/delegator?sslmode=disable"),
		RunnerImage:          getEnv("AGENTIC_RUNNER_IMAGE", "agentic-delegator-runner:dev"),
		RunnerNetwork:        getEnv("AGENTIC_RUNNER_NETWORK", ""),
		RunnerDNS:            getEnvCSV("AGENTIC_RUNNER_DNS", "1.1.1.1,1.0.0.1"),
		WorkDirHost:          getEnv("AGENTIC_WORK_DIR", "/tmp/agentic-delegator"),
		LogDir:               getEnv("AGENTIC_LOG_DIR", ""),
		MaxConcurrentPerUser: getEnvInt("AGENTIC_MAX_CONCURRENT_PER_USER", 3),
		MaxConcurrentGlobal:  getEnvInt("AGENTIC_MAX_CONCURRENT_GLOBAL", 10),
		MaxJobDuration:       time.Duration(getEnvInt("AGENTIC_MAX_JOB_DURATION_SECONDS", 1800)) * time.Second,
		CookieSecure:         getEnvBool("AGENTIC_COOKIE_SECURE", false),
		DBMaxOpenConns:       getEnvInt("AGENTIC_DB_MAX_OPEN_CONNS", 20),
		DBMaxIdleConns:       getEnvInt("AGENTIC_DB_MAX_IDLE_CONNS", 5),

		GHAppID:            getEnvInt64("AGENTIC_GH_APP_ID", 0),
		GHAppPrivateKey:    []byte(getEnv("AGENTIC_GH_APP_PRIVATE_KEY", "")),
		GHAppSlug:          getEnv("AGENTIC_GH_APP_SLUG", ""),
		GHClientID:         getEnv("AGENTIC_GH_CLIENT_ID", ""),
		GHClientSecret:     getEnv("AGENTIC_GH_CLIENT_SECRET", ""),
		GHOAuthRedirectURL: getEnv("AGENTIC_GH_OAUTH_REDIRECT_URL", ""),
		GHWebhookSecret:    []byte(getEnv("AGENTIC_GH_WEBHOOK_SECRET", "")),
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

	if c.LogDir == "" {
		c.LogDir = c.WorkDirHost + "/logs"
	}

	return c, nil
}

// ValidateForServe fails fast at startup if any setting required to actually
// serve requests (GitHub App/OAuth, runner image) is missing — rather than
// booting and breaking on the first user request.
func (c *Config) ValidateForServe() error {
	var missing []string
	check := func(name string, empty bool) {
		if empty {
			missing = append(missing, name)
		}
	}
	check("AGENTIC_GH_APP_ID", c.GHAppID == 0)
	check("AGENTIC_GH_APP_PRIVATE_KEY", len(c.GHAppPrivateKey) == 0)
	check("AGENTIC_GH_APP_SLUG", c.GHAppSlug == "")
	check("AGENTIC_GH_CLIENT_ID", c.GHClientID == "")
	check("AGENTIC_GH_CLIENT_SECRET", c.GHClientSecret == "")
	check("AGENTIC_GH_OAUTH_REDIRECT_URL", c.GHOAuthRedirectURL == "")
	check("AGENTIC_GH_WEBHOOK_SECRET", len(c.GHWebhookSecret) == 0)
	check("AGENTIC_RUNNER_IMAGE", c.RunnerImage == "")
	if len(missing) > 0 {
		return fmt.Errorf("missing required serve config: %s", strings.Join(missing, ", "))
	}
	return nil
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

func getEnvBool(name string, def bool) bool {
	if v := os.Getenv(name); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

// getEnvCSV splits a comma-separated env var, trims whitespace around each
// element, and drops empties. def is the comma-separated default used when the
// var is unset/empty.
func getEnvCSV(name, def string) []string {
	v := os.Getenv(name)
	if v == "" {
		v = def
	}
	var out []string
	for p := range strings.SplitSeq(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func getEnvInt64(name string, def int64) int64 {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}
