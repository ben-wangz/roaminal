package server

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
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
		writeJSON(w, http.StatusOK, map[string]any{"configSource": map[string]any{"status": "unavailable", "readable": false, "writable": false, "warnings": []any{}, "blockers": []string{"ssh_directory"}}, "tmuxOptionsSource": map[string]any{"status": "unavailable", "readable": false, "writable": false}, "definitions": []any{map[string]any{"connectionDefinitionId": "local", "type": "local"}}})
		return
	}
	collection, err := repo.Collection(keys)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config unavailable")
		return
	}
	s.enrichConnectionOptions(&collection)
	w.Header().Set("ETag", collection.ETag)
	writeJSON(w, http.StatusOK, collection)
}

func (s *Server) enrichConnectionOptions(collection *sshconfig.Collection) {
	if s.connectionOptions == nil {
		collection.TmuxOptionsSource = connectionoptions.Source{Status: "unavailable", Reason: "options store unavailable"}
		return
	}
	aliases := make(map[string]bool)
	if !collection.ConfigSource.Readable && collection.ConfigSource.Status != "missing" {
		loaded, _ := s.connectionOptions.Load(nil)
		collection.TmuxOptionsSource = loaded.Source
		return
	}
	for _, definition := range collection.Definitions {
		if definition.Type == "ssh" {
			aliases[definition.HostAlias] = true
		}
	}
	loaded, _ := s.connectionOptions.Load(aliases)
	collection.TmuxOptionsSource = loaded.Source
	for index := range collection.Definitions {
		definition := &collection.Definitions[index]
		if option, ok := loaded.Options[definition.HostAlias]; ok && definition.Type == "ssh" {
			if option.Enabled {
				definition.Tmux = &sshconfig.TmuxOptions{Enabled: true, SessionName: option.SessionName}
			}
			definition.FileSystem = &sshconfig.FileSystemOptions{Pwd: option.Pwd}
		} else if definition.Type == "ssh" {
			definition.FileSystem = &sshconfig.FileSystemOptions{Pwd: connectionoptions.DefaultPwd}
		}
	}
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
	if body.Tmux != nil || body.FileSystem != nil {
		if err := s.saveConnectionOptions(body.HostAlias, body.Tmux, body.FileSystem); err != nil {
			s.definitionError(w, err)
			return
		}
	}
	s.enrichConnectionOptions(&collection)
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
	if body.Tmux != nil || body.FileSystem != nil {
		if err := s.saveConnectionOptions(alias, body.Tmux, body.FileSystem); err != nil {
			s.definitionError(w, err)
			return
		}
	}
	s.enrichConnectionOptions(&collection)
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
	s.enrichConnectionOptions(&collection)
	writeCollection(w, collection)
}

func (s *Server) deleteConnectionDefinition(w http.ResponseWriter, r *http.Request, _ string) {
	alias, err := decodeAlias(r.PathValue("connectionDefinitionId"))
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
	if err := s.saveTmuxOption(alias, nil); err != nil {
		s.definitionError(w, err)
		return
	}
	s.enrichConnectionOptions(&collection)
	writeCollection(w, collection)
}
