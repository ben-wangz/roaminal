package server

import "net/http"

// browserWebsocket authenticates the viewer before the browser runtime sees
// the upgrade. A missing runtime is a feature-level outage, not a server or
// terminal health failure.
func (s *Server) browserWebsocket(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	authSessionID, err := s.auth.Authenticate(websocketToken(r))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if s.browser == nil || !s.browser.Available() {
		writeError(w, http.StatusServiceUnavailable, "browser runtime unavailable")
		return
	}
	s.browser.Handle(r.Context(), w, r, authSessionID)
}
