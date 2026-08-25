package server

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/agent"
	"github.com/ben-wangz/roaminal/backend/internal/api"
	"github.com/ben-wangz/roaminal/backend/internal/auth"
	"github.com/ben-wangz/roaminal/backend/internal/clientdiag"
	systemclock "github.com/ben-wangz/roaminal/backend/internal/clock"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/definition"
	"github.com/ben-wangz/roaminal/backend/internal/filesystem"
	"github.com/ben-wangz/roaminal/backend/internal/monitor"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/ben-wangz/roaminal/backend/internal/workspace"
)

type Server struct {
	cfg               config.Config
	auth              *auth.Manager
	workspace         *workspace.Service
	terms             connectionService
	monitor           *monitor.Monitor
	ids               ports.IDGenerator
	clock             ports.Clock
	worker            ports.TerminalWorker
	bootID            string
	version           string
	handler           http.Handler
	api               http.Handler
	started           time.Time
	static            http.Handler
	definitions       *definition.Service
	filesystem        *filesystem.Service
	diagnostics       *clientdiag.Sink
	agentProvisioning agent.ProvisioningService
	agentTelemetry    agent.TelemetryService
}

type Dependencies struct {
	Config            config.Config
	Version           string
	BootID            string
	Auth              *auth.Manager
	Workspace         *workspace.Service
	Connections       connectionService
	Monitor           *monitor.Monitor
	IDs               ports.IDGenerator
	Clock             ports.Clock
	Worker            ports.TerminalWorker
	Static            http.Handler
	Definitions       *definition.Service
	Diagnostics       *clientdiag.Sink
	FileSystem        *filesystem.Service
	AgentProvisioning agent.ProvisioningService
	AgentTelemetry    agent.TelemetryService
}

// New constructs the complete HTTP application graph once. Optional feature
// dependencies are represented explicitly in Dependencies; no setter rebuilds
// the router after construction.
func New(deps Dependencies) *Server {
	static := deps.Static
	if static == nil {
		static = http.NotFoundHandler()
	}
	runtimeClock := deps.Clock
	if runtimeClock == nil {
		runtimeClock = systemclock.System{}
	}
	s := &Server{
		cfg: deps.Config, version: deps.Version, bootID: deps.BootID, auth: deps.Auth, workspace: deps.Workspace,
		terms: deps.Connections, monitor: deps.Monitor, ids: deps.IDs, clock: runtimeClock, worker: deps.Worker,
		started: runtimeClock.Now(), static: static, definitions: deps.Definitions,
		diagnostics: deps.Diagnostics,
	}
	s.filesystem = deps.FileSystem
	s.agentProvisioning = deps.AgentProvisioning
	s.agentTelemetry = deps.AgentTelemetry
	s.api = s.newAPIRouter()
	s.handler = http.HandlerFunc(s.serve)
	return s
}

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if s.ids != nil {
		if value, err := s.ids.NewID(); err == nil && value != "" {
			w.Header().Set("X-Roaminal-Request-ID", value)
		}
	}
	if strings.HasPrefix(r.URL.Path, api.HTTPPrefix+"/") || r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, api.WebSocketPrefix+"/") {
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

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	if !s.worker.Available() {
		writeError(w, http.StatusServiceUnavailable, "terminal worker unavailable")
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Status: "ok"})
}
func (s *Server) versionInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, api.VersionResponse{Name: "roaminal", Version: s.version, APIVersion: api.Version, BootID: s.bootID, ClientDiagnosticsEnabled: s.cfg.ClientDiagnosticsEnabled})
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
