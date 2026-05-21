// core/usecase/ports/clock.go
package ports

import "time"

// Clock is a port for time. Use cases must depend on this rather than
// time.Now() directly so tests can freeze and advance time.
type Clock interface {
	Now() time.Time
}
