package ports

import "time"

// Clock is the application-facing time source. Feature services use it for
// timestamps and elapsed-time decisions so policy tests do not depend on wall
// clock state.
type Clock interface {
	Now() time.Time
	Since(time.Time) time.Duration
}

// RandomSource is the application-facing entropy source. Infrastructure owns
// the concrete cryptographic implementation.
type RandomSource interface {
	Read([]byte) (int, error)
}

// IDGenerator supplies identifiers for user-owned application resources.
// Implementations belong to infrastructure and are injected by the
// composition root so application services do not depend on crypto packages.
type IDGenerator interface {
	NewID() (string, error)
}
