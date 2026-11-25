// Package db provides the same repository implementation for SQLite and PostgreSQL.
package db

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
)

const (
	SQLite   = "sqlite"
	Postgres = "postgres"
)

type Config struct {
	Driver      string
	SQLitePath  string
	PostgresDSN string
}

// FromEnv defaults to SQLite and rejects active settings for the other driver.
func FromEnv(stateDir string) (Config, error) {
	c := Config{
		Driver:      os.Getenv("ROAMINAL_DB_DRIVER"),
		SQLitePath:  os.Getenv("ROAMINAL_DB_SQLITE_PATH"),
		PostgresDSN: os.Getenv("ROAMINAL_DB_POSTGRES_DSN"),
	}
	if c.Driver == "" {
		c.Driver = SQLite
	}
	if c.SQLitePath == "" && c.Driver == SQLite {
		c.SQLitePath = filepath.Join(stateDir, "db", "roaminal.sqlite3")
	}
	return c, c.Validate()
}

func (c Config) Validate() error {
	switch c.Driver {
	case SQLite:
		if c.PostgresDSN != "" || c.SQLitePath == "" || !filepath.IsAbs(c.SQLitePath) {
			return errors.New("sqlite requires an absolute path and no postgres DSN")
		}
	case Postgres:
		if c.SQLitePath != "" || c.PostgresDSN == "" {
			return errors.New("postgres requires a DSN and no sqlite path")
		}
		if _, err := postgresURL(c.PostgresDSN); err != nil {
			return err
		}
	default:
		return errors.New("unsupported database driver")
	}
	return nil
}

func postgresURL(dsn string) (*url.URL, error) {
	invalid := errors.New("postgres requires a TCP URI with sslmode=verify-full")
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" {
		return nil, invalid
	}
	values, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(values["sslmode"]) != 1 || values.Get("sslmode") != "verify-full" {
		return nil, invalid
	}
	for _, key := range []string{"host", "hostaddr", "port", "service", "servicefile"} {
		if values.Has(key) {
			return nil, invalid
		}
	}
	return u, nil
}
