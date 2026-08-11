package terminal

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/worker"
)

type ExitStatus struct {
	ExitCode *int `json:"exitCode"`
	Signal   *int `json:"signal"`
}

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

var ErrClientCapacity = errors.New("client capacity reached")
var ErrConnectionCapacity = errors.New("connection capacity reached")
var ErrControlNotOwner = errors.New("terminal control is owned by another client")

type Summary struct {
	ID                     string    `json:"-"`
	ConnectionInstanceID   string    `json:"connectionInstanceId"`
	ConnectionDefinitionID string    `json:"connectionDefinitionId"`
	Type                   string    `json:"type"`
	Purpose                string    `json:"purpose"`
	Lifecycle              string    `json:"lifecycle"`
	SourceState            string    `json:"sourceState"`
	SourceHostAlias        *string   `json:"sourceHostAlias,omitempty"`
	CreatedAt              time.Time `json:"createdAt"`
	UpdatedAt              time.Time `json:"updatedAt"`
	Title                  string    `json:"title"`
	TitleMode              string    `json:"titleMode"`
	Cwd                    string    `json:"cwd"`
	Cols                   int       `json:"cols"`
	Rows                   int       `json:"rows"`
	Attention              bool      `json:"attention"`
	GenerationStatus       string    `json:"generationStatus,omitempty"`
	GenerationError        string    `json:"generationError,omitempty"`
	TmuxEnabled            bool      `json:"tmuxEnabled,omitempty"`
	TmuxSessionName        string    `json:"tmuxSessionName,omitempty"`
	TmuxPrefixKey          string    `json:"tmuxPrefixKey,omitempty"`
	TmuxPrefixSource       string    `json:"tmuxPrefixSource,omitempty"`
}

type Client struct {
	Messages    chan []byte
	done        chan struct{}
	mu          sync.Mutex
	queued      int64
	closed      bool
	closeCode   int
	closeReason string
}

func newClient() *Client                          { return &Client{Messages: make(chan []byte, 256), done: make(chan struct{})} }
func (c *Client) Done() <-chan struct{}           { return c.done }
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
	case c.Messages <- data:
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
	store              *persistence.Store
	worker             *worker.Client
	mu                 sync.RWMutex
	sessions           map[string]*Session
	pending            map[string]*Session
	createReservations int
	fatal              chan error
	runtimeID          string
}
type Session struct {
	manager       *Manager
	mu            sync.Mutex
	meta          persistence.ConnectionInstanceMeta
	cmd           *exec.Cmd
	pty           *os.File
	clients       map[*Client]struct{}
	reservations  int
	controlOwner  *Client
	sequence      uint64
	pending       []byte
	markerPending string
	snapshotTimer *time.Timer
	dirtySince    time.Time
	currentExecID string
	currentExec   *executionRecord
	attention     bool
	closed        bool
	exitStatus    *ExitStatus
	command       string
	onExit        func(ExitStatus)
	readDone      chan struct{}
	retiring      bool
	retired       bool
	ephemeral     bool
	published     bool
	onMarker      func(string)
	lastActivity  time.Time
	detachedAt    time.Time
	pendingOwner  string
}

func NewManager(cfg config.Config, store *persistence.Store, terminalWorker *worker.Client) *Manager {
	return &Manager{cfg: cfg, store: store, worker: terminalWorker, sessions: make(map[string]*Session), pending: make(map[string]*Session), fatal: make(chan error, 1)}
}
func (m *Manager) Fatal() <-chan error       { return m.fatal }
func (m *Manager) WorkerFatal(err error)     { m.fail(err) }
func (m *Manager) SetRuntimeID(id string)    { m.runtimeID = id }
func (m *Manager) RuntimeID() string         { return m.runtimeID }
func (m *Manager) InitialCwd() string        { return m.cfg.InitialCwd }
func (m *Manager) PersistenceDegraded() bool { return m.store != nil && m.store.PersistenceDegraded() }
