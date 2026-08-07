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

func validateExecution(record ExecutionRecord) error {
	if !utf8.ValidString(record.Command) || !utf8.ValidString(record.Input) || !utf8.ValidString(record.Output) {
		return errors.New("execution contains invalid UTF-8")
	}
	if len([]byte(record.Command))+len([]byte(record.Input)) > 64*1024 || len([]byte(record.Output)) > 960*1024 {
		return errors.New("execution exceeds size limit")
	}
	if record.ExitCode == nil || record.StartedAt.IsZero() || record.CompletedAt.IsZero() || record.CompletedAt.Before(record.StartedAt) || record.DurationMs < 0 {
		return errors.New("execution is not completed")
	}
	return nil
}

func validateSessionMeta(meta SessionMeta) error {
	if meta.FormatVersion != 0 && meta.FormatVersion != FormatVersion && meta.FormatVersion != SessionFormatVersion {
		return errors.New("unsupported session format version")
	}
	if !uuidPattern.MatchString(meta.ID) {
		return errors.New("invalid session id")
	}
	if !utf8.ValidString(meta.EffectiveTitle()) || len([]byte(meta.EffectiveTitle())) > 512 || !utf8.ValidString(meta.AutomaticTitle) || len([]byte(meta.AutomaticTitle)) > 512 || !utf8.ValidString(meta.InitialCwd) || !utf8.ValidString(meta.Cwd) {
		return errors.New("invalid session text")
	}
	if meta.TitleOverride != nil {
		if err := ValidateTitleOverride(*meta.TitleOverride); err != nil {
			return err
		}
	}
	if !filepath.IsAbs(meta.InitialCwd) || !filepath.IsAbs(meta.Cwd) || len([]byte(meta.InitialCwd)) > 4096 || len([]byte(meta.Cwd)) > 4096 {
		return errors.New("invalid session cwd")
	}
	if meta.Cols < 2 || meta.Cols > 1000 || meta.Rows < 1 || meta.Rows > 1000 || meta.CreatedAt.IsZero() || meta.UpdatedAt.IsZero() || meta.UpdatedAt.Before(meta.CreatedAt) {
		return errors.New("invalid session dimensions or timestamp")
	}
	if len(meta.Executions) > 100 {
		return errors.New("too many executions")
	}
	for _, record := range meta.Executions {
		if err := validateExecution(record); err != nil {
			return err
		}
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
