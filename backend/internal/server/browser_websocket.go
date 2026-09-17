package server

import (
	"context"
	"net/http"
)

// browserWebsocket authenticates the viewer before the browser runtime sees
// the upgrade. A missing runtime is a feature-level outage, not a server or
// terminal health failure.
func (s *Server) browserWebsocket(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	// Subscribe before authenticating so a deletion that races the auth check
	// cannot leave a newly accepted browser stream unwatched.
	stopEnrollmentWatch := s.watchEnrollment(ctx, cancel)
	defer stopEnrollmentWatch()
	authSessionID, err := s.auth.Authenticate(websocketToken(r))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := ctx.Err(); err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if s.browser == nil || !s.browser.Available() {
		writeError(w, http.StatusServiceUnavailable, "browser runtime unavailable")
		return
	}
	s.browser.Handle(ctx, w, r, authSessionID)
}
