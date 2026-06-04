// core/adapter/clock/system_clock_test.go
package clock_test

import (
	"testing"
	"time"

	"agentic-delegator/core/adapter/clock"
	"agentic-delegator/core/usecase/ports"
)

func TestSystemClock_satisfiesPort(t *testing.T) {
	var _ ports.Clock = clock.System{}
}

func TestSystemClock_NowIsRecent(t *testing.T) {
	c := clock.System{}
	before := time.Now()
	now := c.Now()
	after := time.Now()
	if now.Before(before) || now.After(after) {
		t.Fatalf("Now() should fall between before/after; got %v outside [%v, %v]", now, before, after)
	}
}
