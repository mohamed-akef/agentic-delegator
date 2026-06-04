// core/testutil/fake_id_generator.go
package testutil

import (
	"fmt"
	"sync/atomic"
)

// FakeIDGenerator hands out deterministic IDs of the form j_1, j_2, … so tests
// can assert against stable identifiers.
type FakeIDGenerator struct {
	jobCounter  uint64
	keyCounter  uint64
	userCounter uint64
}

func (g *FakeIDGenerator) NewJobID() string {
	return fmt.Sprintf("j_%d", atomic.AddUint64(&g.jobCounter, 1))
}

func (g *FakeIDGenerator) NewAPIKeyID() string {
	return fmt.Sprintf("k_%d", atomic.AddUint64(&g.keyCounter, 1))
}

func (g *FakeIDGenerator) NewUserID() string {
	return fmt.Sprintf("u_%d", atomic.AddUint64(&g.userCounter, 1))
}

// NewAPIKeyPlaintext returns a deterministic plaintext + its 8-char prefix.
func (g *FakeIDGenerator) NewAPIKeyPlaintext() (string, string) {
	n := atomic.AddUint64(&g.keyCounter, 1)
	plain := fmt.Sprintf("agdkey_test_%016d", n)
	return plain, plain[:8]
}
