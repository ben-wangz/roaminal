package persistence

import (
	"errors"
	"unicode/utf8"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

func validateAuthSession(session AuthSession) error {
	if !uuidPattern.MatchString(session.ID) || !hex64Pattern.MatchString(session.PasswordFingerprint) || !hex64Pattern.MatchString(session.RefreshTokenHash) {
		return errors.New("invalid auth session identity or hash")
	}
	if session.CreatedAt.IsZero() || session.LastSeenAt.IsZero() || session.RefreshExpiresAt.IsZero() || session.RotatedAt.IsZero() {
		return errors.New("invalid auth session timestamp")
	}
	if !session.RefreshExpiresAt.After(session.CreatedAt) || session.LastSeenAt.Before(session.CreatedAt) || session.RotatedAt.Before(session.CreatedAt) {
		return errors.New("invalid auth session lifetime")
	}
	if !utf8.ValidString(session.UserAgent) || len([]byte(session.UserAgent)) > 500 {
		return errors.New("invalid auth user agent")
	}
	return nil
}

// ValidateConnectionInstanceOrder bounds the per-login-session sidebar layout.
func ValidateConnectionInstanceOrder(order []string) error {
	return domain.ValidateConnectionInstanceOrder(order)
}

// ValidateConnectionInstanceLayout bounds and validates a per-login-session
// grouped sidebar layout. A nil layout is accepted for older auth files.
func ValidateConnectionInstanceLayout(layout *ConnectionInstanceLayout) error {
	return domain.ValidateConnectionInstanceLayout((*domain.ConnectionInstanceLayout)(layout))
}

func validateConnectionInstanceMeta(meta ConnectionInstanceMeta) error {
	if meta.FormatVersion != 0 && meta.FormatVersion != ConnectionFormatVersion {
		return errors.New("unsupported connection instance format version")
	}
	return domain.ValidateConnectionInstanceMeta(meta)
}

func ValidateTitleOverride(value string) error {
	return domain.ValidateTitleOverride(value)
}

func validateSnapshotHeader(header SnapshotHeader) error {
	return domain.ValidateSnapshotHeader(header)
}
