package server

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"github.com/ben-wangz/roaminal/backend/internal/sshkey"
)

func keySet(keys []sshkey.Key) map[string]bool {
	result := make(map[string]bool, len(keys))
	for _, key := range keys {
		result[key.FileName] = true
	}
	return result
}

func (s *Server) sources() (*sshconfig.Repository, map[string]bool) {
	if s.sshConfig == nil {
		return nil, map[string]bool{}
	}
	if s.sshKeys == nil {
		return s.sshConfig, map[string]bool{}
	}
	return s.sshConfig, keySet(s.sshKeys.List())
}

func (s *Server) listConnectionDefinitions(w http.ResponseWriter, _ *http.Request, _ string) {
	repo, keys := s.sources()
	if repo == nil {
		writeJSON(w, http.StatusOK, map[string]any{"configSource": map[string]any{"status": "unavailable", "readable": false, "writable": false, "warnings": []any{}, "blockers": []string{"ssh_directory"}}, "definitions": []any{map[string]any{"connectionDefinitionId": "local", "type": "local"}}})
		return
	}
	collection, err := repo.Collection(keys)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config unavailable")
		return
	}
	w.Header().Set("ETag", collection.ETag)
	writeJSON(w, http.StatusOK, collection)
}

type definitionBody struct {
	Type                  string   `json:"type"`
	HostAlias             string   `json:"hostAlias"`
	HostName              *string  `json:"hostName"`
	User                  *string  `json:"user"`
	Port                  *uint16  `json:"port"`
	IdentityFileNames     []string `json:"identityFileNames"`
	IdentitiesOnly        *string  `json:"identitiesOnly"`
	StrictHostKeyChecking *string  `json:"strictHostKeyChecking"`
	UserKnownHostsFile    *string  `json:"userKnownHostsFile"`
	ServerAliveInterval   *uint32  `json:"serverAliveInterval"`
}

func editFromBody(body definitionBody) sshconfig.Edit {
	return sshconfig.Edit{HostAlias: body.HostAlias, HostName: body.HostName, User: body.User, Port: body.Port, IdentityFileNames: body.IdentityFileNames, IdentitiesOnly: body.IdentitiesOnly, StrictHostKeyChecking: body.StrictHostKeyChecking, UserKnownHostsFile: body.UserKnownHostsFile, ServerAliveInterval: body.ServerAliveInterval}
}

func matchETag(r *http.Request) string { return strings.TrimSpace(r.Header.Get("If-Match")) }
func writeCollection(w http.ResponseWriter, collection sshconfig.Collection) {
	w.Header().Set("ETag", collection.ETag)
	writeJSON(w, http.StatusOK, collection)
}

func decodeAlias(path, prefix string) (string, error) {
	raw := strings.TrimPrefix(path, prefix)
	if strings.Contains(raw, "/") {
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
	repo, keys := s.sources()
	if repo == nil {
		writeError(w, http.StatusServiceUnavailable, "ssh directory unavailable")
		return
	}
	collection, err := repo.Create(matchETag(r), keys, editFromBody(body))
	if err != nil {
		s.definitionError(w, err)
		return
	}
	writeCollection(w, collection)
}

func (s *Server) updateConnectionDefinition(w http.ResponseWriter, r *http.Request, _ string) {
	alias, err := decodeAlias(r.URL.Path, "/api/connection-definitions/")
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
	repo, keys := s.sources()
	if repo == nil {
		writeError(w, http.StatusServiceUnavailable, "ssh directory unavailable")
		return
	}
	collection, err := repo.Update(matchETag(r), keys, alias, editFromBody(body))
	if err != nil {
		s.definitionError(w, err)
		return
	}
	writeCollection(w, collection)
}

func (s *Server) duplicateConnectionDefinition(w http.ResponseWriter, r *http.Request, _ string) {
	path := strings.TrimSuffix(r.URL.Path, "/duplicate")
	alias, err := decodeAlias(path, "/api/connection-definitions/")
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
	repo, keys := s.sources()
	if repo == nil {
		writeError(w, http.StatusServiceUnavailable, "ssh directory unavailable")
		return
	}
	collection, err := repo.Duplicate(matchETag(r), keys, alias, body.HostAlias)
	if err != nil {
		s.definitionError(w, err)
		return
	}
	writeCollection(w, collection)
}

func (s *Server) deleteConnectionDefinition(w http.ResponseWriter, r *http.Request, _ string) {
	alias, err := decodeAlias(r.URL.Path, "/api/connection-definitions/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid definition id")
		return
	}
	repo, keys := s.sources()
	if repo == nil {
		writeError(w, http.StatusServiceUnavailable, "ssh directory unavailable")
		return
	}
	collection, err := repo.Delete(matchETag(r), keys, alias)
	if err != nil {
		s.definitionError(w, err)
		return
	}
	writeCollection(w, collection)
}

func (s *Server) definitionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sshconfig.ErrPreconditionRequired):
		writeError(w, http.StatusPreconditionRequired, "config etag is required", "if_match")
	case errors.Is(err, sshconfig.ErrPreconditionFailed):
		writeError(w, http.StatusPreconditionFailed, "config etag does not match", "if_match")
	case errors.Is(err, sshconfig.ErrFieldNotEditable):
		writeError(w, http.StatusUnprocessableEntity, err.Error(), "field")
	case errors.Is(err, os.ErrNotExist):
		writeError(w, http.StatusNotFound, "definition not found")
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
}

func (s *Server) listSSHKeys(w http.ResponseWriter, _ *http.Request, _ string) {
	if s.sshKeys == nil {
		writeJSON(w, http.StatusOK, map[string]any{"keys": []sshkey.Key{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": s.sshKeys.List()})
}

func (s *Server) publicSSHKey(w http.ResponseWriter, r *http.Request, _ string) {
	if s.sshKeys == nil {
		writeError(w, http.StatusNotFound, "key not found")
		return
	}
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/ssh-keys/"), "/public-key")
	value, err := s.sshKeys.Public(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "public key not found")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"publicKey": value})
}
