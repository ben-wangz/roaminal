package agent

import (
	"context"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/connection"
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
		return Initialization{}, errf("agent_transport_unavailable", 409, "The SSH transport is unavailable.", err)
	}
	effective, err := s.terms.ResolveEndpoint(ctx, id)
	if err != nil {
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
	s.pruneOperationsLocked(time.Now().UTC())
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
			_ = s.setTargetInitialization(target, operationID)
			return result, nil
		}
	}
	operationID, err := randomID()
	if err != nil {
		s.mu.Unlock()
		return Initialization{}, errf("agent_install_failed", 502, "The Agent initialization could not start.", err)
	}
	operation := &Initialization{ID: operationID, ConnectionInstanceID: id, Endpoint: endpoint, TmuxSessionName: summary.TmuxSessionName, WebhookURL: webhookURL, Status: "running", StartedAt: time.Now().UTC(), PriorComponent: priorComponent}
	s.operations[operationID] = operation
	s.endpointOps[endpoint.Key] = operationID
	s.mu.Unlock()
	_ = s.setTargetInitialization(target, operationID)
	result := *operation
	go s.executeInitialization(operationID, id, summary, endpoint, target, webhookURL, webhookOrigin, webhookChanged, priorComponent, sessionID, sessionCreated)
	return result, nil
}

func (s *Service) findSummary(id string) (connection.Summary, bool) {
	if s.terms == nil {
		return connection.Summary{}, false
	}
	for _, summary := range s.terms.Summaries() {
		if summary.ID == id {
			return summary, true
		}
	}
	return connection.Summary{}, false
}

