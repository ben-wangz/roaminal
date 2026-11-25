package agent

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

func ValidEndpointKey(key string) bool {
	if len(key) != base64.RawURLEncoding.EncodedLen(sha256.Size) {
		return false
	}
	_, err := base64.RawURLEncoding.Strict().DecodeString(key)
	return err == nil
}

// ValidateEndpointRecord accepts the partially populated projection the
// scheduler can create when SSH is unavailable. A populated endpoint must
// match the canonical key originally computed by NormalizeEndpoint.
func ValidateEndpointRecord(key string, record domain.AgentEndpointRecord) bool {
	if !ValidEndpointKey(key) || !utf8.ValidString(record.User) || !utf8.ValidString(record.Host) {
		return false
	}
	if record.User != "" || record.Host != "" || record.Port != 0 {
		// Use the persisted normalized fields directly. Normalizing a second
		// time would, for example, trim a second trailing dot from a host.
		if record.User == "" || record.Host == "" || record.Port < 1 || record.Port > 65535 ||
			strings.ContainsFunc(record.User+record.Host, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) ||
			key != endpointKey(record.User, record.Host, record.Port) {
			return false
		}
	}
	for name, target := range record.Targets {
		if !connectionoptions.ValidSessionName(name) || target.SessionName != name || target.SessionCreated < 0 {
			return false
		}
	}
	return true
}
