package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/internal/auth"
	"github.com/ben-wangz/roaminal/internal/config"
	"github.com/ben-wangz/roaminal/internal/monitor"
	"github.com/ben-wangz/roaminal/internal/terminal"
	"github.com/ben-wangz/roaminal/internal/webassets"
	"github.com/ben-wangz/roaminal/internal/worker"
	"github.com/coder/websocket"
)

type Server struct {
	cfg     config.Config
	auth    *auth.Manager
	terms   *terminal.Manager
	monitor *monitor.Monitor
	worker  *worker.Client
	bootID  string
	version string
	handler http.Handler
	started time.Time
}

func New(cfg config.Config, version, bootID string, authManager *auth.Manager, terms *terminal.Manager, monitorService *monitor.Monitor, terminalWorker *worker.Client) *Server {
	s := &Server{cfg: cfg, version: version, bootID: bootID, auth: authManager, terms: terms, monitor: monitorService, worker: terminalWorker, started: time.Now()}
	s.handler = http.HandlerFunc(s.serve)
	return s
}

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, "/ws/") {
		if r.Method != http.MethodOptions && !s.sameOrigin(r) {
			writeError(w, http.StatusForbidden, "origin denied")
			return
		}
		s.routeAPI(w, r)
		return
	}
	webassets.Handler().ServeHTTP(w, r)
}

func (s *Server) sameOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if !strings.EqualFold(u.Host, r.Host) {
		return false
	}
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = "http"
	}
	if r.TLS != nil {
		proto = "https"
	}
	return strings.EqualFold(u.Scheme, proto)
}

