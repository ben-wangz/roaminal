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
	"github.com/ben-wangz/roaminal/backend/internal/identity"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/coder/websocket"
)

const (
	commandLimit = 128 * 1024
	eventLimit   = 12 * 1024 * 1024

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
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	writeMu    sync.Mutex
	generation string
	viewport   viewportSize
}

type viewer struct {
	clientID string
	runtime  *process
	events   chan []byte
	done     chan struct{}
	closeOne sync.Once
	visible  bool
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
	viewer := &viewer{clientID: clientID, runtime: runtime, events: make(chan []byte, 32), done: make(chan struct{})}
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
	if m.shutdown {
		current.closeOne.Do(func() { close(current.done) })
		return
	}
	if m.viewers == nil {
		m.viewers = make(map[*viewer]struct{})
	}
	m.viewers[current] = struct{}{}
	m.enqueueLocked(current, m.stateEventLocked(current))
}

func (m *Manager) removeViewer(current *viewer) {
	m.mu.Lock()
	if _, ok := m.viewers[current]; !ok {
		m.mu.Unlock()
		return
	}
	delete(m.viewers, current)
	current.closeOne.Do(func() { close(current.done) })
	if m.process == current.runtime && m.primary == current.clientID && !m.hasClientLocked(current.runtime, current.clientID) {
		m.primary = ""
		m.broadcastPrimaryLocked()
	}
	runtime := current.runtime
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
	generation, err := (identity.UUIDGenerator{}).NewID()
	if err != nil || generation == "" {
		generation = fmt.Sprintf("browser-%d", time.Now().UnixNano())
	}
	runtime := &process{cmd: cmd, stdin: stdin, generation: generation, viewport: viewportSize{Width: 1280, Height: 720}}
	m.process = runtime
	m.primary = ""
	m.primaryKnown = false
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
		m.broadcastEvent(runtime, append([]byte(nil), scanner.Bytes()...))
	}
	m.processEnded(runtime)
}

func (m *Manager) waitProcess(runtime *process) {
	_ = runtime.cmd.Wait()
	m.processEnded(runtime)
}

func (m *Manager) processEnded(runtime *process) {
	m.mu.Lock()
	if m.process != runtime {
		m.mu.Unlock()
		return
	}
	m.process = nil
	m.primary = ""
	m.primaryKnown = false
	viewers := make([]*viewer, 0, len(m.viewers))
	for current := range m.viewers {
		viewers = append(viewers, current)
	}
	m.mu.Unlock()
	for _, current := range viewers {
		if current.runtime != runtime {
			continue
		}
		m.enqueue(current, map[string]any{"type": "error", "error": "The remote browser worker stopped.", "code": "browser_worker_stopped"})
		current.closeOne.Do(func() { close(current.done) })
	}
}

func (m *Manager) broadcastEvent(runtime *process, data []byte) {
	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		return
	}
	m.mu.Lock()
	if m.process != runtime {
		m.mu.Unlock()
		return
	}
	if typ, _ := event["type"].(string); typ == "viewport" {
		if width, ok := integerField(event["width"]); ok {
			if height, ok := integerField(event["height"]); ok {
				runtime.viewport = normalizeViewport(viewportSize{Width: width, Height: height})
			}
		}
	}
	event["generation"] = runtime.generation
	for current := range m.viewers {
		if current.runtime != runtime {
			continue
		}
		copyEvent := cloneMap(event)
		if m.primaryKnown {
			copyEvent["primary"] = m.primary == current.clientID
		}
		m.enqueueLocked(current, copyEvent)
	}
	m.mu.Unlock()
}

func (m *Manager) stateEventLocked(current *viewer) map[string]any {
	event := map[string]any{
		"type":       "state",
		"generation": current.runtime.generation,
		"width":      current.runtime.viewport.Width,
		"height":     current.runtime.viewport.Height,
	}
	if m.primaryKnown {
		event["primary"] = m.primary == current.clientID
	}
	return event
}

func (m *Manager) broadcastPrimaryLocked() {
	if m.process == nil {
		return
	}
	for current := range m.viewers {
		if current.runtime != m.process {
			continue
		}
		m.enqueueLocked(current, map[string]any{
			"type":       "primary",
			"generation": m.process.generation,
			"primary":    m.primary != "" && m.primary == current.clientID,
		})
	}
}

func (m *Manager) enqueue(current *viewer, message any) {
	data, err := json.Marshal(message)
	if err != nil {
		return
	}
	m.mu.Lock()
	if _, ok := m.viewers[current]; !ok {
		m.mu.Unlock()
		return
	}
	m.enqueueLocked(current, data)
	m.mu.Unlock()
}

