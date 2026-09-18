package browser

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/identity"
)

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
