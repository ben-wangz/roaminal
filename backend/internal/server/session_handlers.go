package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
)

type createConnectionRequest struct {
	ConnectionDefinitionID            string  `json:"connectionDefinitionId"`
	Cols                              int     `json:"cols,omitempty"`
	Rows                              int     `json:"rows,omitempty"`
	InitialCwd                        *string `json:"initialCwd,omitempty"`
	ReuseFromConnectionInstanceID     *string `json:"reuseFromConnectionInstanceId,omitempty"`
	ReconnectFromConnectionInstanceID *string `json:"reconnectFromConnectionInstanceId,omitempty"`
	RelaunchFromConnectionInstanceID  *string `json:"relaunchFromConnectionInstanceId,omitempty"`
}

func (s *Server) listConnectionInstances(w http.ResponseWriter, _ *http.Request, _ string) {
	writeJSON(w, http.StatusOK, map[string]any{"connectionInstances": s.terms.Summaries()})
}

func (s *Server) getConnectionInstance(w http.ResponseWriter, r *http.Request, _ string) {
	id := strings.TrimPrefix(r.URL.Path, "/api/connection-instances/")
	if strings.Contains(id, "/") || id == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	for _, item := range s.terms.Summaries() {
		if item.ID == id {
			writeJSON(w, http.StatusOK, item)
			return
		}
	}
	writeError(w, http.StatusNotFound, "not found")
}

func (s *Server) createConnectionInstance(w http.ResponseWriter, r *http.Request, _ string) {
	var body createConnectionRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	if body.ConnectionDefinitionID != "local" && body.ConnectionDefinitionID != "" {
		writeError(w, http.StatusUnprocessableEntity, "remote connection definitions are not available", "connection_definition_id")
		return
	}
	cwd := ""
	if body.InitialCwd != nil {
		cwd = *body.InitialCwd
	}
	result, err := s.terms.Create(r.Context(), cwd, body.Cols, body.Rows)
	if err != nil {
		if strings.Contains(err.Error(), "capacity") {
			writeError(w, http.StatusConflict, "connection capacity reached", "capacity")
		} else {
			writeError(w, http.StatusBadRequest, err.Error(), "connection")
		}
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) closeConnectionInstance(w http.ResponseWriter, r *http.Request, _ string) {
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/connection-instances/"), "/close")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err := s.terms.Close(r.Context(), id); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "not found")
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	for _, item := range s.terms.Summaries() {
		if item.ID == id {
			writeJSON(w, http.StatusOK, item)
			return
		}
	}
	writeError(w, http.StatusNotFound, "not found")
}

func (s *Server) deleteConnectionInstance(w http.ResponseWriter, r *http.Request, _ string) {
	id := strings.TrimPrefix(r.URL.Path, "/api/connection-instances/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err := s.terms.Delete(r.Context(), id); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "not found")
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
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
	idPath := strings.TrimPrefix(r.URL.Path, "/api/connection-instances/")
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