func (m *Manager) enqueueLocked(current *viewer, message any) []byte {
	var data []byte
	if encoded, ok := message.([]byte); ok {
		data = encoded
	} else {
		data, _ = json.Marshal(message)
	}
	if len(data) == 0 || len(data) > eventLimit {
		return data
	}
	select {
	case current.events <- data:
	default:
		// A slow viewer must not let the worker's stdout grow without bound.
		// Preserve lifecycle and ownership messages by evicting an older frame
		// when the bounded queue is full.
		if eventType(data) != "frame" {
			select {
			case <-current.events:
			default:
			}
			select {
			case current.events <- data:
			default:
			}
		}
	}
	return data
}

func eventType(data []byte) string {
	var event struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(data, &event) != nil {
		return ""
	}
	return event.Type
}

func (m *Manager) command(current *viewer, data []byte) error {
	if len(data) > commandLimit {
		return errors.New("browser command too large")
	}
	var command map[string]json.RawMessage
	if err := json.Unmarshal(data, &command); err != nil {
		return errors.New("invalid browser command")
	}
	var typ string
	if err := json.Unmarshal(command["type"], &typ); err != nil || typ == "" {
		return errors.New("browser command type is required")
	}
	if clientID := rawString(command["clientId"]); clientID != "" && clientID != current.clientID {
		return errors.New("browser client identity mismatch")
	}
	if (typ == "open" || typ == "navigate") && commandURL(command) == "" {
		return errors.New("browser address must be HTTP or HTTPS")
	}
	switch typ {
	case "open", "navigate", "sync", "back", "forward", "reload", "input", "dialog", "close", "ping":
		return m.forward(current.runtime, data)
	case "resize":
		return m.resize(current, command)
	case "visibility":
		return m.visibility(current, command)
	default:
		return errors.New("unknown browser command")
	}
}

func (m *Manager) forward(runtime *process, data []byte) error {
	m.mu.Lock()
	if m.process != runtime {
		m.mu.Unlock()
		return errors.New("browser worker unavailable")
	}
	m.mu.Unlock()
	return writeRuntime(runtime, data)
}

func (m *Manager) visibility(current *viewer, command map[string]json.RawMessage) error {
	var visible bool
	if err := json.Unmarshal(command["visible"], &visible); err != nil {
		return errors.New("browser visibility is required")
	}
	m.mu.Lock()
	if _, ok := m.viewers[current]; !ok || m.process != current.runtime {
		m.mu.Unlock()
		return errors.New("browser worker unavailable")
	}
	wasVisible := m.anyVisibleLocked(current.runtime)
	current.visible = visible
	isVisible := m.anyVisibleLocked(current.runtime)
	runtime := current.runtime
	m.mu.Unlock()
	if wasVisible == isVisible {
		return nil
	}
	return writeRuntime(runtime, map[string]any{"type": "visibility", "visible": isVisible})
}

func (m *Manager) resize(current *viewer, command map[string]json.RawMessage) error {
	if rawString(command["clientId"]) == "" {
		return errors.New("browser client identity is required")
	}
	width, widthOK := integerRaw(command["width"])
	height, heightOK := integerRaw(command["height"])
	if !widthOK || !heightOK {
		return errors.New("browser viewport dimensions are required")
	}
	size := normalizeViewport(viewportSize{Width: width, Height: height})
	if size.Width != width || size.Height != height {
		return errors.New("browser viewport dimensions are out of range")
	}
	generation := rawString(command["generation"])
	primaryIntent := false
	if value, ok := optionalBool(command["primaryIntent"]); ok {
		primaryIntent = value
	}
	takeover, _ := optionalBool(command["takeover"])
	if !primaryIntent {
		m.rejectResize(current, "primary_client_required", "Only the primary client can resize the remote page.", generation, size)
		return nil
	}
	m.mu.Lock()
	if _, ok := m.viewers[current]; !ok || m.process != current.runtime {
		m.mu.Unlock()
		return errors.New("browser worker unavailable")
	}
	runtime := current.runtime
	if generation == "" || generation != runtime.generation {
		m.mu.Unlock()
		m.rejectResize(current, "stale_browser_generation", "The remote browser generation has changed.", generation, size)
		return nil
	}
	if m.primaryKnown && m.primary != current.clientID && !takeover {
		m.mu.Unlock()
		m.rejectResize(current, "not_primary_client", "Another browser client controls the viewport.", generation, size)
		return nil
	}
	previousPrimary := m.primary
	previousPrimaryKnown := m.primaryKnown
	m.primary = current.clientID
	m.primaryKnown = true
	previousViewport := runtime.viewport
	if takeover || previousPrimary != m.primary || previousViewport != size {
		if err := writeRuntimeLocked(runtime, map[string]any{"type": "resize", "width": size.Width, "height": size.Height}); err != nil {
			m.primary = previousPrimary
			m.primaryKnown = previousPrimaryKnown
			runtime.viewport = previousViewport
			m.mu.Unlock()
			return err
		}
	}
	runtime.viewport = size
	if previousPrimary != m.primary {
		m.broadcastPrimaryLocked()
	}
	m.enqueueLocked(current, map[string]any{
		"type":       "resize_accepted",
		"generation": runtime.generation,
		"width":      size.Width,
		"height":     size.Height,
		"primary":    true,
		"takeover":   takeover,
	})
	m.mu.Unlock()
	return nil
}

