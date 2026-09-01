package connection

import (
	"context"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

// InstanceService owns the terminal runtime for connection instances. The
// outer Manager coordinates definitions, SSH transports, tmux, and auxiliary
// remote capabilities; terminal lifecycle state stays behind this boundary.
type InstanceService struct {
	runtime ports.TerminalRuntime
}

func NewInstanceService(runtime ports.TerminalRuntime) *InstanceService {
	return &InstanceService{runtime: runtime}
}

func (s *InstanceService) Start(ctx context.Context) error { return s.runtime.Start(ctx) }

func (s *InstanceService) Shutdown(ctx context.Context) { s.runtime.Shutdown(ctx) }

func (s *InstanceService) Delete(ctx context.Context, id string) error {
	return s.runtime.Delete(ctx, id)
}

func (s *InstanceService) Fatal() <-chan error { return s.runtime.Fatal() }

func (s *InstanceService) WorkerFatal(err error) { s.runtime.WorkerFatal(err) }

func (s *InstanceService) PersistenceDegraded() bool { return s.runtime.PersistenceDegraded() }

func (s *InstanceService) InitialCwd() string { return s.runtime.InitialCwd() }

func (s *InstanceService) RuntimeID() string { return s.runtime.RuntimeID() }

func (s *InstanceService) Summaries() []Summary {
	items := s.runtime.Summaries()
	result := make([]Summary, 0, len(items))
	for _, item := range items {
		result = append(result, connectionSummary(item))
	}
	return result
}

func (s *InstanceService) Create(ctx context.Context, cwd string, cols, rows int) (Summary, error) {
	item, err := s.runtime.Create(ctx, cwd, cols, rows)
	return connectionSummary(item), err
}

func (s *InstanceService) AttachReserved(ctx context.Context, id string) (ports.TerminalClient, error) {
	return s.runtime.AttachReserved(ctx, id)
}

func (s *InstanceService) AttachPendingReserved(ctx context.Context, id string) (ports.TerminalClient, error) {
	return s.runtime.AttachPendingReserved(ctx, id)
}

func (s *InstanceService) ReserveAttach(id string) error { return s.runtime.ReserveAttach(id) }

func (s *InstanceService) ReservePendingAttach(id string) error {
	return s.runtime.ReservePendingAttach(id)
}

func (s *InstanceService) ReleaseAttach(id string) { s.runtime.ReleaseAttach(id) }

func (s *InstanceService) ReleasePendingAttach(id string) { s.runtime.ReleasePendingAttach(id) }

func (s *InstanceService) Detach(id string, client ports.TerminalClient) {
	s.runtime.Detach(id, client)
}

func (s *InstanceService) DetachPending(id string, client ports.TerminalClient) {
	s.runtime.DetachPending(id, client)
}

func (s *InstanceService) Input(id string, client ports.TerminalClient, data string) error {
	return s.runtime.Input(id, client, data)
}

func (s *InstanceService) InputPending(id string, client ports.TerminalClient, data string) error {
	return s.runtime.InputPending(id, client, data)
}

func (s *InstanceService) Resize(id string, client ports.TerminalClient, cols, rows int) error {
	return s.runtime.Resize(id, client, cols, rows)
}

func (s *InstanceService) ResizePending(id string, client ports.TerminalClient, cols, rows int) error {
	return s.runtime.ResizePending(id, client, cols, rows)
}

func (s *InstanceService) ClaimControl(id string, client ports.TerminalClient) error {
	return s.runtime.ClaimControl(id, client)
}

func (s *InstanceService) ClaimPendingControl(id string, client ports.TerminalClient) error {
	return s.runtime.ClaimPendingControl(id, client)
}

func (s *InstanceService) TouchPending(id string) { s.runtime.TouchPending(id) }

func (s *InstanceService) PendingOwner(id string) string { return s.runtime.PendingOwner(id) }

func (s *InstanceService) PromotePending(id string, meta domain.ConnectionInstanceMeta) (Summary, error) {
	item, err := s.runtime.PromotePending(id, meta)
	return connectionSummary(item), err
}

func (s *InstanceService) CreatePendingProcessOwned(ctx context.Context, meta domain.ConnectionInstanceMeta, argv []string, extraEnv []string, ownerID string, onMarker func(string), onExit func(ports.TerminalExitStatus)) (Summary, error) {
	item, err := s.runtime.CreatePendingProcessOwned(ctx, meta, argv, extraEnv, ownerID, onMarker, onExit)
	return connectionSummary(item), err
}

func (s *InstanceService) CreateProcessWithExit(ctx context.Context, meta domain.ConnectionInstanceMeta, argv []string, extraEnv []string, onExit func(ports.TerminalExitStatus)) (Summary, error) {
	item, err := s.runtime.CreateProcessWithExit(ctx, meta, argv, extraEnv, onExit)
	return connectionSummary(item), err
}

func (s *InstanceService) MarkSourceState(id, state string) error {
	return s.runtime.MarkSourceState(id, state)
}

func (s *InstanceService) MarkGenerationResult(id, state, detail string) error {
	return s.runtime.MarkGenerationResult(id, state, detail)
}

func (s *InstanceService) SetTitle(id string, title *string) (Summary, error) {
	item, err := s.runtime.SetTitle(id, title)
	return connectionSummary(item), err
}

func (s *InstanceService) AbortPending(ctx context.Context, id string) error {
	return s.runtime.AbortPending(ctx, id)
}

func connectionSummary(item ports.TerminalInstanceSummary) Summary {
	return Summary{
		ID: item.ID, ConnectionInstanceID: item.ConnectionInstanceID, ConnectionDefinitionID: item.ConnectionDefinitionID,
		Type: item.Type, Purpose: item.Purpose, Lifecycle: item.Lifecycle, SourceState: item.SourceState,
		SourceHostAlias: item.SourceHostAlias, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		Title: item.Title, TitleMode: item.TitleMode, Cwd: item.Cwd, Cols: item.Cols, Rows: item.Rows,
		TerminalType: item.TerminalType,
		Attention:    item.Attention, GenerationStatus: item.GenerationStatus, GenerationError: item.GenerationError,
		TmuxEnabled: item.TmuxEnabled, TmuxSessionName: item.TmuxSessionName, TmuxPrefixKey: item.TmuxPrefixKey,
		TmuxPrefixSource: item.TmuxPrefixSource,
	}
}
