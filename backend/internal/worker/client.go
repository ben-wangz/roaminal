package worker

import (
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	systemclock "github.com/ben-wangz/roaminal/backend/internal/clock"
	"github.com/ben-wangz/roaminal/backend/internal/identity"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/ben-wangz/roaminal/backend/internal/random"
)

var ErrUnavailable = errors.New("terminal worker unavailable")
var ErrWriterQueueFull = errors.New("terminal worker writer queue full")
var ErrWriterStalled = errors.New("terminal worker writer stalled")

var _ ports.TerminalWorker = (*Client)(nil)

type Frame struct {
	Header  json.RawMessage
	Payload []byte
}
type Result struct {
	Header  responseHeader
	Payload []byte
}
type Client struct {
	path        string
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	stdout      io.ReadCloser
	writeQueue  chan writeRequest
	queueMu     sync.Mutex
	queuedBytes int64
	waitMu      sync.Mutex
	waiters     map[string]chan Result
	ready       chan error
	done        chan struct{}
	closeOnce   sync.Once
	callbackMu  sync.Mutex
	onFatal     func(error)
	stopping    atomic.Bool
	clock       ports.Clock
	ids         ports.IDGenerator
}
type writeRequest struct {
	data      []byte
	queueSize int
	done      chan error
}

type Dependencies struct {
	Clock ports.Clock
	IDs   ports.IDGenerator
}

func New(path string, onFatal func(error), dependencies ...Dependencies) *Client {
	deps := Dependencies{Clock: systemclock.System{}}
	if len(dependencies) > 0 {
		if dependencies[0].Clock != nil {
			deps.Clock = dependencies[0].Clock
		}
		deps.IDs = dependencies[0].IDs
	}
	if deps.IDs == nil {
		deps.IDs = identity.UUIDGenerator{Random: random.CryptoSource{}}
	}
	return &Client{path: path, waiters: make(map[string]chan Result), ready: make(chan error, 1), done: make(chan struct{}), onFatal: onFatal, clock: deps.Clock, ids: deps.IDs}
}

func (c *Client) now() time.Time { return c.clock.Now() }

func (c *Client) newID() string {
	if c.ids != nil {
		if value, err := c.ids.NewID(); err == nil && value != "" {
			return value
		}
	}
	return "worker-id-unavailable"
}
