package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func (s *Session) waitLoop() {
	err := s.cmd.Wait()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.meta.Lifecycle = "exited"
	status := &ExitStatus{}
	if exit, ok := s.cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
		if code := exit.ExitStatus(); code >= 0 {
			status.ExitCode = &code
		}
		if sig := int(exit.Signal()); sig > 0 {
			status.Signal = &sig
		}
	}
	if s.exitStatus == nil {
		s.exitStatus = status
	}
	if status.ExitCode != nil {
		s.meta.ExitCode = status.ExitCode
	}
	if status.Signal != nil {
		value := fmt.Sprintf("%d", *status.Signal)
		s.meta.ExitSignal = &value
	}
	s.meta.UpdatedAt = s.manager.now().UTC()
	id, ephemeral := s.meta.ID, s.ephemeral
	s.broadcastMessageLocked(terminatedStreamMessage(s.exitStatus))
	onExit := s.takeExitHookLocked()
	s.mu.Unlock()
	defer s.closePTY()
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		fmt.Fprintf(os.Stderr, "Roaminal session %s exited: %v\n", id, err)
	}
	parent, stop := context.WithTimeout(context.Background(), retirementTimeout)
	defer stop()
	ctx, cancel := s.manager.storage.withContext(parent)
	defer cancel()
	if onExit != nil {
		onExit(ctx, *status)
	}
	if ephemeral {
		s.manager.finishEphemeral(ctx, s)
		return
	}
	if err := s.manager.retireSession(ctx, s); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "Roaminal connection instance %s cleanup warning: %v\n", id, err)
	}
}

func (s *Session) takeExitHookLocked() ports.TerminalExitHook {
	hook := s.onExit
	if hook == nil {
		return nil
	}
	done := make(chan struct{})
	s.onExit, s.exitHookDone = nil, done
	return func(ctx context.Context, status ExitStatus) {
		defer close(done)
		_ = s.manager.storage.run(ctx, func(ctx context.Context) error {
			hook(ctx, status)
			return nil
		})
	}
}

func (s *Session) awaitExitHook(ctx context.Context) error {
	if err := s.mu.lock(ctx); err != nil {
		return err
	}
	done := s.exitHookDone
	s.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
	return ctx.Err()
}

func statusCode(status *ExitStatus) int {
	if status == nil || status.ExitCode == nil {
		return 0
	}
	return *status.ExitCode
}
func (s *Session) readLoop(id string) {
	if s.readDone != nil {
		defer close(s.readDone)
	}
	buffer := make([]byte, 64*1024)
	for {
		n, err := s.pty.Read(buffer)
		if n > 0 {
			s.handleOutput(buffer[:n])
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) {
				fmt.Fprintf(os.Stderr, "Roaminal PTY read %s: %v\n", id, err)
			}
			return
		}
	}
}
func (s *Session) handleOutput(chunk []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Strip invisible markers at the byte level first, so a rune split across
	// a marker boundary (the shell integration emits markers between commands)
	// reassembles once the marker bytes are removed.
	stripped := s.parseMarkersLocked(chunk)
	if len(stripped) == 0 {
		return
	}
	s.pending = append(s.pending, stripped...)
	cleaned, rest := decodeUTF8(s.pending)
	s.pending = rest
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
	s.broadcastMessageLocked(outputStreamMessage(cleaned))
	s.scheduleSnapshotLocked()
}

// decodeUTF8 decodes the complete UTF-8 prefix of data and returns it with
// the remaining bytes of a trailing incomplete rune, which the caller buffers
// until the next chunk arrives.
func decodeUTF8(data []byte) (string, []byte) {
	if len(data) == 0 {
		return "", nil
	}
	var out strings.Builder
	for len(data) > 0 {
		runeValue, size := utf8.DecodeRune(data)
		if runeValue == utf8.RuneError && size == 1 {
			if !utf8.FullRune(data) {
				return out.String(), append([]byte(nil), data...)
			}
			out.WriteRune(utf8.RuneError)
			data = data[1:]
			continue
		}
		out.Write(data[:size])
		data = data[size:]
	}
	return out.String(), nil
}
