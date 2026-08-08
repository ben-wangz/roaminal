package connection

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestShortPathTokenKeepsMuxSocketPathBounded(t *testing.T) {
	bootID := "552c38b7-80a0-42d6-995a-5c0267d7461a"
	sessionID := "b2c6e673-c603-4e80-9d68-878525019bd8"
	controlPath := filepath.Join("/tmp/roaminal-mux", "rm-"+shortPathToken(bootID), "t-"+shortPathToken(sessionID), "ctl")
	if got := len(controlPath); got > 70 {
		t.Fatalf("control path length = %d, want <= 70: %s", got, controlPath)
	}
	if got := shortPathToken(strings.Repeat("a", 64)); len(got) != 12 {
		t.Fatalf("short token length = %d, want 12", len(got))
	}
}
