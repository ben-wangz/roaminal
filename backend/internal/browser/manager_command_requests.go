package browser

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/identity"
)

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
