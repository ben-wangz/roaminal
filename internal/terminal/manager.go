package terminal

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/ben-wangz/roaminal/internal/config"
	"github.com/ben-wangz/roaminal/internal/persistence"
	"github.com/ben-wangz/roaminal/internal/worker"
	"github.com/creack/pty"
)

type ExitStatus struct {
	ExitCode *int `json:"exitCode"`
	Signal   *int `json:"signal"`
}

var ErrClientCapacity = errors.New("client capacity reached")
var ErrControlNotOwner = errors.New("terminal control is owned by another client")

type Summary struct {
	ID         string      `json:"id"`
	CreatedAt  time.Time   `json:"createdAt"`
	UpdatedAt  time.Time   `json:"updatedAt"`
	Shell      string      `json:"shell"`
	InitialCwd string      `json:"initialCwd"`
	Title      string      `json:"title"`
	TitleMode  string      `json:"titleMode"`
	Cwd        string      `json:"cwd"`
	Cols       int         `json:"cols"`
	Rows       int         `json:"rows"`
	Closed     bool        `json:"closed"`
	Attention  bool        `json:"attention"`
	ExitStatus *ExitStatus `json:"exitStatus"`
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
func (c *Client) close() {
	c.closeWith(1000, "")
}
func (c *Client) closeWith(code int, reason string) {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		c.closeCode = code
		c.closeReason = reason
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
		c.closed = true
		c.closeCode = 1013
		c.closeReason = "slow_client"
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
		c.closed = true
		c.closeCode = 1013
		c.closeReason = "slow_client"
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
	createReservations int
	fatal              chan error
}

type Session struct {
	manager       *Manager
	mu            sync.Mutex
	meta          persistence.SessionMeta
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
	currentExec   *persistence.ExecutionRecord
	attention     bool
	closed        bool
}

func NewManager(cfg config.Config, store *persistence.Store, terminalWorker *worker.Client) *Manager {
	return &Manager{cfg: cfg, store: store, worker: terminalWorker, sessions: make(map[string]*Session), fatal: make(chan error, 1)}
}
func (m *Manager) Fatal() <-chan error       { return m.fatal }
func (m *Manager) WorkerFatal(err error)     { m.fail(err) }
func (m *Manager) PersistenceDegraded() bool { return m.store.PersistenceDegraded() }

func (m *Manager) Start(ctx context.Context) error {
	metas, err := m.store.ListSessions()
	if err != nil {
		return err
	}
	for _, meta := range metas {
		if err := m.restore(ctx, meta); err != nil {
			fmt.Fprintf(os.Stderr, "Roaminal session %s restore warning: %v\n", meta.ID, err)
		}
	}
	return nil
}

func (m *Manager) restore(ctx context.Context, meta persistence.SessionMeta) error {
	cwd := meta.Cwd
	if info, err := os.Stat(cwd); err != nil || !info.IsDir() {
		cwd = m.cfg.InitialCwd
		meta.Cwd = cwd
	}
	header, payload, snapshotErr := m.store.LoadSnapshot(meta.ID)
	if snapshotErr != nil && !errors.Is(snapshotErr, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "Roaminal session %s snapshot warning: %v\n", meta.ID, snapshotErr)
	}
	workerReady := false
	if snapshotErr == nil {
		workerReady = true
		if err := m.worker.Restore(ctx, meta.ID, header.Cols, header.Rows, header.ScrollbackLines, header.ThroughSequence, payload); err != nil {
			_ = m.worker.CloseSession(ctx, meta.ID)
			return err
		}
	} else {
		if err := m.worker.Create(ctx, meta.ID, meta.Cols, meta.Rows, m.cfg.ScrollbackLines); err != nil {
			return err
		}
		workerReady = true
	}
	session, err := m.startShell(meta, cwd)
	if err != nil {
		if workerReady {
			_ = m.worker.CloseSession(ctx, meta.ID)
		}
		return err
	}
	if snapshotErr == nil {
		session.sequence, _ = strconv.ParseUint(header.ThroughSequence, 10, 64)
		if header.Cols != meta.Cols || header.Rows != meta.Rows {
			if err := pty.Setsize(session.pty, &pty.Winsize{Cols: uint16(meta.Cols), Rows: uint16(meta.Rows)}); err != nil {
				m.abortSession(ctx, session, workerReady)
				return err
			}
			session.sequence++
			if err := m.worker.Resize(meta.ID, strconv.FormatUint(session.sequence, 10), meta.Cols, meta.Rows); err != nil {
				m.abortSession(ctx, session, workerReady)
				return err
			}
		}
	}
	m.startLoops(session)
	m.mu.Lock()
	m.sessions[meta.ID] = session
	m.mu.Unlock()
	if err := m.store.SaveSession(meta); err != nil {
		m.mu.Lock()
		delete(m.sessions, meta.ID)
		m.mu.Unlock()
		m.abortSession(ctx, session, true)
		return err
	}
	return nil
}

