package connection

import (
	"regexp"
	"testing"
)

func TestTerminalIDIsUUIDv4(t *testing.T) {
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	for range 100 {
		if id := terminalID(); !pattern.MatchString(id) {
			t.Fatalf("terminalID=%q is not a UUIDv4", id)
		}
	}
}
