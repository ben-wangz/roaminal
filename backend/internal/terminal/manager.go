package terminal

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/clock"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

type ExitStatus = ports.TerminalExitStatus

type executionRecord struct {
	Command     string    `json:"command"`
	ExitCode    *int      `json:"exitCode"`
	Input       string    `json:"input"`
	Output      string    `json:"output"`
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
	DurationMs  int64     `json:"durationMs"`
	Truncated   bool      `json:"truncated"`
}

var ErrClientCapacity = ports.ErrClientCapacity
var ErrConnectionCapacity = ports.ErrConnectionCapacity
var ErrControlNotOwner = ports.ErrControlNotOwner

type Client struct {
	messages    chan []byte
	done        chan struct{}
	mu          sync.Mutex
	queued      int64
	closed      bool
	closeCode   int
	closeReason string
}

func newClient() *Client                          { return &Client{messages: make(chan []byte, 256), done: make(chan struct{})} }
func (c *Client) Done() <-chan struct{}           { return c.done }
func (c *Client) Messages() <-chan []byte         { return c.messages }
func (c *Client) EnqueueControl(data []byte) bool { return c.enqueue(data, false) }
func (c *Client) Consumed(size int) {
	c.mu.Lock()
	c.queued -= int64(size)
	if c.queued < 0 {
		c.queued = 0
	}
	c.mu.Unlock()
}
func (c *Client) close() { c.closeWith(1000, "") }
func (c *Client) closeWith(code int, reason string) {
	c.mu.Lock()
	if !c.closed {
		c.closed, c.closeCode, c.closeReason = true, code, reason
		close(c.done)
	}
	c.mu.Unlock()
}
func (c *Client) CloseReason() (int, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeCode, c.closeReason
}
func (c *Client) enqueue(data []byte, count bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	if count && c.queued+int64(len(data)) > 4*1024*1024 {
		c.closed, c.closeCode, c.closeReason = true, 1013, "slow_client"
		close(c.done)
		return false
	}
	if count {
		c.queued += int64(len(data))
	}
	select {
	case c.messages <- data:
		return true
	default:
		if count {
			c.queued -= int64(len(data))
		}
		c.closed, c.closeCode, c.closeReason = true, 1013, "slow_client"
		close(c.done)
		return false
	}
}

type Manager struct {
	cfg                config.Config
	repositories       Repositories
	worker             ports.TerminalWorker
	clock              ports.Clock
	ids                ports.IDGenerator
	mu                 sync.RWMutex
	sessions           map[string]*Session
	pending            map[string]*Session
	createReservations int
	creates            sync.WaitGroup
	createCtx          context.Context
	stopCreating       context.CancelFunc
	stopping           bool
	storage            *repositoryScope
	fatal              chan error
	runtimeID          string
}

var _ ports.TerminalRuntime = (*Manager)(nil)

type Session struct {
	manager        *Manager
	mu             cleanupMutex
	snapshotMu     cleanupMutex
	retireMu       cleanupMutex
	meta           domain.ConnectionInstanceMeta
	cmd            *exec.Cmd
	pty            *os.File
	clients        map[*Client]struct{}
	reservations   int
	controlOwner   *Client
	sequence       uint64
	streamSequence uint64
	pending        []byte
	markerPending  []byte
	snapshotTimer  *time.Timer
	dirtySince     time.Time
	currentExecID  string
	currentExec    *executionRecord
	attention      bool
	closed         bool
	exitStatus     *ExitStatus
	command        string
	onExit         ports.TerminalExitHook
	exitHookDone   chan struct{}
	readDone       chan struct{}
	retiring       atomic.Bool
	retired        atomic.Bool
	ephemeral      bool
	published      bool
	onMarker       func(string)
	lastActivity   time.Time
	detachedAt     time.Time
	pendingOwner   string
}

func NewManagerWithRepositories(cfg config.Config, repositories Repositories, terminalWorker ports.TerminalWorker, runtimeClock ports.Clock, ids ports.IDGenerator, runtimeID string) *Manager {
	if runtimeClock == nil {
		runtimeClock = clock.System{}
	}
	createCtx, stopCreating := context.WithCancel(context.Background())
	return &Manager{cfg: cfg, repositories: repositories, worker: terminalWorker, clock: runtimeClock, ids: ids, runtimeID: runtimeID,
		sessions: make(map[string]*Session), pending: make(map[string]*Session), storage: newRepositoryScope(),
		createCtx: createCtx, stopCreating: stopCreating, fatal: make(chan error, 1)}
}
func (m *Manager) Fatal() <-chan error   { return m.fatal }
func (m *Manager) WorkerFatal(err error) { m.fail(err) }
func (m *Manager) RuntimeID() string     { return m.runtimeID }
func (m *Manager) InitialCwd() string    { return m.cfg.InitialCwd }
func (m *Manager) now() time.Time        { return m.clock.Now() }
func (m *Manager) newID() (string, error) {
	if m.ids == nil {
		return "", ports.ErrIDUnavailable
	}
	return m.ids.NewID()
}
func (m *Manager) PersistenceDegraded() bool {
	return m.repositories.PersistenceDegraded != nil && m.repositories.PersistenceDegraded()
}

func (m *Manager) hasPersistence() bool { return m.repositories.available() }

func (m *Manager) saveMeta(meta domain.ConnectionInstanceMeta) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return m.saveMetaContext(ctx, meta)
}

func (m *Manager) saveMetaContext(ctx context.Context, meta domain.ConnectionInstanceMeta) error {
	return m.storage.run(ctx, func(ctx context.Context) error { return m.repositories.saveMeta(ctx, meta) })
}

func (m *Manager) loadSnapshot(ctx context.Context, id string) ([]byte, error) {
	var payload []byte
	err := m.storage.run(ctx, func(ctx context.Context) error {
		var err error
		payload, err = m.repositories.loadSnapshot(ctx, domain.ConnectionInstanceID(id))
		return err
	})
	return payload, err
}

func (m *Manager) retirePersistedInstance(ctx context.Context, id string) error {
	return m.storage.run(ctx, func(ctx context.Context) error {
		return m.repositories.retireInstance(ctx, domain.ConnectionInstanceID(id))
	})
}
