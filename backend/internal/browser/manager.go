package browser

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/api"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/coder/websocket"
)

const (
	commandLimit = 128 * 1024
	eventLimit   = 12 * 1024 * 1024
)

type Manager struct {
	cfg      config.Config
	worker   string
	mu       sync.Mutex
	process  *process
	client   bool
	shutdown bool
}

type process struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	writeMu sync.Mutex
	events  chan []byte
}

func New(cfg config.Config) *Manager {
	return &Manager{cfg: cfg, worker: cfg.BrowserWorkerPath}
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
	if !m.reserveClient() {
		writeError(w, http.StatusConflict, "browser viewer already connected")
		return
	}
	defer m.releaseClient()
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{api.WebSocketProtocol}})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	worker, err := m.ensureProcess()
	if err != nil {
		_ = conn.Close(websocket.StatusCode(1011), "browser worker unavailable")
		return
	}
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
		case event, ok := <-worker.events:
			if !ok {
				_ = conn.Close(websocket.StatusCode(1011), "browser worker stopped")
				return
			}
			if len(event) > eventLimit {
				_ = conn.Close(websocket.StatusCode(1009), "browser event too large")
				return
			}
			if err := conn.Write(ctx, websocket.MessageText, event); err != nil {
				return
			}
		case command := <-commands:
			if err := m.command(command); err != nil {
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

func (m *Manager) reserveClient() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client || m.shutdown {
		return false
	}
	m.client = true
	return true
}

func (m *Manager) releaseClient() {
	m.mu.Lock()
	m.client = false
	m.mu.Unlock()
}

func (m *Manager) ensureProcess() (*process, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.shutdown {
		return nil, errors.New("browser runtime stopped")
	}
	if m.process != nil {
		return m.process, nil
	}
	if !m.Available() {
		return nil, errors.New("browser worker unavailable")
	}
	argv := []string{m.worker}
	if filepath.Ext(m.worker) == ".mjs" {
		argv = []string{"node", m.worker}
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = browserEnvironment(m.cfg.BrowserChromiumPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("browser worker stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("browser worker stdout: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("start browser worker: %w", err)
	}
	runtime := &process{cmd: cmd, stdin: stdin, events: make(chan []byte, 32)}
	m.process = runtime
	go m.readProcess(runtime, stdout)
	go m.waitProcess(runtime)
	return runtime, nil
}

func browserEnvironment(chromiumPath string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, current := range os.Environ() {
		key, _, _ := strings.Cut(current, "=")
		switch key {
		case "ROAMINAL_PASSWORD", "ROAMINAL_WEB_PUSH_VAPID_PRIVATE_KEY", "ROAMINAL_WEB_PUSH_VAPID_PUBLIC_KEY", "ROAMINAL_WEB_PUSH_SUBJECT":
			continue
		}
		env = append(env, current)
	}
	entry := "ROAMINAL_BROWSER_CHROMIUM_PATH=" + chromiumPath
	for index, current := range env {
		if strings.HasPrefix(current, "ROAMINAL_BROWSER_CHROMIUM_PATH=") {
			env[index] = entry
			return env
		}
	}
	return append(env, entry)
}

func (m *Manager) readProcess(runtime *process, stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), eventLimit)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		select {
		case runtime.events <- line:
		default:
			// A slow viewer must not let the worker's stdout grow without bound.
		}
	}
	close(runtime.events)
}

func (m *Manager) waitProcess(runtime *process) {
	_ = runtime.cmd.Wait()
	m.mu.Lock()
	if m.process == runtime {
		m.process = nil
	}
	m.mu.Unlock()
}

func (m *Manager) command(data []byte) error {
	var command map[string]json.RawMessage
	if err := json.Unmarshal(data, &command); err != nil {
		return errors.New("invalid browser command")
	}
	var typ string
	if err := json.Unmarshal(command["type"], &typ); err != nil || typ == "" {
		return errors.New("browser command type is required")
	}
	if (typ == "open" || typ == "navigate") && commandURL(command) == "" {
		return errors.New("browser address must be HTTP or HTTPS")
	}
	switch typ {
	case "open", "navigate", "sync", "back", "forward", "reload", "resize", "visibility", "input", "dialog", "close", "ping":
	default:
		return errors.New("unknown browser command")
	}
	m.mu.Lock()
	runtime := m.process
	m.mu.Unlock()
	if runtime == nil {
		return errors.New("browser worker unavailable")
	}
	if len(data) > commandLimit {
		return errors.New("browser command too large")
	}
	runtime.writeMu.Lock()
	defer runtime.writeMu.Unlock()
	_, err := fmt.Fprintf(runtime.stdin, "%s\n", data)
	return err
}

func (m *Manager) Shutdown(_ context.Context) {
	m.mu.Lock()
	m.shutdown = true
	runtime := m.process
	m.process = nil
	m.mu.Unlock()
	if runtime != nil && runtime.cmd.Process != nil {
		_ = syscall.Kill(-runtime.cmd.Process.Pid, syscall.SIGTERM)
	}
}
