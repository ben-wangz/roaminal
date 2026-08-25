package identity

import (
	"crypto/rand"
	"fmt"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

// UUIDGenerator is the production implementation of ports.IDGenerator.
// Keeping UUID generation here prevents persistence adapters from becoming a
// general-purpose identity service.
type UUIDGenerator struct {
	Random ports.RandomSource
}

func (g UUIDGenerator) NewID() (string, error) {
	var value [16]byte
	reader := g.Random
	if reader == nil {
		reader = cryptoRandomSource{}
	}
	if _, err := reader.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:]), nil
}

type cryptoRandomSource struct{}

func (cryptoRandomSource) Read(value []byte) (int, error) { return rand.Read(value) }
