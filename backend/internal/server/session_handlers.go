package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/terminal"
)

type createSessionRequest struct {
	Cwd  string `json:"cwd,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request, _ string) {
	var body createSessionRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	result, err := s.terms.Create(r.Context(), body.Cwd, body.Cols, body.Rows)
	if err != nil {
		if strings.Contains(err.Error(), "capacity") {
			writeError(w, 409, "session capacity reached")
		} else {
			writeError(w, 400, err.Error())
		}
		return
	}
	writeJSON(w, 201, result)
}
func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request, _ string) {
	id := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if err := s.terms.Delete(r.Context(), id); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, 404, "not found")
		} else {
			writeError(w, 500, "internal error")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type updateSessionTitleRequest struct {
	Title json.RawMessage `json:"title"`
}

func (s *Server) updateSessionTitle(w http.ResponseWriter, r *http.Request, _ string) {
	var body updateSessionTitleRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	if len(body.Title) == 0 {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	var title *string
	if strings.TrimSpace(string(body.Title)) != "null" {
		var value string
		if err := json.Unmarshal(body.Title, &value); err != nil {
			writeError(w, http.StatusBadRequest, "title must be a string or null")
			return
		}
		title = &value
	}
	idPath := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	id := strings.TrimSuffix(idPath, "/title")
	result, err := s.terms.SetTitle(id, title)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "not found")
		} else if errors.Is(err, os.ErrProcessDone) {
			writeError(w, http.StatusConflict, "terminal is closed")
		} else if strings.Contains(err.Error(), "title") {
			writeError(w, http.StatusBadRequest, err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}

var _ = terminal.ErrClientCapacity
