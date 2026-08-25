package definition

import (
	"errors"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
)

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
	current, err := s.options.Load(nil)
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
	present := value.Enabled || value.Pwd != ""
	return s.options.UpdateAlias(alias, value, present)
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
	return s.options.RemoveAlias(alias)
}
