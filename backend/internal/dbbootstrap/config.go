// Package dbbootstrap creates the stable credentials for a chart-managed
// PostgreSQL installation. It is used only by the bootstrap Job.
package dbbootstrap

import (
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const envPrefix = "ROAMINAL_DB_BOOTSTRAP_"

type Config struct {
	Namespace, Secret, Marker, Owner, Host string
	Database, Username, DSNKey, CAFile     string
}

var (
	dnsLabel  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	pgName    = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)
	secretKey = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,253}$`)
)

func FromEnv() Config {
	get := func(key string) string { return os.Getenv(envPrefix + key) }
	return Config{
		Namespace: get("NAMESPACE"), Secret: get("SECRET"), Marker: get("MARKER"), Owner: get("OWNER"), Host: get("HOST"),
		Database: get("DATABASE"), Username: get("USERNAME"), DSNKey: get("DSN_KEY"), CAFile: get("CA_FILE"),
	}
}

func dnsName(value string) bool {
	if len(value) > 253 {
		return false
	}
	for part := range strings.SplitSeq(value, ".") {
		if !dnsLabel.MatchString(part) {
			return false
		}
	}
	return true
}

func (c Config) validate() error {
	if !dnsLabel.MatchString(c.Namespace) || !dnsName(c.Secret) || !dnsName(c.Marker) || !dnsName(c.Owner) || !dnsName(c.Host) {
		return errors.New("invalid PostgreSQL bootstrap resource or DNS identity")
	}
	if !pgName.MatchString(c.Database) || !pgName.MatchString(c.Username) || c.Database == "postgres" || c.Username == "postgres" || strings.HasPrefix(c.Username, "pg_") {
		return errors.New("invalid PostgreSQL bootstrap database or owner")
	}
	if !secretKey.MatchString(c.DSNKey) || c.DSNKey == "." || c.DSNKey == ".." || c.DSNKey == "username" || c.DSNKey == "password" || !filepath.IsAbs(c.CAFile) {
		return errors.New("invalid PostgreSQL bootstrap DSN key or CA path")
	}
	return nil
}

func (c Config) dsn(password string) string {
	return (&url.URL{
		Scheme: "postgresql", Host: net.JoinHostPort(c.Host, "5432"), Path: "/" + c.Database, User: url.UserPassword(c.Username, password),
		RawQuery: url.Values{"sslmode": {"verify-full"}, "sslrootcert": {c.CAFile}}.Encode(),
	}).String()
}
