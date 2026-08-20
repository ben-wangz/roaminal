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

// Load reads the optional add-on without ever replacing an invalid file. The
// caller supplies the successfully parsed SSH aliases so stale entries can be
// removed only after the primary SSH config is known to be readable.
func (s *Store) Load(aliases map[string]bool) (Collection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := Collection{Options: map[string]Tmux{}}
	info, err := os.Lstat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		result.Source = Source{Status: "missing"}
		return result, nil
	}
	if err != nil {
		result.Source = Source{Status: "unavailable", Reason: "options file cannot be inspected"}
		return result, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		result.Source = Source{Status: "unsafe", Reason: ErrOptionsSymlink.Error()}
		return result, ErrOptionsSymlink
	}
	if !info.Mode().IsRegular() {
		result.Source = Source{Status: "invalid", Reason: "options path is not a regular file"}
		return result, ErrInvalidFormat
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		result.Source = Source{Status: "unreadable", Reason: "options file cannot be read"}
		return result, err
	}
	if len(data) > MaxBytes {
		result.Source = Source{Status: "invalid", Reason: "options file exceeds size limit"}
		return result, ErrInvalidFormat
	}
	var decoded file
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&decoded); err != nil {
		result.Source = Source{Status: "invalid", Reason: "options file is not valid YAML"}
		return result, fmt.Errorf("%w: %v", ErrInvalidFormat, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		result.Source = Source{Status: "invalid", Reason: "options file contains multiple documents"}
		return result, ErrInvalidFormat
	} else if !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "EOF") {
		result.Source = Source{Status: "invalid", Reason: "options file contains multiple documents"}
		return result, ErrInvalidFormat
	}
	if (decoded.FormatVersion != 1 && decoded.FormatVersion != FormatVersion) || decoded.Connections == nil {
		result.Source = Source{Status: "invalid", Reason: "unsupported options format"}
		return result, ErrInvalidFormat
	}
	reconcile := aliases != nil
	for alias, settings := range decoded.Connections {
		if alias == "" {
			result.Source = Source{Status: "invalid", Reason: "invalid connection tmux settings"}
			return result, ErrInvalidFormat
		}
		enabled := settings.Tmux != nil && settings.Tmux.Enabled
		if settings.Tmux != nil && enabled && !ValidSessionName(settings.Tmux.SessionName) {
			result.Source = Source{Status: "invalid", Reason: "invalid connection tmux settings"}
			return result, ErrInvalidFormat
		}
		pwd := DefaultPwd
		if settings.FileSystem != nil && settings.FileSystem.Pwd != "" {
			pwd = settings.FileSystem.Pwd
		}
		if !ValidPwd(pwd) {
			result.Source = Source{Status: "invalid", Reason: "invalid FileSystem pwd"}
			return result, ErrInvalidPwd
		}
		if !enabled && settings.FileSystem == nil {
			continue
		}
		if reconcile && !aliases[alias] {
			continue
		}
		option := Tmux{Pwd: pwd}
		if settings.Tmux != nil {
			option.Enabled = settings.Tmux.Enabled
			option.SessionName = settings.Tmux.SessionName
		}
		result.Options[alias] = option
	}
	result.Source = Source{Status: "available", Readable: true}
	if writable, reason := s.canWrite(info); writable {
		result.Source.Writable = true
	} else {
		result.Source.Reason = reason
	}
	if reconcile && len(result.Options) != len(decoded.Connections) && result.Source.Writable {
		// Reconciliation is best-effort. A readonly projected file remains
		// valid and the in-memory view still ignores aliases absent from SSH.
		_ = s.saveLocked(result.Options)
	}
	return result, nil
}
