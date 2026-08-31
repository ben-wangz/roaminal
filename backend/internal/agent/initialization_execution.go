package agent

import (
	"context"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func (s *Service) executeInitialization(operationID, id string, _ ports.ConnectionInstanceView, endpoint Endpoint, target Target, priorComponent, sessionID string, sessionCreated int64) {
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

	componentCurrent := remoteState.Configured && remoteState.Provider == "codex" &&
		remoteState.ComponentVersion == manifest.ComponentVersion && remoteState.ComponentSHA256 == asset.SHA256 && remoteState.HooksConfigured
	if componentCurrent {
		component := priorComponent
		if component == "" || component == "initializing" || component == "error" {
			component = "needs_trust"
		}
		if err := s.markTargetComponent(target, component, manifest.ComponentVersion); err != nil {
			s.failInitialization(operationID, target, errf("agent_store_unavailable", 503, "Agent state storage is unavailable.", err))
			return
		}
		s.completeInitialization(operationID, target, "already_current", false, component, manifest.ComponentVersion)
		return
	}

	if err := s.store.Update(endpoint.Key, func(record *EndpointRecord) error {
		record.ComponentVersion = manifest.ComponentVersion
		record.ComponentSHA256 = asset.SHA256
		record.InstallationState = "initializing"
		record.UpdatedAt = s.now().UTC().Format(time.RFC3339Nano)
		return nil
	}); err != nil {
		s.failInitialization(operationID, target, errf("agent_store_unavailable", 503, "Agent state storage is unavailable.", err))
		return
	}

	request := localInstallRequest{SchemaVersion: 1, ComponentVersion: manifest.ComponentVersion, ComponentSHA256: asset.SHA256}
	if err := s.installRemote(ctx, operationID, id, binary, request); err != nil {
		s.failInitialization(operationID, target, err)
		return
	}
	if err := s.verifyRemoteWithLogging(ctx, operationID, id, manifest.ComponentVersion, asset.SHA256); err != nil {
		s.failInitialization(operationID, target, err)
		return
	}

	component := "needs_trust"
	if priorComponent == "ready" {
		component = "ready"
	}
	if err := s.markTargetComponent(target, component, manifest.ComponentVersion); err != nil {
		s.failInitialization(operationID, target, errf("agent_store_unavailable", 503, "Agent state storage is unavailable.", err))
		return
	}
	s.completeInitialization(operationID, target, "installed", true, component, manifest.ComponentVersion)
	_ = sessionID
	_ = sessionCreated
}
