package agent

import (
	"encoding/base64"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/ben-wangz/roaminal/backend/internal/random"
)

func randomToken(sources ...ports.RandomSource) (string, string, error) {
	var raw [32]byte
	source := ports.RandomSource(random.CryptoSource{})
	if len(sources) > 0 && sources[0] != nil {
		source = sources[0]
	}
	if _, err := source.Read(raw[:]); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	return token, tokenHash(token), nil
}

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

func (s *Service) allowEvent(key string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rate == nil {
		s.rate = map[string]eventRate{}
	}
	window := s.rate[key]
	if window.LastTokens.IsZero() {
		window = eventRate{LastTokens: now, Tokens: 30}
	}
	if elapsed := now.Sub(window.LastTokens).Seconds(); elapsed > 0 {
		window.Tokens = minFloat(30, window.Tokens+elapsed*2)
		window.LastTokens = now
	}
	if window.Tokens < 1 {
		s.rate[key] = window
		return false
	}
	window.Tokens--
	s.rate[key] = window
	return true
}

func minFloat(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}
