package agent

import "github.com/ben-wangz/roaminal/backend/internal/ports"

func (s *Service) targetFor(summary ports.ConnectionInstanceView) (Target, EndpointRecord, bool) {
	s.mu.Lock()
	target, bound := s.bindings[summary.ID]
	s.mu.Unlock()
	if bound {
		if target.SessionName != summary.TmuxSessionName {
			return Target{}, EndpointRecord{}, false
		}
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
	return Target{}, EndpointRecord{}, false
}
