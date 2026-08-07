package server

import (
	"errors"
	"net/http"
	"os"

	"github.com/ben-wangz/roaminal/backend/internal/monitor"
	"github.com/ben-wangz/roaminal/backend/internal/terminal"
)

type heartbeatUpdate struct {
	Updates struct {
		Sessions []struct {
			ID     string `json:"id"`
			Resize *struct {
				Cols int `json:"cols"`
				Rows int `json:"rows"`
			} `json:"resize,omitempty"`
		} `json:"sessions"`
	} `json:"updates"`
}
type heartbeatResponse struct {
	Sessions []terminal.Summary  `json:"sessions"`
	System   monitor.SystemStats `json:"system"`
	Runtime  struct {
		BootID              string `json:"bootId"`
		PersistenceDegraded bool   `json:"persistenceDegraded"`
		ScrollbackLines     int    `json:"scrollbackLines"`
	} `json:"runtime"`
}

func (s *Server) heartbeatGet(w http.ResponseWriter, _ *http.Request, _ string) {
	writeJSON(w, 200, s.heartbeat())
}
func (s *Server) heartbeatPost(w http.ResponseWriter, r *http.Request, _ string) {
	var body heartbeatUpdate
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	seen := map[string]bool{}
	for _, update := range body.Updates.Sessions {
		if seen[update.ID] {
			writeError(w, 400, "duplicate session id")
			return
		}
		seen[update.ID] = true
		if update.Resize != nil && (update.Resize.Cols < 2 || update.Resize.Cols > 1000 || update.Resize.Rows < 1 || update.Resize.Rows > 1000) {
			writeError(w, 400, "invalid heartbeat update")
			return
		}
	}
	for _, update := range body.Updates.Sessions {
		if update.Resize != nil {
			if err := s.terms.Resize(update.ID, nil, update.Resize.Cols, update.Resize.Rows); err != nil && !errors.Is(err, os.ErrNotExist) {
				writeError(w, 400, "invalid heartbeat update")
				return
			}
		}
	}
	writeJSON(w, 200, s.heartbeat())
}
func (s *Server) heartbeat() heartbeatResponse {
	result := heartbeatResponse{Sessions: s.terms.Summaries(), System: s.monitor.Stats()}
	result.Runtime.BootID = s.bootID
	result.Runtime.PersistenceDegraded = s.terms.PersistenceDegraded()
	result.Runtime.ScrollbackLines = s.cfg.ScrollbackLines
	return result
}
