// core/testutil/fake_runner_service.go
package testutil

import (
	"context"
	"fmt"
	"sync"

	"agentic-delegator/core/usecase/ports"
)

// FakeRunnerService records every Start call and exposes Complete to drive
// the onComplete callback from tests deterministically.
type FakeRunnerService struct {
	mu           sync.Mutex
	StartErr     error
	started      map[string]ports.RunnerStartSpec
	alive        map[string]bool
	callbacks    map[string]func(ports.RunnerResult)
	nextCounter  int
	StartedSpecs []ports.RunnerStartSpec // append-only history for assertions
}

func NewFakeRunnerService() *FakeRunnerService {
	return &FakeRunnerService{
		started:   map[string]ports.RunnerStartSpec{},
		alive:     map[string]bool{},
		callbacks: map[string]func(ports.RunnerResult){},
	}
}

var _ ports.RunnerService = (*FakeRunnerService)(nil)

func (r *FakeRunnerService) Start(ctx context.Context, spec ports.RunnerStartSpec, onComplete func(ports.RunnerResult)) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.StartErr != nil {
		return "", r.StartErr
	}
	r.nextCounter++
	id := fmt.Sprintf("ctr_%d", r.nextCounter)
	r.started[id] = spec
	r.alive[id] = true
	r.callbacks[id] = onComplete
	r.StartedSpecs = append(r.StartedSpecs, spec)
	return id, nil
}

func (r *FakeRunnerService) Inspect(ctx context.Context, containerID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.alive[containerID], nil
}

func (r *FakeRunnerService) Stop(ctx context.Context, containerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.alive, containerID)
	return nil
}

// Complete simulates the container with this ID exiting. The recorded
// onComplete callback is invoked synchronously.
func (r *FakeRunnerService) Complete(containerID string, result ports.RunnerResult) {
	r.mu.Lock()
	cb := r.callbacks[containerID]
	delete(r.alive, containerID)
	r.mu.Unlock()
	if cb != nil {
		cb(result)
	}
}

// SetAlive forces an entry into the alive map. Used by tests that don't go
// through Start (e.g., startup recovery tests).
func (r *FakeRunnerService) SetAlive(containerID string, alive bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.alive[containerID] = alive
}
