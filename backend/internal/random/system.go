package random

import (
	cryptorand "crypto/rand"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

// CryptoSource is the production cryptographic entropy implementation.
type CryptoSource struct{}

func (CryptoSource) Read(value []byte) (int, error) { return cryptorand.Read(value) }

var _ ports.RandomSource = CryptoSource{}
