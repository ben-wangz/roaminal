package connectionoptions

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const (
	FormatVersion = 2
	MaxBytes      = 64 << 10
	DefaultPwd    = "$HOME"
)

var sessionNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

var (
	ErrInvalidFormat      = errors.New("invalid SSH connection options format")
	ErrInvalidSessionName = errors.New("invalid tmux session name")
	ErrInvalidPwd         = errors.New("invalid FileSystem pwd")
	ErrOptionsNotWritable = errors.New("SSH connection options are not writable")
	ErrOptionsSymlink     = errors.New("SSH connection options must not be a symlink")
)

type Tmux struct {
	Enabled     bool   `json:"enabled" yaml:"enabled"`
	SessionName string `json:"sessionName" yaml:"sessionName"`
	Pwd         string `json:"pwd" yaml:"pwd"`
}

type Source struct {
	Status   string   `json:"status"`
	Readable bool     `json:"readable"`
	Writable bool     `json:"writable"`
	Reason   string   `json:"reason,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

type Collection struct {
	Options map[string]Tmux `json:"options"`
	Source  Source          `json:"source"`
}

type Snapshot struct {
	Data   []byte
	Exists bool
}

type Store struct {
	path string
	mu   sync.Mutex
}

type file struct {
	FormatVersion int                           `yaml:"formatVersion"`
	Connections   map[string]connectionSettings `yaml:"connections"`
}

type connectionSettings struct {
	Tmux       *tmuxSettings       `yaml:"tmux"`
	FileSystem *filesystemSettings `yaml:"filesystem"`
}

type tmuxSettings struct {
	Enabled     bool   `yaml:"enabled"`
	SessionName string `yaml:"sessionName"`
}

type filesystemSettings struct {
	Pwd string `yaml:"pwd"`
}

func New(stateDir string) *Store {
	return &Store{path: filepath.Join(stateDir, "ssh-connection-options.yaml")}
}

func (s *Store) Path() string { return s.path }

// Snapshot captures the raw options file for coordinated definition writes.
// It is intentionally separate from Load because recovery must preserve even
// an invalid file byte-for-byte.
func (s *Store) Snapshot() (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info, err := os.Lstat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, nil
	}
	if err != nil {
		return Snapshot{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Snapshot{}, ErrOptionsSymlink
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return Snapshot{}, err
	}
	if len(data) > MaxBytes {
		return Snapshot{}, ErrInvalidFormat
	}
	return Snapshot{Data: append([]byte(nil), data...), Exists: true}, nil
}

// Restore re-establishes a pre-mutation options snapshot after the paired
// SSH config write fails. The operation is atomic and retains symlink checks.
func (s *Store) Restore(snapshot Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !snapshot.Exists {
		if info, err := os.Lstat(s.path); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return ErrOptionsSymlink
			}
			return os.Remove(s.path)
		} else if errors.Is(err, os.ErrNotExist) {
			return nil
		} else {
			return err
		}
	}
	return atomicWrite(s.path, snapshot.Data)
}

func ValidSessionName(name string) bool { return sessionNamePattern.MatchString(name) }

func ValidPwd(value string) bool {
	if value == "" || len(value) > 4096 || strings.ContainsAny(value, "\x00\r\n") {
		return false
	}
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return false
		}
	}
	if value == "$HOME" || value == "~" || strings.HasPrefix(value, "$HOME/") || strings.HasPrefix(value, "~/") {
		return true
	}
	return strings.HasPrefix(value, "/")
}

// Load reads the optional add-on without mutating it. When aliases is
// provided, the returned view is limited to those aliases, but stale entries
// remain on disk until an explicit delete or rename operation removes them.
func (s *Store) Load(aliases map[string]bool) (Collection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked(aliases)
}

// UpdateAlias changes one intentional definition-owned entry without
// reconstructing or filtering unrelated connections.
func (s *Store) UpdateAlias(alias string, value Tmux, present bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	decoded, _, source, err := s.decodeLocked()
	if err != nil {
		return err
	}
	if source.Status == "missing" {
		decoded = file{FormatVersion: FormatVersion, Connections: map[string]connectionSettings{}}
	}
	if !present {
		delete(decoded.Connections, alias)
		return s.saveFileLocked(decoded)
	}
	if alias == "" || (value.Enabled && !ValidSessionName(value.SessionName)) {
		return ErrInvalidSessionName
	}
	if value.Pwd == "" {
		value.Pwd = DefaultPwd
	}
	if !ValidPwd(value.Pwd) {
		return ErrInvalidPwd
	}
	settings := connectionSettings{FileSystem: &filesystemSettings{Pwd: value.Pwd}}
	if value.Enabled {
		settings.Tmux = &tmuxSettings{Enabled: true, SessionName: value.SessionName}
	}
	decoded.Connections[alias] = settings
	return s.saveFileLocked(decoded)
}

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

// RemoveAlias is the only options-file operation used for intentional
// connection-definition deletion. It is separate from Load so a partial SSH
// config read can never erase unrelated settings as a side effect.
func (s *Store) RemoveAlias(alias string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	decoded, _, source, err := s.decodeLocked()
	if err != nil {
		return err
	}
	if source.Status == "missing" {
		return nil
	}
	delete(decoded.Connections, alias)
	return s.saveFileLocked(decoded)
}

// MoveAlias preserves settings across an intentional Host alias rename.
func (s *Store) MoveAlias(oldAlias, newAlias string) error {
	if oldAlias == newAlias {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	decoded, _, source, err := s.decodeLocked()
	if err != nil {
		return err
	}
	if source.Status == "missing" {
		return nil
	}
	if value, ok := decoded.Connections[oldAlias]; ok {
		decoded.Connections[newAlias] = value
		delete(decoded.Connections, oldAlias)
	}
	return s.saveFileLocked(decoded)
}

// CopyAlias explicitly duplicates settings for a duplicated connection
// definition. It never relies on alias filtering or implicit reconciliation.
func (s *Store) CopyAlias(oldAlias, newAlias string) error {
	if oldAlias == newAlias {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	decoded, _, source, err := s.decodeLocked()
	if err != nil {
		return err
	}
	if source.Status == "missing" {
		return nil
	}
	if value, ok := decoded.Connections[oldAlias]; ok {
		decoded.Connections[newAlias] = value
	}
	return s.saveFileLocked(decoded)
}
