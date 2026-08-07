package worker

import (
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
)

var ErrUnavailable = errors.New("terminal worker unavailable")
var ErrWriterQueueFull = errors.New("terminal worker writer queue full")
var ErrWriterStalled = errors.New("terminal worker writer stalled")

type Frame struct {
	Header  map[string]json.RawMessage
	Payload []byte
}
type Result struct {
	Header  map[string]json.RawMessage
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
}
type writeRequest struct {
	data      []byte
	queueSize int
	done      chan error
}

func New(path string, onFatal func(error)) *Client {
	return &Client{path: path, waiters: make(map[string]chan Result), ready: make(chan error, 1), done: make(chan struct{}), onFatal: onFatal}
}
