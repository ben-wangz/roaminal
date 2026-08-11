package terminal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/creack/pty"
)

func (m *Manager) Start(ctx context.Context) error {
	metas, err := m.store.ListConnectionInstances()
	if err != nil {
		return err
	}
	for _, meta := range metas {
		if err := m.retirePersisted(ctx, meta); err != nil {
			fmt.Fprintf(os.Stderr, "Roaminal session %s startup cleanup warning: %v\n", meta.ID, err)
		}
	}
	return nil
}

func (m *Manager) retirePersisted(_ context.Context, meta persistence.ConnectionInstanceMeta) error {
	if meta.Lifecycle == "live" || meta.Lifecycle == "" {
		meta.Lifecycle = "interrupted"
		meta.BackendRuntimeID = m.runtimeID
		meta.UpdatedAt = time.Now().UTC()
		if err := m.store.SaveConnectionInstance(meta); err != nil {
			return err
		}
	}
	if err := m.store.ArchiveConnectionInstance(meta.ID); err != nil {
		return err
	}
	return m.store.DeleteConnectionInstance(meta.ID)
}

func (m *Manager) startSession(ctx context.Context, meta persistence.ConnectionInstanceMeta, cwd string, createWorker bool) (*Session, error) {
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
	return session, nil
}

func (m *Manager) startShell(meta persistence.ConnectionInstanceMeta, cwd string) (*Session, error) {
	rcfile := findRCFile()
	return m.startCommand(meta, cwd, []string{"/bin/bash", "--noprofile", "--rcfile", rcfile, "-i"}, []string{"ROAMINAL_SHELL_READY=1"})
}

func (m *Manager) startCommand(meta persistence.ConnectionInstanceMeta, cwd string, argv []string, extraEnv []string) (*Session, error) {
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
	return &Session{manager: m, meta: meta, cmd: cmd, pty: file, clients: make(map[*Client]struct{}), command: argv[0], readDone: make(chan struct{})}, nil
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
