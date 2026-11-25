package browser

import (
	"strings"
	"testing"
)

func TestBrowserEnvironmentDoesNotInheritServerConfiguration(t *testing.T) {
	t.Setenv("ROAMINAL_PASSWORD", "test-password")
	t.Setenv("ROAMINAL_DB_POSTGRES_DSN", "postgres://test-secret")
	t.Setenv("ROAMINAL_BROWSER_CHROMIUM_PATH", "/wrong/chromium")
	t.Setenv("ROAMINAL_CUSTOM_SECRET", "test-secret")
	t.Setenv("BROWSER_TEST_KEY", "preserved")

	env := browserEnvironment("/usr/bin/chromium")
	for _, entry := range env {
		if strings.HasPrefix(entry, "ROAMINAL_") && entry != "ROAMINAL_BROWSER_CHROMIUM_PATH=/usr/bin/chromium" {
			t.Fatalf("browser process inherited server configuration key %q", strings.SplitN(entry, "=", 2)[0])
		}
	}
	if !containsEnvironment(env, "BROWSER_TEST_KEY=preserved") || !containsEnvironment(env, "ROAMINAL_BROWSER_CHROMIUM_PATH=/usr/bin/chromium") {
		t.Fatal("browser environment lost required entries")
	}
}

func containsEnvironment(env []string, entry string) bool {
	for _, value := range env {
		if value == entry {
			return true
		}
	}
	return false
}
