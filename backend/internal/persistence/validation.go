package persistence

import (
	"errors"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
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

func validateConnectionInstanceMeta(meta ConnectionInstanceMeta) error {
	if meta.FormatVersion != 0 && meta.FormatVersion != ConnectionFormatVersion {
		return errors.New("unsupported connection instance format version")
	}
	if !uuidPattern.MatchString(meta.ID) {
		return errors.New("invalid connection instance id")
	}
	if !utf8.ValidString(meta.EffectiveTitle()) || len([]byte(meta.EffectiveTitle())) > 512 || !utf8.ValidString(meta.AutomaticTitle) || len([]byte(meta.AutomaticTitle)) > 512 || !utf8.ValidString(meta.InitialCwd) || !utf8.ValidString(meta.Cwd) || !utf8.ValidString(meta.GenerationStatus) || len([]byte(meta.GenerationStatus)) > 64 || !utf8.ValidString(meta.GenerationError) || len([]byte(meta.GenerationError)) > 512 {
		return errors.New("invalid connection instance text")
	}
	if meta.TitleOverride != nil {
		if err := ValidateTitleOverride(*meta.TitleOverride); err != nil {
			return err
		}
	}
	if !filepath.IsAbs(meta.InitialCwd) || !filepath.IsAbs(meta.Cwd) || len([]byte(meta.InitialCwd)) > 4096 || len([]byte(meta.Cwd)) > 4096 {
		return errors.New("invalid connection instance cwd")
	}
	if meta.Cols < 2 || meta.Cols > 1000 || meta.Rows < 1 || meta.Rows > 1000 || meta.CreatedAt.IsZero() || meta.UpdatedAt.IsZero() || meta.UpdatedAt.Before(meta.CreatedAt) {
		return errors.New("invalid connection instance dimensions or timestamp")
	}
	if meta.TmuxPrefixKey != "" && (len(meta.TmuxPrefixKey) != 1 || meta.TmuxPrefixKey[0] < 'a' || meta.TmuxPrefixKey[0] > 'z') {
		return errors.New("invalid tmux prefix key")
	}
	if meta.TmuxPrefixSource != "" && meta.TmuxPrefixSource != "runtime" && meta.TmuxPrefixSource != "fallback" && meta.TmuxPrefixSource != "unsupported" {
		return errors.New("invalid tmux prefix source")
	}
	if (meta.TmuxPrefixSource == "runtime" || meta.TmuxPrefixSource == "fallback") && meta.TmuxPrefixKey == "" {
		return errors.New("tmux prefix source requires a key")
	}
	if meta.TmuxPrefixSource == "unsupported" && meta.TmuxPrefixKey != "" {
		return errors.New("unsupported tmux prefix cannot have a key")
	}
	if !meta.TmuxEnabled && (meta.TmuxPrefixKey != "" || meta.TmuxPrefixSource != "") {
		return errors.New("non-tmux connection has tmux prefix metadata")
	}
	return nil
}

func ValidateTitleOverride(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("title must not be empty")
	}
	if !utf8.ValidString(value) || len([]byte(value)) > 512 {
		return errors.New("title exceeds size limit or contains invalid UTF-8")
	}
	runes := []rune(value)
	if len(runes) > 128 {
		return errors.New("title exceeds 128 characters")
	}
	for _, r := range runes {
		if unicode.IsControl(r) || r == 0x7f || (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
			return errors.New("title contains a prohibited control character")
		}
	}
	return nil
}

func validateSnapshotHeader(header SnapshotHeader) error {
	if header.Cols < 2 || header.Cols > 1000 || header.Rows < 1 || header.Rows > 1000 || header.ScrollbackLines < 0 || header.ScrollbackLines > 50000 || !sequencePattern.MatchString(header.ThroughSequence) {
		return errors.New("invalid snapshot header")
	}
	return nil
}