func (m *Manager) startSession(ctx context.Context, meta persistence.SessionMeta, cwd string, createWorker bool) (*Session, error) {
	workerReady := false
	if createWorker {
		if err := m.worker.Create(ctx, meta.ID, meta.Cols, meta.Rows, m.cfg.ScrollbackLines); err != nil {
			return nil, err
		}
		workerReady = true
	}
	session, err := m.startShell(meta, cwd)
	if err != nil {
		if workerReady {
			_ = m.worker.CloseSession(ctx, meta.ID)
		}
		return nil, err
	}
	m.startLoops(session)
	return session, nil
}

func (m *Manager) startShell(meta persistence.SessionMeta, cwd string) (*Session, error) {
	rcfile := findRCFile()
	cmd := exec.Command("/bin/bash", "--noprofile", "--rcfile", rcfile, "-i")
	// creack/pty setsid makes Bash its own process-group leader; Pdeathsig
	// prevents an orphan if the Go process exits unexpectedly.
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "ROAMINAL_SESSION_ID="+meta.ID, "ROAMINAL_SHELL_READY=1")
	file, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(meta.Cols), Rows: uint16(meta.Rows)})
	if err != nil {
		return nil, fmt.Errorf("start bash: %w", err)
	}
	session := &Session{manager: m, meta: meta, cmd: cmd, pty: file, clients: make(map[*Client]struct{})}
	return session, nil
}

func (m *Manager) startLoops(session *Session) {
	go session.readLoop()
	go session.waitLoop()
}

func (m *Manager) abortSession(ctx context.Context, session *Session, workerReady bool) {
	session.mu.Lock()
	session.closed = true
	_ = signalProcessGroup(session.cmd, syscall.SIGTERM)
	_ = session.pty.Close()
	session.mu.Unlock()
	if workerReady {
		_ = m.worker.CloseSession(ctx, session.meta.ID)
	}
}

func findRCFile() string {
	paths := []string{os.Getenv("ROAMINAL_SHELL_RC"), filepath.Join(mustWD(), "shell", "roaminal-bashrc"), "/opt/roaminal/shell/roaminal-bashrc"}
	for _, path := range paths {
		if path != "" {
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}
	return "/etc/bash.bashrc"
}
func mustWD() string { path, _ := os.Getwd(); return path }

func (s *Session) waitLoop() {
	err := s.cmd.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	status := &ExitStatus{}
	if exit, ok := s.cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
		if code := exit.ExitStatus(); code >= 0 {
			status.ExitCode = &code
		}
		if sig := int(exit.Signal()); sig > 0 {
			status.Signal = &sig
		}
	}
	s.meta.UpdatedAt = time.Now().UTC()
	_ = s.manager.store.SaveSession(s.meta)
	s.broadcastLocked(message(map[string]any{"type": "status", "status": "terminated", "code": statusCode(status), "signal": status.Signal}))
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		fmt.Fprintf(os.Stderr, "Roaminal session %s exited: %v\n", s.meta.ID, err)
	}
}
func statusCode(status *ExitStatus) int {
	if status == nil || status.ExitCode == nil {
		return 0
	}
	return *status.ExitCode
}

