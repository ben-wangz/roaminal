package filesystem

import (
	"errors"
	"fmt"
)

var (
	ErrUnsupported       = errors.New("filesystem unsupported")
	ErrInstanceNotFound  = errors.New("filesystem connection instance not found")
	ErrNoTransport       = errors.New("filesystem has no SSH transport")
	ErrRootUnavailable   = errors.New("filesystem root unavailable")
	ErrInvalidPath       = errors.New("filesystem path invalid")
	ErrPathOutsideRoot   = errors.New("filesystem path outside root")
	ErrNotFound          = errors.New("filesystem path not found")
	ErrPermissionDenied  = errors.New("filesystem permission denied")
	ErrListingFailed     = errors.New("filesystem listing failed")
	ErrFilenameEncoding  = errors.New("filesystem filename encoding invalid")
	ErrDirectoryTooLarge = errors.New("filesystem directory too large")
	ErrProtocol          = errors.New("filesystem protocol error")
	ErrTimeout           = errors.New("filesystem operation timed out")
	ErrInvalidCursor     = errors.New("filesystem cursor invalid")
)

type RootChangedError struct {
	Root RootContext
}

func (e *RootChangedError) Error() string { return "filesystem root changed" }
func (e *RootChangedError) Unwrap() error { return ErrRootUnavailable }

type RemoteOperationError struct {
	Operation string
	Err       error
}

func (e *RemoteOperationError) Error() string {
	if e.Err == nil {
		return e.Operation
	}
	return fmt.Sprintf("%s: %v", e.Operation, e.Err)
}

func (e *RemoteOperationError) Unwrap() error { return e.Err }
