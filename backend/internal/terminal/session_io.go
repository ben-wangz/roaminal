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
	"time"
	"unicode/utf8"
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
	s.meta.UpdatedAt = time.Now().UTC()
	if s.manager.store != nil {
		_ = s.manager.store.SaveSession(s.meta)
	}
	s.broadcastLocked(message(map[string]any{"type": "status", "status": "terminated", "code": statusCode(status), "signal": status.Signal, "exitStatus": s.exitStatus}))
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		fmt.Fprintf(os.Stderr, "Roaminal session %s exited: %v\n", s.meta.ID, err)
	}
	onExit := s.onExit
	s.mu.Unlock()
	if onExit != nil {
		onExit(*status)
	}
	if err := s.manager.retireSession(context.Background(), s); err != nil {
		fmt.Fprintf(os.Stderr, "Roaminal session %s cleanup warning: %v\n", s.meta.ID, err)
	}
}

func statusCode(status *ExitStatus) int {
	if status == nil || status.ExitCode == nil {
		return 0
	}
	return *status.ExitCode
}
func (s *Session) readLoop() {
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
