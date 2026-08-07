package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
	c.FrontendDir = filepath.Join("..", "frontend", "dist")
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
