package persistence

import (
	"errors"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

const (
	// StorageSchemaVersion is the on-disk schema owned by the 0.3 runtime.
	// Older files are accepted only by the startup migration path.
	StorageSchemaVersion            = 3
	FormatVersion                   = StorageSchemaVersion
	ConnectionFormatVersion         = StorageSchemaVersion
	LegacyConnectionFormatVersion   = 1
	PreviousConnectionFormatVersion = 2
	SnapshotMagic                   = "ROAMINAL-SNAPSHOT/1"
	SnapshotMaxSize                 = 256 * 1024 * 1024
)

var ErrNotFound = os.ErrNotExist
var errWorldPermissions = errors.New("directory has world permissions")
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var hex64Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var sequencePattern = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

const UngroupedConnectionInstanceGroupID = "ungrouped"

type ConnectionInstanceGroup = domain.ConnectionInstanceGroup
type ConnectionInstanceLayout = domain.ConnectionInstanceLayout
type ConnectionInstanceMeta = domain.ConnectionInstanceMeta

// AuthSession is the current storage representation of an authentication
// record. Workspace layout is deliberately stored in another file.
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

type SnapshotHeader = domain.SnapshotHeader

type Store struct {
	Root                      string
	ConnectionsDir            string
	AuditDir                  string
	DiagnosticsDir            string
	Layout                    Layout
	degradedMu                sync.RWMutex
	workspaceMu               sync.Mutex
	messagesMu                sync.Mutex
	pushMu                    sync.Mutex
	notificationPreferencesMu sync.Mutex
	degradedIDs               map[string]struct{}
	globalError               bool
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
