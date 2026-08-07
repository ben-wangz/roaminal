package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

func newID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:]), nil
}

func opaque(prefix string) (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
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
	id, err := newID()
	if err != nil {
		return ChallengeResponse{}, err
	}
	var saltBytes [32]byte
	if _, err := rand.Read(saltBytes[:]); err != nil {
		return ChallengeResponse{}, err
	}
	expires := time.Now().UTC().Add(ChallengeTTL)
	value := challenge{ID: id, Salt: base64.RawURLEncoding.EncodeToString(saltBytes[:]), ExpiresAt: expires}
	m.challenges[id] = value
	for key, item := range m.challenges {
		if !item.ExpiresAt.After(time.Now()) {
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
