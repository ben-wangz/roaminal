package persistence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

// LegacyData is an offline representation with no paths that a live upload can
// use. Snapshot references are checked again when their bytes are imported.
type LegacyData struct {
	Fingerprint   string
	Enrollment    *domain.TOTPEnrollmentRecord
	Auth          []domain.AuthSessionRecord
	Layouts       map[string]domain.ConnectionInstanceLayout
	MessageState  domain.MessageState
	Messages      []domain.MessageRecord
	Subscriptions []domain.PushSubscriptionRecord
	Preferences   []domain.NotificationPreference
	Options       map[string]connectionoptions.Tmux
	Endpoints     map[string]domain.AgentEndpointRecord
	Instances     []LegacyInstance
	Uploads       []domain.UploadJobRecord
}

type legacyReader struct {
	stateDir string
	finger   hash.Hash
}

const legacyJSONMax = 64 << 20

// ReadLegacy validates a stopped, coherent file-store copy without opening it
// for writes. sshConfig identifies the matching native config if a prepared
// legacy definition journal needs recovery.
func ReadLegacy(loc LegacyLocation, sshConfig string) (LegacyData, error) {
	if !loc.Present {
		return LegacyData{}, errors.New("no legacy application state found")
	}
	if err := loc.checkRecovery(); err != nil {
		return LegacyData{}, err
	}
	r := legacyReader{stateDir: loc.StateDir, finger: sha256.New()}
	data := LegacyData{Layouts: make(map[string]domain.ConnectionInstanceLayout)}
	if err := r.readAuth(loc.Root, &data); err != nil {
		return LegacyData{}, err
	}
	if err := r.readRecords(loc.Root, &data); err != nil {
		return LegacyData{}, err
	}
	options, err := r.readDefinitionOptions(sshConfig)
	if err != nil {
		return LegacyData{}, err
	}
	data.Options = options
	if err := r.readInstances(loc.Root, &data); err != nil {
		return LegacyData{}, err
	}
	if err := r.readUploads(loc.Root, &data); err != nil {
		return LegacyData{}, err
	}
	data.Fingerprint = hex.EncodeToString(r.finger.Sum(nil))
	return data, nil
}

func (r legacyReader) read(path string, limit int64, private bool) ([]byte, error) {
	data, err := readLegacyFile(path, limit, private)
	if err != nil {
		return nil, err
	}
	name, err := filepath.Rel(r.stateDir, path)
	if err != nil {
		return nil, err
	}
	// The relative pathname and fixed-size digest make the fingerprint
	// independent of the copy's path, mtimes, and concatenation boundaries.
	_, _ = fmt.Fprintf(r.finger, "%q:%x\n", name, sha256.Sum256(data))
	return data, nil
}

func readLegacyFile(path string, limit int64, private bool) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit || (private && info.Mode().Perm()&0o077 != 0) {
		return nil, errors.New("legacy data is not a safe regular file or exceeds its size limit")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	current, err := file.Stat()
	if err != nil || !os.SameFile(info, current) {
		return nil, errors.New("legacy source changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) != info.Size() || int64(len(data)) > limit {
		return nil, errors.New("legacy file could not be read at its reported size")
	}
	return data, nil
}

func (r legacyReader) readJSON(dir, name string, value any, limit int64, private bool, validate func() error) error {
	data, err := r.read(filepath.Join(dir, name), limit, private)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || decodeStrict(data, value) != nil || validate() != nil {
		return fmt.Errorf("invalid legacy %s", name)
	}
	return nil
}

func (r legacyReader) readAuth(root string, result *LegacyData) error {
	var enrollment domain.TOTPEnrollmentRecord
	if err := r.readJSON(root, totpSecretFile, &enrollment, maxTOTPEnrollmentFileSize, true, func() error { return validateTOTPEnrollment(enrollment) }); err != nil {
		return err
	}
	if enrollment.EnrollmentID != "" {
		result.Enrollment = &enrollment
	}
	data, err := r.read(filepath.Join(root, "auth-sessions.json"), legacyJSONMax, true)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("unreadable legacy auth-sessions.json")
	}
	if err == nil && decodeLegacyAuth(data, result) != nil {
		return errors.New("invalid legacy auth-sessions.json")
	}
	var layouts workspaceLayoutFile
	return r.readJSON(root, "workspace-layouts.json", &layouts, legacyJSONMax, false, func() error {
		if layouts.FormatVersion != StorageSchemaVersion {
			return errors.New("unsupported workspace schema")
		}
		for _, record := range layouts.Layouts {
			if err := addLegacyLayout(result, record.AuthenticationSessionID, record.Layout); err != nil {
				return err
			}
		}
		return nil
	})
}

func decodeLegacyAuth(data []byte, result *LegacyData) error {
	var version struct {
		FormatVersion int `json:"formatVersion"`
	}
	if err := json.Unmarshal(data, &version); err != nil {
		return err
	}
	switch version.FormatVersion {
	case StorageSchemaVersion:
		var file AuthFile
		if err := decodeStrict(data, &file); err != nil {
			return err
		}
		seen := make(map[string]bool, len(file.Sessions))
		for _, record := range file.Sessions {
			if validateAuthSession(record) != nil || seen[record.ID] {
				return errors.New("invalid authentication record")
			}
			seen[record.ID] = true
			if result.Enrollment == nil || record.EnrollmentID != result.Enrollment.EnrollmentID {
				continue // A revoked or pre-TOTP binding cannot be restored.
			}
			result.Auth = append(result.Auth, domain.AuthSessionRecord(record))
		}
	case LegacyConnectionFormatVersion:
		var file legacyAuthFile
		if err := decodeStrict(data, &file); err != nil {
			return err
		}
		seen := make(map[string]bool, len(file.Sessions))
		for _, record := range file.Sessions {
			if validateAuthSession(record.unboundRecord()) != nil || seen[record.ID] {
				return errors.New("invalid legacy authentication record")
			}
			seen[record.ID] = true
			layout, exists, err := migrateLegacyLayout(record.ConnectionInstanceLayout, record.ConnectionInstanceOrder)
			if err != nil {
				return err
			}
			if exists {
				if err := addLegacyLayout(result, record.ID, layout); err != nil {
					return err
				}
			}
		}
	default:
		return errors.New("unsupported authentication schema")
	}
	return nil
}

func addLegacyLayout(result *LegacyData, id string, layout domain.ConnectionInstanceLayout) error {
	if _, exists := result.Layouts[id]; exists || !uuidPattern.MatchString(id) || domain.ValidateConnectionInstanceLayout(&layout) != nil {
		return errors.New("invalid or duplicate legacy workspace layout")
	}
	result.Layouts[id] = layout
	return nil
}
