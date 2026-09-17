package browser

import (
	"encoding/json"
	"errors"
	"fmt"
)

func (m *Manager) command(current *viewer, data []byte) error {
	if len(data) > commandLimit {
		return errors.New("browser command too large")
	}
	var command map[string]json.RawMessage
	if err := json.Unmarshal(data, &command); err != nil {
		return errors.New("invalid browser command")
	}
	var typ string
	if err := json.Unmarshal(command["type"], &typ); err != nil || typ == "" {
		return errors.New("browser command type is required")
	}
	if clientID := rawString(command["clientId"]); clientID != "" && clientID != current.clientID {
		return errors.New("browser client identity mismatch")
	}
	if (typ == "open" || typ == "navigate") && commandURL(command) == "" {
		return errors.New("browser address must be HTTP or HTTPS")
	}
	switch typ {
	case "open", "navigate", "sync", "back", "forward", "reload", "input", "dialog", "close", "ping":
		return m.forward(current.runtime, data)
	case "resize":
		return m.resize(current, command)
	case "visibility":
		return m.visibility(current, command)
	default:
		return errors.New("unknown browser command")
	}
}

func (m *Manager) forward(runtime *process, data []byte) error {
	m.mu.Lock()
	if m.process != runtime {
		m.mu.Unlock()
		return errors.New("browser worker unavailable")
	}
	m.mu.Unlock()
	return writeRuntime(runtime, data)
}

func (m *Manager) visibility(current *viewer, command map[string]json.RawMessage) error {
	var visible bool
	if err := json.Unmarshal(command["visible"], &visible); err != nil {
		return errors.New("browser visibility is required")
	}
	m.mu.Lock()
	if _, ok := m.viewers[current]; !ok || m.process != current.runtime {
		m.mu.Unlock()
		return errors.New("browser worker unavailable")
	}
	wasVisible := m.anyVisibleLocked(current.runtime)
	current.visible = visible
	isVisible := m.anyVisibleLocked(current.runtime)
	runtime := current.runtime
	m.mu.Unlock()
	if wasVisible == isVisible {
		return nil
	}
	return writeRuntime(runtime, map[string]any{"type": "visibility", "visible": isVisible})
}

func (m *Manager) resize(current *viewer, command map[string]json.RawMessage) error {
	if rawString(command["clientId"]) == "" {
		return errors.New("browser client identity is required")
	}
	width, widthOK := integerRaw(command["width"])
	height, heightOK := integerRaw(command["height"])
	if !widthOK || !heightOK {
		return errors.New("browser viewport dimensions are required")
	}
	size := normalizeViewport(viewportSize{Width: width, Height: height})
	if size.Width != width || size.Height != height {
		return errors.New("browser viewport dimensions are out of range")
	}
	generation := rawString(command["generation"])
	primaryIntent := false
	if value, ok := optionalBool(command["primaryIntent"]); ok {
		primaryIntent = value
	}
	takeover, _ := optionalBool(command["takeover"])
	if !primaryIntent {
		m.rejectResize(current, "primary_client_required", "Only the primary client can resize the remote page.", generation, size)
		return nil
	}
	m.mu.Lock()
	if _, ok := m.viewers[current]; !ok || m.process != current.runtime {
		m.mu.Unlock()
		return errors.New("browser worker unavailable")
	}
	runtime := current.runtime
	if generation == "" || generation != runtime.generation {
		m.mu.Unlock()
		m.rejectResize(current, "stale_browser_generation", "The remote browser generation has changed.", generation, size)
		return nil
	}
	if m.primaryKnown && m.primary != current.clientID && !takeover {
		m.mu.Unlock()
		m.rejectResize(current, "not_primary_client", "Another browser client controls the viewport.", generation, size)
		return nil
	}
	previousPrimary := m.primary
	previousPrimaryKnown := m.primaryKnown
	m.primary = current.clientID
	m.primaryKnown = true
	previousViewport := runtime.viewport
	if takeover || previousPrimary != m.primary || previousViewport != size {
		if err := writeRuntimeLocked(runtime, map[string]any{"type": "resize", "width": size.Width, "height": size.Height}); err != nil {
			m.primary = previousPrimary
			m.primaryKnown = previousPrimaryKnown
			runtime.viewport = previousViewport
			m.mu.Unlock()
			return err
		}
	}
	runtime.viewport = size
	if previousPrimary != m.primary {
		m.broadcastPrimaryLocked()
	}
	m.enqueueLocked(current, map[string]any{
		"type":       "resize_accepted",
		"generation": runtime.generation,
		"width":      size.Width,
		"height":     size.Height,
		"primary":    true,
		"takeover":   takeover,
	})
	m.mu.Unlock()
	return nil
}

func (m *Manager) rejectResize(current *viewer, code, message, generation string, size viewportSize) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.viewers[current]; !ok {
		return
	}
	event := map[string]any{"type": "resize_rejected", "code": code, "error": message, "primary": false}
	if current.runtime == m.process {
		event["generation"] = current.runtime.generation
		event["width"] = current.runtime.viewport.Width
		event["height"] = current.runtime.viewport.Height
	} else {
		if generation != "" {
			event["generation"] = generation
		}
		if size.Width > 0 && size.Height > 0 {
			event["width"] = size.Width
			event["height"] = size.Height
		}
	}
	m.enqueueLocked(current, event)
}

func (m *Manager) sendCommandError(current *viewer, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.viewers[current]; !ok {
		return
	}
	m.enqueueLocked(current, map[string]any{"type": "error", "error": err.Error(), "code": "invalid_browser_command"})
}

func isResizeCommand(data []byte) bool {
	var command struct {
		Type string `json:"type"`
	}
	return json.Unmarshal(data, &command) == nil && command.Type == "resize"
}

func writeRuntime(runtime *process, message any) error {
	if data, ok := message.([]byte); ok {
		return writeRuntimeLocked(runtime, data)
	}
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return writeRuntimeLocked(runtime, data)
}

func writeRuntimeLocked(runtime *process, message any) error {
	data, ok := message.([]byte)
	if !ok {
		encoded, err := json.Marshal(message)
		if err != nil {
			return err
		}
		data = encoded
	}
	runtime.writeMu.Lock()
	defer runtime.writeMu.Unlock()
	_, err := fmt.Fprintf(runtime.stdin, "%s\n", data)
	return err
}
