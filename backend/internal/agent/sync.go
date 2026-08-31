package agent

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

const (
	syncStatusAvailable   = "available"
	syncStatusPending     = "pending"
	syncStatusMissing     = "missing"
	syncStatusTmuxMissing = "tmux_missing"
	syncStatusStale       = "stale"
	syncStatusInvalid     = "invalid"
	syncStatusUnavailable = "unavailable"
	syncInstanceTimeout   = 15 * time.Second
)

// Start owns one backend scheduler. The first pass is immediate; later passes
// are independent of browser heartbeat traffic and cannot be disabled by one
// failed connection instance.
func (s *Service) Start(parent context.Context) {
	if s == nil {
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	s.mu.Lock()
	if s.syncCancel != nil {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.syncCancel = cancel
	interval := s.syncInterval
	s.syncWait.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.syncWait.Done()
		s.SyncOnce(ctx)
		if interval <= 0 {
			return
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.SyncOnce(ctx)
			}
		}
	}()
}

func (s *Service) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	cancel := s.syncCancel
	s.syncCancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		s.syncWait.Wait()
	}
}

// SyncOnce is exported for startup checks and focused tests. It only selects
// live SSH/tmux instances; all other connection types are invisible here.
func (s *Service) SyncOnce(ctx context.Context) {
	if s == nil || s.terms == nil || s.store == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for _, view := range s.terms.ConnectionInstanceViews() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if !eligibleSyncView(view) {
			continue
		}
		instanceCtx, cancel := context.WithTimeout(ctx, syncInstanceTimeout)
		err := s.syncInstance(instanceCtx, view)
		cancel()
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("level=INFO event=agent_state_sync_failed connection_instance_id=%q tmux_session=%q error_type=%T", view.ConnectionInstanceID, view.TmuxSessionName, err)
		}
	}
}

func eligibleSyncView(view ports.ConnectionInstanceView) bool {
	return view.ID != "" && view.Type == "ssh" && view.Lifecycle == "live" && view.TmuxEnabled && strings.TrimSpace(view.TmuxSessionName) != ""
}

func (s *Service) syncInstance(ctx context.Context, view ports.ConnectionInstanceView) error {
	endpointKey, record, target, known := s.cachedTarget(view)
	if !known {
		effective, err := s.terms.ResolveEndpoint(ctx, view.ID)
		if err != nil {
			return s.recordSyncFailure(view, "unavailable", err)
		}
		endpoint, err := NormalizeEndpoint(effective)
		if err != nil {
			return s.recordSyncFailure(view, "invalid", err)
		}
		endpointKey, record, target = endpoint.Key, EndpointRecord{}, Target{EndpointKey: endpoint.Key, SessionName: view.TmuxSessionName}
		if stored, ok := s.store.Get(endpointKey); ok {
			record = stored
		}
		if err := s.rememberEndpoint(endpoint, view); err != nil {
			return s.recordSyncFailureForTarget(endpointKey, target, syncStatusUnavailable, err)
		}
		s.rememberTarget(view, target)
	}
	if _, err := s.terms.RemoteTransferInfo(view.ID); err != nil {
		return s.recordSyncFailureForTarget(endpointKey, target, "unavailable", err)
	}
	snapshot, err := s.readRemoteState(ctx, view.ID, view.TmuxSessionName)
	if err != nil {
		return s.recordSyncFailureForTarget(endpointKey, target, syncErrorStatus(err), err)
	}
	if err := validateSnapshot(snapshot, view.TmuxSessionName); err != nil {
		return s.recordSyncFailureForTarget(endpointKey, target, syncStatusInvalid, err)
	}
	return s.acceptSnapshot(endpointKey, target, view, record, snapshot)
}

func (s *Service) rememberEndpoint(endpoint Endpoint, view ports.ConnectionInstanceView) error {
	return s.store.Update(endpoint.Key, func(record *EndpointRecord) error {
		if record.User == "" {
			record.User, record.Host, record.Port = endpoint.User, endpoint.Host, endpoint.Port
		}
		if view.SourceHostAlias != nil && !contains(record.Aliases, *view.SourceHostAlias) {
			record.Aliases = append(record.Aliases, *view.SourceHostAlias)
		}
		record.UpdatedAt = s.now().UTC().Format(time.RFC3339Nano)
		return nil
	})
}

func (s *Service) cachedTarget(view ports.ConnectionInstanceView) (string, EndpointRecord, Target, bool) {
	s.mu.Lock()
	target, bound := s.bindings[view.ID]
	s.mu.Unlock()
	if !bound || target.SessionName != view.TmuxSessionName {
		return "", EndpointRecord{}, Target{}, false
	}
	record, ok := s.store.Get(target.EndpointKey)
	if !ok {
		return "", EndpointRecord{}, Target{}, false
	}
	return target.EndpointKey, record, target, true
}

func (s *Service) rememberTarget(view ports.ConnectionInstanceView, target Target) {
	s.mu.Lock()
	if s.bindings == nil {
		s.bindings = map[string]Target{}
	}
	s.bindings[view.ID] = target
	s.mu.Unlock()
}

func (s *Service) recordSyncFailure(view ports.ConnectionInstanceView, status string, cause error) error {
	s.mu.Lock()
	target, bound := s.bindings[view.ID]
	s.mu.Unlock()
	if bound {
		target.SessionName = view.TmuxSessionName
		return s.recordSyncFailureForTarget(target.EndpointKey, target, status, cause)
	}
	target, _, ok := s.targetFor(view)
	if ok {
		return s.recordSyncFailureForTarget(target.EndpointKey, target, status, cause)
	}
	return cause
}

func (s *Service) recordSyncFailureForTarget(endpointKey string, target Target, status string, cause error) error {
	return s.store.Update(endpointKey, func(record *EndpointRecord) error {
		value := record.Targets[target.SessionName]
		value.SessionName = target.SessionName
		value.SyncStatus = status
		value.LastSyncedAt = s.now().UTC()
		value.SyncError = safeSyncError(cause)
		record.Targets[target.SessionName] = value
		record.UpdatedAt = s.now().UTC().Format(time.RFC3339Nano)
		return nil
	})
}

func safeSyncError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 256 {
		return message[:256]
	}
	return message
}
