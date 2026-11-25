package definition

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
	"github.com/ben-wangz/roaminal/backend/internal/sshkey"
)

func (s *Service) Keys() []sshkey.Key {
	if s == nil || s.keys == nil {
		return []sshkey.Key{}
	}
	return s.keys.List()
}

func (s *Service) PublicKey(id string) (string, error) {
	if s == nil || s.keys == nil {
		return "", os.ErrNotExist
	}
	return s.keys.Public(id)
}

func (s *Service) DeleteKey(id string) error {
	if s == nil || s.keys == nil {
		return sshfs.ErrUnavailable
	}
	return s.keys.Delete(id)
}

func (s *Service) knownKeys() map[string]bool {
	if s == nil || s.keys == nil {
		return map[string]bool{}
	}
	result := make(map[string]bool)
	for _, key := range s.keys.List() {
		result[key.FileName] = true
	}
	return result
}

func (s *Service) enrich(collection *sshconfig.Collection) (map[string]connectionoptions.Tmux, error) {
	if s.options == nil {
		collection.TmuxOptionsSource = connectionoptions.Source{Status: "unavailable", Reason: "options store unavailable"}
		return nil, connectionoptions.ErrOptionsUnavailable
	}
	// A mutation may use options for an alias not currently present in the
	// native file; the collection and its ETag expose only visible options.
	loaded, err := s.options.Load(nil)
	collection.TmuxOptionsSource = loaded.Source
	if err != nil {
		if collection.TmuxOptionsSource.Status == "" {
			collection.TmuxOptionsSource.Status = "unavailable"
		}
		if collection.TmuxOptionsSource.Reason == "" {
			collection.TmuxOptionsSource.Reason = connectionoptions.ErrOptionsUnavailable.Error()
		}
		return nil, err
	}
	visible := make(map[string]connectionoptions.Tmux)
	for index := range collection.Definitions {
		definition := &collection.Definitions[index]
		if definition.Type != "ssh" {
			continue
		}
		pwd := connectionoptions.DefaultPwd
		if option, ok := loaded.Options[definition.HostAlias]; ok {
			visible[definition.HostAlias] = option
			if option.Enabled {
				definition.Tmux = &sshconfig.TmuxOptions{Enabled: true, SessionName: option.SessionName}
			}
			pwd = option.Pwd
		}
		definition.FileSystem = &sshconfig.FileSystemOptions{Pwd: pwd}
	}
	if s.mutations != nil {
		// JSON orders the alias keys; the native tag is opaque and fixed-width.
		data, _ := json.Marshal(visible)
		digest := sha256.Sum256(append([]byte(collection.ETag), data...))
		collection.ETag = `"` + hex.EncodeToString(digest[:]) + `"`
	}
	return loaded.Options, nil
}
