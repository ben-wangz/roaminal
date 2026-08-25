package api

import "fmt"

// ErrorCode is the machine-readable part of the public 0.3 error contract.
// Feature adapters may add a feature-specific value, but they must keep the
// value stable and must not derive it from an arbitrary error string.
type ErrorCode string

const (
	ErrorInvalidRequest        ErrorCode = "invalid_request"
	ErrorUnauthorized          ErrorCode = "unauthorized"
	ErrorForbidden             ErrorCode = "forbidden"
	ErrorNotFound              ErrorCode = "not_found"
	ErrorConflict              ErrorCode = "conflict"
	ErrorCapabilityUnavailable ErrorCode = "capability_unavailable"
	ErrorTimeout               ErrorCode = "timeout"
	ErrorRateLimited           ErrorCode = "rate_limited"
	ErrorInternal              ErrorCode = "internal_error"
)

// ApplicationError is the typed error exchanged between application services
// and HTTP/WebSocket adapters. Cause is for local diagnostics and is never
// serialized directly.
type ApplicationError struct {
	Code      ErrorCode
	Status    int
	Message   string
	Retryable bool
	Field     string
	Details   any
	Cause     error
}

func NewApplicationError(code ErrorCode, status int, message string) *ApplicationError {
	return &ApplicationError{Code: code, Status: status, Message: message, Retryable: status == 408 || status == 429 || status == 502 || status == 503 || status == 504}
}

func (e *ApplicationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *ApplicationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