func (s *Server) routeAPI(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/ws/") {
		s.websocket(w, r)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/healthz":
		s.health(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/version":
		s.versionInfo(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/challenge":
		s.challenge(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/login":
		s.login(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/refresh":
		s.refresh(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/logout":
		s.logout(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/auth/session":
		s.withAuth(w, r, s.currentSession)
	case r.Method == http.MethodGet && r.URL.Path == "/api/auth/sessions":
		s.withAuth(w, r, s.authSessions)
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/logout-others":
		s.withAuth(w, r, s.logoutOthers)
	case r.Method == http.MethodGet && r.URL.Path == "/api/heartbeat":
		s.withAuth(w, r, s.heartbeatGet)
	case r.Method == http.MethodPost && r.URL.Path == "/api/heartbeat":
		s.withAuth(w, r, s.heartbeatPost)
	case r.Method == http.MethodPost && r.URL.Path == "/api/sessions":
		s.withAuth(w, r, s.createSession)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/auth/sessions/"):
		s.withAuth(w, r, s.revokeAuthSession)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/sessions/"):
		s.withAuth(w, r, s.deleteSession)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	if !s.worker.Available() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "terminal worker unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
func (s *Server) versionInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"name": "roaminal", "version": s.version, "apiVersion": "roaminal.v1", "bootId": s.bootID})
}

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
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	_ = s.auth.Logout(body.RefreshToken, bearer(r))
	w.WriteHeader(http.StatusNoContent)
}

type authenticatedHandler func(http.ResponseWriter, *http.Request, string)

func (s *Server) withAuth(w http.ResponseWriter, r *http.Request, fn authenticatedHandler) {
	sessionID, err := s.auth.Authenticate(bearer(r))
	if err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	fn(w, r, sessionID)
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
	writeJSON(w, 200, map[string]any{"sessions": s.auth.List(sessionID)})
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

type heartbeatUpdate struct {
	Updates struct {
		Sessions []struct {
			ID     string `json:"id"`
			Resize *struct {
				Cols int `json:"cols"`
				Rows int `json:"rows"`
			} `json:"resize,omitempty"`
		} `json:"sessions"`
	} `json:"updates"`
}
type heartbeatResponse struct {
	Sessions []terminal.Summary  `json:"sessions"`
	System   monitor.SystemStats `json:"system"`
	Runtime  struct {
		BootID              string `json:"bootId"`
		PersistenceDegraded bool   `json:"persistenceDegraded"`
	} `json:"runtime"`
}

func (s *Server) heartbeatGet(w http.ResponseWriter, _ *http.Request, _ string) {
	writeJSON(w, 200, s.heartbeat())
}
func (s *Server) heartbeatPost(w http.ResponseWriter, r *http.Request, _ string) {
	var body heartbeatUpdate
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	seen := map[string]bool{}
	for _, update := range body.Updates.Sessions {
		if seen[update.ID] {
			writeError(w, 400, "duplicate session id")
			return
		}
		seen[update.ID] = true
		if update.Resize != nil {
			if err := s.terms.Resize(update.ID, nil, update.Resize.Cols, update.Resize.Rows); err != nil && !errors.Is(err, os.ErrNotExist) {
				writeError(w, 400, "invalid heartbeat update")
				return
			}
		}
	}
	writeJSON(w, 200, s.heartbeat())
}
func (s *Server) heartbeat() heartbeatResponse {
	result := heartbeatResponse{Sessions: s.terms.Summaries(), System: s.monitor.Stats()}
	result.Runtime.BootID = s.bootID
	result.Runtime.PersistenceDegraded = s.terms.PersistenceDegraded()
	return result
}

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

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/ws/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, 404, "not found")
		return
	}
	if _, err := s.auth.Authenticate(websocketToken(r)); err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	if s.terms.ClientCount(id) >= s.cfg.MaxClientsPerSession {
		writeError(w, 429, "client capacity reached")
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"roaminal.v1"}})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	client, err := s.terms.Attach(ctx, id)
	if err != nil {
		_ = conn.Close(websocket.StatusCode(1011), "attach failed")
		return
	}
	defer s.terms.Detach(id, client)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			select {
			case <-ctx.Done():
				return
			case <-client.Done():
				return
			case data := <-client.Messages:
				if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
					cancel()
					return
				}
				client.Consumed(len(data))
			}
		}
	}()
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			cancel()
			break
		}
		if typ != websocket.MessageText || len(data) > 1024*1024 {
			_ = conn.Close(websocket.StatusMessageTooBig, "message too large")
			break
		}
		var msg map[string]json.RawMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			_ = conn.Close(websocket.StatusPolicyViolation, "invalid_message")
			break
		}
		var kind string
		_ = json.Unmarshal(msg["type"], &kind)
		switch kind {
		case "input":
			var value string
			if err := json.Unmarshal(msg["data"], &value); err != nil || s.terms.Input(id, client, value) != nil {
				_ = conn.Close(websocket.StatusPolicyViolation, "invalid_message")
				break
			}
		case "resize":
			var value struct {
				Cols int `json:"cols"`
				Rows int `json:"rows"`
			}
			if json.Unmarshal(msg["resize"], &value) != nil {
				_ = json.Unmarshal(data, &value)
			}
			if err := s.terms.Resize(id, client, value.Cols, value.Rows); err != nil {
				_ = conn.Close(websocket.StatusPolicyViolation, "invalid_message")
			}
		case "ping":
			_ = client.EnqueueControl([]byte(`{"type":"pong"}`))
		case "claim_terminal_control":
		default:
			_ = conn.Close(websocket.StatusPolicyViolation, "invalid_message")
		}
	}
	<-writerDone
}

func bearer(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return strings.TrimSpace(value[7:])
	}
	return value
}
func websocketToken(r *http.Request) string {
	protocols := strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",")
	for _, protocol := range protocols {
		protocol = strings.TrimSpace(protocol)
		if strings.HasPrefix(protocol, "roaminal.auth.") {
			return strings.TrimPrefix(protocol, "roaminal.auth.")
		}
	}
	return bearer(r)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeError(w, 400, "content type must be application/json")
		return errors.New("content type")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		writeError(w, 400, "invalid JSON body")
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		writeError(w, 400, "invalid JSON body")
		return errors.New("multiple values")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