func (s *Session) readLoop() {
	buffer := make([]byte, 64*1024)
	for {
		n, err := s.pty.Read(buffer)
		if n > 0 {
			s.handleOutput(buffer[:n])
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				fmt.Fprintf(os.Stderr, "Roaminal PTY read %s: %v\n", s.meta.ID, err)
			}
			return
		}
	}
}

func (s *Session) handleOutput(chunk []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = append(s.pending, chunk...)
	text, complete := decodeUTF8(s.pending)
	if !complete {
		return
	}
	s.pending = nil
	cleaned := s.parseMarkersLocked(text)
	if cleaned == "" {
		return
	}
	if s.currentExec != nil {
		s.currentExec.Output += cleaned
		if len([]byte(s.currentExec.Output)) > 960*1024 {
			data := []byte(s.currentExec.Output)
			s.currentExec.Output = string(data[len(data)-960*1024:])
			s.currentExec.Truncated = true
		}
	}
	s.sequence++
	if err := s.manager.worker.Write(s.meta.ID, strconv.FormatUint(s.sequence, 10), []byte(cleaned)); err != nil {
		s.manager.fail(err)
		return
	}
	s.broadcastLocked(message(map[string]any{"type": "output", "data": cleaned}))
	s.scheduleSnapshotLocked()
}

func decodeUTF8(data []byte) (string, bool) {
	if len(data) == 0 {
		return "", true
	}
	var out strings.Builder
	for len(data) > 0 {
		runeValue, size := utf8.DecodeRune(data)
		if runeValue == utf8.RuneError && size == 1 {
			if !utf8.FullRune(data) {
				return out.String(), false
			}
			out.WriteRune(utf8.RuneError)
			data = data[1:]
			continue
		}
		out.Write(data[:size])
		data = data[size:]
	}
	return out.String(), true
}

func (s *Session) parseMarkersLocked(text string) string {
	text = s.markerPending + text
	s.markerPending = ""
	const titlePrefix = "\x1b]0;"
	const markerPrefix = "\x1b]777;roaminal;"
	var cleaned strings.Builder
	for index := 0; index < len(text); {
		relative := strings.IndexByte(text[index:], '\x1b')
		if relative < 0 {
			cleaned.WriteString(text[index:])
			break
		}
		escape := index + relative
		cleaned.WriteString(text[index:escape])
		remainder := text[escape:]
		if strings.HasPrefix(remainder, titlePrefix) {
			endRel := strings.IndexByte(remainder[len(titlePrefix):], '\x07')
			if endRel < 0 {
				s.markerPending = remainder
				break
			}
			title := truncateUTF8(remainder[len(titlePrefix):len(titlePrefix)+endRel], 512)
			s.meta.AutomaticTitle = title
			s.meta.SyncEffectiveTitle()
			s.meta.UpdatedAt = time.Now().UTC()
			_ = s.manager.store.SaveSession(s.meta)
			s.broadcastMetaLocked()
			cleaned.WriteString(remainder[:len(titlePrefix)+endRel+1])
			index = escape + len(titlePrefix) + endRel + 1
			continue
		}
		if strings.HasPrefix(remainder, markerPrefix) {
			endRel := strings.IndexByte(remainder[len(markerPrefix):], '\x07')
			if endRel < 0 {
				s.markerPending = remainder
				break
			}
			marker := remainder[len(markerPrefix) : len(markerPrefix)+endRel]
			s.applyMarkerLocked(marker)
			index = escape + len(markerPrefix) + endRel + 1
			continue
		}
		if isControlPrefix(remainder, titlePrefix) || isControlPrefix(remainder, markerPrefix) {
			s.markerPending = remainder
			break
		}
		cleaned.WriteByte(remainder[0])
		index = escape + 1
	}
	return cleaned.String()
}

func isControlPrefix(value, full string) bool {
	return len(value) < len(full) && strings.HasPrefix(full, value)
}

func truncateUTF8(value string, maxBytes int) string {
	data := []byte(value)
	if len(data) <= maxBytes {
		return value
	}
	data = data[:maxBytes]
	for len(data) > 0 && !utf8.Valid(data) {
		data = data[:len(data)-1]
	}
	return string(data)
}

