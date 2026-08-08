package terminal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/creack/pty"
)

func (m *Manager) Start(ctx context.Context) error {
	metas, err := m.store.ListSessions()
	if err != nil {
		return err
	}
	if m.store.ConnectionLayout() {
		for _, meta := range metas {
			if err := m.restoreHistory(ctx, meta); err != nil {
				return err
			}
		}
		return nil
	}
	for _, meta := range metas {
		if err := m.restore(ctx, meta); err != nil {
			fmt.Fprintf(os.Stderr, "Roaminal session %s restore warning: %v\n", meta.ID, err)
		}
	}
	return nil
}

func (m *Manager) restoreHistory(ctx context.Context, meta persistence.SessionMeta) error {
	if meta.Lifecycle == "live" || meta.Lifecycle == "" {
		meta.Lifecycle = "interrupted"
		meta.BackendRuntimeID = m.runtimeID
	}
	header, payload, err := m.store.LoadSnapshot(meta.ID)
	if err == nil {
		if err := m.worker.Restore(ctx, meta.ID, header.Cols, header.Rows, header.ScrollbackLines, header.ThroughSequence, payload); err != nil {
			return err
		}
	} else if errors.Is(err, os.ErrNotExist) {
		// Metadata can be committed before the first debounced snapshot. Keep
		// that historical instance attachable with an empty worker terminal.
		if err := m.worker.Create(ctx, meta.ID, meta.Cols, meta.Rows, m.cfg.ScrollbackLines); err != nil {
			return err
		}
	} else {
		return err
	}
	session := &Session{manager: m, meta: meta, clients: make(map[*Client]struct{}), closed: true}
	if err == nil {
		session.sequence, _ = strconv.ParseUint(header.ThroughSequence, 10, 64)
	}
	m.mu.Lock()
	m.sessions[meta.ID] = session
	m.mu.Unlock()
	return m.store.SaveSession(meta)
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
	if m.store != nil {
		if err := m.store.SaveSession(meta); err != nil {
			m.mu.Lock()
			delete(m.sessions, meta.ID)
			m.mu.Unlock()
			m.abortSession(ctx, session, true)
			return err
		}
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
	return m.startCommand(meta, cwd, []string{"/bin/bash", "--noprofile", "--rcfile", rcfile, "-i"}, []string{"ROAMINAL_SHELL_READY=1"})
}

func (m *Manager) startCommand(meta persistence.SessionMeta, cwd string, argv []string, extraEnv []string) (*Session, error) {
	if len(argv) == 0 {
		return nil, errors.New("empty process argv")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), append([]string{"TERM=xterm-256color", "ROAMINAL_TERMINAL_ID=" + meta.ID}, extraEnv...)...)
	file, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(meta.Cols), Rows: uint16(meta.Rows)})
	if err != nil {
		return nil, fmt.Errorf("start bash: %w", err)
	}
	return &Session{manager: m, meta: meta, cmd: cmd, pty: file, clients: make(map[*Client]struct{}), command: argv[0]}, nil
}
func (m *Manager) startLoops(session *Session) { go session.readLoop(); go session.waitLoop() }
func (m *Manager) abortSession(ctx context.Context, session *Session, workerReady bool) {
	session.mu.Lock()
	session.closed = true
	cmd := session.cmd
	_ = session.pty.Close()
	session.mu.Unlock()
	_ = terminateSessionProcessGroup(ctx, cmd)
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