func (s *Service) prepareTarget(id string, summary connection.Summary, endpoint Endpoint, target Target, sessionID string, sessionCreated int64, origin string) error {
	if err := s.store.Update(endpoint.Key, func(record *EndpointRecord) error {
		if record.User == "" {
			record.User, record.Host, record.Port = endpoint.User, endpoint.Host, endpoint.Port
			record.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
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
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
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

func (s *Service) executeInitialization(operationID, id string, summary connection.Summary, endpoint Endpoint, target Target, webhookURL, webhookOrigin string, webhookChanged bool, priorComponent string, sessionID string, sessionCreated int64) {
	lock := s.endpointMutex(endpoint.Key)
	lock.Lock()
	defer lock.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	remoteOS, remoteArch, err := s.remotePlatformWithRetry(ctx, id)
	if err != nil {
		s.failInitialization(operationID, target, err)
		return
	}
	asset, binary, manifest, err := s.loadAsset(remoteOS, remoteArch)
	if err != nil {
		s.failInitialization(operationID, target, err)
		return
	}
	remoteState, err := s.existingProbeWithRetry(ctx, id)
	if err != nil {
		s.failInitialization(operationID, target, err)
		return
	}
	remoteFingerprint := remoteState.TokenFingerprint
	record, exists := s.store.Get(endpoint.Key)
	if (remoteState.Configured && remoteFingerprint == "") || (remoteFingerprint != "" && !exists) {
		s.failInitialization(operationID, target, errf("agent_binding_conflict", 409, "The remote Agent binding belongs to another Roaminal state.", nil))
		return
	}
	if remoteFingerprint != "" && exists && record.ActiveTokenHash == remoteFingerprint {
		componentCurrent := !webhookChanged && remoteState.EndpointKey == endpoint.Key &&
			remoteState.ComponentVersion == manifest.ComponentVersion &&
			remoteState.ComponentSHA256 == asset.SHA256 && remoteState.HooksConfigured
		if !componentCurrent {
			if err := s.repairExisting(ctx, id, target, endpoint, webhookURL, remoteFingerprint, manifest.ComponentVersion, asset.SHA256, webhookOrigin, binary); err != nil {
				s.failInitialization(operationID, target, err)
				return
			}
			s.completeInitialization(operationID, target, "upgraded", true, "needs_trust", manifest.ComponentVersion)
			return
		}
		component := record.InstallationState
		if component == "" || component == "initializing" || component == "error" {
			component = "needs_trust"
			if componentCurrent && priorComponent == "ready" {
				component = "ready"
			}
		}
		if err := s.markTargetComponent(target, component, manifest.ComponentVersion); err != nil {
			s.failInitialization(operationID, target, errf("agent_store_unavailable", 503, "Agent state storage is unavailable.", err))
			return
		}
		s.completeInitialization(operationID, target, "already_current", false, component, manifest.ComponentVersion)
		return
	}
	if remoteFingerprint != "" && exists && record.PendingTokenHash == remoteFingerprint {
		if err := s.commitPending(endpoint.Key, target, manifest.ComponentVersion, asset.SHA256); err != nil {
			s.failInitialization(operationID, target, errf("agent_store_unavailable", 503, "Agent state storage is unavailable.", err))
			return
		}
		componentCurrent := !webhookChanged && remoteState.EndpointKey == endpoint.Key &&
			remoteState.ComponentVersion == manifest.ComponentVersion &&
			remoteState.ComponentSHA256 == asset.SHA256 && remoteState.HooksConfigured
		if !componentCurrent {
			if err := s.repairExisting(ctx, id, target, endpoint, webhookURL, remoteFingerprint, manifest.ComponentVersion, asset.SHA256, webhookOrigin, binary); err != nil {
				s.failInitialization(operationID, target, err)
				return
			}
			s.completeInitialization(operationID, target, "upgraded", true, "needs_trust", manifest.ComponentVersion)
			return
		}
		s.completeInitialization(operationID, target, "already_current", false, "needs_trust", manifest.ComponentVersion)
		return
	}
	if remoteFingerprint != "" && exists && record.PreviousTokenHash != remoteFingerprint {
		s.failInitialization(operationID, target, errf("agent_binding_conflict", 409, "The remote Agent binding belongs to another Roaminal state.", nil))
		return
	}
	token, tokenHashValue, err := randomToken()
	if err != nil {
		s.failInitialization(operationID, target, errf("agent_install_failed", 502, "The Agent token could not be created.", err))
		return
	}
	installedComponent := "needs_trust"
	if err := s.store.Update(endpoint.Key, func(value *EndpointRecord) error {
		value.PendingTokenHash = tokenHashValue
		value.PendingCreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		value.ComponentVersion = manifest.ComponentVersion
		value.ComponentSHA256 = asset.SHA256
		value.InstallationState = "initializing"
		return nil
	}); err != nil {
		s.failInitialization(operationID, target, errf("agent_store_unavailable", 503, "Agent state storage is unavailable.", err))
		return
	}
	request := installRequest{
		SchemaVersion:                   1,
		Endpoint:                        Endpoint{Key: endpoint.Key, User: endpoint.User, Host: endpoint.Host, Port: endpoint.Port},
		WebhookURL:                      webhookURL,
		ExpectedCurrentTokenFingerprint: remoteFingerprint,
		ReplacementToken:                token,
		ComponentVersion:                manifest.ComponentVersion,
		ComponentSHA256:                 asset.SHA256,
		TmuxSessionName:                 target.SessionName,
	}
	if err := s.installRemote(ctx, id, binary, request); err != nil {
		s.failInitialization(operationID, target, err)
		return
	}
	if err := s.verifyRemote(ctx, id, tokenHashValue, endpoint.Key, asset.SHA256); err != nil {
		s.failInitialization(operationID, target, err)
		return
	}
	if err := s.store.Update(endpoint.Key, func(value *EndpointRecord) error {
		if value.ActiveTokenHash != "" {
			value.PreviousTokenHash = value.ActiveTokenHash
			value.PreviousTokenExpiresAt = time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339Nano)
		}
		value.ActiveTokenHash = tokenHashValue
		value.PendingTokenHash, value.PendingCreatedAt = "", ""
		value.ComponentVersion, value.ComponentSHA256 = manifest.ComponentVersion, asset.SHA256
		state := value.Targets[target.SessionName]
		state.SessionID, state.SessionCreated = sessionID, sessionCreated
		component := "needs_trust"
		if value.InstallationState == "ready" || state.Component == "ready" {
			component = "ready"
		}
		installedComponent = component
		value.WebhookOrigin, value.InstallationState = webhookOrigin, component
		state.Component, state.ComponentVersion, state.Activity = component, manifest.ComponentVersion, "unknown"
		value.Targets[target.SessionName] = state
		value.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return nil
	}); err != nil {
		s.failInitialization(operationID, target, errf("agent_store_unavailable", 503, "Agent state storage is unavailable.", err))
		return
	}
	s.completeInitialization(operationID, target, "installed", true, installedComponent, manifest.ComponentVersion)
}

func (s *Service) repairExisting(ctx context.Context, id string, target Target, endpoint Endpoint, webhookURL, fingerprint, version, checksum, webhookOrigin string, binary []byte) error {
	request := installRequest{
		SchemaVersion:                   1,
		Endpoint:                        Endpoint{Key: endpoint.Key, User: endpoint.User, Host: endpoint.Host, Port: endpoint.Port},
		WebhookURL:                      webhookURL,
		ExpectedCurrentTokenFingerprint: fingerprint,
		ComponentVersion:                version,
		ComponentSHA256:                 checksum,
		TmuxSessionName:                 target.SessionName,
	}
	if err := s.installRemote(ctx, id, binary, request); err != nil {
		return err
	}
	if err := s.verifyRemote(ctx, id, fingerprint, endpoint.Key, checksum); err != nil {
		return err
	}
	return s.store.Update(target.EndpointKey, func(record *EndpointRecord) error {
		record.ComponentVersion, record.ComponentSHA256 = version, checksum
		state := record.Targets[target.SessionName]
		state.SessionName = target.SessionName
		component := "needs_trust"
		if record.InstallationState == "ready" || state.Component == "ready" {
			component = "ready"
		}
		record.WebhookOrigin, record.InstallationState = webhookOrigin, component
		state.Component, state.ComponentVersion, state.Activity = component, version, "unknown"
		record.Targets[target.SessionName] = state
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return nil
	})
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
