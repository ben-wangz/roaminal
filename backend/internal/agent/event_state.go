package agent

import "time"

func (s *Service) rememberEventLocked(endpointKey, eventID string, now time.Time) bool {
	items := s.eventIDs[endpointKey]
	if items == nil {
		items = map[string]time.Time{}
		s.eventIDs[endpointKey] = items
	}
	for id, seenAt := range items {
		if now.Sub(seenAt) > 24*time.Hour {
			delete(items, id)
		}
	}
	if _, exists := items[eventID]; exists {
		return true
	}
	if len(items) >= 2048 {
		oldestID := ""
		var oldest time.Time
		for id, seenAt := range items {
			if oldestID == "" || seenAt.Before(oldest) {
				oldestID, oldest = id, seenAt
			}
		}
		if oldestID != "" {
			delete(items, oldestID)
		}
	}
	items[eventID] = now
	return false
}

func (s *Service) markComponentReady(endpointKey, sessionName, sessionID string, sessionCreated int64, version string) error {
	return s.store.Update(endpointKey, func(record *EndpointRecord) error {
		record.InstallationState = "ready"
		if record.ComponentVersion == "" {
			record.ComponentVersion = version
		}
		state := record.Targets[sessionName]
		state.SessionName = sessionName
		state.SessionID, state.SessionCreated = sessionID, sessionCreated
		state.Component = "ready"
		if state.ComponentVersion == "" {
			state.ComponentVersion = version
		}
		state.Activity = "unknown"
		state.ErrorCode, state.ErrorMessage, state.InitializationID = "", "", ""
		record.Targets[sessionName] = state
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return nil
	})
}
