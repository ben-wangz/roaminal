package ports

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	ErrRemoteInstanceNotFound = errors.New("remote connection instance not found")
	ErrRemoteNoTransport      = errors.New("no remote transport")
	ErrTransportUnavailable   = errors.New("ssh transport unavailable")
	ErrTransportDraining      = errors.New("ssh transport is draining")
	ErrTmuxNotEnabled         = errors.New("tmux is not enabled for this connection")
	ErrClientCapacity         = errors.New("client capacity reached")
	ErrConnectionCapacity     = errors.New("connection capacity reached")
	ErrControlNotOwner        = errors.New("terminal control is owned by another client")
	ErrIDUnavailable          = errors.New("id generator unavailable")
	ErrRevisionConflict       = errors.New("workspace layout revision conflict")
)

// MonitorTarget is an opaque, monitor-owned probe target. The connection
// adapter may use the key to reuse probe state, but the monitor service never
// receives an SSH transport or command handle.
type MonitorTarget struct {
	OwnerID string
}

type MonitorProbeRequest struct {
	Script string
	Nonce  string
}

type MonitorProbeResult struct {
	Output []byte
}

type MonitorProbe interface {
	Target(context.Context, string) (MonitorTarget, error)
	Probe(context.Context, MonitorTarget, MonitorProbeRequest) (MonitorProbeResult, error)
}

// ConnectionInstanceView is the small connection-instance projection needed
// by remote filesystem policy. It intentionally excludes terminal/runtime
// details and is safe to pass across feature boundaries.
type ConnectionInstanceView struct {
	ID                     string
	ConnectionInstanceID   string
	ConnectionDefinitionID string
	Purpose                string
	Type                   string
	Lifecycle              string
	SourceState            string
	SourceHostAlias        *string
	TmuxEnabled            bool
	TmuxSessionName        string
}

type EffectiveEndpoint struct {
	User string `json:"user"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

type RemoteCommand struct {
	Script      string
	Args        []string
	Stdin       io.Reader
	OutputLimit int64
	Timeout     time.Duration
}

type RemoteResult struct {
	Output      []byte
	ErrorOutput []byte
}

type RemoteTransferInfo struct {
	Alias       string
	ControlPath string
	SSHPath     string
}

// RemoteTransferLease keeps the historical SSH control transport alive for
// the lifetime of a transfer. The lease exposes only immutable command data;
// transport ownership and release stay inside the connection adapter.
type RemoteTransferLease interface {
	Info() RemoteTransferInfo
	Close()
}

type RemoteTransferProvider interface {
	AcquireRemoteTransfer(context.Context, string) (RemoteTransferLease, error)
}

// RemoteFileEntry and RemoteRoot are adapter results. They deliberately do
// not contain filesystem service policy such as relative-path normalization,
// root revisions, pagination cursors, or upload state.
type RemoteFileEntry struct {
	Name       string
	Type       string
	Size       *int64
	ModifiedAt *time.Time
	Mode       uint32
	Symlink    bool
}

type RemoteRoot struct {
	AbsolutePath string
	Source       string
	Status       string
}

// RemoteFileSystem hides shell scripts, remote framing, and transfer-tool
// selection from the FileSystem application service.
type RemoteFileSystem interface {
	ConnectionInstance(string) (ConnectionInstanceView, error)
	ProbeTmuxRoot(context.Context, string, string) (RemoteRoot, error)
	ProbeConfiguredRoot(context.Context, string, string) (RemoteRoot, error)
	List(context.Context, string, string, string) ([]RemoteFileEntry, error)
	Stat(context.Context, string, string, string) (RemoteFileEntry, error)
	OpenContent(context.Context, string, string, string, int64, int64) (io.ReadCloser, error)
	AcquireRemoteTransfer(context.Context, string) (RemoteTransferLease, error)
	ResolveUploadTarget(context.Context, string, string, string) (string, error)
	UploadConflicts(context.Context, string, string, string, []string) ([]string, error)
	RsyncAvailable(context.Context, string) (bool, error)
	CreateDirectories(context.Context, string, string, string, []string) error
	ShouldUploadWithScp(context.Context, string, string, string, int64) (bool, error)
}

type RemoteExecutor interface {
	RemoteTransferProvider
	ConnectionInstance(string) (ConnectionInstanceView, error)
	RunRemote(context.Context, string, RemoteCommand) (RemoteResult, error)
	OpenRemote(context.Context, string, RemoteCommand) (io.ReadCloser, error)
	RemoteTransferInfo(string) (RemoteTransferInfo, error)
}
