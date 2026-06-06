// core/usecase/ports/runner_service.go
package ports

import (
	"context"

	"agentic-delegator/core/domain"
)

// RunnerStartSpec is everything needed to spawn one runner container.
type RunnerStartSpec struct {
	JobID      domain.JobID
	Repo       string
	BaseBranch string
	WorkBranch string
	Spec       domain.SpecSource
	GitCreds   domain.GitCreds
	Anthropic  domain.AnthropicCreds
	Model      string // empty = adapter default
	LogPath    string // path the runner streams stdout/stderr to
}

// RunnerResult is what the adapter reports when the container exits.
type RunnerResult struct {
	JobID    domain.JobID
	ExitCode int
	PRURL    string // empty if no PR opened
	Error    string // populated when ExitCode != 0

	// NotificationWebhook is the URL the runner discovered in the target repo's
	// .agentic-delegator.yml (empty if none). The completion path fires it.
	NotificationWebhook string
	// LogTail is the last slice of runner output, captured by the adapter for
	// inclusion in the completion webhook payload.
	LogTail string
}

// RunnerService is the outbound port for spawning, supervising, and
// terminating runner containers.
type RunnerService interface {
	// Start spawns a container. The adapter wires its own completion path:
	// when the container exits, it must call onComplete with the result.
	// Returns the container ID once the container is started.
	Start(ctx context.Context, spec RunnerStartSpec, onComplete func(RunnerResult)) (containerID string, err error)

	// Inspect reports whether the container is still alive.
	Inspect(ctx context.Context, containerID string) (alive bool, err error)

	// Stop forcibly terminates a running container. Idempotent.
	Stop(ctx context.Context, containerID string) error
}
