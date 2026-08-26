package agent

import (
	"context"
	"errors"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

var agentTransportRetryDelays = []time.Duration{0, 250 * time.Millisecond, 750 * time.Millisecond, 2 * time.Second}

func (s *Service) remotePlatformWithRetry(ctx context.Context, id string) (string, string, error) {
	var remoteOS, remoteArch string
	var err error
	for attempt, delay := range agentTransportRetryDelays {
		if delay > 0 {
			if err := waitContext(ctx, delay); err != nil {
				return "", "", err
			}
		}
		remoteOS, remoteArch, err = s.remotePlatform(ctx, id)
		if err == nil || !retryableProbeError(err) || attempt == len(agentTransportRetryDelays)-1 {
			return remoteOS, remoteArch, err
		}
	}
	return "", "", err
}

func (s *Service) existingProbeWithRetry(ctx context.Context, id string) (remoteProbe, error) {
	var probe remoteProbe
	var err error
	for attempt, delay := range agentTransportRetryDelays {
		if delay > 0 {
			if waitErr := waitContext(ctx, delay); waitErr != nil {
				return remoteProbe{}, waitErr
			}
		}
		probe, err = s.existingProbe(ctx, id)
		if err == nil || !retryableProbeError(err) || attempt == len(agentTransportRetryDelays)-1 {
			return probe, err
		}
	}
	return probe, err
}

func retryableProbeError(err error) bool {
	var agentErr *Error
	return errors.As(err, &agentErr) && agentErr.Code == "agent_remote_probe_failed"
}

func (s *Service) acquireRemoteTransferWithRetry(ctx context.Context, operationID, instanceID string) (ports.RemoteTransferLease, error) {
	provider, ok := s.terms.(ports.RemoteTransferProvider)
	if !ok {
		return nil, errf("agent_transport_unavailable", 503, "The SSH transport is unavailable.", ports.ErrTransportUnavailable)
	}
	var lastErr error
	for attempt, delay := range agentTransportRetryDelays {
		if delay > 0 {
			if err := waitContext(ctx, delay); err != nil {
				return nil, err
			}
		}
		logAgentInfo("agent_initialization_transport_acquire_started", operationID, instanceID, "attempt=%d", attempt+1)
		lease, err := provider.AcquireRemoteTransfer(ctx, instanceID)
		if err == nil {
			logAgentInfo("agent_initialization_transport_acquired", operationID, instanceID, "attempt=%d", attempt+1)
			return lease, nil
		}
		lastErr = err
		retry := errors.Is(err, ports.ErrTransportUnavailable) && attempt < len(agentTransportRetryDelays)-1
		logAgentInfo("agent_initialization_transport_acquire_failed", operationID, instanceID, "attempt=%d retryable=%t error_type=%T", attempt+1, retry, err)
		if !retry {
			break
		}
	}
	return nil, errf("agent_transport_unavailable", 503, "The SSH transport became unavailable during Agent initialization.", lastErr)
}

func waitContext(ctx context.Context, delay time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
