package server

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/auth"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/connection"
	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/monitor"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"github.com/ben-wangz/roaminal/backend/internal/sshkey"
	"github.com/ben-wangz/roaminal/backend/internal/worker"
)

type Server struct {
	cfg               config.Config
	auth              *auth.Manager
	terms             *connection.Manager
	monitor           *monitor.Monitor
	worker            *worker.Client
	bootID            string
	version           string
	handler           http.Handler
	api               http.Handler
	started           time.Time
	static            http.Handler
	sshConfig         *sshconfig.Repository
	sshKeys           *sshkey.Inventory
	connectionOptions *connectionoptions.Store
}

func New(cfg config.Config, version, bootID string, authManager *auth.Manager, terms *connection.Manager, monitorService *monitor.Monitor, terminalWorker *worker.Client) *Server {
	return NewWithStatic(cfg, version, bootID, authManager, terms, monitorService, terminalWorker, http.NotFoundHandler())
}

func NewWithStatic(cfg config.Config, version, bootID string, authManager *auth.Manager, terms *connection.Manager, monitorService *monitor.Monitor, terminalWorker *worker.Client, static http.Handler) *Server {
	s := &Server{cfg: cfg, version: version, bootID: bootID, auth: authManager, terms: terms, monitor: monitorService, worker: terminalWorker, started: time.Now(), static: static}
	s.api = s.newAPIRouter()
	s.handler = http.HandlerFunc(s.serve)
	return s
}

func NewWithSources(cfg config.Config, version, bootID string, authManager *auth.Manager, terms *connection.Manager, monitorService *monitor.Monitor, terminalWorker *worker.Client, static http.Handler, configRepo *sshconfig.Repository, keys *sshkey.Inventory, options *connectionoptions.Store) *Server {
	s := NewWithStatic(cfg, version, bootID, authManager, terms, monitorService, terminalWorker, static)
	s.sshConfig, s.sshKeys, s.connectionOptions = configRepo, keys, options
	return s
}

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, "/ws/") {
		if r.Method != http.MethodOptions && !s.sameOrigin(r) {
			writeError(w, http.StatusForbidden, "origin denied")
			return
		}
		s.api.ServeHTTP(w, r)
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
	return strings.EqualFold(u.Scheme, requestOriginScheme(r))
}

// requestOriginScheme returns the scheme browsers use in Origin headers.
// Some WebSocket-aware proxies report the transport scheme as ws/wss, while
// the browser still sends http/https for the page origin.
func requestOriginScheme(r *http.Request) string {
	proto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
	if r.TLS != nil {
		proto = "https"
	}
	if proto == "" {
		proto = "http"
	}
	switch strings.ToLower(proto) {
	case "ws":
		return "http"
	case "wss":
		return "https"
	default:
		return proto
	}
}

type methodRoute map[string]http.Handler

func (route methodRoute) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if handler, ok := route[r.Method]; ok {
		handler.ServeHTTP(w, r)
		return
	}
	if len(route) > 0 {
		allowed := make([]string, 0, len(route))
		for method := range route {
			allowed = append(allowed, method)
		}
		w.Header().Set("Allow", strings.Join(allowed, ", "))
	}
	writeError(w, http.StatusMethodNotAllowed, "method not allowed", "method")
}

func (s *Server) authenticatedRoute(fn authenticatedHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.withAuth(w, r, fn)
	})
}

func (s *Server) newAPIRouter() http.Handler {
	mux := http.NewServeMux()
	plain := func(method string, fn http.HandlerFunc) http.Handler { return methodRoute{method: fn} }
	protected := func(method string, fn authenticatedHandler) http.Handler {
		return methodRoute{method: s.authenticatedRoute(fn)}
	}
	mux.Handle("/healthz", plain(http.MethodGet, s.health))
	mux.Handle("/api/version", plain(http.MethodGet, s.versionInfo))
	mux.Handle("/api/auth/challenge", plain(http.MethodPost, s.challenge))
	mux.Handle("/api/auth/login", plain(http.MethodPost, s.login))
	mux.Handle("/api/auth/refresh", plain(http.MethodPost, s.refresh))
	mux.Handle("/api/auth/logout", plain(http.MethodPost, s.logout))
	mux.Handle("/api/auth/session", protected(http.MethodGet, s.currentSession))
	mux.Handle("/api/auth/sessions", protected(http.MethodGet, s.authSessions))
	mux.Handle("/api/auth/sessions/{authSessionId}", protected(http.MethodDelete, s.revokeAuthSession))
	mux.Handle("/api/auth/logout-others", protected(http.MethodPost, s.logoutOthers))
	mux.Handle("/api/heartbeat", methodRoute{
		http.MethodGet:  s.authenticatedRoute(s.heartbeatGet),
		http.MethodPost: s.authenticatedRoute(s.heartbeatPost),
	})
	mux.Handle("/api/connection-instances", methodRoute{
		http.MethodGet:  s.authenticatedRoute(s.listConnectionInstances),
		http.MethodPost: s.authenticatedRoute(s.createConnectionInstance),
	})
	mux.Handle("/api/connection-instances/{connectionInstanceId}", methodRoute{
		http.MethodGet:    s.authenticatedRoute(s.getConnectionInstance),
		http.MethodDelete: s.authenticatedRoute(s.deleteConnectionInstance),
	})
	mux.Handle("/api/connection-instances/{connectionInstanceId}/remote-monitor", protected(http.MethodGet, s.remoteMonitor))
	mux.Handle("/api/connection-instances/{connectionInstanceId}/title", protected(http.MethodPatch, s.updateConnectionTitle))
	mux.Handle("/api/connection-launches", protected(http.MethodPost, s.createConnectionLaunch))
	mux.Handle("/api/connection-launches/{launchId}", protected(http.MethodDelete, s.deleteConnectionLaunch))
	mux.Handle("/api/connection-definitions", methodRoute{
		http.MethodGet:  s.authenticatedRoute(s.listConnectionDefinitions),
		http.MethodPost: s.authenticatedRoute(s.createConnectionDefinition),
	})
	mux.Handle("/api/connection-definitions/{connectionDefinitionId}", methodRoute{
		http.MethodPut:    s.authenticatedRoute(s.updateConnectionDefinition),
		http.MethodDelete: s.authenticatedRoute(s.deleteConnectionDefinition),
	})
	mux.Handle("/api/connection-definitions/{connectionDefinitionId}/duplicate", protected(http.MethodPost, s.duplicateConnectionDefinition))
	mux.Handle("/api/ssh-keys", protected(http.MethodGet, s.listSSHKeys))
	mux.Handle("/api/ssh-keys/{keyId}", protected(http.MethodDelete, s.deleteSSHKey))
	mux.Handle("/api/ssh-keys/{keyId}/public-key", protected(http.MethodGet, s.publicSSHKey))
	mux.Handle("/api/ssh-key-generations", protected(http.MethodPost, s.generateSSHKey))
	mux.Handle("/ws/connection-instances/{connectionInstanceId}", plain(http.MethodGet, s.websocket))
	mux.Handle("/ws/connection-launches/{launchId}", plain(http.MethodGet, s.websocket))
	return mux
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
