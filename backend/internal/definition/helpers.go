package definition

import (
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

func (s *Service) enrich(collection *sshconfig.Collection) {
	if s.options == nil {
		collection.TmuxOptionsSource = connectionoptions.Source{Status: "unavailable", Reason: "options store unavailable"}
		return
	}
	if !collection.ConfigSource.Readable && collection.ConfigSource.Status != "missing" {
		loaded, loadErr := s.options.Load(nil)
		collection.TmuxOptionsSource = loaded.Source
		if loadErr != nil && collection.TmuxOptionsSource.Reason == "" {
			collection.TmuxOptionsSource.Reason = loadErr.Error()
		}
		return
	}
	loaded, loadErr := s.options.Load(sshAliases(*collection))
	collection.TmuxOptionsSource = loaded.Source
	if loadErr != nil {
		if collection.TmuxOptionsSource.Reason == "" {
			collection.TmuxOptionsSource.Reason = loadErr.Error()
		}
		return
	}
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

func sshAliases(collection sshconfig.Collection) map[string]bool {
	return sshAliasesFromDefinitions(collection.Definitions)
}

func sshAliasesFromRepo(repo *sshconfig.Repository, keys map[string]bool) map[string]bool {
	collection, err := repo.Collection(keys)
	if err != nil {
		return map[string]bool{}
	}
	return sshAliases(collection)
}

func sshAliasesFromDefinitions(definitions []sshconfig.Definition) map[string]bool {
	aliases := make(map[string]bool)
	for _, item := range definitions {
		if item.Type == "ssh" {
			aliases[item.HostAlias] = true
		}
	}
	return aliases
}
