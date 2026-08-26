package agent

import (
	"context"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func (s *Service) executeInitialization(operationID, id string, summary ports.ConnectionInstanceView, endpoint Endpoint, target Target, webhookURL, webhookOrigin string, webhookChanged bool, priorComponent string, sessionID string, sessionCreated int64) {
	lock := s.endpointMutex(endpoint.Key)
	lock.Lock()
	defer lock.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	logAgentInfo("agent_initialization_worker_started", operationID, id, "endpoint_key=%q tmux_session=%q", endpoint.Key, target.SessionName)
	lease, err := s.acquireRemoteTransferWithRetry(ctx, operationID, id)
	if err != nil {
		logAgentInfo("agent_initialization_transport_unavailable", operationID, id, "error_type=%T", err)
		s.failInitialization(operationID, target, err)
		return
	}
	defer func() {
		lease.Close()
		logAgentInfo("agent_initialization_transport_released", operationID, id, "")
	}()

	phaseStarted := time.Now()
	logAgentInfo("agent_initialization_phase_started", operationID, id, "phase=%q", "remote_platform")
	remoteOS, remoteArch, err := s.remotePlatformWithRetry(ctx, id)
	if err != nil {
		logAgentInfo("agent_initialization_phase_failed", operationID, id, "phase=%q duration_ms=%d error_type=%T", "remote_platform", durationMillis(phaseStarted), err)
		s.failInitialization(operationID, target, err)
		return
	}
	logAgentInfo("agent_initialization_phase_completed", operationID, id, "phase=%q duration_ms=%d remote_os=%q remote_arch=%q", "remote_platform", durationMillis(phaseStarted), remoteOS, remoteArch)
	asset, binary, manifest, err := s.loadAsset(remoteOS, remoteArch)
	if err != nil {
		s.failInitialization(operationID, target, err)
		return
	}
	phaseStarted = time.Now()
	logAgentInfo("agent_initialization_phase_started", operationID, id, "phase=%q", "existing_probe")
	remoteState, err := s.existingProbeWithRetry(ctx, id)
	if err != nil {
		logAgentInfo("agent_initialization_phase_failed", operationID, id, "phase=%q duration_ms=%d error_type=%T", "existing_probe", durationMillis(phaseStarted), err)
		s.failInitialization(operationID, target, err)
		return
	}
	logAgentInfo("agent_initialization_phase_completed", operationID, id, "phase=%q duration_ms=%d configured=%t", "existing_probe", durationMillis(phaseStarted), remoteState.Configured)
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
			if err := s.repairExisting(ctx, operationID, id, target, endpoint, webhookURL, remoteFingerprint, manifest.ComponentVersion, asset.SHA256, webhookOrigin, binary); err != nil {
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
			if err := s.repairExisting(ctx, operationID, id, target, endpoint, webhookURL, remoteFingerprint, manifest.ComponentVersion, asset.SHA256, webhookOrigin, binary); err != nil {
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
	token, tokenHashValue, err := randomToken(s.random)
	if err != nil {
		s.failInitialization(operationID, target, errf("agent_install_failed", 502, "The Agent token could not be created.", err))
		return
	}
	installedComponent := "needs_trust"
	if err := s.store.Update(endpoint.Key, func(value *EndpointRecord) error {
		value.PendingTokenHash = tokenHashValue
		value.PendingCreatedAt = s.now().UTC().Format(time.RFC3339Nano)
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
		Endpoint:                        installEndpointFor(endpoint),
		WebhookURL:                      webhookURL,
		ExpectedCurrentTokenFingerprint: remoteFingerprint,
		ReplacementToken:                token,
		ComponentVersion:                manifest.ComponentVersion,
		ComponentSHA256:                 asset.SHA256,
		TmuxSessionName:                 target.SessionName,
	}
	if err := s.installRemote(ctx, operationID, id, binary, request); err != nil {
		s.failInitialization(operationID, target, err)
		return
	}
	if err := s.verifyRemoteWithLogging(ctx, operationID, id, tokenHashValue, endpoint.Key, asset.SHA256); err != nil {
		s.failInitialization(operationID, target, err)
		return
	}
	if err := s.store.Update(endpoint.Key, func(value *EndpointRecord) error {
		if value.ActiveTokenHash != "" {
			value.PreviousTokenHash = value.ActiveTokenHash
			value.PreviousTokenExpiresAt = s.now().UTC().Add(10 * time.Minute).Format(time.RFC3339Nano)
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
		value.UpdatedAt = s.now().UTC().Format(time.RFC3339Nano)
		return nil
	}); err != nil {
		s.failInitialization(operationID, target, errf("agent_store_unavailable", 503, "Agent state storage is unavailable.", err))
		return
	}
	s.completeInitialization(operationID, target, "installed", true, installedComponent, manifest.ComponentVersion)
}

func (s *Service) repairExisting(ctx context.Context, operationID, id string, target Target, endpoint Endpoint, webhookURL, fingerprint, version, checksum, webhookOrigin string, binary []byte) error {
	request := installRequest{
		SchemaVersion:                   1,
		Endpoint:                        installEndpointFor(endpoint),
		WebhookURL:                      webhookURL,
		ExpectedCurrentTokenFingerprint: fingerprint,
		ComponentVersion:                version,
		ComponentSHA256:                 checksum,
		TmuxSessionName:                 target.SessionName,
	}
	if err := s.installRemote(ctx, operationID, id, binary, request); err != nil {
		return err
	}
	if err := s.verifyRemoteWithLogging(ctx, operationID, id, fingerprint, endpoint.Key, checksum); err != nil {
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
		record.UpdatedAt = s.now().UTC().Format(time.RFC3339Nano)
		return nil
	})
}

func (s *Service) verifyRemoteWithLogging(ctx context.Context, operationID, instanceID, expectedFingerprint, endpointKey, checksum string) error {
	started := time.Now()
	logAgentInfo("agent_component_verification_started", operationID, instanceID, "component_sha256=%q", checksum)
	err := s.verifyRemote(ctx, instanceID, expectedFingerprint, endpointKey, checksum)
	if err != nil {
		logAgentInfo("agent_component_verification_failed", operationID, instanceID, "duration_ms=%d error_type=%T", durationMillis(started), err)
		return err
	}
	logAgentInfo("agent_component_verification_completed", operationID, instanceID, "duration_ms=%d", durationMillis(started))
	return nil
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