func (m *Manager) rejectResize(current *viewer, code, message, generation string, size viewportSize) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.viewers[current]; !ok {
		return
	}
	event := map[string]any{"type": "resize_rejected", "code": code, "error": message, "primary": false}
	if current.runtime == m.process {
		event["generation"] = current.runtime.generation
		event["width"] = current.runtime.viewport.Width
		event["height"] = current.runtime.viewport.Height
	} else {
		if generation != "" {
			event["generation"] = generation
		}
		if size.Width > 0 && size.Height > 0 {
			event["width"] = size.Width
			event["height"] = size.Height
		}
	}
	m.enqueueLocked(current, event)
}

func (m *Manager) sendCommandError(current *viewer, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.viewers[current]; !ok {
		return
	}
	m.enqueueLocked(current, map[string]any{"type": "error", "error": err.Error(), "code": "invalid_browser_command"})
}

func isResizeCommand(data []byte) bool {
	var command struct {
		Type string `json:"type"`
	}
	return json.Unmarshal(data, &command) == nil && command.Type == "resize"
}

func writeRuntime(runtime *process, message any) error {
	if data, ok := message.([]byte); ok {
		return writeRuntimeLocked(runtime, data)
	}
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return writeRuntimeLocked(runtime, data)
}

func writeRuntimeLocked(runtime *process, message any) error {
	data, ok := message.([]byte)
	if !ok {
		encoded, err := json.Marshal(message)
		if err != nil {
			return err
		}
		data = encoded
	}
	runtime.writeMu.Lock()
	defer runtime.writeMu.Unlock()
	_, err := fmt.Fprintf(runtime.stdin, "%s\n", data)
	return err
}

func normalizeViewport(size viewportSize) viewportSize {
	return viewportSize{
		Width:  maxInt(minViewportWidth, minInt(maxViewportWidth, size.Width)),
		Height: maxInt(minViewportHeight, minInt(maxViewportHeight, size.Height)),
	}
}

func rawString(value json.RawMessage) string {
	var result string
	if json.Unmarshal(value, &result) != nil {
		return ""
	}
	return strings.TrimSpace(result)
}

func integerRaw(value json.RawMessage) (int, bool) {
	var result int
	if len(value) == 0 || json.Unmarshal(value, &result) != nil {
		return 0, false
	}
	return result, true
}

func integerField(value any) (int, bool) {
	number, ok := value.(float64)
	if !ok || number != float64(int(number)) {
		return 0, false
	}
	return int(number), true
}

func optionalBool(value json.RawMessage) (bool, bool) {
	if len(value) == 0 {
		return false, false
	}
	var result bool
	if json.Unmarshal(value, &result) != nil {
		return false, false
	}
	return result, true
}

func cloneMap(value map[string]any) map[string]any {
	copyValue := make(map[string]any, len(value))
	for key, item := range value {
		copyValue[key] = item
	}
	return copyValue
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func (m *Manager) Shutdown(_ context.Context) {
	m.mu.Lock()
	m.shutdown = true
	runtime := m.process
	m.process = nil
	for current := range m.viewers {
		current.closeOne.Do(func() { close(current.done) })
	}
	m.viewers = make(map[*viewer]struct{})
	m.primary = ""
	m.primaryKnown = false
	m.mu.Unlock()
	if runtime != nil && runtime.cmd.Process != nil {
		_ = syscall.Kill(-runtime.cmd.Process.Pid, syscall.SIGTERM)
	}
}
