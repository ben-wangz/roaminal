package server

import (
	"errors"
	"net/http"

	"github.com/ben-wangz/roaminal/backend/internal/auth"
)

func (s *Server) challenge(w http.ResponseWriter, r *http.Request) {
	noStoreAuthResponse(w)
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
	noStoreAuthResponse(w)
	var body struct {
		ChallengeID string `json:"challengeId"`
		Response    string `json:"response"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	result, err := s.auth.Login(body.ChallengeID, body.Response, r.UserAgent())
	if err != nil {
		writeAuthError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

// totpSetup serves the restricted enrollment stage. It accepts only a
// setup-purpose pending token and never returns normal credentials.
func (s *Server) totpSetup(w http.ResponseWriter, r *http.Request) {
	noStoreAuthResponse(w)
	var body struct {
		PendingToken string `json:"pendingToken"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	result, err := s.auth.Setup(body.PendingToken)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

// totpConfirm completes the enrollment. A durable enrollment invalidates all
// existing credentials and always requires a fresh password proof.
func (s *Server) totpConfirm(w http.ResponseWriter, r *http.Request) {
	noStoreAuthResponse(w)
	var body struct {
		PendingToken string `json:"pendingToken"`
		Code         string `json:"code"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	result, err := s.auth.ConfirmSetup(body.PendingToken, body.Code)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

// totpVerify completes a configured login and returns the normal token
// response after successful TOTP validation.
func (s *Server) totpVerify(w http.ResponseWriter, r *http.Request) {
	noStoreAuthResponse(w)
	var body struct {
		PendingToken string `json:"pendingToken"`
		Code         string `json:"code"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	result, err := s.auth.VerifyTOTP(body.PendingToken, body.Code, r.UserAgent())
	if err != nil {
		writeAuthError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

// writeAuthError maps authentication errors onto the shared error envelope.
// Every mandatory-TOTP denial stays distinct from invalid credentials so the
// frontend can steer its next stage without leaking secret material.
func writeAuthError(w http.ResponseWriter, err error) {
	noStoreAuthResponse(w)
	switch {
	case errors.Is(err, auth.ErrInvalidChallenge):
		writeError(w, 400, "invalid login challenge")
	case errors.Is(err, auth.ErrLocked):
		writeError(w, 403, "service locked")
	case errors.Is(err, auth.ErrInvalidPending):
		writeError(w, 401, "invalid or expired pending token", "pendingToken")
	case errors.Is(err, auth.ErrAlreadyConfigured):
		writeError(w, 409, "TOTP enrollment already configured")
	case errors.Is(err, auth.ErrEnrollmentStorage):
		writeError(w, 503, "auth enrollment storage unavailable")
	default:
		writeError(w, 401, "unauthorized")
	}
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	noStoreAuthResponse(w)
	var body struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	result, err := s.auth.Refresh(body.RefreshToken, r.UserAgent())
	if err != nil {
		writeAuthError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	noStoreAuthResponse(w)
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

func noStoreAuthResponse(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
}

func (s *Server) revokeAuthSession(w http.ResponseWriter, r *http.Request, _ string) {
	noStoreAuthResponse(w)
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
	noStoreAuthResponse(w)
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
	noStoreAuthResponse(w)
	result, err := s.auth.Current(sessionID)
	if err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) authSessions(w http.ResponseWriter, _ *http.Request, sessionID string) {
	noStoreAuthResponse(w)
	writeJSON(w, 200, authSessionCollectionResponse{Sessions: s.auth.List(sessionID)})
}
