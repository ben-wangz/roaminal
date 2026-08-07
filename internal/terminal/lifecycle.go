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

	"github.com/ben-wangz/roaminal/internal/persistence"
	"github.com/creack/pty"
)

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
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "ROAMINAL_SESSION_ID="+meta.ID, "ROAMINAL_SHELL_READY=1")
	file, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(meta.Cols), Rows: uint16(meta.Rows)})
	if err != nil {
		return nil, fmt.Errorf("start bash: %w", err)
	}
	return &Session{manager: m, meta: meta, cmd: cmd, pty: file, clients: make(map[*Client]struct{})}, nil
}
func (m *Manager) startLoops(session *Session) { go session.readLoop(); go session.waitLoop() }
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
