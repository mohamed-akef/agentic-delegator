// core/adapter/docker/network.go
package docker

import (
	"context"
	"fmt"
	"os/exec"
)

// PreflightNetwork fails fast at startup if a runner egress network is
// configured but absent. Running unfiltered after an operator opted into
// filtering is a security footgun, so we refuse to serve. An empty name means
// egress filtering is disabled (dev/local/CI) and is a no-op.
func PreflightNetwork(ctx context.Context, name string) error {
	return checkNetwork(name, func(n string) error {
		return exec.CommandContext(ctx, "docker", "network", "inspect", n).Run()
	})
}

// checkNetwork holds the pure decision so it is unit-testable without docker:
// empty name => skip; otherwise the injected inspect must succeed.
func checkNetwork(name string, inspect func(string) error) error {
	if name == "" {
		return nil
	}
	if err := inspect(name); err != nil {
		return fmt.Errorf("runner network %q not found — create it (see deploy/firewall) or unset AGENTIC_RUNNER_NETWORK: %w", name, err)
	}
	return nil
}
