package clock

import (
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

// System is the production wall clock implementation.
type System struct{}

func (System) Now() time.Time                      { return time.Now() }
func (System) Since(start time.Time) time.Duration { return time.Since(start) }

// Func adapts a deterministic function for unit tests and small adapters.
type Func func() time.Time

func (f Func) Now() time.Time {
	if f == nil {
		return time.Time{}
	}
	return f()
}

func (f Func) Since(start time.Time) time.Duration { return f.Now().Sub(start) }

var _ ports.Clock = System{}
