package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func applyArgs(c *Config, args []string) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--debug" || arg == "-d" {
			c.Debug = true
			continue
		}
		if arg == "--accept-terms" || arg == "-y" {
			c.AcceptTerms = true
			continue
		}
		key, value, hasValue := strings.Cut(arg, "=")
		if !hasValue {
			switch key {
			case "--host", "-h", "--port", "-p", "--password", "-a", "--websocket-ping", "--scrollback-lines", "--max-sessions", "--max-clients-per-session", "--cwd", "--frontend-dir", "--auth-access-ttl", "--auth-refresh-ttl", "--auth-max-attempts":
				if i+1 >= len(args) {
					return fmt.Errorf("missing value for %s", key)
				}
				i++
				value = args[i]
			default:
				return fmt.Errorf("unknown argument %s", arg)
			}
		}
		switch key {
		case "--host", "-h":
			c.Host = value
		case "--port", "-p":
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("port: %w", err)
			}
			c.Port = n
		case "--password", "-a":
			c.Password = value
		case "--websocket-ping":
			d, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			c.WebsocketPingInterval = d
		case "--scrollback-lines":
			n, err := strconv.Atoi(value)
			if err != nil {
				return err
			}
			c.ScrollbackLines = n
		case "--max-sessions":
			n, err := strconv.Atoi(value)
			if err != nil {
				return err
			}
			c.MaxSessions = n
		case "--max-clients-per-session":
			n, err := strconv.Atoi(value)
			if err != nil {
				return err
			}
			c.MaxClientsPerSession = n
		case "--cwd":
			c.InitialCwd = value
		case "--frontend-dir":
			c.FrontendDir = value
		case "--auth-access-ttl":
			d, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			c.AuthAccessTTL = d
		case "--auth-refresh-ttl":
			d, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			c.AuthRefreshTTL = d
		case "--auth-max-attempts":
			n, err := strconv.Atoi(value)
			if err != nil {
				return err
			}
			c.AuthMaxAttempts = n
		default:
			return fmt.Errorf("unknown argument %s", arg)
		}
	}
	return nil
}

func applyEnv(c *Config) error {
	set := func(name string, fn func(string) error) error {
		if value, ok := os.LookupEnv(name); ok {
			return fn(value)
		}
		return nil
	}
	if err := set("ROAMINAL_HOST", func(v string) error { c.Host = v; return nil }); err != nil {
		return err
	}
	if err := set("ROAMINAL_PORT", func(v string) error { n, err := strconv.Atoi(v); c.Port = n; return err }); err != nil {
		return err
	}
	if err := set("ROAMINAL_PASSWORD", func(v string) error { c.Password = v; return nil }); err != nil {
		return err
	}
	if err := set("ROAMINAL_WEBSOCKET_PING_INTERVAL", func(v string) error { d, err := time.ParseDuration(v); c.WebsocketPingInterval = d; return err }); err != nil {
		return err
	}
	if err := set("ROAMINAL_SCROLLBACK_LINES", func(v string) error { n, err := strconv.Atoi(v); c.ScrollbackLines = n; return err }); err != nil {
		return err
	}
	if err := set("ROAMINAL_MAX_SESSIONS", func(v string) error { n, err := strconv.Atoi(v); c.MaxSessions = n; return err }); err != nil {
		return err
	}
	if err := set("ROAMINAL_MAX_CLIENTS_PER_SESSION", func(v string) error { n, err := strconv.Atoi(v); c.MaxClientsPerSession = n; return err }); err != nil {
		return err
	}
	if err := set("ROAMINAL_DEBUG", func(v string) error { b, err := parseBool(v); c.Debug = b; return err }); err != nil {
		return err
	}
	if err := set("ROAMINAL_ACCEPT_TERMS", func(v string) error { b, err := parseBool(v); c.AcceptTerms = b; return err }); err != nil {
		return err
	}
	if err := set("ROAMINAL_CWD", func(v string) error { c.InitialCwd = v; return nil }); err != nil {
		return err
	}
	if err := set("ROAMINAL_FRONTEND_DIR", func(v string) error { c.FrontendDir = v; return nil }); err != nil {
		return err
	}
	if err := set("ROAMINAL_AUTH_ACCESS_TTL", func(v string) error { d, err := time.ParseDuration(v); c.AuthAccessTTL = d; return err }); err != nil {
		return err
	}
	if err := set("ROAMINAL_AUTH_REFRESH_TTL", func(v string) error { d, err := time.ParseDuration(v); c.AuthRefreshTTL = d; return err }); err != nil {
		return err
	}
	if err := set("ROAMINAL_AUTH_MAX_ATTEMPTS", func(v string) error { n, err := strconv.Atoi(v); c.AuthMaxAttempts = n; return err }); err != nil {
		return err
	}
	return nil
}

func parseBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes":
		return true, nil
	case "0", "false", "no":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q", value)
	}
}
