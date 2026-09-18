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
	typ, _ := event["type"].(string)
	if typ == "command_result" || typ == "pong" {
		internalRequestID := rawStringValue(event["requestId"])
		pending, ok := runtime.pending[internalRequestID]
		if !ok {
			m.mu.Unlock()
			return
		}
		delete(runtime.pending, internalRequestID)
		if pending.timer != nil {
			pending.timer.Stop()
		}
		if _, live := m.viewers[pending.viewer]; live && pending.viewer.runtime == runtime {
			event["requestId"] = pending.requestID
			event["clientId"] = pending.viewer.clientID
			copyEvent := cloneMap(event)
			if m.primaryKnown {
				copyEvent["primary"] = m.primary == pending.viewer.clientID
			}
			m.enqueueLocked(pending.viewer, copyEvent)
			if success, exists := event["success"].(bool); exists && !success {
				m.enqueueLocked(pending.viewer, m.stateEventLocked(pending.viewer))
			}
		}
		m.mu.Unlock()
		return
	}
	if value, ok := event["pageOperation"].(float64); ok && int64(value) != runtime.pageOperation {
		m.mu.Unlock()
		return
	}
	if value, ok := event["revision"].(float64); ok && value < float64(runtime.pageRevision) {
		m.mu.Unlock()
		return
	}
	if value, ok := event["pageGeneration"].(string); ok && value != "" {
		// Worker events are authoritative, but an event for an old page must not
		// resurrect cached content after a newer open or close.
		if runtime.pageGeneration != "" && value != runtime.pageGeneration {
			m.mu.Unlock()
			return
		}
		runtime.pageGeneration = value
	}
	if value, ok := event["revision"].(float64); ok && value >= float64(runtime.pageRevision) {
		runtime.pageRevision = int64(value)
	}
	if value, ok := event["pageStatus"].(string); ok && value != "" {
		runtime.pageStatus = value
	}
	if runtime.pageStatus == "loading" || runtime.pageStatus == "ready" || runtime.pageStatus == "none" || runtime.pageStatus == "closed" {
		runtime.pageError = ""
	}
	if value, ok := event["error"].(string); ok && runtime.pageStatus == "error" {
		runtime.pageError = value
	}
	if value, ok := event["url"].(string); ok {
		runtime.pageURL = value
	}
	if value, ok := event["title"].(string); ok {
		runtime.pageTitle = value
	}
	if dialog, ok := event["dialog"].(map[string]any); ok {
		runtime.pageDialog = cloneMap(dialog)
	}
	if typ == "dialog" {
		runtime.pageDialog = map[string]any{
			"dialogId": event["dialogId"], "kind": event["kind"], "message": event["message"], "defaultPrompt": event["defaultPrompt"],
		}
	}
	if typ == "dialogClosed" || typ == "closed" || runtime.pageStatus == "none" || runtime.pageStatus == "closed" {
		runtime.pageDialog = nil
	}
	if typ == "closed" || runtime.pageStatus == "closed" || runtime.pageStatus == "none" {
		runtime.latestFrame = nil
		if typ == "closed" || runtime.pageStatus == "closed" {
			runtime.pageURL = ""
			runtime.pageTitle = ""
		}
	}
	if typ, _ := event["type"].(string); typ == "viewport" {
		if width, ok := integerField(event["width"]); ok {
			if height, ok := integerField(event["height"]); ok {
				runtime.viewport = normalizeViewport(viewportSize{Width: width, Height: height})
			}
		}
	}
	event["generation"] = runtime.generation
	if runtime.pageGeneration != "" {
		event["pageGeneration"] = runtime.pageGeneration
	}
	event["pageStatus"] = runtime.pageStatus
	event["pageOperation"] = runtime.pageOperation
	event["revision"] = runtime.pageRevision
	if typ == "state" || typ == "ready" || typ == "loaded" || typ == "closed" || typ == "dialog" {
		event["url"] = runtime.pageURL
		event["title"] = runtime.pageTitle
		event["error"] = runtime.pageError
		event["dialog"] = runtime.pageDialog
	}
	if typ == "frame" {
		runtime.latestFrame = append([]byte(nil), data...)
	}
	for current := range m.viewers {
		if current.runtime != runtime {
			continue
		}
		if typ == "frame" && !current.visible {
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
	pageStatus := current.runtime.pageStatus
	if pageStatus == "" {
		pageStatus = "none"
	}
	event := map[string]any{
		"type":           "state",
		"generation":     current.runtime.generation,
		"width":          current.runtime.viewport.Width,
		"height":         current.runtime.viewport.Height,
		"pageStatus":     pageStatus,
		"pageGeneration": current.runtime.pageGeneration,
		"pageOperation":  current.runtime.pageOperation,
		"revision":       current.runtime.pageRevision,
		"url":            current.runtime.pageURL,
		"title":          current.runtime.pageTitle,
		"error":          current.runtime.pageError,
		"dialog":         current.runtime.pageDialog,
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
		// Frames are replaceable; lifecycle and ownership messages are not.
		if eventType(data) == "frame" {
			return data
		}
		queued := make([][]byte, 0, cap(current.events))
		removedFrame := false
		for {
			select {
			case item := <-current.events:
				if !removedFrame && eventType(item) == "frame" {
					removedFrame = true
					continue
				}
				queued = append(queued, item)
			default:
				goto drained
			}
		}
	drained:
		for _, item := range queued {
			current.events <- item
		}
		if removedFrame {
			current.events <- data
		} else {
			// All queue slots contain control messages. Disconnect this slow
			// viewer so it can reconnect and obtain a fresh authoritative state.
			current.closeOne.Do(func() { close(current.done) })
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
