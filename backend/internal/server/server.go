package server

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/auth"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/connection"
	"github.com/ben-wangz/roaminal/backend/internal/monitor"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"github.com/ben-wangz/roaminal/backend/internal/sshkey"
	"github.com/ben-wangz/roaminal/backend/internal/worker"
)

type Server struct {
	cfg       config.Config
	auth      *auth.Manager
	terms     *connection.Manager
	monitor   *monitor.Monitor
	worker    *worker.Client
	bootID    string
	version   string
	handler   http.Handler
	started   time.Time
	static    http.Handler
	sshConfig *sshconfig.Repository
	sshKeys   *sshkey.Inventory
}

func New(cfg config.Config, version, bootID string, authManager *auth.Manager, terms *connection.Manager, monitorService *monitor.Monitor, terminalWorker *worker.Client) *Server {
	return NewWithStatic(cfg, version, bootID, authManager, terms, monitorService, terminalWorker, http.NotFoundHandler())
}

func NewWithStatic(cfg config.Config, version, bootID string, authManager *auth.Manager, terms *connection.Manager, monitorService *monitor.Monitor, terminalWorker *worker.Client, static http.Handler) *Server {
	s := &Server{cfg: cfg, version: version, bootID: bootID, auth: authManager, terms: terms, monitor: monitorService, worker: terminalWorker, started: time.Now(), static: static}
	s.handler = http.HandlerFunc(s.serve)
	return s
}

func NewWithSources(cfg config.Config, version, bootID string, authManager *auth.Manager, terms *connection.Manager, monitorService *monitor.Monitor, terminalWorker *worker.Client, static http.Handler, configRepo *sshconfig.Repository, keys *sshkey.Inventory) *Server {
	s := NewWithStatic(cfg, version, bootID, authManager, terms, monitorService, terminalWorker, static)
	s.sshConfig, s.sshKeys = configRepo, keys
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
	s.static.ServeHTTP(w, r)
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
	case r.Method == http.MethodGet && r.URL.Path == "/api/connection-instances":
		s.withAuth(w, r, s.listConnectionInstances)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/connection-instances/") && !strings.HasSuffix(r.URL.Path, "/title"):
		s.withAuth(w, r, s.getConnectionInstance)
	case r.Method == http.MethodPost && r.URL.Path == "/api/connection-instances":
		s.withAuth(w, r, s.createConnectionInstance)
	case r.Method == http.MethodGet && r.URL.Path == "/api/connection-definitions":
		s.withAuth(w, r, s.listConnectionDefinitions)
	case r.Method == http.MethodPost && r.URL.Path == "/api/connection-definitions":
		s.withAuth(w, r, s.createConnectionDefinition)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/connection-definitions/"):
		s.withAuth(w, r, s.updateConnectionDefinition)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/connection-definitions/") && strings.HasSuffix(r.URL.Path, "/duplicate"):
		s.withAuth(w, r, s.duplicateConnectionDefinition)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/connection-definitions/"):
		s.withAuth(w, r, s.deleteConnectionDefinition)
	case r.Method == http.MethodGet && r.URL.Path == "/api/ssh-keys":
		s.withAuth(w, r, s.listSSHKeys)
	case r.Method == http.MethodPost && r.URL.Path == "/api/ssh-key-generations":
		s.withAuth(w, r, s.generateSSHKey)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/ssh-keys/") && strings.HasSuffix(r.URL.Path, "/public-key"):
		s.withAuth(w, r, s.publicSSHKey)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/ssh-keys/"):
		s.withAuth(w, r, s.deleteSSHKey)
	case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/connection-instances/") && strings.HasSuffix(r.URL.Path, "/title"):
		s.withAuth(w, r, s.updateSessionTitle)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/connection-instances/") && strings.HasSuffix(r.URL.Path, "/close"):
		s.withAuth(w, r, s.closeConnectionInstance)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/auth/sessions/"):
		s.withAuth(w, r, s.revokeAuthSession)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/connection-instances/"):
		s.withAuth(w, r, s.deleteConnectionInstance)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	if !s.worker.Available() {
		writeError(w, http.StatusServiceUnavailable, "terminal worker unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
func (s *Server) versionInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"name": "roaminal", "version": s.version, "apiVersion": "roaminal.v1", "bootId": s.bootID})
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