func decodeMarker(value string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err == nil {
		return decoded, nil
	}
	return base64.RawStdEncoding.DecodeString(value)
}

func (s *Session) applyMarkerLocked(marker string) {
	kind, value, _ := strings.Cut(marker, ":")
	switch kind {
	case "cwd":
		if decoded, err := decodeMarker(value); err == nil && len(decoded) <= 4096 {
			if path := string(decoded); filepath.IsAbs(path) {
				s.meta.Cwd = path
				s.meta.UpdatedAt = time.Now().UTC()
				_ = s.manager.store.SaveSession(s.meta)
				s.broadcastMetaLocked()
			}
		}
	case "start":
		if decoded, err := decodeMarker(value); err == nil {
			command := string(decoded)
			if strings.Contains(command, "_roaminal_") || strings.Contains(command, "ROAMINAL_") {
				return
			}
			id, _ := newID()
			now := time.Now().UTC()
			s.currentExecID = id
			s.currentExec = &persistence.ExecutionRecord{Command: command, StartedAt: now}
			s.broadcastLocked(message(map[string]any{"type": "execution", "phase": "started", "executionId": id, "command": command, "startedAt": now}))
		}
	case "finish":
		if s.currentExec == nil {
			return
		}
		code, err := strconv.Atoi(value)
		if err != nil {
			code = 0
		}
		s.currentExec.ExitCode = &code
		s.currentExec.CompletedAt = time.Now().UTC()
		s.currentExec.DurationMs = s.currentExec.CompletedAt.Sub(s.currentExec.StartedAt).Milliseconds()
		record := *s.currentExec
		s.meta.Executions = append(s.meta.Executions, record)
		if len(s.meta.Executions) > 100 {
			s.meta.Executions = s.meta.Executions[len(s.meta.Executions)-100:]
		}
		s.meta.UpdatedAt = time.Now().UTC()
		if s.controlOwner == nil {
			s.attention = true
		}
		_ = s.manager.store.SaveSession(s.meta)
		s.broadcastLocked(message(map[string]any{"type": "execution", "phase": "completed", "executionId": s.currentExecID, "entry": record}))
		s.currentExec, s.currentExecID = nil, ""
	}
}

func (s *Session) scheduleSnapshotLocked() {
	if s.snapshotTimer == nil {
		s.dirtySince = time.Now()
		s.snapshotTimer = time.AfterFunc(250*time.Millisecond, func() { s.saveSnapshot() })
	}
}
func (s *Session) saveSnapshot() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	sequence := s.sequence
	meta := s.meta
	s.snapshotTimer = nil
	s.dirtySince = time.Time{}
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	payload, through, err := s.manager.worker.Snapshot(ctx, meta.ID, strconv.FormatUint(sequence, 10))
	if err != nil {
		s.manager.storeDegraded(s.meta.ID, err)
		return
	}
	headerSeq, _ := strconv.ParseUint(through, 10, 64)
	_ = headerSeq
	if err := s.manager.store.SaveSnapshot(meta.ID, persistence.SnapshotHeader{Cols: meta.Cols, Rows: meta.Rows, ScrollbackLines: s.manager.cfg.ScrollbackLines, ThroughSequence: through}, payload); err != nil {
		s.manager.storeDegraded(meta.ID, err)
		return
	}
	s.mu.Lock()
	if !s.closed && s.sequence != sequence {
		s.scheduleSnapshotLocked()
	}
	s.mu.Unlock()
}

func (s *Session) broadcastLocked(data []byte) {
	for client := range s.clients {
		if !client.enqueue(data, true) {
			delete(s.clients, client)
		}
	}
}
func message(value any) []byte { data, _ := json.Marshal(value); return data }

func (m *Manager) storeDegraded(id string, err error) {
	m.store.MarkSessionDegraded(id)
	fmt.Fprintf(os.Stderr, "Roaminal persistence degraded: %v\n", err)
}
func (m *Manager) fail(err error) {
	select {
	case m.fatal <- err:
	default:
	}
}

