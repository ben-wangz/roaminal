package connectionoptions

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
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
