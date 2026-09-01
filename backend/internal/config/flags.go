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
		if arg == "--client-diagnostics" {
			if i+1 >= len(args) {
				return fmt.Errorf("missing value for %s", arg)
			}
			i++
			parsed, err := parseBool(args[i])
			if err != nil {
				return fmt.Errorf("client diagnostics: %w", err)
			}
			c.ClientDiagnosticsEnabled = parsed
			continue
		}
		key, value, hasValue := strings.Cut(arg, "=")
		if !hasValue {
			switch key {
			case "--host", "-h", "--port", "-p", "--password", "-a", "--websocket-ping", "--scrollback-lines", "--max-connection-instances", "--max-clients-per-connection-instance", "--cwd", "--frontend-dir", "--auth-access-ttl", "--auth-refresh-ttl", "--auth-max-attempts", "--agent-hooks-dir", "--web-push-vapid-public-key", "--web-push-vapid-private-key", "--web-push-subject", "--filesystem-image-preview-cache-dir", "--filesystem-image-preview-cache-target-mib", "--filesystem-image-preview-cache-max-age", "--filesystem-image-preview-cache-cleanup-interval", "--filesystem-image-preview-max-conversions", "--filesystem-image-preview-max-source-mib", "--filesystem-image-preview-max-output-mib", "--filesystem-image-preview-max-static-pixels", "--filesystem-image-preview-max-frames", "--filesystem-image-preview-max-animated-pixels", "--filesystem-image-preview-conversion-timeout":
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
		case "--max-connection-instances":
			n, err := strconv.Atoi(value)
			if err != nil {
				return err
			}
			c.MaxConnectionInstances = n
		case "--max-clients-per-connection-instance":
			n, err := strconv.Atoi(value)
			if err != nil {
				return err
			}
			c.MaxClientsPerConnectionInstance = n
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
		case "--client-diagnostics":
			b, err := parseBool(value)
			if err != nil {
				return fmt.Errorf("client diagnostics: %w", err)
			}
			c.ClientDiagnosticsEnabled = b
		case "--agent-hooks-dir":
			c.AgentHooksDir = value
		case "--web-push-vapid-public-key":
			c.WebPushVAPIDPublicKey = value
		case "--web-push-vapid-private-key":
			c.WebPushVAPIDPrivateKey = value
		case "--web-push-subject":
			c.WebPushSubject = value
		case "--filesystem-image-preview-cache-dir", "--filesystem-image-preview-cache-target-mib", "--filesystem-image-preview-cache-max-age", "--filesystem-image-preview-cache-cleanup-interval", "--filesystem-image-preview-max-conversions", "--filesystem-image-preview-max-source-mib", "--filesystem-image-preview-max-output-mib", "--filesystem-image-preview-max-static-pixels", "--filesystem-image-preview-max-frames", "--filesystem-image-preview-max-animated-pixels", "--filesystem-image-preview-conversion-timeout":
			return applyFilesystemImagePreviewArg(c, key, value)
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
	if err := set("ROAMINAL_MAX_CONNECTION_INSTANCES", func(v string) error {
		n, err := strconv.Atoi(v)
		c.MaxConnectionInstances = n
		return err
	}); err != nil {
		return err
	}
	if err := set("ROAMINAL_MAX_CLIENTS_PER_CONNECTION_INSTANCE", func(v string) error {
		n, err := strconv.Atoi(v)
		c.MaxClientsPerConnectionInstance = n
		return err
	}); err != nil {
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
	if err := set("ROAMINAL_CLIENT_DIAGNOSTICS_ENABLED", func(v string) error { b, err := parseBool(v); c.ClientDiagnosticsEnabled = b; return err }); err != nil {
		return err
	}
	if err := set("ROAMINAL_AGENT_HOOKS_DIR", func(v string) error { c.AgentHooksDir = v; return nil }); err != nil {
		return err
	}
	if err := set("ROAMINAL_WEB_PUSH_VAPID_PUBLIC_KEY", func(v string) error { c.WebPushVAPIDPublicKey = v; return nil }); err != nil {
		return err
	}
	if err := set("ROAMINAL_WEB_PUSH_VAPID_PRIVATE_KEY", func(v string) error { c.WebPushVAPIDPrivateKey = v; return nil }); err != nil {
		return err
	}
	if err := set("ROAMINAL_WEB_PUSH_SUBJECT", func(v string) error { c.WebPushSubject = v; return nil }); err != nil {
		return err
	}
	if err := applyFilesystemImagePreviewEnv(c); err != nil {
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
