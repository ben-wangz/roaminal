package server

import (
	"errors"
	"net/http"
	"os"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
	"github.com/ben-wangz/roaminal/backend/internal/sshkey"
)

func (s *Server) saveConnectionOptions(alias string, tmux *sshconfig.TmuxOptions, filesystem *sshconfig.FileSystemOptions) error {
	if s.connectionOptions == nil {
		return nil
	}
	repo, keys := s.sources()
	if repo == nil {
		return sshfs.ErrUnavailable
	}
	collection, err := repo.Collection(keys)
	if err != nil {
		return err
	}
	if !collection.ConfigSource.Readable && collection.ConfigSource.Status != "missing" {
		return errors.New("config cannot be read")
	}
	aliases := make(map[string]bool)
	for _, definition := range collection.Definitions {
		if definition.Type == "ssh" {
			aliases[definition.HostAlias] = true
		}
	}
	current, err := s.connectionOptions.Load(aliases)
	if err != nil {
		return err
	}
	options := current.Options
	currentOption := options[alias]
	if tmux != nil {
		if !tmux.Enabled {
			currentOption.Enabled = false
			currentOption.SessionName = ""
		} else {
			currentOption.Enabled = true
			currentOption.SessionName = tmux.SessionName
		}
	}
	if filesystem != nil {
		currentOption.Pwd = filesystem.Pwd
	}
	if !currentOption.Enabled && currentOption.Pwd == "" {
		delete(options, alias)
	} else {
		if currentOption.Pwd == "" {
			currentOption.Pwd = connectionoptions.DefaultPwd
		}
		options[alias] = currentOption
	}
	return s.connectionOptions.Save(options)
}

func (s *Server) saveTmuxOption(alias string, value *sshconfig.TmuxOptions) error {
	if value == nil {
		if s.connectionOptions == nil {
			return nil
		}
		repo, keys := s.sources()
		if repo == nil {
			return sshfs.ErrUnavailable
		}
		collection, err := repo.Collection(keys)
		if err != nil {
			return err
		}
		aliases := make(map[string]bool)
		for _, definition := range collection.Definitions {
			if definition.Type == "ssh" {
				aliases[definition.HostAlias] = true
			}
		}
		current, err := s.connectionOptions.Load(aliases)
		if err != nil {
			return err
		}
		delete(current.Options, alias)
		return s.connectionOptions.Save(current.Options)
	}
	return s.saveConnectionOptions(alias, value, nil)
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
	case errors.Is(err, connectionoptions.ErrOptionsSymlink), errors.Is(err, connectionoptions.ErrOptionsNotWritable):
		writeError(w, http.StatusConflict, err.Error(), "connection_options")
	case errors.Is(err, connectionoptions.ErrInvalidFormat), errors.Is(err, connectionoptions.ErrInvalidSessionName), errors.Is(err, connectionoptions.ErrInvalidPwd):
		writeError(w, http.StatusBadRequest, err.Error(), "connection_options")
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
	id := r.PathValue("keyId")
	value, err := s.sshKeys.Public(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "public key not found")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"publicKey": value})
}

func (s *Server) deleteSSHKey(w http.ResponseWriter, r *http.Request, _ string) {
	if s.sshKeys == nil {
		writeError(w, http.StatusServiceUnavailable, "ssh directory unavailable", "ssh_keys")
		return
	}
	id := r.PathValue("keyId")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid key id")
		return
	}
	if err := s.sshKeys.Delete(id); err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			writeError(w, http.StatusNotFound, "key not found")
		case errors.Is(err, sshfs.ErrNotWritable), errors.Is(err, sshfs.ErrUnavailable):
			writeError(w, http.StatusConflict, err.Error(), "ssh_keys")
		default:
			writeError(w, http.StatusBadRequest, err.Error(), "ssh_keys")
		}
		return
	}
	writeSuccess(w)
}
