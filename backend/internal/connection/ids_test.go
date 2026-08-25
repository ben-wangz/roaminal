package connection

import (
	"regexp"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/identity"
	"github.com/ben-wangz/roaminal/backend/internal/random"
)

func TestTerminalIDIsUUIDv4(t *testing.T) {
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	generator := identity.UUIDGenerator{Random: random.CryptoSource{}}
	for range 100 {
		id, err := generator.NewID()
		if err != nil {
			t.Fatal(err)
		}
		if !pattern.MatchString(id) {
			t.Fatalf("terminalID=%q is not a UUIDv4", id)
		}
	}
}
