package server

import (
	"context"

	"github.com/ben-wangz/roaminal/backend/internal/connection"
	"github.com/ben-wangz/roaminal/backend/internal/filesystem"
	"github.com/ben-wangz/roaminal/backend/internal/monitor"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/ben-wangz/roaminal/backend/internal/sshkey"
)

// connectionService is the application-facing capability set. HTTP handlers
// do not require the concrete manager's SSH, tmux, or runtime internals.
type connectionService interface {
	filesystem.RemoteExecutor
	Summaries() []ports.ConnectionInstanceSummary
	Create(context.Context, string, int, int) (ports.ConnectionInstanceSummary, error)
	CreateRemote(context.Context, string, int, int, string) (ports.ConnectionInstanceSummary, error)
	CreateRemoteLaunchOwned(context.Context, string, int, int, string, string) (ports.ConnectionInstanceSummary, error)
	AbortRemoteLaunch(context.Context, string) error
	Delete(context.Context, string) error
	SetTitle(string, *string) (ports.ConnectionInstanceSummary, error)
	RemoteMonitor(context.Context, string) (monitor.RemoteMonitorSnapshot, error)
	ResolveEndpoint(context.Context, string) (ports.EffectiveEndpoint, error)
	GenerateKey(context.Context, sshkey.GenerationRequest, int, int) (ports.ConnectionInstanceSummary, error)
	PersistenceDegraded() bool

	Fatal() <-chan error
	WorkerFatal(error)
	InitialCwd() string
	RuntimeID() string

	AttachReserved(context.Context, string) (ports.TerminalClient, error)
	AttachPendingReserved(context.Context, string) (ports.TerminalClient, error)
	ReserveAttach(string) error
	ReservePendingAttach(string) error
	ReleaseAttach(string)
	ReleasePendingAttach(string)
	Detach(string, ports.TerminalClient)
	DetachPending(string, ports.TerminalClient)
	Input(string, ports.TerminalClient, string) error
	InputPending(string, ports.TerminalClient, string) error
	Resize(string, ports.TerminalClient, int, int) error
	ResizePending(string, ports.TerminalClient, int, int) error
	ClaimControl(string, ports.TerminalClient) error
	ClaimPendingControl(string, ports.TerminalClient) error
	TouchPending(string)
	PendingOwner(string) string
}

var _ connectionService = (*connection.Manager)(nil)
