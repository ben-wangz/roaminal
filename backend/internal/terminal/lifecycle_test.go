package terminal

import (
	"strings"
	"testing"
)

func TestTerminalEnvironmentEnablesUTF8ForLegacyLocale(t *testing.T) {
	base := []string{"LC_ALL=POSIX", "LC_CTYPE=POSIX", "LANG=C", "TERM=dumb"}
	got := terminalEnvironment(base, []string{"TERM=xterm-256color", "ROAMINAL_TERMINAL_ID=test"})

	if value := environmentValue(got, "LC_ALL"); value != "" {
		t.Fatalf("LC_ALL should be removed when it disables UTF-8, got %q", value)
	}
	if value := environmentValue(got, "LC_CTYPE"); value != "C.UTF-8" {
		t.Fatalf("LC_CTYPE = %q, want C.UTF-8", value)
	}
	if value := environmentValue(got, "TERM"); value != "xterm-256color" {
		t.Fatalf("TERM = %q, want xterm-256color", value)
	}
	if count := countEnvironmentKey(got, "LC_CTYPE"); count != 1 {
		t.Fatalf("LC_CTYPE appears %d times, want once", count)
	}
}

func TestTerminalEnvironmentPreservesUTF8Locale(t *testing.T) {
	base := []string{"LC_ALL=en_US.UTF-8", "LC_CTYPE=POSIX", "LANG=C"}
	got := terminalEnvironment(base, nil)

	if value := environmentValue(got, "LC_ALL"); value != "en_US.UTF-8" {
		t.Fatalf("UTF-8 LC_ALL changed to %q", value)
	}
	if value := environmentValue(got, "LC_CTYPE"); value != "POSIX" {
		t.Fatalf("LC_CTYPE changed despite UTF-8 LC_ALL: %q", value)
	}
}

func countEnvironmentKey(environment []string, key string) int {
	count := 0
	for _, entry := range environment {
		name, _, ok := strings.Cut(entry, "=")
		if ok && name == key {
			count++
		}
	}
	return count
}
