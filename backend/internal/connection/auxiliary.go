package connection

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

const auxiliaryOutputLimit = 8 * 1024

func (m *Manager) reserveAuxiliary(transport *Transport) bool {
	if m.transportPool == nil || transport == nil {
		return false
	}
	m.transportPool.mu.Lock()
	defer m.transportPool.mu.Unlock()
	if m.transportPool.transports[transport.OwnerID] != transport || !transportAcceptsAuxiliary(transport) {
		return false
	}
	transport.AuxiliaryChannels++
	return true
}

func (m *Manager) acquireAuxiliary(ctx context.Context, transport *Transport) bool {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return false
		default:
		}
	}
	if !m.reserveAuxiliary(transport) {
		return false
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			m.releaseAuxiliary(transport)
			return false
		default:
		}
	}
	if m.transportReady(transport) {
		return true
	}
	m.releaseAuxiliary(transport)
	return false
}

func (m *Manager) releaseAuxiliary(transport *Transport) {
	if m.transportPool == nil || transport == nil {
		return
	}
	m.transportPool.mu.Lock()
	if transport.AuxiliaryChannels > 0 {
		transport.AuxiliaryChannels--
	}
	shouldStop := transport.OwnerClosed && transport.Channels == 0 && transport.AuxiliaryChannels == 0
	if shouldStop {
		if m.transportPool.transports[transport.OwnerID] == transport {
			delete(m.transportPool.transports, transport.OwnerID)
		} else {
			shouldStop = false
		}
	}
	m.transportPool.mu.Unlock()
	if shouldStop {
		m.clearRemoteState(transport.OwnerID)
		m.stopTransport(context.Background(), transport)
	}
}

func (m *Manager) runAuxiliary(ctx context.Context, transport *Transport, remoteArgs ...string) ([]byte, error) {
	return m.runAuxiliaryInput(ctx, transport, nil, remoteArgs...)
}

type RemoteCommand = ports.RemoteCommand
type RemoteResult = ports.RemoteResult

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
		return result, classifyAuxiliaryError(err, result.ErrorOutput)
	}
	if int64(len(data)) > limit {
		return result, errors.New("remote output exceeded limit")
	}
	if closeErr != nil {
		return result, classifyAuxiliaryError(closeErr, result.ErrorOutput)
	}
	return result, nil
}

func (m *Manager) OpenRemote(ctx context.Context, id string, command RemoteCommand) (io.ReadCloser, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if command.Script == "" {
		return nil, errors.New("remote script is empty")
	}
	transport, err := m.auxiliaryTransport(ctx, id)
	if err != nil {
		return nil, err
	}
	cancel := func() {}
	if command.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, command.Timeout)
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

func (m *Manager) runAuxiliaryInput(ctx context.Context, transport *Transport, input io.Reader, remoteArgs ...string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if m.sshPath == "" || transport == nil || !m.acquireAuxiliary(ctx, transport) {
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
		return nil, classifyAuxiliaryError(err, stderr.data.Bytes())
	}
	if stdout.overflow || stderr.overflow {
		return nil, errors.New("auxiliary output exceeded limit")
	}
	return stdout.data.Bytes(), nil
}

func classifyAuxiliaryError(err error, stderr []byte) error {
	if err == nil || errors.Is(err, ports.ErrTransportUnavailable) {
		return err
	}
	message := strings.ToLower(string(stderr))
	for _, marker := range []string{"control socket", "controlpath", "mux_client", "master session"} {
		if strings.Contains(message, marker) {
			return fmt.Errorf("%w: %v", ports.ErrTransportUnavailable, err)
		}
	}
	return err
}

func withAuxiliaryTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, 2*time.Second)
}
