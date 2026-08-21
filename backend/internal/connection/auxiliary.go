package connection

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const auxiliaryOutputLimit = 8 * 1024

func (m *Manager) reserveAuxiliary(transport *Transport) bool {
	m.transportMu.Lock()
	defer m.transportMu.Unlock()
	if !transportAcceptsReuse(transport) {
		return false
	}
	transport.AuxiliaryChannels++
	return true
}

func (m *Manager) releaseAuxiliary(transport *Transport) {
	m.transportMu.Lock()
	if transport.AuxiliaryChannels > 0 {
		transport.AuxiliaryChannels--
	}
	shouldStop := transport.OwnerClosed && transport.Channels == 0 && transport.AuxiliaryChannels == 0
	if shouldStop {
		delete(m.transports, transport.OwnerID)
	}
	m.transportMu.Unlock()
	if shouldStop {
		m.clearRemoteState(transport.OwnerID)
		m.stopTransport(context.Background(), transport)
	}
}

func (m *Manager) runAuxiliary(ctx context.Context, transport *Transport, remoteArgs ...string) ([]byte, error) {
	return m.runAuxiliaryInput(ctx, transport, nil, remoteArgs...)
}

type RemoteCommand struct {
	Script      string
	Args        []string
	Stdin       io.Reader
	OutputLimit int64
	Timeout     time.Duration
}

type RemoteResult struct {
	Output      []byte
	ErrorOutput []byte
}

type auxiliaryReader struct {
	manager   *Manager
	transport *Transport
	stdout    io.ReadCloser
	stderr    *cappedBuffer
	cmd       *exec.Cmd
	mu        sync.Mutex
	finished  bool
	waitErr   error
	cancel    context.CancelFunc
	done      chan struct{}
}

func (m *Manager) RunRemote(ctx context.Context, id string, command RemoteCommand) (RemoteResult, error) {
	reader, err := m.OpenRemote(ctx, id, command)
	if err != nil {
		return RemoteResult{}, err
	}
	limit := command.OutputLimit
	if limit <= 0 {
		limit = auxiliaryOutputLimit
	}
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	closeErr := reader.Close()
	result := RemoteResult{Output: data}
	if auxiliary, ok := reader.(*auxiliaryReader); ok {
		result.ErrorOutput = auxiliary.errorOutput()
	}
	if err != nil {
		return result, err
	}
	if int64(len(data)) > limit {
		return result, errors.New("remote output exceeded limit")
	}
	if closeErr != nil {
		return result, closeErr
	}
	return result, nil
}

func (m *Manager) OpenRemote(ctx context.Context, id string, command RemoteCommand) (io.ReadCloser, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	transport, err := m.remoteTransport(id)
	if err != nil {
		return nil, err
	}
	cancel := func() {}
	if command.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, command.Timeout)
	}
	if command.Script == "" {
		cancel()
		return nil, errors.New("remote script is empty")
	}
	if !m.reserveAuxiliary(transport) {
		cancel()
		return nil, ErrTransportUnavailable
	}
	args := m.auxiliarySSHArgs(transport)
	args = append(args, remoteCommandInvocation(command)...)
	cmd := exec.CommandContext(ctx, m.sshPath, args...)
	if command.Stdin == nil {
		cmd.Stdin = strings.NewReader(command.Script)
	} else {
		cmd.Stdin = command.Stdin
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.releaseAuxiliary(transport)
		cancel()
		return nil, err
	}
	stderr := &cappedBuffer{limit: auxiliaryOutputLimit}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		m.releaseAuxiliary(transport)
		cancel()
		return nil, err
	}
	return &auxiliaryReader{manager: m, transport: transport, stdout: stdout, stderr: stderr, cmd: cmd, cancel: cancel, done: make(chan struct{})}, nil
}

func remoteCommandInvocation(command RemoteCommand) []string {
	args := []string{"sh"}
	if command.Stdin == nil {
		args = append(args, "-s", "--")
	} else {
		args = append(args, "-c", shellQuote(command.Script), "--")
	}
	for _, value := range command.Args {
		args = append(args, shellQuote(value))
	}
	return args
}

func (m *Manager) auxiliarySSHArgs(transport *Transport) []string {
	return []string{"-T", "-o", "ControlMaster=no", "-o", "ControlPersist=no", "-o", "ControlPath=" + transport.ControlPath, "-o", "CanonicalizeHostname=no", "-o", "ProxyCommand=/bin/false", "-o", "BatchMode=yes", "-o", "ClearAllForwardings=yes", "-o", "PermitLocalCommand=no", "-o", "RemoteCommand=none", "--", transport.Alias}
}

func (r *auxiliaryReader) Read(p []byte) (int, error) {
	n, err := r.stdout.Read(p)
	if err != nil {
		r.finish()
	}
	return n, err
}

func (r *auxiliaryReader) Close() error {
	if !r.finishedState() {
		_ = r.stdout.Close()
		if r.cmd.Process != nil {
			_ = r.cmd.Process.Kill()
		}
	}
	r.finish()
	r.mu.Lock()
	err := r.waitErr
	r.mu.Unlock()
	return err
}

func (r *auxiliaryReader) finishedState() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.finished
}

func (r *auxiliaryReader) errorOutput() []byte {
	if r.stderr == nil {
		return nil
	}
	return append([]byte(nil), r.stderr.data.Bytes()...)
}

func (r *auxiliaryReader) finish() {
	r.mu.Lock()
	if r.finished {
		done := r.done
		r.mu.Unlock()
		<-done
		return
	}
	r.finished = true
	r.mu.Unlock()
	err := r.cmd.Wait()
	if r.cancel != nil {
		r.cancel()
	}
	r.manager.releaseAuxiliary(r.transport)
	r.mu.Lock()
	r.waitErr = err
	close(r.done)
	r.mu.Unlock()
}

func (m *Manager) runAuxiliaryInput(ctx context.Context, transport *Transport, input io.Reader, remoteArgs ...string) ([]byte, error) {
	if m.sshPath == "" || transport == nil || !m.reserveAuxiliary(transport) {
		return nil, ErrTransportUnavailable
	}
	defer m.releaseAuxiliary(transport)
	args := []string{"-T", "-o", "ControlMaster=no", "-o", "ControlPersist=no", "-o", "ControlPath=" + transport.ControlPath, "-o", "CanonicalizeHostname=no", "-o", "ProxyCommand=/bin/false", "-o", "BatchMode=yes", "-o", "ClearAllForwardings=yes", "-o", "PermitLocalCommand=no", "-o", "RemoteCommand=none", "--", transport.Alias}
	args = append(args, remoteArgs...)
	command := exec.CommandContext(ctx, m.sshPath, args...)
	stdout, stderr := &cappedBuffer{limit: auxiliaryOutputLimit}, &cappedBuffer{limit: auxiliaryOutputLimit}
	command.Stdout, command.Stderr = stdout, stderr
	command.Stdin = input
	if err := command.Run(); err != nil {
		return nil, err
	}
	if stdout.overflow || stderr.overflow {
		return nil, errors.New("auxiliary output exceeded limit")
	}
	return stdout.data.Bytes(), nil
}

func withAuxiliaryTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, 2*time.Second)
}
