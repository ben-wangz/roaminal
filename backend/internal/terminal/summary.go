package terminal

import (
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"os"
	"sort"
	"strings"
	"time"
)

func (m *Manager) Summaries() []Summary {
	m.mu.RLock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.mu.RUnlock()
	result := make([]Summary, 0, len(sessions))
	for _, session := range sessions {
		result = append(result, m.summary(session))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result
}
func (m *Manager) ClientCount(id string) int {
	m.mu.RLock()
	session := m.sessions[id]
	m.mu.RUnlock()
	if session == nil {
		return 0
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	return len(session.clients)
}
func (m *Manager) summary(session *Session) Summary {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.meta.SyncEffectiveTitle()
	command := session.command
	if command == "" {
		command = "/bin/bash"
	}
	return Summary{ID: session.meta.ID, ConnectionInstanceID: session.meta.ID, ConnectionDefinitionID: session.meta.ConnectionDefinitionID, Type: session.meta.Type, Purpose: session.meta.Purpose, Lifecycle: lifecycle(session), SourceState: session.meta.SourceState, SourceHostAlias: session.meta.SourceHostAlias, HostVerificationAssessment: session.meta.HostVerificationAssessment, CreatedAt: session.meta.CreatedAt, UpdatedAt: session.meta.UpdatedAt, Shell: command, InitialCwd: session.meta.InitialCwd, Title: session.meta.EffectiveTitle(), TitleMode: titleMode(session.meta), Cwd: session.meta.Cwd, Cols: session.meta.Cols, Rows: session.meta.Rows, Closed: session.closed, Attention: session.attention, ExitStatus: session.exitStatus, GenerationStatus: session.meta.GenerationStatus, GenerationError: session.meta.GenerationError, GenerationStaging: session.meta.GenerationStaging}
}
func lifecycle(session *Session) string {
	if session.closed {
		if session.meta.Lifecycle == "interrupted" {
			return "interrupted"
		}
		return "exited"
	}
	return "live"
}
func titleMode(meta persistence.SessionMeta) string {
	if meta.TitleOverride != nil {
		return "custom"
	}
	return "automatic"
}
func (s *Session) broadcastMetaLocked() {
	s.meta.SyncEffectiveTitle()
	s.broadcastLocked(message(map[string]any{"type": "meta", "title": s.meta.EffectiveTitle(), "titleMode": titleMode(s.meta), "cwd": s.meta.Cwd, "cols": s.meta.Cols, "rows": s.meta.Rows, "sourceState": s.meta.SourceState, "generationStatus": s.meta.GenerationStatus, "generationError": s.meta.GenerationError, "generationStaging": s.meta.GenerationStaging}))
}

func (m *Manager) MarkSourceState(id, state string) error {
	m.mu.RLock()
	session := m.sessions[id]
	m.mu.RUnlock()
	if session == nil {
		return os.ErrNotExist
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if state == "deleted" || (state == "changed" && session.meta.SourceState == "current") {
		session.meta.SourceState = state
	}
	if m.store != nil {
		if err := m.store.SaveSession(session.meta); err != nil {
			return err
		}
	}
	session.broadcastMetaLocked()
	return nil
}

func (m *Manager) MarkGenerationResult(id, state, detail string) error {
	m.mu.RLock()
	session := m.sessions[id]
	m.mu.RUnlock()
	if session == nil {
		return os.ErrNotExist
	}
	session.mu.Lock()
	session.meta.GenerationStatus = state
	session.meta.GenerationError = detail
	if state == "succeeded" {
		session.meta.GenerationStaging = ""
	}
	session.meta.UpdatedAt = time.Now().UTC()
	if m.store != nil {
		if err := m.store.SaveSession(session.meta); err != nil {
			session.mu.Unlock()
			return err
		}
	}
	session.broadcastMetaLocked()
	session.mu.Unlock()
	return nil
}
func (m *Manager) SetTitle(id string, title *string) (Summary, error) {
	m.mu.RLock()
	session := m.sessions[id]
	m.mu.RUnlock()
	if session == nil {
		return Summary{}, os.ErrNotExist
	}
	var override *string
	if title != nil {
		value := strings.TrimSpace(*title)
		if err := persistence.ValidateTitleOverride(value); err != nil {
			return Summary{}, err
		}
		override = &value
	}
	session.mu.Lock()
	oldOverride, oldUpdated := session.meta.TitleOverride, session.meta.UpdatedAt
	session.meta.TitleOverride = override
	session.meta.SyncEffectiveTitle()
	session.meta.UpdatedAt = time.Now().UTC()
	if m.store != nil {
		if err := m.store.SaveSession(session.meta); err != nil {
			session.meta.TitleOverride, session.meta.UpdatedAt = oldOverride, oldUpdated
			session.meta.SyncEffectiveTitle()
			session.mu.Unlock()
			return Summary{}, err
		}
	}
	session.broadcastMetaLocked()
	session.mu.Unlock()
	return m.summary(session), nil
}
