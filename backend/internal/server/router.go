package server

import (
	"net/http"

	"github.com/ben-wangz/roaminal/backend/internal/api"
)

func (s *Server) newAPIRouter() http.Handler {
	mux := http.NewServeMux()
	plain := func(method string, fn http.HandlerFunc) http.Handler { return methodRoute{method: fn} }
	protected := func(method string, fn authenticatedHandler) http.Handler {
		return methodRoute{method: s.authenticatedRoute(fn)}
	}
	mux.Handle("/healthz", plain(http.MethodGet, s.health))
	mux.Handle(api.HTTPPrefix+"/version", plain(http.MethodGet, s.versionInfo))
	if s.cfg.ClientDiagnosticsEnabled && s.diagnostics != nil {
		mux.Handle(api.HTTPPrefix+"/client-diagnostics", protected(http.MethodPost, s.clientDiagnostics))
	}
	mux.Handle(api.HTTPPrefix+"/auth/challenge", plain(http.MethodPost, s.challenge))
	mux.Handle(api.HTTPPrefix+"/auth/login", plain(http.MethodPost, s.login))
	mux.Handle(api.HTTPPrefix+"/auth/refresh", plain(http.MethodPost, s.refresh))
	mux.Handle(api.HTTPPrefix+"/auth/logout", plain(http.MethodPost, s.logout))
	mux.Handle(api.HTTPPrefix+"/auth/session", protected(http.MethodGet, s.currentSession))
	mux.Handle(api.HTTPPrefix+"/auth/sessions", protected(http.MethodGet, s.authSessions))
	mux.Handle(api.HTTPPrefix+"/auth/sessions/{authSessionId}", protected(http.MethodDelete, s.revokeAuthSession))
	mux.Handle(api.HTTPPrefix+"/auth/logout-others", protected(http.MethodPost, s.logoutOthers))
	mux.Handle(api.HTTPPrefix+"/heartbeat", methodRoute{
		http.MethodGet:  s.authenticatedRoute(s.heartbeatGet),
		http.MethodPost: s.authenticatedRoute(s.heartbeatPost),
	})
	mux.Handle(api.HTTPPrefix+"/messages", methodRoute{
		http.MethodGet: s.authenticatedRoute(s.listMessages),
	})
	mux.Handle(api.HTTPPrefix+"/messages/read-state", methodRoute{
		http.MethodPut: s.authenticatedRoute(s.markMessagesRead),
	})
	mux.Handle(api.HTTPPrefix+"/connection-instances", methodRoute{
		http.MethodGet:  s.authenticatedRoute(s.listConnectionInstances),
		http.MethodPost: s.authenticatedRoute(s.createConnectionInstance),
	})
	mux.Handle(api.HTTPPrefix+"/connection-instances/{connectionInstanceId}", methodRoute{
		http.MethodGet:    s.authenticatedRoute(s.getConnectionInstance),
		http.MethodDelete: s.authenticatedRoute(s.deleteConnectionInstance),
	})
	mux.Handle(api.HTTPPrefix+"/connection-instances/{connectionInstanceId}/agent", protected(http.MethodGet, s.agentSummary))
	mux.Handle(api.HTTPPrefix+"/connection-instances/{connectionInstanceId}/agent/initializations", methodRoute{
		http.MethodPost: s.authenticatedRoute(s.startAgentInitialization),
	})
	mux.Handle(api.HTTPPrefix+"/agent/initializations/{initializationId}", protected(http.MethodGet, s.getAgentInitialization))
	mux.Handle(api.HTTPPrefix+"/agent/events", plain(http.MethodPost, s.agentEvent))
	mux.Handle(api.HTTPPrefix+"/connection-instances/order", protected(http.MethodPut, s.reorderConnectionInstances))
	mux.Handle(api.HTTPPrefix+"/connection-instance-groups", methodRoute{
		http.MethodGet:  s.authenticatedRoute(s.listConnectionInstanceGroups),
		http.MethodPost: s.authenticatedRoute(s.createConnectionInstanceGroup),
	})
	mux.Handle(api.HTTPPrefix+"/connection-instance-groups/layout", protected(http.MethodPut, s.replaceConnectionInstanceLayout))
	mux.Handle(api.HTTPPrefix+"/connection-instance-groups/{groupId}", methodRoute{
		http.MethodPatch:  s.authenticatedRoute(s.renameConnectionInstanceGroup),
		http.MethodDelete: s.authenticatedRoute(s.deleteConnectionInstanceGroup),
	})
	mux.Handle(api.HTTPPrefix+"/connection-instances/{connectionInstanceId}/remote-monitor", protected(http.MethodGet, s.remoteMonitor))
	mux.Handle(api.HTTPPrefix+"/connection-instances/{connectionInstanceId}/title", protected(http.MethodPatch, s.updateConnectionTitle))
	mux.Handle(api.HTTPPrefix+"/connection-instances/{connectionInstanceId}/filesystem/root", protected(http.MethodGet, s.filesystemRoot))
	mux.Handle(api.HTTPPrefix+"/connection-instances/{connectionInstanceId}/filesystem/entries", protected(http.MethodGet, s.filesystemEntries))
	mux.Handle(api.HTTPPrefix+"/connection-instances/{connectionInstanceId}/filesystem/stat", protected(http.MethodGet, s.filesystemStat))
	mux.Handle(api.HTTPPrefix+"/connection-instances/{connectionInstanceId}/filesystem/content", protected(http.MethodGet, s.filesystemContent))
	mux.Handle(api.HTTPPrefix+"/connection-instances/{connectionInstanceId}/filesystem/uploads", methodRoute{
		http.MethodPost: s.authenticatedRoute(s.filesystemCreateUpload),
	})
	mux.Handle(api.HTTPPrefix+"/connection-instances/{connectionInstanceId}/filesystem/uploads/{uploadId}", methodRoute{
		http.MethodGet:    s.authenticatedRoute(s.filesystemGetUpload),
		http.MethodDelete: s.authenticatedRoute(s.filesystemCancelUpload),
	})
	mux.Handle(api.HTTPPrefix+"/connection-launches", protected(http.MethodPost, s.createConnectionLaunch))
	mux.Handle(api.HTTPPrefix+"/connection-launches/{launchId}", protected(http.MethodDelete, s.deleteConnectionLaunch))
	mux.Handle(api.HTTPPrefix+"/connection-definitions", methodRoute{
		http.MethodGet:  s.authenticatedRoute(s.listConnectionDefinitions),
		http.MethodPost: s.authenticatedRoute(s.createConnectionDefinition),
	})
	mux.Handle(api.HTTPPrefix+"/connection-definitions/{connectionDefinitionId}", methodRoute{
		http.MethodPut:    s.authenticatedRoute(s.updateConnectionDefinition),
		http.MethodDelete: s.authenticatedRoute(s.deleteConnectionDefinition),
	})
	mux.Handle(api.HTTPPrefix+"/connection-definitions/{connectionDefinitionId}/duplicate", protected(http.MethodPost, s.duplicateConnectionDefinition))
	mux.Handle(api.HTTPPrefix+"/ssh-keys", protected(http.MethodGet, s.listSSHKeys))
	mux.Handle(api.HTTPPrefix+"/ssh-keys/{keyId}", protected(http.MethodDelete, s.deleteSSHKey))
	mux.Handle(api.HTTPPrefix+"/ssh-keys/{keyId}/public-key", protected(http.MethodGet, s.publicSSHKey))
	mux.Handle(api.HTTPPrefix+"/ssh-key-generations", protected(http.MethodPost, s.generateSSHKey))
	mux.Handle(api.WebSocketPrefix+"/connection-instances/{connectionInstanceId}", plain(http.MethodGet, s.websocket))
	mux.Handle(api.WebSocketPrefix+"/connection-launches/{launchId}", plain(http.MethodGet, s.websocket))
	return mux
}
