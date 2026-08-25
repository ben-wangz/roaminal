package definition

import (
	"errors"
	"os"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
	"github.com/ben-wangz/roaminal/backend/internal/sshkey"
)

// Service owns connection-definition source precedence, SSH key references,
// and the persisted tmux/FileSystem options associated with a definition.
// HTTP handlers receive complete collections but do not access these adapters.
type Service struct {
	configRepo *sshconfig.Repository
	keys       *sshkey.Inventory
	options    *connectionoptions.Store
}

func New(configRepo *sshconfig.Repository, keys *sshkey.Inventory, options *connectionoptions.Store) *Service {
	return &Service{configRepo: configRepo, keys: keys, options: options}
}

func (s *Service) Available() bool { return s != nil && s.configRepo != nil }

func (s *Service) Collection() (sshconfig.Collection, error) {
	if !s.Available() {
		return sshconfig.Collection{}, sshfs.ErrUnavailable
	}
	collection, err := s.configRepo.Collection(s.knownKeys())
	if err != nil {
		return sshconfig.Collection{}, err
	}
	s.enrich(&collection)
	return collection, nil
}

func (s *Service) Create(ifMatch string, edit sshconfig.Edit, tmux *sshconfig.TmuxOptions, filesystem *sshconfig.FileSystemOptions) (sshconfig.Collection, error) {
	if !s.Available() {
		return sshconfig.Collection{}, sshfs.ErrUnavailable
	}
	collection, err := s.configRepo.Create(ifMatch, s.knownKeys(), edit)
	if err != nil {
		return sshconfig.Collection{}, err
	}
	if tmux != nil || filesystem != nil {
		if err := s.SaveOptions(edit.HostAlias, tmux, filesystem); err != nil {
			return sshconfig.Collection{}, err
		}
	}
	s.enrich(&collection)
	return collection, nil
}

func (s *Service) Update(ifMatch, alias string, edit sshconfig.Edit, tmux *sshconfig.TmuxOptions, filesystem *sshconfig.FileSystemOptions) (sshconfig.Collection, error) {
	if !s.Available() {
		return sshconfig.Collection{}, sshfs.ErrUnavailable
	}
	collection, err := s.configRepo.Update(ifMatch, s.knownKeys(), alias, edit)
	if err != nil {
		return sshconfig.Collection{}, err
	}
	if tmux != nil || filesystem != nil {
		if err := s.SaveOptions(alias, tmux, filesystem); err != nil {
			return sshconfig.Collection{}, err
		}
	}
	s.enrich(&collection)
	return collection, nil
}

func (s *Service) Duplicate(ifMatch, alias, newAlias string) (sshconfig.Collection, error) {
	if !s.Available() {
		return sshconfig.Collection{}, sshfs.ErrUnavailable
	}
	collection, err := s.configRepo.Duplicate(ifMatch, s.knownKeys(), alias, newAlias)
	if err != nil {
		return sshconfig.Collection{}, err
	}
	s.enrich(&collection)
	return collection, nil
}

func (s *Service) Delete(ifMatch, alias string) (sshconfig.Collection, error) {
	if !s.Available() {
		return sshconfig.Collection{}, sshfs.ErrUnavailable
	}
	collection, err := s.configRepo.Delete(ifMatch, s.knownKeys(), alias)
	if err != nil {
		return sshconfig.Collection{}, err
	}
	if err := s.SaveTmuxOption(alias, nil); err != nil {
		return sshconfig.Collection{}, err
	}
	s.enrich(&collection)
	return collection, nil
}

func (s *Service) SaveOptions(alias string, tmux *sshconfig.TmuxOptions, filesystem *sshconfig.FileSystemOptions) error {
	if s.options == nil {
		return nil
	}
	if !s.Available() {
		return sshfs.ErrUnavailable
	}
	collection, err := s.configRepo.Collection(s.knownKeys())
	if err != nil {
		return err
	}
	if !collection.ConfigSource.Readable && collection.ConfigSource.Status != "missing" {
		return errors.New("config cannot be read")
	}
	aliases := sshAliases(collection)
	current, err := s.options.Load(aliases)
	if err != nil {
		return err
	}
	options := current.Options
	value := options[alias]
	if tmux != nil {
		if !tmux.Enabled {
			value.Enabled = false
			value.SessionName = ""
		} else {
			value.Enabled = true
			value.SessionName = tmux.SessionName
		}
	}
	if filesystem != nil {
		value.Pwd = filesystem.Pwd
	}
	if !value.Enabled && value.Pwd == "" {
		delete(options, alias)
	} else {
		if value.Pwd == "" {
			value.Pwd = connectionoptions.DefaultPwd
		}
		options[alias] = value
	}
	return s.options.Save(options)
}

func (s *Service) SaveTmuxOption(alias string, value *sshconfig.TmuxOptions) error {
	if value != nil {
		return s.SaveOptions(alias, value, nil)
	}
	if s.options == nil {
		return nil
	}
	if !s.Available() {
		return sshfs.ErrUnavailable
	}
	current, err := s.options.Load(sshAliasesFromRepo(s.configRepo, s.knownKeys()))
	if err != nil {
		return err
	}
	delete(current.Options, alias)
	return s.options.Save(current.Options)
}

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
		loaded, _ := s.options.Load(nil)
		collection.TmuxOptionsSource = loaded.Source
		return
	}
	loaded, _ := s.options.Load(sshAliases(*collection))
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
