package server

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
)

func (s *Server) listConnectionDefinitions(w http.ResponseWriter, _ *http.Request, _ string) {
	if s.definitions == nil || !s.definitions.Available() {
		writeJSON(w, http.StatusOK, unavailableDefinitionCollectionResponse{
			ConfigSource:      sshconfig.ConfigSource{Status: "unavailable", Readable: false, Writable: false, Warnings: []sshconfig.Warning{}, Blockers: []string{"ssh_directory"}},
			TmuxOptionsSource: connectionoptions.Source{Status: "unavailable", Readable: false, Writable: false},
			Definitions:       []sshconfig.Definition{{ConnectionDefinitionID: "local", Type: "local"}},
		})
		return
	}
	collection, err := s.definitions.Collection()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config unavailable")
		return
	}
	w.Header().Set("ETag", collection.ETag)
	writeJSON(w, http.StatusOK, collection)
}

type definitionBody struct {
	Type                  string                       `json:"type"`
	HostAlias             string                       `json:"hostAlias"`
	HostName              *string                      `json:"hostName"`
	User                  *string                      `json:"user"`
	Port                  *uint16                      `json:"port"`
	IdentityFileNames     []string                     `json:"identityFileNames"`
	IdentitiesOnly        *string                      `json:"identitiesOnly"`
	StrictHostKeyChecking *string                      `json:"strictHostKeyChecking"`
	UserKnownHostsFile    *string                      `json:"userKnownHostsFile"`
	ServerAliveInterval   *uint32                      `json:"serverAliveInterval"`
	Tmux                  *sshconfig.TmuxOptions       `json:"tmux"`
	FileSystem            *sshconfig.FileSystemOptions `json:"filesystem"`
}

func editFromBody(body definitionBody) sshconfig.Edit {
	return sshconfig.Edit{HostAlias: body.HostAlias, HostName: body.HostName, User: body.User, Port: body.Port, IdentityFileNames: body.IdentityFileNames, IdentitiesOnly: body.IdentitiesOnly, StrictHostKeyChecking: body.StrictHostKeyChecking, UserKnownHostsFile: body.UserKnownHostsFile, ServerAliveInterval: body.ServerAliveInterval}
}

func matchETag(r *http.Request) string { return strings.TrimSpace(r.Header.Get("If-Match")) }

func writeCollection(w http.ResponseWriter, collection sshconfig.Collection) {
	w.Header().Set("ETag", collection.ETag)
	writeJSON(w, http.StatusOK, collection)
}

func decodeAlias(raw string) (string, error) {
	if raw == "" || strings.Contains(raw, "/") {
		return "", errors.New("invalid definition id")
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(decoded, "ssh.") {
		return "", errors.New("invalid definition id")
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(decoded, "ssh."))
	if err != nil || len(data) == 0 {
		return "", errors.New("invalid definition id")
	}
	return string(data), nil
}

func (s *Server) createConnectionDefinition(w http.ResponseWriter, r *http.Request, _ string) {
	var body definitionBody
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	if body.Type != "ssh" {
		writeError(w, http.StatusBadRequest, "only ssh definitions can be created")
		return
	}
	if s.definitions == nil || !s.definitions.Available() {
		writeError(w, http.StatusServiceUnavailable, "ssh directory unavailable")
		return
	}
	collection, err := s.definitions.Create(matchETag(r), editFromBody(body), body.Tmux, body.FileSystem)
	if err != nil {
		s.definitionError(w, err)
		return
	}
	writeCollection(w, collection)
}

func (s *Server) updateConnectionDefinition(w http.ResponseWriter, r *http.Request, _ string) {
	alias, err := decodeAlias(r.PathValue("connectionDefinitionId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid definition id")
		return
	}
	var body definitionBody
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	if body.Type != "ssh" {
		writeError(w, http.StatusBadRequest, "only ssh definitions can be updated")
		return
	}
	if s.definitions == nil || !s.definitions.Available() {
		writeError(w, http.StatusServiceUnavailable, "ssh directory unavailable")
		return
	}
	collection, err := s.definitions.Update(matchETag(r), alias, editFromBody(body), body.Tmux, body.FileSystem)
	if err != nil {
		s.definitionError(w, err)
		return
	}
	writeCollection(w, collection)
}

func (s *Server) duplicateConnectionDefinition(w http.ResponseWriter, r *http.Request, _ string) {
	alias, err := decodeAlias(r.PathValue("connectionDefinitionId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid definition id")
		return
	}
	var body struct {
		HostAlias string `json:"hostAlias"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	if s.definitions == nil || !s.definitions.Available() {
		writeError(w, http.StatusServiceUnavailable, "ssh directory unavailable")
		return
	}
	collection, err := s.definitions.Duplicate(matchETag(r), alias, body.HostAlias)
	if err != nil {
		s.definitionError(w, err)
		return
	}
	writeCollection(w, collection)
}

func (s *Server) deleteConnectionDefinition(w http.ResponseWriter, r *http.Request, _ string) {
	alias, err := decodeAlias(r.PathValue("connectionDefinitionId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid definition id")
		return
	}
	if s.definitions == nil || !s.definitions.Available() {
		writeError(w, http.StatusServiceUnavailable, "ssh directory unavailable")
		return
	}
	collection, err := s.definitions.Delete(matchETag(r), alias)
	if err != nil {
		s.definitionError(w, err)
		return
	}
	writeCollection(w, collection)
}
