package server

import (
	"context"
	"net/http"
	"time"
)

func (s *Server) readiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.worker == nil || !s.worker.Available() {
		writeError(w, http.StatusServiceUnavailable, "terminal worker unavailable")
		return
	}
	if s.databaseReady != nil {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := s.databaseReady(ctx); err != nil {
			writeError(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
	}
	writeJSON(w, http.StatusOK, successResponse{Status: "ok"})
}
