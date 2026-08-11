package clientdiag

import (
	"strings"
	"testing"
)

func TestRedactTextRemovesCredentialMaterialAndURLSecrets(t *testing.T) {
	value := `Authorization: Bearer abc.def password=secret https://user:pass@example.test/path?token=abc#fragment roaminal.auth.access-secret -----BEGIN OPENSSH PRIVATE KEY-----secret-----END OPENSSH PRIVATE KEY-----`
	redacted := RedactText(value, 4096)
	for _, forbidden := range []string{"abc.def", "secret", "user:pass", "token=abc", "access-secret", "BEGIN OPENSSH PRIVATE KEY"} {
		if strings.Contains(redacted, forbidden) {
			t.Fatalf("redacted value contains %q: %q", forbidden, redacted)
		}
	}
	if !strings.Contains(redacted, "Authorization=[REDACTED]") || !strings.Contains(redacted, "https://example.test/path") {
		t.Fatalf("expected stable redaction markers: %q", redacted)
	}
}

func TestRedactTextNormalizesControlCharactersAndUTF8Limit(t *testing.T) {
	value := "hello\x00\x1bworld"
	redacted := RedactText(value, 5)
	if redacted != "hello" {
		t.Fatalf("redacted = %q, want hello", redacted)
	}
	if strings.ContainsAny(RedactText("a\x00b", 100), "\x00\x1b") {
		t.Fatal("control characters were not removed")
	}
}
