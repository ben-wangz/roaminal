package auth

import "github.com/ben-wangz/roaminal/backend/internal/persistence"

func cloneConnectionInstanceLayout(layout *persistence.ConnectionInstanceLayout) *persistence.ConnectionInstanceLayout {
	if layout == nil {
		return nil
	}
	copyLayout := *layout
	copyLayout.GroupOrder = cloneStrings(layout.GroupOrder)
	copyLayout.UngroupedConnectionInstanceIDs = cloneStrings(layout.UngroupedConnectionInstanceIDs)
	copyLayout.Groups = make([]persistence.ConnectionInstanceGroup, len(layout.Groups))
	for index, group := range layout.Groups {
		copyLayout.Groups[index] = group
		copyLayout.Groups[index].ConnectionInstanceIDs = cloneStrings(group.ConnectionInstanceIDs)
	}
	return &copyLayout
}

func cloneStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return append([]string{}, values...)
}

// ConnectionInstanceLayout returns the grouped sidebar layout saved for a
// login session. A missing layout indicates a legacy flat-order session.
func (m *Manager) ConnectionInstanceLayout(sessionID string) (persistence.ConnectionInstanceLayout, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.refresh[sessionID]
	if !ok || entry.ConnectionInstanceLayout == nil {
		return persistence.ConnectionInstanceLayout{}, false
	}
	return *cloneConnectionInstanceLayout(entry.ConnectionInstanceLayout), true
}

// SetConnectionInstanceLayout atomically replaces a login session's grouped
// sidebar layout and keeps the legacy flat order synchronized for migration.
func (m *Manager) SetConnectionInstanceLayout(sessionID string, layout persistence.ConnectionInstanceLayout) error {
	if err := persistence.ValidateConnectionInstanceLayout(&layout); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.refresh[sessionID]
	if !ok {
		return ErrNotFound
	}
	previousLayout := cloneConnectionInstanceLayout(entry.ConnectionInstanceLayout)
	previousOrder := append([]string(nil), entry.ConnectionInstanceOrder...)
	entry.ConnectionInstanceLayout = cloneConnectionInstanceLayout(&layout)
	entry.ConnectionInstanceOrder = flattenConnectionInstanceLayout(layout)
	m.refresh[sessionID] = entry
	if err := m.persistLocked(); err != nil {
		entry.ConnectionInstanceLayout = previousLayout
		entry.ConnectionInstanceOrder = previousOrder
		m.refresh[sessionID] = entry
		return err
	}
	return nil
}

func flattenConnectionInstanceLayout(layout persistence.ConnectionInstanceLayout) []string {
	groups := make(map[string][]string, len(layout.Groups))
	for _, group := range layout.Groups {
		groups[group.GroupID] = group.ConnectionInstanceIDs
	}
	result := make([]string, 0, len(layout.UngroupedConnectionInstanceIDs))
	for _, groupID := range layout.GroupOrder {
		if groupID == persistence.UngroupedConnectionInstanceGroupID {
			result = append(result, layout.UngroupedConnectionInstanceIDs...)
			continue
		}
		result = append(result, groups[groupID]...)
	}
	return result
}
