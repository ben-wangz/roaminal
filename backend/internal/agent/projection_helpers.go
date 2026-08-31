package agent

import (
	"encoding/hex"
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

func (s *Service) matchingConnectionInstances(endpointKey, sessionName string, record EndpointRecord) []string {
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
			if target.EndpointKey == endpointKey && target.SessionName == sessionName {
				result = append(result, connectionInstanceID(view))
			}
			continue
		}
		alias := ""
		if view.SourceHostAlias != nil {
			alias = *view.SourceHostAlias
		}
		if _, aliasMatches := aliases[alias]; aliasMatches && view.TmuxSessionName == sessionName {
			result = append(result, connectionInstanceID(view))
		}
	}
	return uniqueStrings(result)
}

func (s *Service) notificationConnectionLabel(matchingIDs []string, record EndpointRecord) string {
	if s.terms != nil && len(matchingIDs) > 0 {
		matched := make(map[string]struct{}, len(matchingIDs))
		for _, id := range matchingIDs {
			matched[id] = struct{}{}
		}
		for _, view := range s.terms.ConnectionInstanceViews() {
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
