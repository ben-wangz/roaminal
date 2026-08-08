package server

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/connection"
	"github.com/ben-wangz/roaminal/backend/internal/sshkey"
)

func (s *Server) generateSSHKey(w http.ResponseWriter, r *http.Request, _ string) {
	var body sshkey.GenerationRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	result, err := s.terms.GenerateKey(r.Context(), body, 120, 36)
	if err != nil {
		switch {
		case errors.Is(err, connection.ErrTransportUnavailable):
			writeError(w, http.StatusServiceUnavailable, "ssh key generation unavailable", "key_generation")
		case errors.Is(err, os.ErrExist), strings.Contains(err.Error(), "already exists"), strings.Contains(err.Error(), "symlink"):
			writeError(w, http.StatusConflict, err.Error(), "file_name")
		case strings.Contains(err.Error(), "not writable"), strings.Contains(err.Error(), "unavailable"):
			writeError(w, http.StatusConflict, err.Error(), "key_generation")
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusCreated, result)
}
