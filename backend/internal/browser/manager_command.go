package browser

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/identity"
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
	runtime := current.runtime
	runtime.commandMu.Lock()
	defer runtime.commandMu.Unlock()
	switch typ {
	case "open", "navigate":
		return m.navigate(current, command)
	case "close":
		return m.closePage(current, command)
	case "sync":
		return m.syncUnlocked(current)
	case "back", "forward", "reload", "input", "dialog", "ping":
		if typ == "ping" {
			return m.forwardCommand(current, command)
		}
		return m.forwardPageCommand(current, command)
	case "resize":
		return m.resize(current, command)
	case "visibility":
		return m.visibility(current, command)
	default:
		return errors.New("unknown browser command")
	}
}

func (m *Manager) sync(current *viewer) error {
	runtime := current.runtime
	runtime.commandMu.Lock()
	defer runtime.commandMu.Unlock()
	return m.syncUnlocked(current)
}

func (m *Manager) syncUnlocked(current *viewer) error {
	m.mu.Lock()
	if _, ok := m.viewers[current]; !ok || m.process != current.runtime {
		m.mu.Unlock()
		return errors.New("browser worker unavailable")
	}
	runtime := current.runtime
	m.enqueueLocked(current, m.stateEventLocked(current))
	hasPage := runtime.pageStatus != "none" && runtime.pageStatus != "closed" && runtime.pageGeneration != ""
	pageGeneration := runtime.pageGeneration
	pageOperation := runtime.pageOperation
	frame := append([]byte(nil), runtime.latestFrame...)
	if hasPage && len(frame) > 0 {
		m.enqueueLocked(current, frame)
	}
	m.mu.Unlock()
	if !hasPage {
		return nil
	}
	return writeRuntime(runtime, map[string]any{"type": "sync", "pageGeneration": pageGeneration, "pageOperation": pageOperation})
}

func (m *Manager) navigate(current *viewer, command map[string]json.RawMessage) error {
	url := commandURL(command)
	m.mu.Lock()
	if _, ok := m.viewers[current]; !ok || m.process != current.runtime {
		m.mu.Unlock()
		return errors.New("browser worker unavailable")
	}
	runtime := current.runtime
	if runtime.pageStatus == "closing" {
		m.enqueueLocked(current, map[string]any{"type": "command_result", "clientId": current.clientID, "requestId": rawString(command["requestId"]), "success": false, "code": "page_closing", "error": "The remote browser page is closing."})
		m.mu.Unlock()
		return nil
	}
	if !pageOperationMatches(command, runtime.pageOperation) {
		m.enqueueLocked(current, map[string]any{"type": "command_result", "clientId": current.clientID, "requestId": rawString(command["requestId"]), "success": false, "code": "stale_browser_page", "error": "The remote browser page has changed.", "pageGeneration": runtime.pageGeneration, "pageOperation": runtime.pageOperation})
		m.enqueueLocked(current, m.stateEventLocked(current))
		m.mu.Unlock()
		return nil
	}
	pageGeneration := runtime.pageGeneration
	if runtime.pageGeneration != "" {
		requested := rawString(command["pageGeneration"])
		pageExists := runtime.pageStatus != "none" && runtime.pageStatus != "closed"
		if (pageExists && (requested == "" || requested != runtime.pageGeneration)) || (!pageExists && requested != "" && requested != runtime.pageGeneration) {
			m.enqueueLocked(current, map[string]any{"type": "command_result", "clientId": current.clientID, "requestId": rawString(command["requestId"]), "success": false, "code": "stale_browser_page", "error": "The remote browser page has changed.", "generation": runtime.generation, "pageGeneration": runtime.pageGeneration})
			m.enqueueLocked(current, m.stateEventLocked(current))
			m.mu.Unlock()
			return nil
		}
	}
	if pageGeneration == "" || runtime.pageStatus == "none" || runtime.pageStatus == "closed" {
		pageGeneration = newPageGeneration()
	}
	runtime.pageOperation++
	runtime.pageGeneration = pageGeneration
	runtime.pageStatus = "loading"
	runtime.pageURL = url
	runtime.pageTitle = ""
	runtime.pageError = ""
	runtime.pageDialog = nil
	runtime.latestFrame = nil
	runtime.pageRevision++
	command["pageGeneration"] = json.RawMessage(strconv.Quote(pageGeneration))
	command["pageOperation"] = json.RawMessage(strconv.FormatInt(runtime.pageOperation, 10))
	command["url"] = json.RawMessage(strconv.Quote(url))
	m.prepareWorkerCommandLocked(current, command)
	data, err := json.Marshal(command)
	if err != nil {
		m.mu.Unlock()
		return errors.New("invalid browser command")
	}
	m.broadcastStateLocked(runtime)
	m.mu.Unlock()
	if err := writeRuntime(runtime, data); err != nil {
		return err
	}
	return nil
}

