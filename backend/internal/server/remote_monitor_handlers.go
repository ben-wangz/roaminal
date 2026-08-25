package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func (s *Server) remoteMonitor(w http.ResponseWriter, r *http.Request, _ string) {
	id := r.PathValue("connectionInstanceId")
	if id == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	result, err := s.terms.RemoteMonitor(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, ports.ErrRemoteInstanceNotFound):
			writeError(w, http.StatusNotFound, "not found")
		case errors.Is(err, ports.ErrRemoteNoTransport):
			writeError(w, http.StatusConflict, "no remote transport", "no_remote_transport")
		case errors.Is(err, ports.ErrTransportUnavailable):
			retryable := true
			writeCodedErrorWithRetry(w, http.StatusServiceUnavailable, "remote transport unavailable", "remote_transport_unavailable", nil, &retryable)
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			writeError(w, http.StatusRequestTimeout, "remote monitor timeout")
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}
