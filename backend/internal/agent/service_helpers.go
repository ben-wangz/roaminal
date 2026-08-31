package agent

import (
	"encoding/base64"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/ben-wangz/roaminal/backend/internal/random"
)

func randomID(sources ...ports.RandomSource) (string, error) {
	var raw [16]byte
	source := ports.RandomSource(random.CryptoSource{})
	if len(sources) > 0 && sources[0] != nil {
		source = sources[0]
	}
	if _, err := source.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func (s *Service) newID() (string, error) {
	if s.ids != nil {
		return s.ids.NewID()
	}
	return randomID(s.random)
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
