package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/clientdiag"
)

func (s *Server) clientDiagnostics(w http.ResponseWriter, r *http.Request, sessionID string) {
	var body clientdiag.Batch
	if err := decodeClientDiagnostics(w, r, &body); err != nil {
		return
	}
	if err := s.diagnostics.Accept(sessionID, r.UserAgent(), body); err != nil {
		switch {
		case errors.Is(err, clientdiag.ErrRateLimited):
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "client diagnostics rate limited")
		case errors.Is(err, clientdiag.ErrInvalid):
			writeError(w, http.StatusBadRequest, "invalid client diagnostics")
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeClientDiagnostics(w http.ResponseWriter, r *http.Request, target any) error {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeError(w, http.StatusBadRequest, "invalid client diagnostics")
		return errors.New("content type")
	}
	r.Body = http.MaxBytesReader(w, r.Body, clientdiag.MaxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid client diagnostics")
		}
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return err
		}
		writeError(w, http.StatusBadRequest, "invalid client diagnostics")
		return errors.New("multiple JSON values")
	}
	return nil
}
