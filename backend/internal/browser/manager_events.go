package browser

import "encoding/json"

func (m *Manager) broadcastEvent(runtime *process, data []byte) {
	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		return
	}
	m.mu.Lock()
	if m.process != runtime {
		m.mu.Unlock()
		return
	}
	if typ, _ := event["type"].(string); typ == "viewport" {
		if width, ok := integerField(event["width"]); ok {
			if height, ok := integerField(event["height"]); ok {
				runtime.viewport = normalizeViewport(viewportSize{Width: width, Height: height})
			}
		}
	}
	event["generation"] = runtime.generation
	for current := range m.viewers {
		if current.runtime != runtime {
			continue
		}
		copyEvent := cloneMap(event)
		if m.primaryKnown {
			copyEvent["primary"] = m.primary == current.clientID
		}
		m.enqueueLocked(current, copyEvent)
	}
	m.mu.Unlock()
}

func (m *Manager) stateEventLocked(current *viewer) map[string]any {
	event := map[string]any{
		"type":       "state",
		"generation": current.runtime.generation,
		"width":      current.runtime.viewport.Width,
		"height":     current.runtime.viewport.Height,
	}
	if m.primaryKnown {
		event["primary"] = m.primary == current.clientID
	}
	return event
}

func (m *Manager) broadcastPrimaryLocked() {
	if m.process == nil {
		return
	}
	for current := range m.viewers {
		if current.runtime != m.process {
			continue
		}
		m.enqueueLocked(current, map[string]any{
			"type":       "primary",
			"generation": m.process.generation,
			"primary":    m.primary != "" && m.primary == current.clientID,
		})
	}
}

func (m *Manager) enqueue(current *viewer, message any) {
	data, err := json.Marshal(message)
	if err != nil {
		return
	}
	m.mu.Lock()
	if _, ok := m.viewers[current]; !ok {
		m.mu.Unlock()
		return
	}
	m.enqueueLocked(current, data)
	m.mu.Unlock()
}

func (m *Manager) enqueueLocked(current *viewer, message any) []byte {
	var data []byte
	if encoded, ok := message.([]byte); ok {
		data = encoded
	} else {
		data, _ = json.Marshal(message)
	}
	if len(data) == 0 || len(data) > eventLimit {
		return data
	}
	select {
	case current.events <- data:
	default:
		// A slow viewer must not let the worker's stdout grow without bound.
		// Preserve lifecycle and ownership messages by evicting an older frame
		// when the bounded queue is full.
		if eventType(data) != "frame" {
			select {
			case <-current.events:
			default:
			}
			select {
			case current.events <- data:
			default:
			}
		}
	}
	return data
}

func eventType(data []byte) string {
	var event struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(data, &event) != nil {
		return ""
	}
	return event.Type
}
