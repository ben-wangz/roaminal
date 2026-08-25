package ports

import (
	"context"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

// TerminalExitStatus is the application-facing process outcome. The PTY
// implementation may use a richer internal representation, but connection
// lifecycle code only needs these stable fields.
type TerminalExitStatus struct {
	ExitCode *int `json:"exitCode"`
	Signal   *int `json:"signal"`
}

// TerminalRuntime is the connection-instance lifecycle port implemented by
// the terminal feature. Connection orchestration depends on capabilities,
// not on the PTY manager or its worker implementation.
type TerminalRuntime interface {
	Start(context.Context) error
	Shutdown(context.Context)
	Delete(context.Context, string) error
	Fatal() <-chan error
	WorkerFatal(error)
	PersistenceDegraded() bool
	InitialCwd() string
	RuntimeID() string
	Summaries() []TerminalInstanceSummary
	Create(context.Context, string, int, int) (TerminalInstanceSummary, error)
	AttachReserved(context.Context, string) (TerminalClient, error)
	AttachPendingReserved(context.Context, string) (TerminalClient, error)
	ReserveAttach(string) error
	ReservePendingAttach(string) error
	ReleaseAttach(string)
	ReleasePendingAttach(string)
	Detach(string, TerminalClient)
	DetachPending(string, TerminalClient)
	Input(string, TerminalClient, string) error
	InputPending(string, TerminalClient, string) error
	Resize(string, TerminalClient, int, int) error
	ResizePending(string, TerminalClient, int, int) error
	ClaimControl(string, TerminalClient) error
	ClaimPendingControl(string, TerminalClient) error
	TouchPending(string)
	PendingOwner(string) string
	PromotePending(string, domain.ConnectionInstanceMeta) (TerminalInstanceSummary, error)
	CreatePendingProcessOwned(context.Context, domain.ConnectionInstanceMeta, []string, []string, string, func(string), func(TerminalExitStatus)) (TerminalInstanceSummary, error)
	CreateProcessWithExit(context.Context, domain.ConnectionInstanceMeta, []string, []string, func(TerminalExitStatus)) (TerminalInstanceSummary, error)
	MarkSourceState(string, string) error
	MarkGenerationResult(string, string, string) error
	SetTitle(string, *string) (TerminalInstanceSummary, error)
	AbortPending(context.Context, string) error
}

// TerminalWorker is the terminal runtime's worker-engine port. The worker
// process and its framing protocol are infrastructure adapters.
type TerminalWorker interface {
	Create(context.Context, string, int, int, int) error
	Restore(context.Context, string, int, int, int, string, []byte) error
	Write(string, string, []byte) error
	Resize(string, string, int, int) error
	Snapshot(context.Context, string, string) ([]byte, string, error)
	CloseSession(context.Context, string) error
	Shutdown(context.Context) error
	Available() bool
}

// TerminalClient is the connection-facing stream client. Implementations own
// queueing and close state; callers can only observe or enqueue protocol data.
type TerminalClient interface {
	Done() <-chan struct{}
	CloseReason() (int, string)
	Messages() <-chan []byte
	Consumed(int)
	EnqueueControl([]byte) bool
}
