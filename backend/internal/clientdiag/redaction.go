package clientdiag

import (
	"net/url"
	"regexp"
)

var (
	privateKeyPattern = regexp.MustCompile(`(?is)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)
	bearerPattern     = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	protocolPattern   = regexp.MustCompile(`(?i)\broaminal\.auth\.[A-Za-z0-9._~+/=-]+`)
	secretPattern     = regexp.MustCompile(`(?i)\b(accessToken|refreshToken|password|passphrase|privateKey|authorization)\b\s*[:=]\s*("[^"]*"|'[^']*'|[^,\s}]+)`)
	urlPattern        = regexp.MustCompile(`https?://[^\s"'<>]+`)
)

// RedactText is intentionally conservative and is applied again by the server.
func RedactText(value string, maxBytes int) string {
	if value == "" {
		return ""
	}
	value = privateKeyPattern.ReplaceAllString(value, "[REDACTED_PRIVATE_KEY]")
	value = bearerPattern.ReplaceAllString(value, "Bearer [REDACTED]")
	value = protocolPattern.ReplaceAllString(value, "roaminal.auth.[REDACTED]")
	value = secretPattern.ReplaceAllString(value, "$1=[REDACTED]")
	value = urlPattern.ReplaceAllStringFunc(value, redactURL)
	value = cleanControlCharacters(value)
	return truncateUTF8(value, maxBytes)
}

func redactURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return "[REDACTED_URL]"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}
