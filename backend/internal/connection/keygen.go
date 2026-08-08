package connection

import (
	"context"
	"fmt"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/sshkey"
	"github.com/ben-wangz/roaminal/backend/internal/terminal"
)

func (m *Manager) GenerateKey(ctx context.Context, request sshkey.GenerationRequest, cols, rows int) (Summary, error) {
	if m.keys == nil {
		return Summary{}, ErrTransportUnavailable
	}
	id := terminalID()
	paths, err := m.keys.PrepareGeneration(id, request)
	if err != nil {
		return Summary{}, err
	}
	cwd := m.InitialCwd()
	meta := persistence.SessionMeta{ID: id, BackendRuntimeID: m.RuntimeID(), ConnectionDefinitionID: "local", Type: "local", Purpose: "ssh_key_generation", Lifecycle: "live", SourceState: "current", InitialCwd: cwd, Cwd: cwd, Cols: cols, Rows: rows, AutomaticTitle: "Generate " + request.FileName, GenerationStatus: "running", GenerationStaging: paths.StagingDirectory}
	argv := m.keys.GenerationCommand(paths, request)
	result, err := m.Manager.CreateProcessWithExit(ctx, meta, argv, nil, func(status terminal.ExitStatus) {
		m.finishKeyGeneration(id, paths, status)
	})
	if err != nil {
		return Summary{}, err
	}
	return result, nil
}

func (m *Manager) finishKeyGeneration(id string, paths sshkey.GenerationPaths, status terminal.ExitStatus) {
	if status.ExitCode == nil || *status.ExitCode != 0 {
		_ = m.Manager.MarkGenerationResult(id, "failed", "ssh-keygen exited unsuccessfully")
		return
	}
	if err := m.keys.Promote(paths); err != nil {
		_ = m.Manager.MarkGenerationResult(id, "promotion_failed", safeGenerationError(err))
		return
	}
	_ = m.Manager.MarkGenerationResult(id, "succeeded", "")
}

func safeGenerationError(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 512 {
		message = message[:512]
	}
	return fmt.Sprintf("key promotion failed: %s", message)
}
