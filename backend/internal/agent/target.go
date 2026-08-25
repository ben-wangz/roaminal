package agent

import (
	"context"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func (s *Service) targetFor(summary ports.ConnectionInstanceView) (Target, EndpointRecord, bool) {
	s.mu.Lock()
	target, bound := s.bindings[summary.ID]
	s.mu.Unlock()
	if bound {
		record, ok := s.store.Get(target.EndpointKey)
		return target, record, ok
	}
	alias := ""
	if summary.SourceHostAlias != nil {
		alias = *summary.SourceHostAlias
	}
	for key, record := range s.store.Snapshot() {
		for _, candidate := range record.Aliases {
			if candidate == alias {
				target := Target{EndpointKey: key, SessionName: summary.TmuxSessionName}
				return target, record, true
			}
		}
	}
	if endpointKey, ok := s.resolveCachedEndpoint(summary); ok {
		target := Target{EndpointKey: endpointKey, SessionName: summary.TmuxSessionName}
		if record, exists := s.store.Get(endpointKey); exists {
			return target, record, true
		}
	}
	return Target{}, EndpointRecord{}, false
}

func (s *Service) resolveCachedEndpoint(summary ports.ConnectionInstanceView) (string, bool) {
	alias := ""
	if summary.SourceHostAlias != nil {
		alias = *summary.SourceHostAlias
	}
	now := s.now()
	s.mu.Lock()
	if s.endpointCache == nil {
		s.endpointCache = map[string]endpointCacheEntry{}
	}
	entry, cached := s.endpointCache[summary.ID]
	if cached && entry.Alias == alias && entry.SourceState == summary.SourceState && now.Before(entry.ExpiresAt) {
		key := entry.Key
		s.mu.Unlock()
		return key, key != ""
	}
	s.mu.Unlock()
	if s.terms == nil {
		return "", false
	}
	effective, err := s.terms.ResolveEndpoint(context.Background(), summary.ID)
	key := ""
	if err == nil {
		if endpoint, normalizeErr := NormalizeEndpoint(effective); normalizeErr == nil {
			key = endpoint.Key
		}
	}
	s.mu.Lock()
	s.endpointCache[summary.ID] = endpointCacheEntry{Alias: alias, SourceState: summary.SourceState, Key: key, ExpiresAt: now.Add(30 * time.Second)}
	s.mu.Unlock()
	return key, key != ""
}
