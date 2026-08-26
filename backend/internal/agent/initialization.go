package agent

import (
	"context"
	"errors"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func (s *Service) StartInitialization(ctx context.Context, id, origin string) (Initialization, error) {
	if s.store.Err() != nil {
		return Initialization{}, errf("agent_store_unavailable", 503, "Agent state storage is unavailable.", s.store.Err())
	}
	summary, ok := s.findSummary(id)
	if !ok {
		return Initialization{}, errf("agent_instance_not_found", 404, "The connection instance was not found.", nil)
	}
	if summary.Type != "ssh" || !summary.TmuxEnabled || summary.TmuxSessionName == "" {
		return Initialization{}, errf("agent_connection_unsupported", 409, "This connection does not support the Codex Agent.", nil)
	}
	if summary.Lifecycle != "live" {
		return Initialization{}, errf("agent_connection_not_live", 409, "The connection is no longer live.", nil)
	}
	if _, err := s.terms.RemoteTransferInfo(id); err != nil {
		return Initialization{}, errf("agent_transport_unavailable", 503, "The SSH transport is unavailable.", err)
	}
	effective, err := s.terms.ResolveEndpoint(ctx, id)
	if err != nil {
		if errors.Is(err, ports.ErrTransportUnavailable) {
			return Initialization{}, errf("agent_transport_unavailable", 503, "The SSH transport is unavailable.", err)
		}
		return Initialization{}, errf("agent_endpoint_unresolved", 422, "The SSH endpoint could not be resolved.", err)
	}
	endpoint, err := NormalizeEndpoint(effective)
	if err != nil {
		return Initialization{}, errf("agent_endpoint_unresolved", 422, "The SSH endpoint could not be normalized.", err)
	}
	webhookURL, webhookOrigin, err := s.webhookURL(origin)
	if err != nil {
		return Initialization{}, err
	}
	preflightCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	sessionID, sessionCreated, preflightErr := s.targetPreflight(preflightCtx, id, summary.TmuxSessionName)
	cancel()
	if preflightErr != nil {
		if errors.Is(preflightErr, ports.ErrTransportUnavailable) {
			return Initialization{}, errf("agent_transport_unavailable", 503, "The SSH transport is unavailable.", preflightErr)
		}
		return Initialization{}, errf("agent_tmux_session_not_found", 409, "The configured tmux session could not be found.", preflightErr)
	}
	previousRecord, previousExists := s.store.Get(endpoint.Key)
	webhookChanged := previousExists && previousRecord.WebhookOrigin != webhookOrigin
	target := Target{EndpointKey: endpoint.Key, SessionName: summary.TmuxSessionName}
	priorComponent := ""
	if previousExists {
		priorComponent = previousRecord.InstallationState
		if previousTarget := previousRecord.Targets[target.SessionName]; previousTarget.Component == "ready" {
			priorComponent = "ready"
		}
	}
	if err := s.prepareTarget(id, summary, endpoint, target, sessionID, sessionCreated, webhookOrigin); err != nil {
		return Initialization{}, errf("agent_store_unavailable", 503, "Agent state storage is unavailable.", err)
	}
	s.mu.Lock()
	s.pruneOperationsLocked(s.now().UTC())
	if operationID := s.endpointOps[endpoint.Key]; operationID != "" {
		operation := s.operations[operationID]
		if operation != nil && operation.Status == "running" {
			result := *operation
			result.ConnectionInstanceID = id
			result.TmuxSessionName = summary.TmuxSessionName
			result.Endpoint = endpoint
			result.WebhookURL = webhookURL
			result.Joined = true
			s.mu.Unlock()
			logAgentInfo("agent_initialization_joined", operationID, id, "endpoint_key=%q tmux_session=%q", endpoint.Key, summary.TmuxSessionName)
			_ = s.setTargetInitialization(target, operationID)
			return result, nil
		}
	}
	operationID, err := s.newID()
	if err != nil {
		s.mu.Unlock()
		return Initialization{}, errf("agent_install_failed", 502, "The Agent initialization could not start.", err)
	}
	operation := &Initialization{ID: operationID, ConnectionInstanceID: id, Endpoint: endpoint, TmuxSessionName: summary.TmuxSessionName, WebhookURL: webhookURL, Status: "running", StartedAt: s.now().UTC(), PriorComponent: priorComponent}
	s.operations[operationID] = operation
	s.endpointOps[endpoint.Key] = operationID
	s.mu.Unlock()
	logAgentInfo("agent_initialization_started", operationID, id, "endpoint_key=%q tmux_session=%q", endpoint.Key, summary.TmuxSessionName)
	_ = s.setTargetInitialization(target, operationID)
	result := *operation
	go s.executeInitialization(operationID, id, summary, endpoint, target, webhookURL, webhookOrigin, webhookChanged, priorComponent, sessionID, sessionCreated)
	return result, nil
}

func (s *Service) findSummary(id string) (ports.ConnectionInstanceView, bool) {
	if s.terms == nil {
		return ports.ConnectionInstanceView{}, false
	}
	for _, summary := range s.terms.ConnectionInstanceViews() {
		if summary.ID == id {
			return summary, true
		}
	}
	return ports.ConnectionInstanceView{}, false
}

func (s *Service) prepareTarget(id string, summary ports.ConnectionInstanceView, endpoint Endpoint, target Target, sessionID string, sessionCreated int64, origin string) error {
	if err := s.store.Update(endpoint.Key, func(record *EndpointRecord) error {
		if record.User == "" {
			record.User, record.Host, record.Port = endpoint.User, endpoint.Host, endpoint.Port
			record.CreatedAt = s.now().UTC().Format(time.RFC3339Nano)
		}
		if summary.SourceHostAlias != nil && !contains(record.Aliases, *summary.SourceHostAlias) {
			record.Aliases = append(record.Aliases, *summary.SourceHostAlias)
		}
		record.WebhookOrigin = origin
		record.InstallationState = "initializing"
		state := record.Targets[target.SessionName]
		state.SessionName = target.SessionName
		state.SessionID = sessionID
		state.SessionCreated = sessionCreated
		state.Component = "initializing"
		state.Activity = "unknown"
		record.Targets[target.SessionName] = state
		record.UpdatedAt = s.now().UTC().Format(time.RFC3339Nano)
		return nil
	}); err != nil {
		return err
	}
	s.mu.Lock()
	s.bindings[id] = target
	delete(s.runtime, target.EndpointKey+"\x00"+target.SessionName)
	s.mu.Unlock()
	return nil
}