func (m *Manager) closePage(current *viewer, command map[string]json.RawMessage) error {
	requested := rawString(command["pageGeneration"])
	m.mu.Lock()
	if _, ok := m.viewers[current]; !ok || m.process != current.runtime {
		m.mu.Unlock()
		return errors.New("browser worker unavailable")
	}
	runtime := current.runtime
	if !pageOperationMatches(command, runtime.pageOperation) {
		m.enqueueLocked(current, map[string]any{"type": "command_result", "clientId": current.clientID, "requestId": rawString(command["requestId"]), "success": false, "code": "stale_browser_page", "error": "The remote browser page has changed.", "pageGeneration": runtime.pageGeneration, "pageOperation": runtime.pageOperation})
		m.enqueueLocked(current, m.stateEventLocked(current))
		m.mu.Unlock()
		return nil
	}
	if runtime.pageStatus == "closing" || runtime.pageGeneration == "" || runtime.pageStatus == "none" || runtime.pageStatus == "closed" || requested == "" || requested != runtime.pageGeneration {
		result := map[string]any{"type": "command_result", "clientId": current.clientID, "requestId": rawString(command["requestId"]), "pageGeneration": runtime.pageGeneration, "pageOperation": runtime.pageOperation}
		if runtime.pageStatus == "closing" {
			result["success"] = true
			result["code"] = "page_closing"
		} else if runtime.pageStatus == "none" || runtime.pageStatus == "closed" || runtime.pageGeneration == "" {
			result["success"] = true
			result["code"] = "no_browser_page"
		} else {
			result["success"] = false
			result["code"] = "stale_browser_page"
			result["error"] = "The remote browser page has changed."
		}
		m.enqueueLocked(current, result)
		m.enqueueLocked(current, m.stateEventLocked(current))
		m.mu.Unlock()
		return nil
	}
	runtime.pageStatus = "closing"
	runtime.pageDialog = nil
	runtime.pageError = ""
	runtime.latestFrame = nil
	runtime.pageOperation++
	runtime.pageRevision++
	command["pageGeneration"] = json.RawMessage(strconv.Quote(runtime.pageGeneration))
	command["pageOperation"] = json.RawMessage(strconv.FormatInt(runtime.pageOperation, 10))
	m.prepareWorkerCommandLocked(current, command)
	data, err := json.Marshal(command)
	if err != nil {
		m.mu.Unlock()
		return errors.New("invalid browser command")
	}
	m.broadcastStateLocked(runtime)
	m.mu.Unlock()
	return writeRuntime(runtime, data)
}

func (m *Manager) broadcastStateLocked(runtime *process) {
	for current := range m.viewers {
		if current.runtime == runtime {
			m.enqueueLocked(current, m.stateEventLocked(current))
		}
	}
}

