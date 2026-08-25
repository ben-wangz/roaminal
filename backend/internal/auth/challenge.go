package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func opaque(randomSource ports.RandomSource, prefix string) (string, error) {
	var raw [32]byte
	if _, err := randomSource.Read(raw[:]); err != nil {
		return "", err
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func hashToken(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func passwordKey(password string) []byte { sum := sha256.Sum256([]byte(password)); return sum[:] }

func (m *Manager) Challenge() (ChallengeResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, err := m.ids.NewID()
	if err != nil {
		return ChallengeResponse{}, err
	}
	var saltBytes [32]byte
	if _, err := m.random.Read(saltBytes[:]); err != nil {
		return ChallengeResponse{}, err
	}
	expires := m.clock.Now().UTC().Add(ChallengeTTL)
	value := challenge{ID: id, Salt: base64.RawURLEncoding.EncodeToString(saltBytes[:]), ExpiresAt: expires}
	m.challenges[id] = value
	for key, item := range m.challenges {
		if !item.ExpiresAt.After(m.clock.Now()) {
			delete(m.challenges, key)
		}
	}
	return ChallengeResponse{ChallengeID: id, Salt: value.Salt, ExpiresAt: expires, Algorithm: AuthAlgorithm}, nil
}

func Proof(password string, c ChallengeResponse) string {
	message := MessagePrefix + c.ChallengeID + ":" + c.Salt + ":" + c.ExpiresAt.UTC().Format(time.RFC3339Nano)
	h := hmac.New(sha256.New, passwordKey(password))
	_, _ = h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}

func loginMessage(c challenge) string {
	return MessagePrefix + c.ID + ":" + c.Salt + ":" + c.ExpiresAt.UTC().Format(time.RFC3339Nano)
}

func truncate(value string, max int) string {
	data := []byte(strings.TrimSpace(value))
	if len(data) <= max {
		return string(data)
	}
	return string(data[:max])
}
