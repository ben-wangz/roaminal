package connectionoptions

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func (s *Store) loadLocked(aliases map[string]bool) (Collection, error) {
	result := Collection{Options: map[string]Tmux{}}
	decoded, info, source, err := s.decodeLocked()
	result.Source = source
	if err != nil || source.Status == "missing" {
		return result, err
	}
	for alias, settings := range decoded.Connections {
		option, present, optionErr := optionFromSettings(alias, settings)
		if optionErr != nil {
			result.Source = Source{Status: "invalid", Reason: optionErr.Error()}
			return result, optionErr
		}
		if !present || (aliases != nil && !aliases[alias]) {
			continue
		}
		result.Options[alias] = option
	}
	if writable, reason := s.canWrite(info); writable {
		result.Source.Writable = true
	} else {
		result.Source.Reason = reason
	}
	return result, nil
}

func (s *Store) decodeLocked() (file, os.FileInfo, Source, error) {
	info, err := os.Lstat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return file{}, nil, Source{Status: "missing"}, nil
	}
	if err != nil {
		return file{}, nil, Source{Status: "unavailable", Reason: "options file cannot be inspected"}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return file{}, info, Source{Status: "unsafe", Reason: ErrOptionsSymlink.Error()}, ErrOptionsSymlink
	}
	if !info.Mode().IsRegular() {
		return file{}, info, Source{Status: "invalid", Reason: "options path is not a regular file"}, ErrInvalidFormat
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return file{}, info, Source{Status: "unreadable", Reason: "options file cannot be read"}, err
	}
	if len(data) > MaxBytes {
		return file{}, info, Source{Status: "invalid", Reason: "options file exceeds size limit"}, ErrInvalidFormat
	}
	var decoded file
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&decoded); err != nil {
		return file{}, info, Source{Status: "invalid", Reason: "options file is not valid YAML"}, fmt.Errorf("%w: %v", ErrInvalidFormat, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil || (!errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "EOF")) {
		return file{}, info, Source{Status: "invalid", Reason: "options file contains multiple documents"}, ErrInvalidFormat
	}
	if (decoded.FormatVersion != 1 && decoded.FormatVersion != FormatVersion) || decoded.Connections == nil {
		return file{}, info, Source{Status: "invalid", Reason: "unsupported options format"}, ErrInvalidFormat
	}
	for alias, settings := range decoded.Connections {
		if _, _, err := optionFromSettings(alias, settings); err != nil {
			return file{}, info, Source{Status: "invalid", Reason: err.Error()}, err
		}
	}
	source := Source{Status: "available", Readable: true}
	if writable, reason := s.canWrite(info); writable {
		source.Writable = true
	} else {
		source.Reason = reason
	}
	return decoded, info, source, nil
}

func optionFromSettings(alias string, settings connectionSettings) (Tmux, bool, error) {
	if alias == "" {
		return Tmux{}, false, ErrInvalidFormat
	}
	enabled := settings.Tmux != nil && settings.Tmux.Enabled
	if settings.Tmux != nil && enabled && !ValidSessionName(settings.Tmux.SessionName) {
		return Tmux{}, false, ErrInvalidSessionName
	}
	pwd := DefaultPwd
	if settings.FileSystem != nil && settings.FileSystem.Pwd != "" {
		pwd = settings.FileSystem.Pwd
	}
	if !ValidPwd(pwd) {
		return Tmux{}, false, ErrInvalidPwd
	}
	if !enabled && settings.FileSystem == nil {
		return Tmux{}, false, nil
	}
	option := Tmux{Pwd: pwd}
	if settings.Tmux != nil {
		option.Enabled = settings.Tmux.Enabled
		option.SessionName = settings.Tmux.SessionName
	}
	return option, true, nil
}
