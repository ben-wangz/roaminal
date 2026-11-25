package browser

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/identity"
	"github.com/ben-wangz/roaminal/backend/internal/processenv"
)

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
	runtime := &process{cmd: cmd, stdin: stdin, generation: generation, viewport: viewportSize{Width: 1280, Height: 720}, pageStatus: "none", display: defaultBrowserDisplaySettings(), pending: make(map[string]*pendingBrowserRequest)}
	m.process = runtime
	m.primary = ""
	m.primaryKnown = false
	go m.readProcess(runtime, stdout)
	go m.waitProcess(runtime)
	return runtime, nil
}

func browserEnvironment(chromiumPath string) []string {
	return append(processenv.Environment(), "ROAMINAL_BROWSER_CHROMIUM_PATH="+chromiumPath)
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
	for requestID, pending := range runtime.pending {
		if pending.timer != nil {
			pending.timer.Stop()
		}
		delete(runtime.pending, requestID)
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
