package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultHost             = "127.0.0.1"
	DefaultPort             = 9846
	DefaultWebsocketPing    = 10 * time.Second
	DefaultScrollback       = 1000
	DefaultMaxSessions      = 32
	DefaultMaxClients       = 8
	DefaultInitialCwd       = "/workspace"
	DefaultAuthAccessTTL    = 15 * time.Minute
	DefaultAuthRefreshTTL   = 2160 * time.Hour
	DefaultAuthMaxAttempts  = 30
	DefaultShutdownDeadline = 10 * time.Second
	DefaultWorkerHandshake  = 5 * time.Second
	DefaultWorkerControl    = 30 * time.Second
	DefaultWorkerStall      = 10 * time.Second
)

type Config struct {
	Host                  string
	Port                  int
	Password              string
	WebsocketPingInterval time.Duration
	ScrollbackLines       int
	MaxSessions           int
	MaxClientsPerSession  int
	Debug                 bool
	AcceptTerms           bool
	InitialCwd            string
	AuthAccessTTL         time.Duration
	AuthRefreshTTL        time.Duration
	AuthMaxAttempts       int
	StateDir              string
	WorkerPath            string
	Version               string
	PasswordGenerated     bool
}

type fileConfig struct {
	Host                  *string `json:"host"`
	Port                  *int    `json:"port"`
	Password              *string `json:"password"`
	WebsocketPingInterval *string `json:"websocketPingInterval"`
	ScrollbackLines       *int    `json:"scrollbackLines"`
	MaxSessions           *int    `json:"maxSessions"`
	MaxClients            *int    `json:"maxClientsPerSession"`
	Debug                 *bool   `json:"debug"`
	AcceptTerms           *bool   `json:"acceptTerms"`
	InitialCwd            *string `json:"initialCwd"`
	AuthAccessTTL         *string `json:"authAccessTTL"`
	AuthRefreshTTL        *string `json:"authRefreshTTL"`
	AuthMaxAttempts       *int    `json:"authMaxAttempts"`
}

var allowedFileKeys = map[string]bool{
	"host": true, "port": true, "password": true, "websocketPingInterval": true,
	"scrollbackLines": true, "maxSessions": true, "maxClientsPerSession": true,
	"debug": true, "acceptTerms": true, "initialCwd": true, "authAccessTTL": true,
	"authRefreshTTL": true, "authMaxAttempts": true,
}

var ErrHelp = errors.New("help requested")

func defaults() Config {
	return Config{
		Host: DefaultHost, Port: DefaultPort, WebsocketPingInterval: DefaultWebsocketPing,
		ScrollbackLines: DefaultScrollback, MaxSessions: DefaultMaxSessions,
		MaxClientsPerSession: DefaultMaxClients, InitialCwd: DefaultInitialCwd,
		AuthAccessTTL: DefaultAuthAccessTTL, AuthRefreshTTL: DefaultAuthRefreshTTL,
		AuthMaxAttempts: DefaultAuthMaxAttempts,
	}
}

func Load(args []string) (Config, error) {
	for _, arg := range args {
		if arg == "--help" || arg == "-help" {
			fmt.Fprintln(os.Stdout, "Roaminal - persistent Bash terminal\n\nUsage: roaminal [options]\n\nOptions: --host/-h --port/-p --password/-a --websocket-ping --scrollback-lines --max-sessions --max-clients-per-session --cwd --auth-access-ttl --auth-refresh-ttl --auth-max-attempts --debug/-d --accept-terms/-y")
			return Config{}, ErrHelp
		}
	}
	c := defaults()
	passwordProvided := false
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("resolve home directory: %w", err)
	}
	c.StateDir = filepath.Join(home, ".roaminal")
	for _, path := range []string{filepath.Join(home, ".roaminal", "config.json"), filepath.Join(mustGetwd(), "config.json")} {
		values, exists, err := loadFile(path)
		if err != nil {
			return Config{}, err
		}
		if exists {
			passwordProvided = passwordProvided || values.Password != nil
			if err := applyFile(&c, values); err != nil {
				return Config{}, fmt.Errorf("%s: %w", path, err)
			}
		}
	}
	if err := applyArgs(&c, args); err != nil {
		return Config{}, err
	}
	if err := applyEnv(&c); err != nil {
		return Config{}, err
	}
	if _, ok := os.LookupEnv("ROAMINAL_PASSWORD"); ok {
		passwordProvided = true
	}
	for i, arg := range args {
		if arg == "--password" || arg == "-a" || strings.HasPrefix(arg, "--password=") || (i > 0 && (args[i-1] == "--password" || args[i-1] == "-a")) {
			passwordProvided = true
		}
	}
	if c.Password == "" && passwordProvided {
		return Config{}, errors.New("password must be explicitly non-empty")
	}
	if c.Password == "" {
		var raw [16]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return Config{}, fmt.Errorf("generate password: %w", err)
		}
		c.Password = hex.EncodeToString(raw[:])
		c.PasswordGenerated = true
		fmt.Fprintf(os.Stderr, "Roaminal generated password: %s\n", c.Password)
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func mustGetwd() string {
	path, err := os.Getwd()
	if err != nil {
		return "."
	}
	return path
}

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
	if v.MaxSessions != nil {
		c.MaxSessions = *v.MaxSessions
	}
	if v.MaxClients != nil {
		c.MaxClientsPerSession = *v.MaxClients
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
	return nil
}

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
			case "--host", "-h", "--port", "-p", "--password", "-a", "--websocket-ping", "--scrollback-lines", "--max-sessions", "--max-clients-per-session", "--cwd", "--auth-access-ttl", "--auth-refresh-ttl", "--auth-max-attempts":
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
	if c.WebsocketPingInterval < time.Second || c.WebsocketPingInterval > 5*time.Minute {
		return errors.New("websocketPingInterval must be 1s..5m")
	}
	if c.ScrollbackLines < 0 || c.ScrollbackLines > 50000 {
		return errors.New("scrollbackLines must be 0..50000")
	}
	if c.MaxSessions < 1 || c.MaxSessions > 256 {
		return errors.New("maxSessions must be 1..256")
	}
	if c.MaxClientsPerSession < 1 || c.MaxClientsPerSession > 64 {
		return errors.New("maxClientsPerSession must be 1..64")
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
	return nil
}
