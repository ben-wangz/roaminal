package server

import (
	"errors"
	"net/http"

	"github.com/ben-wangz/roaminal/backend/internal/auth"
)

func (s *Server) challenge(w http.ResponseWriter, r *http.Request) {
	var body struct{}
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	result, err := s.auth.Challenge()
	if err != nil {
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ChallengeID string `json:"challengeId"`
		Response    string `json:"response"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	result, err := s.auth.Login(body.ChallengeID, body.Response, r.UserAgent())
	if err != nil {
		if errors.Is(err, auth.ErrInvalidChallenge) {
			writeError(w, 400, "invalid login challenge")
		} else if errors.Is(err, auth.ErrLocked) {
			writeError(w, 403, "service locked")
		} else {
			writeError(w, 401, "unauthorized")
		}
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	result, err := s.auth.Refresh(body.RefreshToken, r.UserAgent())
	if err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refreshToken"`
	}
	if r.ContentLength != 0 {
		if err := decodeJSON(w, r, &body); err != nil {
			return
		}
	}
	sessionID, _ := s.auth.Authenticate(bearer(r))
	if sessionID == "" {
		sessionID, _ = s.auth.SessionIDForRefresh(body.RefreshToken)
	}
	_ = s.auth.Logout(body.RefreshToken, bearer(r))
	if s.notifications != nil && sessionID != "" {
		if err := s.notifications.DeleteAll(r.Context(), sessionID); err != nil {
			logNotificationCleanup("logout", err)
		}
	}
	writeSuccess(w)
}

func (s *Server) revokeAuthSession(w http.ResponseWriter, r *http.Request, _ string) {
	id := r.PathValue("authSessionId")
	if err := s.auth.Revoke(id); err != nil {
		if errors.Is(err, auth.ErrNotFound) {
			writeError(w, 404, "not found")
		} else {
			writeError(w, 500, "internal error")
		}
		return
	}
	if s.notifications != nil {
		if err := s.notifications.DeleteAll(r.Context(), id); err != nil {
			logNotificationCleanup("revoke", err)
		}
	}
	writeSuccess(w)
}
func (s *Server) logoutOthers(w http.ResponseWriter, r *http.Request, sessionID string) {
	otherSessions := s.auth.List(sessionID)
	if err := s.auth.LogoutOthers(sessionID); err != nil {
		writeError(w, 500, "internal error")
		return
	}
	if s.notifications != nil {
		for _, other := range otherSessions {
			if other.ID == sessionID {
				continue
			}
			if err := s.notifications.DeleteAll(r.Context(), other.ID); err != nil {
				logNotificationCleanup("logout_others", err)
			}
		}
	}
	writeSuccess(w)
}

func (s *Server) currentSession(w http.ResponseWriter, _ *http.Request, sessionID string) {
	result, err := s.auth.Current(sessionID)
	if err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) authSessions(w http.ResponseWriter, _ *http.Request, sessionID string) {
	writeJSON(w, 200, authSessionCollectionResponse{Sessions: s.auth.List(sessionID)})
}
