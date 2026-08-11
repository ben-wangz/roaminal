package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

func loadFile(path string) (fileConfig, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return fileConfig{}, false, nil
	}
	if err != nil {
		return fileConfig{}, false, fmt.Errorf("read config: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fileConfig{}, false, fmt.Errorf("parse config: %w", err)
	}
	for key := range raw {
		if !allowedFileKeys[key] {
			return fileConfig{}, false, fmt.Errorf("unknown config field %q", key)
		}
	}
	var values fileConfig
	if err := json.Unmarshal(data, &values); err != nil {
		return fileConfig{}, false, fmt.Errorf("decode config: %w", err)
	}
	return values, true, nil
}

func applyFile(c *Config, v fileConfig) error {
	if v.Host != nil {
		c.Host = *v.Host
	}
	if v.Port != nil {
		c.Port = *v.Port
	}
	if v.Password != nil {
		c.Password = *v.Password
	}
	if v.WebsocketPingInterval != nil {
		d, err := time.ParseDuration(*v.WebsocketPingInterval)
		if err != nil {
			return fmt.Errorf("websocketPingInterval: %w", err)
		}
		c.WebsocketPingInterval = d
	}
	if v.ScrollbackLines != nil {
		c.ScrollbackLines = *v.ScrollbackLines
	}
	if v.MaxConnectionInstances != nil {
		c.MaxConnectionInstances = *v.MaxConnectionInstances
	}
	if v.MaxClientsPerConnectionInstance != nil {
		c.MaxClientsPerConnectionInstance = *v.MaxClientsPerConnectionInstance
	}
	if v.Debug != nil {
		c.Debug = *v.Debug
	}
	if v.AcceptTerms != nil {
		c.AcceptTerms = *v.AcceptTerms
	}
	if v.InitialCwd != nil {
		c.InitialCwd = *v.InitialCwd
	}
	if v.AuthAccessTTL != nil {
		d, err := time.ParseDuration(*v.AuthAccessTTL)
		if err != nil {
			return fmt.Errorf("authAccessTTL: %w", err)
		}
		c.AuthAccessTTL = d
	}
	if v.AuthRefreshTTL != nil {
		d, err := time.ParseDuration(*v.AuthRefreshTTL)
		if err != nil {
			return fmt.Errorf("authRefreshTTL: %w", err)
		}
		c.AuthRefreshTTL = d
	}
	if v.AuthMaxAttempts != nil {
		c.AuthMaxAttempts = *v.AuthMaxAttempts
	}
	if v.ClientDiagnosticsEnabled != nil {
		c.ClientDiagnosticsEnabled = *v.ClientDiagnosticsEnabled
	}
	if v.FrontendDir != nil {
		c.FrontendDir = *v.FrontendDir
	}
	return nil
}