func (m *Manager) Create(ctx context.Context, cwd string, cols, rows int) (Summary, error) {
	if cols == 0 {
		cols = 120
	}
	if rows == 0 {
		rows = 30
	}
	if cols < 2 || cols > 1000 || rows < 1 || rows > 1000 {
		return Summary{}, errors.New("invalid terminal dimensions")
	}
	m.mu.Lock()
	if len(m.sessions)+m.createReservations >= m.cfg.MaxSessions {
		m.mu.Unlock()
		return Summary{}, errors.New("session capacity reached")
	}
	m.createReservations++
	type activity struct {
		cwd                  string
		updatedAt, createdAt time.Time
		id                   string
	}
	activities := make([]activity, 0, len(m.sessions))
	for _, session := range m.sessions {
		session.mu.Lock()
		activities = append(activities, activity{cwd: session.meta.Cwd, updatedAt: session.meta.UpdatedAt, createdAt: session.meta.CreatedAt, id: session.meta.ID})
		session.mu.Unlock()
	}
	m.mu.Unlock()
	reserved := true
	defer func() {
		if reserved {
			m.mu.Lock()
			m.createReservations--
			m.mu.Unlock()
		}
	}()
	sort.Slice(activities, func(i, j int) bool {
		if activities[i].updatedAt.Equal(activities[j].updatedAt) {
			if activities[i].createdAt.Equal(activities[j].createdAt) {
				return activities[i].id > activities[j].id
			}
			return activities[i].createdAt.After(activities[j].createdAt)
		}
		return activities[i].updatedAt.After(activities[j].updatedAt)
	})
	inherited := ""
	if len(activities) > 0 {
		inherited = activities[0].cwd
	}
	if cwd == "" {
		cwd = inherited
	}
	if cwd == "" {
		cwd = m.cfg.InitialCwd
	}
	if !filepath.IsAbs(cwd) {
		return Summary{}, errors.New("cwd must be absolute")
	}
	info, err := os.Stat(cwd)
	if err != nil || !info.IsDir() {
		return Summary{}, errors.New("cwd is not accessible")
	}
	id, err := newID()
	if err != nil {
		return Summary{}, err
	}
	now := time.Now().UTC()
	meta := persistence.SessionMeta{FormatVersion: persistence.SessionFormatVersion, ID: id, AutomaticTitle: "", InitialCwd: cwd, Cwd: cwd, Cols: cols, Rows: rows, CreatedAt: now, UpdatedAt: now, Executions: []persistence.ExecutionRecord{}}
	session, err := m.startSession(ctx, meta, cwd, true)
	if err != nil {
		return Summary{}, err
	}
	if err := m.store.SaveSession(meta); err != nil {
		m.abortSession(ctx, session, true)
		return Summary{}, err
	}
	m.mu.Lock()
	m.createReservations--
	reserved = false
	m.sessions[id] = session
	m.mu.Unlock()
	return m.summary(session), nil
}

func (m *Manager) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	session, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if !ok {
		return os.ErrNotExist
	}
	session.mu.Lock()
	if !session.closed {
		session.closed = true
		_ = signalProcessGroup(session.cmd, syscall.SIGTERM)
		_ = session.pty.Close()
		_ = m.worker.CloseSession(ctx, id)
	}
	for client := range session.clients {
		client.close()
	}
	session.clients = make(map[*Client]struct{})
	session.controlOwner = nil
	session.mu.Unlock()
	return m.store.DeleteSession(id)
}

