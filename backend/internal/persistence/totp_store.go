package persistence

import (
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

// totpSecretFile is credential material inside the resolved store root. It is
// deliberately separate from auth-sessions.json so enrollment can be reset by
// deleting one private file without touching business data.
const totpSecretFile = "2fa-secret"

const maxTOTPEnrollmentFileSize = 64 * 1024

var totpBase32 = base32.StdEncoding.WithPadding(base32.NoPadding)

func totpSecretPath(s *Store) string { return filepath.Join(s.Root, totpSecretFile) }

func validateTOTPEnrollment(record domain.TOTPEnrollmentRecord) error {
	if record.FormatVersion != domain.TOTPFormatVersion {
		return fmt.Errorf("unsupported TOTP enrollment format version %d", record.FormatVersion)
	}
	if !uuidPattern.MatchString(record.EnrollmentID) {
		return errors.New("invalid TOTP enrollment identity")
	}
	secret, err := totpBase32.DecodeString(record.Secret)
	if err != nil {
		return errors.New("invalid TOTP secret encoding")
	}
	if len(secret) < 10 || len(secret) > 128 {
		return errors.New("invalid TOTP secret length")
	}
	if record.LastAcceptedTimeStep < 0 {
		return errors.New("invalid TOTP time step")
	}
	return nil
}

// LoadTOTPEnrollment reads the enrollment file. exists is false only when the
// file is absent; unreadable, unsafe, or malformed files return an error so
// callers fail closed instead of treating the state as unconfigured.
func (s *Store) LoadTOTPEnrollment() (domain.TOTPEnrollmentRecord, bool, error) {
	path := totpSecretPath(s)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return domain.TOTPEnrollmentRecord{}, false, nil
	}
	if err != nil {
		return domain.TOTPEnrollmentRecord{}, true, fmt.Errorf("inspect TOTP enrollment file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return domain.TOTPEnrollmentRecord{}, true, fmt.Errorf("TOTP enrollment file %s is not a regular file", path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return domain.TOTPEnrollmentRecord{}, true, fmt.Errorf("TOTP enrollment file %s has unsafe permissions", path)
	}
	if info.Size() > maxTOTPEnrollmentFileSize {
		return domain.TOTPEnrollmentRecord{}, true, fmt.Errorf("TOTP enrollment file %s is too large", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return domain.TOTPEnrollmentRecord{}, true, fmt.Errorf("read TOTP enrollment file: %w", err)
	}
	var file domain.TOTPEnrollmentRecord
	if err := decodeStrict(data, &file); err != nil {
		return domain.TOTPEnrollmentRecord{}, true, fmt.Errorf("decode TOTP enrollment file %s: %v; repair or remove the file", path, err)
	}
	if err := validateTOTPEnrollment(file); err != nil {
		return domain.TOTPEnrollmentRecord{}, true, fmt.Errorf("invalid TOTP enrollment file %s: %v; repair or remove the file", path, err)
	}
	return file, true, nil
}

// SaveTOTPEnrollment durably replaces the enrollment file with private
// permissions. Validation failures never reach the filesystem.
func (s *Store) SaveTOTPEnrollment(record domain.TOTPEnrollmentRecord) error {
	if err := validateTOTPEnrollment(record); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return s.atomicWrite(totpSecretPath(s), append(data, '\n'))
}

func (s *Store) totpEnrollmentExists() (bool, error) {
	_, err := os.Lstat(totpSecretPath(s))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
