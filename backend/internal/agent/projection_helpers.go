package agent

import (
	"encoding/hex"
	"maps"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func messageFallbackLabel(record EndpointRecord, sessionName string) string {
	if record.User == "" || record.Host == "" || record.Port < 1 {
		return "tmux:" + sessionName
	}
	return endpointDisplay(record.User, record.Host, record.Port) + " / tmux:" + sessionName
}

// The scheduler captures views before opening the checkpoint transaction:
// gathering them can take connection locks or run SSH subprocesses.
func (s *Service) transitionDraft(view ports.ConnectionInstanceView, views []ports.ConnectionInstanceView, target Target, record EndpointRecord, from string, snapshot remoteAgentState, key string) *domain.MessageDraft {
	matching := s.matchingConnectionInstances(views, target, record)
	if len(matching) == 0 && view.ConnectionInstanceID != "" {
		matching = []string{view.ConnectionInstanceID}
	}
	severity := "info"
	if snapshot.State == "error" {
		severity = "error"
	} else if snapshot.State == "relax" {
		severity = "success"
	}
	last := snapshot.Records[len(snapshot.Records)-1]
	return &domain.MessageDraft{
		Kind: "agent_state_transition", Severity: severity, AgentType: snapshot.Provider,
		PresentationKey: "agent_state_transition", OccurredAt: last.Timestamp.UTC(), ReceivedAt: s.now().UTC(),
		EndpointKey: target.EndpointKey, FallbackLabel: messageFallbackLabel(record, target.SessionName),
		ConnectionLabel: notificationConnectionLabel(views, matching, record), TmuxSessionName: target.SessionName,
		TmuxSessionID: snapshot.Tmux.SessionID, TmuxSessionCreated: snapshot.Tmux.SessionCreated,
		ConnectionInstanceIDs: matching, AgentStateFrom: from, AgentStateTo: snapshot.State,
		ConnectionDefinitionIDs: matchingConnectionDefinitions(views, matching, view),
		AgentRuntimeID:          snapshot.RuntimeID, AgentStateIndex: snapshot.LatestIndex,
		IdempotencyKey: key,
	}
}

func (s *Service) matchingConnectionInstances(views []ports.ConnectionInstanceView, target Target, record EndpointRecord) []string {
	s.mu.Lock()
	bindings := maps.Clone(s.bindings)
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
		if boundTarget, bound := bindings[view.ID]; bound {
			if boundTarget == target {
				result = append(result, connectionInstanceID(view))
			}
			continue
		}
		alias := ""
		if view.SourceHostAlias != nil {
			alias = *view.SourceHostAlias
		}
		if _, aliasMatches := aliases[alias]; aliasMatches && view.TmuxSessionName == target.SessionName {
			result = append(result, connectionInstanceID(view))
		}
	}
	return uniqueStrings(result)
}

func notificationConnectionLabel(views []ports.ConnectionInstanceView, matchingIDs []string, record EndpointRecord) string {
	if len(matchingIDs) > 0 {
		matched := make(map[string]struct{}, len(matchingIDs))
		for _, id := range matchingIDs {
			matched[id] = struct{}{}
		}
		for _, view := range views {
			if _, ok := matched[view.ConnectionInstanceID]; !ok {
				if _, ok = matched[view.ID]; !ok {
					continue
				}
			}
			if view.SourceHostAlias != nil {
				alias := strings.TrimSpace(*view.SourceHostAlias)
				if domain.IsSafeConnectionLabel(alias) {
					return alias
				}
			}
		}
	}
	for _, alias := range record.Aliases {
		alias = strings.TrimSpace(alias)
		if domain.IsSafeConnectionLabel(alias) {
			return alias
		}
	}
	return "Remote"
}

func matchingConnectionDefinitions(views []ports.ConnectionInstanceView, instanceIDs []string, fallback ports.ConnectionInstanceView) []string {
	seen := make(map[string]struct{}, len(instanceIDs)+1)
	result := make([]string, 0, len(instanceIDs)+1)
	for _, candidate := range views {
		matched := false
		for _, id := range instanceIDs {
			if id == candidate.ID || id == candidate.ConnectionInstanceID {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		definitionID := candidate.ConnectionDefinitionID
		if definitionID == "" {
			continue
		}
		if _, exists := seen[definitionID]; exists {
			continue
		}
		seen[definitionID] = struct{}{}
		result = append(result, definitionID)
	}
	if fallback.ConnectionDefinitionID != "" {
		if _, exists := seen[fallback.ConnectionDefinitionID]; !exists {
			result = append(result, fallback.ConnectionDefinitionID)
		}
	}
	return result
}

func connectionInstanceID(view ports.ConnectionInstanceView) string {
	if view.ConnectionInstanceID != "" {
		return view.ConnectionInstanceID
	}
	return view.ID
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
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

func validSocketFingerprint(value string) bool {
	if len(value) != 16 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
