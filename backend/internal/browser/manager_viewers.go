package browser

func (m *Manager) hasClientLocked(runtime *process, clientID string) bool {
	for current := range m.viewers {
		if current.runtime == runtime && current.clientID == clientID {
			return true
		}
	}
	return false
}

func (m *Manager) anyVisibleLocked(runtime *process) bool {
	for current := range m.viewers {
		if current.runtime == runtime && current.visible {
			return true
		}
	}
	return false
}
