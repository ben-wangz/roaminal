package browser

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/api"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/identity"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/coder/websocket"
)

const (
	commandLimit          = 128 * 1024
	eventLimit            = 12 * 1024 * 1024
	browserCommandTimeout = 20 * time.Second

	minViewportWidth  = 1
	maxViewportWidth  = 3840
	minViewportHeight = 1
	maxViewportHeight = 2160
)

type viewportSize struct {
	Width  int
	Height int
}

type Manager struct {
	cfg          config.Config
	worker       string
	mu           sync.Mutex
	process      *process
	viewers      map[*viewer]struct{}
	primary      string
	primaryKnown bool
	shutdown     bool
}

type process struct {
	cmd            *exec.Cmd
	stdin          io.WriteCloser
	writeMu        sync.Mutex
	commandMu      sync.Mutex
	generation     string
	viewport       viewportSize
	pageStatus     string
	pageGeneration string
	pageOperation  int64
	pageRevision   int64
	pageURL        string
	pageTitle      string
	pageError      string
	pageDialog     map[string]any
	latestFrame    []byte
	pending        map[string]*pendingBrowserRequest
}

type viewer struct {
	clientID string
	routeID  string
	runtime  *process
	events   chan []byte
	done     chan struct{}
	closeOne sync.Once
	visible  bool
}

type pendingBrowserRequest struct {
	viewer    *viewer
	requestID string
	timer     *time.Timer
}

func New(cfg config.Config) *Manager {
	return &Manager{cfg: cfg, worker: cfg.BrowserWorkerPath, viewers: make(map[*viewer]struct{})}
}

var _ ports.BrowserRuntime = (*Manager)(nil)

func (m *Manager) Available() bool {
	if !m.cfg.BrowserEnabled || strings.TrimSpace(m.worker) == "" {
		return false
	}
	info, err := os.Stat(m.worker)
	if err != nil || info.IsDir() {
		return false
	}
	chromium := strings.TrimSpace(m.cfg.BrowserChromiumPath)
	if chromium == "" {
		return false
	}
	if filepath.IsAbs(chromium) {
		info, err = os.Stat(chromium)
		return err == nil && !info.IsDir()
	}
	_, err = exec.LookPath(chromium)
	return err == nil
}

func (m *Manager) Handle(ctx context.Context, w http.ResponseWriter, r *http.Request, _ string) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{api.WebSocketProtocol}})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	runtime, err := m.ensureProcess()
	if err != nil {
		_ = conn.Close(websocket.StatusCode(1011), "browser worker unavailable")
		return
	}
	clientID := browserClientID(r)
	viewer := &viewer{clientID: clientID, routeID: newBrowserRequestID("viewer"), runtime: runtime, events: make(chan []byte, 32), done: make(chan struct{})}
	m.addViewer(viewer)
	defer m.removeViewer(viewer)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	commands := make(chan []byte, 16)
	go func() {
		for {
			typ, data, readErr := conn.Read(ctx)
			if readErr != nil {
				cancel()
				return
			}
			if typ != websocket.MessageText || len(data) > commandLimit {
				_ = conn.Close(websocket.StatusMessageTooBig, "browser command too large")
				cancel()
				return
			}
			select {
			case commands <- data:
			case <-ctx.Done():
				return
			}
		}
	}()
	ticker := time.NewTicker(m.cfg.WebsocketPingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-viewer.done:
			return
		case event := <-viewer.events:
			if len(event) > eventLimit {
				_ = conn.Close(websocket.StatusCode(1009), "browser event too large")
				return
			}
			if err := conn.Write(ctx, websocket.MessageText, event); err != nil {
				return
			}
		case command := <-commands:
			if err := m.command(viewer, command); err != nil {
				// Browser commands that can be rejected by another viewer are
				// reported on the WebSocket and do not tear down the viewer.
				if isResizeCommand(command) {
					m.sendCommandError(viewer, err)
					continue
				}
				_ = conn.Close(websocket.StatusPolicyViolation, err.Error())
				return
			}
		case <-ticker.C:
			if err := conn.Ping(ctx); err != nil {
				return
			}
		}
	}
}

func browserClientID(r *http.Request) string {
	value := strings.TrimSpace(r.URL.Query().Get("clientId"))
	if value != "" && len(value) <= 128 && !strings.ContainsAny(value, "\r\n\x00") {
		return value
	}
	if generated, err := (identity.UUIDGenerator{}).NewID(); err == nil && generated != "" {
		return generated
	}
	return fmt.Sprintf("browser-%d", time.Now().UnixNano())
}

func (m *Manager) addViewer(current *viewer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.shutdown || m.process != current.runtime {
		current.closeOne.Do(func() { close(current.done) })
		return
	}
	if m.viewers == nil {
		m.viewers = make(map[*viewer]struct{})
	}
	m.viewers[current] = struct{}{}
	m.enqueueLocked(current, m.stateEventLocked(current))
	if len(current.runtime.latestFrame) > 0 && current.runtime.pageStatus != "none" && current.runtime.pageStatus != "closed" {
		m.enqueueLocked(current, current.runtime.latestFrame)
	}
}

func (m *Manager) removeViewer(current *viewer) {
	runtime := current.runtime
	runtime.commandMu.Lock()
	defer runtime.commandMu.Unlock()
	m.mu.Lock()
	if _, ok := m.viewers[current]; !ok {
		m.mu.Unlock()
		return
	}
	delete(m.viewers, current)
	current.closeOne.Do(func() { close(current.done) })
	forgetViewerRequestsLocked(runtime, current)
	if m.process == current.runtime && m.primary == current.clientID && !m.hasClientLocked(current.runtime, current.clientID) {
		m.primary = ""
		m.broadcastPrimaryLocked()
	}
	lastVisible := m.process == runtime && !m.anyVisibleLocked(runtime)
	m.mu.Unlock()
	if lastVisible {
		_ = writeRuntime(runtime, map[string]any{"type": "visibility", "visible": false})
	}
}

func (m *Manager) hasClientLocked(runtime *process, clientID string) bool {
	for current := range m.viewers {
		if current.runtime == runtime && current.clientID == clientID {
			return true
		}
	}
	return false
}

func (m *Manager) anyVisibleLocked(runtime *process) bool {
	for current := range m.viewers {
		if current.runtime == runtime && current.visible {
			return true
		}
	}
	return false
}
