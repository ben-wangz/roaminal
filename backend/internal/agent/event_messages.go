package agent

import (
	"strconv"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

func cloneCompletedTools(value map[string]time.Time) map[string]time.Time {
	result := make(map[string]time.Time, len(value))
	for id, receivedAt := range value {
		result[id] = receivedAt
	}
	return result
}

func (s *Service) appendEventMessages(event webhookEvent, endpointKey, targetName, fallbackLabel string, matchingIDs []string, receivedAt time.Time) error {
	draft := domain.MessageDraft{
		Kind: "agent_reporting_ready", Severity: "info", AgentType: event.AgentType, PresentationKey: "codex_reporting_connected",
		OccurredAt: event.OccurredAt.UTC(), ReceivedAt: receivedAt, EndpointKey: endpointKey, FallbackLabel: fallbackLabel,
		TmuxSessionName: targetName, TmuxSessionID: event.Tmux.SessionID, TmuxSessionCreated: event.Tmux.SessionCreated,
		ConnectionInstanceIDs: matchingIDs,
		IdempotencyKey:        strings.Join([]string{"agent_reporting_ready", endpointKey, event.Tmux.SessionID, strconv.FormatInt(event.Tmux.SessionCreated, 10), event.ComponentVersion}, "\x00"),
	}
	if _, _, err := s.messages.AppendMessage(draft); err != nil {
		return errf("message_store_unavailable", 503, "Agent message storage is unavailable.", err)
	}
	if event.EventName != "Stop" {
		return nil
	}
	draft.Kind = "codex_turn_completed"
	draft.Severity = "success"
	draft.PresentationKey = "codex_turn_finished"
	draft.IdempotencyKey = event.EventID + "\x00codex_turn_completed"
	if _, _, err := s.messages.AppendMessage(draft); err != nil {
		return errf("message_store_unavailable", 503, "Agent message storage is unavailable.", err)
	}
	return nil
}

func messageFallbackLabel(record EndpointRecord, targetName string) string {
	if record.User == "" || record.Host == "" || record.Port < 1 {
		return "tmux:" + targetName
	}
	return endpointDisplay(record.User, record.Host, record.Port) + " / tmux:" + targetName
}

func (s *Service) matchingConnectionInstances(endpointKey, targetName, eventSessionName string, record EndpointRecord) []string {
	if s.terms == nil {
		return nil
	}
	views := s.terms.ConnectionInstanceViews()
	s.mu.Lock()
	bindings := make(map[string]Target, len(s.bindings))
	for id, target := range s.bindings {
		bindings[id] = target
	}
	s.mu.Unlock()
	aliases := make(map[string]struct{}, len(record.Aliases))
	for _, alias := range record.Aliases {
		aliases[alias] = struct{}{}
	}
	result := make([]string, 0, len(views))
	for _, view := range views {
		if view.Lifecycle != "live" || view.ID == "" {
			continue
		}
		if target, bound := bindings[view.ID]; bound {
			if target.EndpointKey == endpointKey && target.SessionName == targetName {
				result = append(result, view.ConnectionInstanceID)
			}
			continue
		}
		alias := ""
		if view.SourceHostAlias != nil {
			alias = *view.SourceHostAlias
		}
		_, aliasMatches := aliases[alias]
		nameMatches := view.TmuxSessionName == targetName || (eventSessionName != "" && view.TmuxSessionName == eventSessionName)
		if aliasMatches && nameMatches {
			result = append(result, view.ConnectionInstanceID)
		}
	}
	return uniqueStrings(result)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := values[:0]
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
