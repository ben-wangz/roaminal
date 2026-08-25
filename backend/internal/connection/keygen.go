package connection

import (
	"context"
	"fmt"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/ben-wangz/roaminal/backend/internal/sshkey"
)

func (m *Manager) GenerateKey(ctx context.Context, request sshkey.GenerationRequest, cols, rows int) (Summary, error) {
	if m.keys == nil {
		return Summary{}, ErrTransportUnavailable
	}
	id, err := m.newID()
	if err != nil {
		return Summary{}, err
	}
	paths, err := m.keys.PrepareGeneration(id, request)
	if err != nil {
		return Summary{}, err
	}
	cwd := m.InitialCwd()
	meta := domain.ConnectionInstanceMeta{ID: id, BackendRuntimeID: m.RuntimeID(), ConnectionDefinitionID: "local", Type: "local", Purpose: "ssh_key_generation", Lifecycle: "live", SourceState: "current", InitialCwd: cwd, Cwd: cwd, Cols: cols, Rows: rows, AutomaticTitle: "Generate " + request.FileName, GenerationStatus: "running"}
	argv := m.keys.GenerationCommand(paths, request)
	result, err := m.instances.CreateProcessWithExit(ctx, meta, argv, nil, func(status ports.TerminalExitStatus) {
		m.finishKeyGeneration(id, paths, status)
	})
	if err != nil {
		_ = m.keys.DiscardGeneration(paths)
		return Summary{}, err
	}
	return result, nil
}

func (m *Manager) finishKeyGeneration(id string, paths sshkey.GenerationPaths, status ports.TerminalExitStatus) {
	if status.ExitCode == nil || *status.ExitCode != 0 {
		_ = m.keys.DiscardGeneration(paths)
		_ = m.instances.MarkGenerationResult(id, "failed", "ssh-keygen exited unsuccessfully")
		return
	}
	if err := m.keys.Promote(paths); err != nil {
		_ = m.keys.DiscardGeneration(paths)
		_ = m.instances.MarkGenerationResult(id, "promotion_failed", safeGenerationError(err))
		return
	}
	_ = m.instances.MarkGenerationResult(id, "succeeded", "")
}

func safeGenerationError(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 512 {
		message = message[:512]
	}
	return fmt.Sprintf("key promotion failed: %s", message)
}
