package persistence

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sync"
	"time"
)

const (
	FormatVersion                 = 1
	ConnectionFormatVersion       = 2
	LegacyConnectionFormatVersion = 1
	SnapshotMagic                 = "ROAMINAL-SNAPSHOT/1"
	SnapshotMaxSize               = 256 * 1024 * 1024
)

var ErrNotFound = os.ErrNotExist
var errWorldPermissions = errors.New("directory has world permissions")
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var hex64Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var sequencePattern = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

const UngroupedConnectionInstanceGroupID = "ungrouped"

type ConnectionInstanceGroup struct {
	GroupID               string   `json:"groupId"`
	Name                  string   `json:"name"`
	ConnectionInstanceIDs []string `json:"connectionInstanceIds"`
}

type ConnectionInstanceLayout struct {
	Revision                       uint64                    `json:"revision"`
	GroupOrder                     []string                  `json:"groupOrder"`
	Groups                         []ConnectionInstanceGroup `json:"groups"`
	UngroupedConnectionInstanceIDs []string                  `json:"ungroupedConnectionInstanceIds"`
}

type ConnectionInstanceMeta struct {
	FormatVersion                 int       `json:"-"`
	ID                            string    `json:"id"`
	Title                         string    `json:"-"`
	AutomaticTitle                string    `json:"automaticTitle"`
	TitleOverride                 *string   `json:"titleOverride"`
	InitialCwd                    string    `json:"initialCwd"`
	Cwd                           string    `json:"cwd"`
	Cols                          int       `json:"cols"`
	Rows                          int       `json:"rows"`
	CreatedAt                     time.Time `json:"createdAt"`
	UpdatedAt                     time.Time `json:"updatedAt"`
	BackendRuntimeID              string    `json:"backendRuntimeId,omitempty"`
	ConnectionDefinitionID        string    `json:"connectionDefinitionId,omitempty"`
	Type                          string    `json:"type,omitempty"`
	Purpose                       string    `json:"purpose,omitempty"`
	SourceHostAlias               *string   `json:"sourceHostAlias,omitempty"`
	Lifecycle                     string    `json:"lifecycle,omitempty"`
	SourceState                   string    `json:"sourceState,omitempty"`
	ExitCode                      *int      `json:"exitCode,omitempty"`
	ExitSignal                    *string   `json:"exitSignal,omitempty"`
	ReuseFromConnectionInstanceID *string   `json:"reuseFromConnectionInstanceId,omitempty"`
	GenerationStatus              string    `json:"generationStatus,omitempty"`
	GenerationError               string    `json:"generationError,omitempty"`
	TmuxEnabled                   bool      `json:"tmuxEnabled,omitempty"`
	TmuxSessionName               string    `json:"tmuxSessionName,omitempty"`
	TmuxPrefixKey                 string    `json:"tmuxPrefixKey,omitempty"`
	TmuxPrefixSource              string    `json:"tmuxPrefixSource,omitempty"`
}

func (meta ConnectionInstanceMeta) EffectiveTitle() string {
	if meta.TitleOverride != nil {
		return *meta.TitleOverride
	}
	if meta.AutomaticTitle != "" {
		return meta.AutomaticTitle
	}
	return meta.Title
}
func (meta *ConnectionInstanceMeta) SyncEffectiveTitle() { meta.Title = meta.EffectiveTitle() }

type AuthSession struct {
	ID                       string                    `json:"id"`
	PasswordFingerprint      string                    `json:"passwordFingerprint"`
	RefreshTokenHash         string                    `json:"refreshTokenHash"`
	CreatedAt                time.Time                 `json:"createdAt"`
	LastSeenAt               time.Time                 `json:"lastSeenAt"`
	RefreshExpiresAt         time.Time                 `json:"refreshExpiresAt"`
	RotatedAt                time.Time                 `json:"rotatedAt"`
	UserAgent                string                    `json:"userAgent"`
	ConnectionInstanceOrder  []string                  `json:"connectionInstanceOrder,omitempty"`
	ConnectionInstanceLayout *ConnectionInstanceLayout `json:"connectionInstanceLayout,omitempty"`
}
type AuthFile struct {
	FormatVersion int           `json:"formatVersion"`
	Sessions      []AuthSession `json:"sessions"`
}

// NewUUID returns a version 4 identifier for persisted user-owned layout items.
func NewUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:]), nil
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
	Root           string
	ConnectionsDir string
	AuditDir       string
	DiagnosticsDir string
	Layout         Layout
	degradedMu     sync.RWMutex
	degradedIDs    map[string]struct{}
	globalError    bool
}

var ErrAmbiguousStateLayout = errors.New("ambiguous state layout")
var ErrLegacySessions = errors.New("legacy sessions are not compatible with connection instances")

type Layout string

const (
	LayoutDirect       Layout = "direct"
	LayoutPrivateChild Layout = "private-child"
)

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
func (s *Store) markConnectionInstanceError(id string, err error) error {
	s.degradedMu.Lock()
	s.degradedIDs[id] = struct{}{}
	s.degradedMu.Unlock()
	return err
}
func (s *Store) clearConnectionInstanceError(id string) {
	s.degradedMu.Lock()
	delete(s.degradedIDs, id)
	s.degradedMu.Unlock()
}
func (s *Store) MarkConnectionInstanceDegraded(id string) { s.markConnectionInstanceError(id, nil) }
