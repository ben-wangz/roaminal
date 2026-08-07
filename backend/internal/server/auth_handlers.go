package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

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
	_ = s.auth.Logout(body.RefreshToken, bearer(r))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) revokeAuthSession(w http.ResponseWriter, r *http.Request, _ string) {
	id := strings.TrimPrefix(r.URL.Path, "/api/auth/sessions/")
	if err := s.auth.Revoke(id); err != nil {
		if errors.Is(err, auth.ErrNotFound) {
			writeError(w, 404, "not found")
		} else {
			writeError(w, 500, "internal error")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) logoutOthers(w http.ResponseWriter, _ *http.Request, sessionID string) {
	if err := s.auth.LogoutOthers(sessionID); err != nil {
		writeError(w, 500, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var _ = json.RawMessage{}
