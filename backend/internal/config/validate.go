package config

import (
	"bytes"
	"crypto/elliptic"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

func (c Config) Validate() error {
	if !c.AcceptTerms {
		return errors.New("acceptTerms must be true to start Roaminal")
	}
	if strings.TrimSpace(c.Host) == "" {
		return errors.New("host must not be empty")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be 1..65535, got %d", c.Port)
	}
	if len([]byte(c.Password)) < 1 || len([]byte(c.Password)) > 1024 {
		return errors.New("password must be 1..1024 UTF-8 bytes")
	}
	if !utf8.ValidString(c.Password) {
		return errors.New("password must be valid UTF-8")
	}
	if c.WebsocketPingInterval < time.Second || c.WebsocketPingInterval > 5*time.Minute {
		return errors.New("websocketPingInterval must be 1s..5m")
	}
	if c.ScrollbackLines < 0 || c.ScrollbackLines > 50000 {
		return errors.New("scrollbackLines must be 0..50000")
	}
	if c.MaxConnectionInstances < 1 || c.MaxConnectionInstances > 256 {
		return errors.New("maxConnectionInstances must be 1..256")
	}
	if c.MaxClientsPerConnectionInstance < 1 || c.MaxClientsPerConnectionInstance > 64 {
		return errors.New("maxClientsPerConnectionInstance must be 1..64")
	}
	if c.AuthAccessTTL < time.Minute || c.AuthAccessTTL > 24*time.Hour {
		return errors.New("authAccessTTL must be 1m..24h")
	}
	if c.AuthRefreshTTL < time.Hour || c.AuthRefreshTTL > 8760*time.Hour || c.AuthRefreshTTL < c.AuthAccessTTL {
		return errors.New("authRefreshTTL must be 1h..8760h and at least access TTL")
	}
	if c.AuthMaxAttempts < 1 || c.AuthMaxAttempts > 1000 {
		return errors.New("authMaxAttempts must be 1..1000")
	}
	if !filepath.IsAbs(c.InitialCwd) {
		return errors.New("initialCwd must be an absolute path")
	}
	info, err := os.Stat(c.InitialCwd)
	if err != nil {
		return fmt.Errorf("initialCwd: %w", err)
	}
	if !info.IsDir() {
		return errors.New("initialCwd must be a directory")
	}
	if c.StateDir == "" {
		return errors.New("state directory must not be empty")
	}
	if strings.TrimSpace(c.AgentHooksDir) == "" {
		return errors.New("agent hooks directory must not be empty")
	}
	if err := validateWebPush(c); err != nil {
		return err
	}
	return nil
}

func validateWebPush(c Config) error {
	publicKey := strings.TrimSpace(c.WebPushVAPIDPublicKey)
	privateKey := strings.TrimSpace(c.WebPushVAPIDPrivateKey)
	subject := strings.TrimSpace(c.WebPushSubject)
	if publicKey == "" && privateKey == "" && subject == "" {
		return nil
	}
	if publicKey == "" || privateKey == "" || subject == "" {
		return errors.New("web push requires VAPID public key, private key, and subject")
	}
	publicBytes, err := base64.RawURLEncoding.DecodeString(publicKey)
	if err != nil || len(publicBytes) != 65 || publicBytes[0] != 4 {
		return errors.New("webPushVAPIDPublicKey must be an uncompressed P-256 public key")
	}
	if x, y := elliptic.Unmarshal(elliptic.P256(), publicBytes); x == nil || y == nil {
		return errors.New("webPushVAPIDPublicKey is not a valid P-256 public key")
	}
	privateBytes, err := base64.RawURLEncoding.DecodeString(privateKey)
	if err != nil || len(privateBytes) != 32 {
		return errors.New("webPushVAPIDPrivateKey must be a 32-byte base64url key")
	}
	privateX, privateY := elliptic.P256().ScalarBaseMult(privateBytes)
	if privateX == nil || privateY == nil || !bytes.Equal(elliptic.Marshal(elliptic.P256(), privateX, privateY), publicBytes) {
		return errors.New("web push VAPID public and private keys do not match")
	}
	if len([]byte(subject)) > 512 || !utf8.ValidString(subject) || !validWebPushSubject(subject) {
		return errors.New("webPushSubject must be an email, mailto URL, or HTTPS URL")
	}
	return nil
}

func validWebPushSubject(subject string) bool {
	if strings.HasPrefix(subject, "mailto:") {
		_, err := mail.ParseAddress(strings.TrimPrefix(subject, "mailto:"))
		return err == nil
	}
	if strings.HasPrefix(subject, "https://") {
		u, err := url.Parse(subject)
		return err == nil && u.Host != "" && u.User == nil && u.Fragment == ""
	}
	_, err := mail.ParseAddress(subject)
	return err == nil
}