func (m *Manager) ReserveAttach(id string) error {
	m.mu.RLock()
	session := m.sessions[id]
	m.mu.RUnlock()
	if session == nil {
		return os.ErrNotExist
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.closed {
		return os.ErrProcessDone
	}
	if len(session.clients)+session.reservations >= m.cfg.MaxClientsPerSession {
		return ErrClientCapacity
	}
	session.reservations++
	return nil
}

func (m *Manager) ReleaseAttach(id string) {
	m.mu.RLock()
	session := m.sessions[id]
	m.mu.RUnlock()
	if session == nil {
		return
	}
	session.mu.Lock()
	if session.reservations > 0 {
		session.reservations--
	}
	session.mu.Unlock()
}

func (m *Manager) Attach(ctx context.Context, id string) (*Client, error) {
	return m.attach(ctx, id, false)
}

func (m *Manager) AttachReserved(ctx context.Context, id string) (*Client, error) {
	return m.attach(ctx, id, true)
}

func (m *Manager) attach(ctx context.Context, id string, reserved bool) (*Client, error) {
	m.mu.RLock()
	session, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return nil, os.ErrNotExist
	}
	client := newClient()
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.closed {
		return nil, os.ErrProcessDone
	}
	if reserved {
		if session.reservations < 1 {
			return nil, ErrClientCapacity
		}
		session.reservations--
	} else if len(session.clients)+session.reservations >= m.cfg.MaxClientsPerSession {
		return nil, ErrClientCapacity
	}
	sequence := strconv.FormatUint(session.sequence, 10)
	snapshot, _, err := m.worker.Snapshot(ctx, id, sequence)
	if err != nil {
		if _, payload, loadErr := m.store.LoadSnapshot(id); loadErr == nil {
			snapshot = payload
		} else {
			return nil, err
		}
	}
	client.enqueue(message(map[string]any{"type": "snapshot", "data": string(snapshot)}), false)
	client.enqueue(message(map[string]any{"type": "meta", "title": session.meta.EffectiveTitle(), "titleMode": titleMode(session.meta), "cwd": session.meta.Cwd, "cols": session.meta.Cols, "rows": session.meta.Rows, "attention": session.attention}), false)
	client.enqueue(message(map[string]any{"type": "status", "status": "ready"}), false)
	session.clients[client] = struct{}{}
	return client, nil
}
func (m *Manager) Detach(id string, client *Client) {
	m.mu.RLock()
	session := m.sessions[id]
	m.mu.RUnlock()
	if session == nil {
		client.close()
		return
	}
	session.mu.Lock()
	delete(session.clients, client)
	if session.controlOwner == client {
		session.controlOwner = nil
	}
	client.close()
	session.mu.Unlock()
}
func (m *Manager) ClaimControl(id string, client *Client) error {
	m.mu.RLock()
	session := m.sessions[id]
	m.mu.RUnlock()
	if session == nil {
		return os.ErrNotExist
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.closed {
		return os.ErrProcessDone
	}
	if _, ok := session.clients[client]; !ok {
		return errors.New("client is not attached")
	}
	session.controlOwner = client
	session.attention = false
	return nil
}
func (m *Manager) Input(id string, client *Client, data string) error {
	if len([]byte(data)) > 1024*1024 {
		return errors.New("input too large")
	}
	m.mu.RLock()
	session := m.sessions[id]
	m.mu.RUnlock()
	if session == nil {
		return os.ErrNotExist
	}
	select {
	case <-client.Done():
		return errors.New("client closed")
	default:
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.closed {
		return os.ErrProcessDone
	}
	if session.controlOwner != client {
		return ErrControlNotOwner
	}
	if session.currentExec != nil {
		combined := append([]byte(session.currentExec.Command), []byte(data)...)
		if len(combined) > 64*1024 {
			combined = combined[:64*1024]
			session.currentExec.Truncated = true
		}
		commandBytes := []byte(session.currentExec.Command)
		if len(commandBytes) > len(combined) {
			commandBytes = combined
		}
		session.currentExec.Input = string(combined[len(commandBytes):])
	}
	_, err := session.pty.Write([]byte(data))
	return err
}
func (m *Manager) Resize(id string, client *Client, cols, rows int) error {
	if cols < 2 || cols > 1000 || rows < 1 || rows > 1000 {
		return errors.New("invalid terminal dimensions")
	}
	m.mu.RLock()
	session := m.sessions[id]
	m.mu.RUnlock()
	if session == nil {
		return os.ErrNotExist
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.closed {
		return os.ErrProcessDone
	}
	if session.controlOwner != client {
		return ErrControlNotOwner
	}
	if err := pty.Setsize(session.pty, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)}); err != nil {
		return err
	}
	session.meta.Cols, session.meta.Rows, session.meta.UpdatedAt = cols, rows, time.Now().UTC()
	session.sequence++
	if err := m.worker.Resize(id, strconv.FormatUint(session.sequence, 10), cols, rows); err != nil {
		m.fail(err)
		return err
	}
	_ = m.store.SaveSession(session.meta)
	session.scheduleSnapshotLocked()
	return nil
}
func (m *Manager) Summaries() []Summary {
	m.mu.RLock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.mu.RUnlock()
	result := make([]Summary, 0, len(sessions))
	for _, session := range sessions {
		result = append(result, m.summary(session))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result
}

func (m *Manager) ClientCount(id string) int {
	m.mu.RLock()
	session := m.sessions[id]
	m.mu.RUnlock()
	if session == nil {
		return 0
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	return len(session.clients)
}
func (m *Manager) summary(session *Session) Summary {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.meta.SyncEffectiveTitle()
	return Summary{ID: session.meta.ID, CreatedAt: session.meta.CreatedAt, UpdatedAt: session.meta.UpdatedAt, Shell: "/bin/bash", InitialCwd: session.meta.InitialCwd, Title: session.meta.EffectiveTitle(), TitleMode: titleMode(session.meta), Cwd: session.meta.Cwd, Cols: session.meta.Cols, Rows: session.meta.Rows, Closed: session.closed, Attention: session.attention}
}

func titleMode(meta persistence.SessionMeta) string {
	if meta.TitleOverride != nil {
		return "custom"
	}
	return "automatic"
}

func (s *Session) broadcastMetaLocked() {
	s.meta.SyncEffectiveTitle()
	s.broadcastLocked(message(map[string]any{"type": "meta", "title": s.meta.EffectiveTitle(), "titleMode": titleMode(s.meta), "cwd": s.meta.Cwd, "cols": s.meta.Cols, "rows": s.meta.Rows}))
}

func (m *Manager) SetTitle(id string, title *string) (Summary, error) {
	m.mu.RLock()
	session := m.sessions[id]
	m.mu.RUnlock()
	if session == nil {
		return Summary{}, os.ErrNotExist
	}
	var override *string
	if title != nil {
		value := strings.TrimSpace(*title)
		if err := persistence.ValidateTitleOverride(value); err != nil {
			return Summary{}, err
		}
		override = &value
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.closed {
		return Summary{}, os.ErrProcessDone
	}
	oldOverride := session.meta.TitleOverride
	oldUpdated := session.meta.UpdatedAt
	session.meta.TitleOverride = override
	session.meta.SyncEffectiveTitle()
	session.meta.UpdatedAt = time.Now().UTC()
	if err := m.store.SaveSession(session.meta); err != nil {
		session.meta.TitleOverride = oldOverride
		session.meta.UpdatedAt = oldUpdated
		session.meta.SyncEffectiveTitle()
		return Summary{}, err
	}
	session.broadcastMetaLocked()
	return Summary{ID: session.meta.ID, CreatedAt: session.meta.CreatedAt, UpdatedAt: session.meta.UpdatedAt, Shell: "/bin/bash", InitialCwd: session.meta.InitialCwd, Title: session.meta.EffectiveTitle(), TitleMode: titleMode(session.meta), Cwd: session.meta.Cwd, Cols: session.meta.Cols, Rows: session.meta.Rows, Closed: session.closed}, nil
}
func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.RLock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.mu.RUnlock()
	for _, session := range sessions {
		session.mu.Lock()
		if session.snapshotTimer != nil {
			session.snapshotTimer.Stop()
		}
		session.mu.Unlock()
		session.saveSnapshot()
		session.mu.Lock()
		if !session.closed {
			_ = signalProcessGroup(session.cmd, syscall.SIGTERM)
			_ = session.pty.Close()
		}
		session.mu.Unlock()
	}
	deadline, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = m.worker.Shutdown(deadline)
}

func signalProcessGroup(cmd *exec.Cmd, signal syscall.Signal) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, signal)
}

func newID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:]), nil
}
