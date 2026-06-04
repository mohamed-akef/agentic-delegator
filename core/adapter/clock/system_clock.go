// core/adapter/clock/system_clock.go
package clock

import "time"

// System implements ports.Clock against the real OS clock.
type System struct{}

func (System) Now() time.Time { return time.Now() }
