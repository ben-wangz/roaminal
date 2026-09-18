package browser

import (
	"encoding/json"
	"errors"
	"strconv"
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