func newPageGeneration() string {
	value, err := (identity.UUIDGenerator{}).NewID()
	if err != nil || value == "" {
		return fmt.Sprintf("page-%d", time.Now().UnixNano())
	}
	return value
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

func (m *Manager) forwardCommand(current *viewer, command map[string]json.RawMessage) error {
	m.mu.Lock()
	if _, ok := m.viewers[current]; !ok || m.process != current.runtime {
		m.mu.Unlock()
		return errors.New("browser worker unavailable")
	}
	runtime := current.runtime
	m.prepareWorkerCommandLocked(current, command)
	data, err := json.Marshal(command)
	m.mu.Unlock()
	if err != nil {
		return errors.New("invalid browser command")
	}
	return writeRuntime(runtime, data)
}

func (m *Manager) forwardPageCommand(current *viewer, command map[string]json.RawMessage) error {
	m.mu.Lock()
	if _, ok := m.viewers[current]; !ok || m.process != current.runtime {
		m.mu.Unlock()
		return errors.New("browser worker unavailable")
	}
	runtime := current.runtime
	pageGeneration := runtime.pageGeneration
	pageOperation := runtime.pageOperation
	pageStatus := runtime.pageStatus
	requested := rawString(command["pageGeneration"])
	if !pageOperationMatches(command, pageOperation) {
		m.enqueueLocked(current, map[string]any{"type": "command_result", "clientId": current.clientID, "requestId": rawString(command["requestId"]), "success": false, "code": "stale_browser_page", "error": "The remote browser page has changed.", "pageGeneration": pageGeneration, "pageOperation": pageOperation})
		m.enqueueLocked(current, m.stateEventLocked(current))
		m.mu.Unlock()
		return nil
	}
	if pageStatus == "closing" {
		m.enqueueLocked(current, map[string]any{"type": "command_result", "clientId": current.clientID, "requestId": rawString(command["requestId"]), "success": false, "code": "page_closing", "error": "The remote browser page is closing.", "pageGeneration": pageGeneration, "pageOperation": pageOperation})
		m.enqueueLocked(current, m.stateEventLocked(current))
		m.mu.Unlock()
		return nil
	}
	if pageStatus == "none" || pageStatus == "closed" || pageGeneration == "" {
		m.enqueueLocked(current, map[string]any{"type": "command_result", "clientId": current.clientID, "requestId": rawString(command["requestId"]), "success": false, "code": "no_browser_page", "pageGeneration": pageGeneration, "pageOperation": pageOperation})
		m.enqueueLocked(current, m.stateEventLocked(current))
		m.mu.Unlock()
		return nil
	}
	if requested == "" || requested != pageGeneration {
		m.enqueueLocked(current, map[string]any{"type": "command_result", "clientId": current.clientID, "requestId": rawString(command["requestId"]), "success": false, "code": "stale_browser_page", "error": "The remote browser page has changed.", "pageGeneration": pageGeneration, "pageOperation": pageOperation})
		m.enqueueLocked(current, m.stateEventLocked(current))
		m.mu.Unlock()
		return nil
	}
	command["pageGeneration"] = json.RawMessage(strconv.Quote(pageGeneration))
	command["pageOperation"] = json.RawMessage(strconv.FormatInt(pageOperation, 10))
	m.prepareWorkerCommandLocked(current, command)
	data, err := json.Marshal(command)
	m.mu.Unlock()
	if err != nil {
		return errors.New("invalid browser command")
	}
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
		if visible {
			return m.syncUnlocked(current)
		}
		return nil
	}
	if err := writeRuntime(runtime, map[string]any{"type": "visibility", "visible": isVisible}); err != nil {
		return err
	}
	if visible {
		return m.syncUnlocked(current)
	}
	return nil
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

// Worker command results are routed through an internal token. The browser
// client identity in a request is only an ownership hint and must not decide
// which WebSocket receives an asynchronous response.
func (m *Manager) prepareWorkerCommandLocked(current *viewer, command map[string]json.RawMessage) {
	routeID := current.routeID
	if routeID == "" {
		routeID = current.clientID
	}
	command["clientId"] = json.RawMessage(strconv.Quote(routeID))
	command["generation"] = json.RawMessage(strconv.Quote(current.runtime.generation))
	originalRequestID := rawString(command["requestId"])
	if originalRequestID == "" {
		return
	}
	runtime := current.runtime
	if runtime.pending == nil {
		runtime.pending = make(map[string]*pendingBrowserRequest)
	}
	internalRequestID := newBrowserRequestID("command")
	pending := &pendingBrowserRequest{viewer: current, requestID: originalRequestID}
	runtime.pending[internalRequestID] = pending
	pending.timer = time.AfterFunc(browserCommandTimeout, func() {
		m.expireBrowserRequest(runtime, internalRequestID)
	})
	command["requestId"] = json.RawMessage(strconv.Quote(internalRequestID))
}

func (m *Manager) expireBrowserRequest(runtime *process, internalRequestID string) {
	m.mu.Lock()
	pending, ok := runtime.pending[internalRequestID]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(runtime.pending, internalRequestID)
	if _, live := m.viewers[pending.viewer]; live && pending.viewer.runtime == runtime && m.process == runtime {
		m.enqueueLocked(pending.viewer, map[string]any{
			"type": "command_result", "clientId": pending.viewer.clientID, "requestId": pending.requestID,
			"success": false, "code": "browser_command_timeout", "error": "The remote browser command timed out.",
			"pageGeneration": runtime.pageGeneration, "pageOperation": runtime.pageOperation,
		})
		m.enqueueLocked(pending.viewer, m.stateEventLocked(pending.viewer))
	}
	m.mu.Unlock()
}

func forgetViewerRequestsLocked(runtime *process, current *viewer) {
	for requestID, pending := range runtime.pending {
		if pending.viewer != current {
			continue
		}
		if pending.timer != nil {
			pending.timer.Stop()
		}
		delete(runtime.pending, requestID)
	}
}

func newBrowserRequestID(prefix string) string {
	value, err := (identity.UUIDGenerator{}).NewID()
	if err == nil && value != "" {
		return prefix + "-" + value
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
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
