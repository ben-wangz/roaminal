package config

import (
	"errors"
	"fmt"
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
	if c.AgentWebhookBaseURL != "" && !strings.HasPrefix(c.AgentWebhookBaseURL, "http://") && !strings.HasPrefix(c.AgentWebhookBaseURL, "https://") {
		return errors.New("agentWebhookBaseURL must use http or https")
	}
	return nil
}
