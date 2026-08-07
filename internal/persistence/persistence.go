package persistence

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

const (
	FormatVersion        = 1
	SessionFormatVersion = 2
	SnapshotMagic        = "ROAMINAL-SNAPSHOT/1"
	SnapshotMaxSize      = 256 * 1024 * 1024
)

var ErrNotFound = os.ErrNotExist
var errWorldPermissions = errors.New("directory has world permissions")
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var hex64Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var sequencePattern = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

type ExecutionRecord struct {
	Command     string    `json:"command"`
	ExitCode    *int      `json:"exitCode"`
	Input       string    `json:"input"`
	Output      string    `json:"output"`
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
	DurationMs  int64     `json:"durationMs"`
	Truncated   bool      `json:"truncated"`
}

type SessionMeta struct {
	FormatVersion  int               `json:"-"`
	ID             string            `json:"id"`
	Title          string            `json:"-"`
	AutomaticTitle string            `json:"automaticTitle"`
	TitleOverride  *string           `json:"titleOverride"`
	InitialCwd     string            `json:"initialCwd"`
	Cwd            string            `json:"cwd"`
	Cols           int               `json:"cols"`
	Rows           int               `json:"rows"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
	Executions     []ExecutionRecord `json:"executions"`
}

func (meta SessionMeta) EffectiveTitle() string {
	if meta.TitleOverride != nil {
		return *meta.TitleOverride
	}
	if meta.AutomaticTitle != "" {
		return meta.AutomaticTitle
	}
	return meta.Title
}
func (meta *SessionMeta) SyncEffectiveTitle() { meta.Title = meta.EffectiveTitle() }

type AuthSession struct {
	ID                  string    `json:"id"`
	PasswordFingerprint string    `json:"passwordFingerprint"`
	RefreshTokenHash    string    `json:"refreshTokenHash"`
	CreatedAt           time.Time `json:"createdAt"`
	LastSeenAt          time.Time `json:"lastSeenAt"`
	RefreshExpiresAt    time.Time `json:"refreshExpiresAt"`
	RotatedAt           time.Time `json:"rotatedAt"`
	UserAgent           string    `json:"userAgent"`
}
type AuthFile struct {
	FormatVersion int           `json:"formatVersion"`
	Sessions      []AuthSession `json:"sessions"`
}
type SnapshotHeader struct {
	Cols            int    `json:"cols"`
	Rows            int    `json:"rows"`
	ScrollbackLines int    `json:"scrollbackLines"`
	ThroughSequence string `json:"throughSequence"`
	ByteLength      int    `json:"byteLength"`
	SHA256          string `json:"sha256"`
}

type Store struct {
	Root        string
	SessionsDir string
	Layout      Layout
	degradedMu  sync.RWMutex
	degradedIDs map[string]struct{}
	globalError bool
}

var ErrAmbiguousStateLayout = errors.New("ambiguous state layout")

type Layout string

const (
	LayoutDirect       Layout = "direct"
	LayoutPrivateChild Layout = "private-child"
)

func New(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}
	rootPrivateErr := ensurePrivateDirectory(root)
	if rootPrivateErr != nil && !errors.Is(rootPrivateErr, errWorldPermissions) {
		return nil, fmt.Errorf("prepare state directory: %w", rootPrivateErr)
	}
	childRoot := filepath.Join(root, "state")
	directHasData, err := stateRootHasData(root)
	if err != nil {
		return nil, fmt.Errorf("inspect direct state directory: %w", err)
	}
	childHasData, err := stateRootHasData(childRoot)
	if err != nil {
		return nil, fmt.Errorf("inspect private state directory: %w", err)
	}
	stateRoot, layout := root, LayoutDirect
	if rootPrivateErr != nil {
		if directHasData {
			return nil, ErrAmbiguousStateLayout
		}
		stateRoot, layout = childRoot, LayoutPrivateChild
		if err := os.MkdirAll(stateRoot, 0o700); err != nil {
			return nil, fmt.Errorf("create private state directory: %w", err)
		}
		if err := ensurePrivateDirectory(stateRoot); err != nil {
			return nil, fmt.Errorf("prepare private state directory: %w", err)
		}
	} else if directHasData && childHasData {
		return nil, ErrAmbiguousStateLayout
	} else if !directHasData && childHasData {
		stateRoot, layout = childRoot, LayoutPrivateChild
		if err := ensurePrivateDirectory(stateRoot); err != nil {
			return nil, fmt.Errorf("prepare private state directory: %w", err)
		}
	}
	sessionsDir := filepath.Join(stateRoot, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o700); err != nil {
		return nil, fmt.Errorf("create sessions directory: %w", err)
	}
	if err := ensurePrivateDirectory(sessionsDir); err != nil {
		return nil, fmt.Errorf("prepare sessions directory: %w", err)
	}
	return &Store{Root: stateRoot, SessionsDir: sessionsDir, Layout: layout, degradedIDs: make(map[string]struct{})}, nil
}

func stateRootHasData(root string) (bool, error) {
	if info, err := os.Stat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	} else if !info.IsDir() {
		return false, errors.New("state root is not a directory")
	}
	if _, err := os.Stat(filepath.Join(root, "auth-sessions.json")); err == nil {
		return true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	entries, err := os.ReadDir(filepath.Join(root, "sessions"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return len(entries) > 0, nil
}

func ensurePrivateDirectory(path string) error {
	if err := os.Chmod(path, 0o700); err == nil {
		return nil
	} else if !os.IsPermission(err) {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("path is not a directory")
	}
	if info.Mode().Perm()&0o007 != 0 {
		return errWorldPermissions
	}
	probe, err := os.CreateTemp(path, ".roaminal-permission-*")
	if err != nil {
		return fmt.Errorf("directory is not writable: %w", err)
	}
	probeName := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(probeName)
		return fmt.Errorf("close write probe: %w", err)
	}
	if err := os.Remove(probeName); err != nil {
		return fmt.Errorf("remove write probe: %w", err)
	}
	return nil
}

func (s *Store) PersistenceDegraded() bool {
	s.degradedMu.RLock()
	degraded := s.globalError || len(s.degradedIDs) > 0
	s.degradedMu.RUnlock()
	return degraded
}
func (s *Store) markError(err error) error {
	s.degradedMu.Lock()
	s.globalError = true
	s.degradedMu.Unlock()
	return err
}
func (s *Store) markSessionError(id string, err error) error {
	s.degradedMu.Lock()
	s.degradedIDs[id] = struct{}{}
	s.degradedMu.Unlock()
	return err
}
func (s *Store) clearSessionError(id string) {
	s.degradedMu.Lock()
	delete(s.degradedIDs, id)
	s.degradedMu.Unlock()
}
func (s *Store) MarkSessionDegraded(id string) { s.markSessionError(id, nil) }
