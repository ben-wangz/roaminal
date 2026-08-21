package agent

import (
	"context"
	"errors"
	"time"
)

func (s *Service) remotePlatformWithRetry(ctx context.Context, id string) (string, string, error) {
	var remoteOS, remoteArch string
	var err error
	for attempt, delay := range []time.Duration{0, 250 * time.Millisecond, 750 * time.Millisecond, 2 * time.Second} {
		if delay > 0 {
			if err := waitContext(ctx, delay); err != nil {
				return "", "", err
			}
		}
		remoteOS, remoteArch, err = s.remotePlatform(ctx, id)
		if err == nil || !retryableProbeError(err) || attempt == 3 {
			return remoteOS, remoteArch, err
		}
	}
	return "", "", err
}

func (s *Service) existingProbeWithRetry(ctx context.Context, id string) (remoteProbe, error) {
	var probe remoteProbe
	var err error
	for attempt, delay := range []time.Duration{0, 250 * time.Millisecond, 750 * time.Millisecond, 2 * time.Second} {
		if delay > 0 {
			if waitErr := waitContext(ctx, delay); waitErr != nil {
				return remoteProbe{}, waitErr
			}
		}
		probe, err = s.existingProbe(ctx, id)
		if err == nil || !retryableProbeError(err) || attempt == 3 {
			return probe, err
		}
	}
	return probe, err
}

func retryableProbeError(err error) bool {
	var agentErr *Error
	return errors.As(err, &agentErr) && agentErr.Code == "agent_remote_probe_failed"
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
