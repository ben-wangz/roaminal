package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func (s *Service) Details(ctx context.Context, summary ports.ConnectionInstanceView) DetailsResponse {
	detail := DetailsResponse{Agent: s.Summary(summary)}
	if target, record, ok := s.targetFor(summary); ok {
		detail.Endpoint = &Endpoint{Key: target.EndpointKey, User: record.User, Host: record.Host, Port: record.Port, Display: endpointDisplay(record.User, record.Host, record.Port)}
		detail.ComponentSHA256 = record.ComponentSHA256
	} else if s.terms != nil && summary.Type == "ssh" && summary.Lifecycle == "live" {
		if effective, err := s.terms.ResolveEndpoint(ctx, summary.ID); err == nil {
			if endpoint, err := NormalizeEndpoint(effective); err == nil {
				detail.Endpoint = &endpoint
			}
		}
	}
	return detail
}

func (s *Service) targetPreflight(ctx context.Context, id, session string) (string, int64, error) {
	if s.terms == nil {
		return "", 0, errors.New("connection manager unavailable")
	}
	result, err := s.terms.RunRemote(ctx, id, ports.RemoteCommand{Script: tmuxTargetPreflightScript, Args: []string{session}, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		return "", 0, err
	}
	parts := strings.Split(strings.TrimSpace(string(result.Output)), "|")
	if len(parts) != 3 || parts[0] != session {
		return "", 0, errors.New("tmux session not found")
	}
	var created int64
	if _, err := fmt.Sscan(parts[2], &created); err != nil || created < 0 {
		return "", 0, errors.New("invalid tmux session identity")
	}
	return parts[1], created, nil
}

const tmuxTargetPreflightScript = `set -eu
tmux display-message -p -t "=$1:" '#{session_name}|#{session_id}|#{session_created}'`
