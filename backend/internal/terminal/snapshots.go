package terminal

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"strconv"
	"time"
)

func (s *Session) scheduleSnapshotLocked() {
	if s.snapshotTimer == nil {
		s.dirtySince = time.Now()
		s.snapshotTimer = time.AfterFunc(250*time.Millisecond, func() { s.saveSnapshot() })
	}
}
func (s *Session) saveSnapshot() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	sequence, meta := s.sequence, s.meta
	s.snapshotTimer, s.dirtySince = nil, time.Time{}
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	payload, through, err := s.manager.worker.Snapshot(ctx, meta.ID, strconv.FormatUint(sequence, 10))
	if err != nil {
		s.manager.storeDegraded(meta.ID, err)
		return
	}
	if s.manager.store == nil {
		return
	}
	if err := s.manager.store.SaveSnapshot(meta.ID, persistence.SnapshotHeader{Cols: meta.Cols, Rows: meta.Rows, ScrollbackLines: s.manager.cfg.ScrollbackLines, ThroughSequence: through}, payload); err != nil {
		s.manager.storeDegraded(meta.ID, err)
		return
	}
	s.mu.Lock()
	if !s.closed && s.sequence != sequence {
		s.scheduleSnapshotLocked()
	}
	s.mu.Unlock()
}
func (s *Session) broadcastLocked(data []byte) {
	for client := range s.clients {
		if !client.enqueue(data, true) {
			delete(s.clients, client)
		}
	}
}
func message(value any) []byte { data, _ := json.Marshal(value); return data }
func (m *Manager) storeDegraded(id string, err error) {
	if m.store != nil {
		m.store.MarkSessionDegraded(id)
	}
	fmt.Printf("Roaminal persistence degraded: %v\n", err)
}
func (m *Manager) fail(err error) {
	select {
	case m.fatal <- err:
	default:
	}
}
