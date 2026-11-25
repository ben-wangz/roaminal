package db

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Store hosts Roaminal's durable application repositories.
type Store struct {
	DB             *gorm.DB
	SQL            *sql.DB
	driver         string
	instanceHealth instanceHealth
}

// ErrConnection identifies a temporary connection failure without a DSN.
var ErrConnection = errors.New("database connection failed")

// Open checks connectivity and applies numbered schema migrations. It does not
// read, remove, or import any legacy application state.
func Open(ctx context.Context, c Config) (*Store, error) { return open(ctx, c, true) }

// OpenExisting opens an initialized target without creating or migrating it.
// Callers must verify the installation identity before making application writes.
func OpenExisting(ctx context.Context, c Config) (*Store, error) { return open(ctx, c, false) }

func open(ctx context.Context, c Config, migrate bool) (*Store, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	var dialector gorm.Dialector
	switch c.Driver {
	case SQLite:
		if err := prepareSQLiteFile(c.SQLitePath, migrate); err != nil {
			return nil, err
		}
		// New SQLite files default to DELETE. Don't set a persistent journal
		// mode on connect, before the caller can verify the target's identity.
		dsn := (&url.URL{Scheme: "file", Path: filepath.Clean(c.SQLitePath), RawQuery: "mode=rw&_foreign_keys=on&_busy_timeout=5000&_txlock=immediate&_synchronous=FULL"}).String()
		dialector = sqlite.Open(dsn)
	case Postgres:
		uri, err := postgresURL(c.PostgresDSN)
		if err != nil {
			return nil, err
		}
		params := uri.Query()
		params.Set("connect_timeout", "5")
		if !params.Has("statement_timeout") {
			params.Set("statement_timeout", "30000")
		}
		uri.RawQuery = params.Encode()
		dialector = postgres.Open(uri.String())
	}
	gdb, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true,
	})
	if err != nil {
		return nil, errors.New("open database failed") // driver errors may contain credentials
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, errors.New("obtain database connection failed")
	}
	store := &Store{DB: gdb, SQL: sqlDB, driver: c.Driver}
	if c.Driver == SQLite {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
	} else {
		sqlDB.SetMaxOpenConns(8)
		sqlDB.SetMaxIdleConns(2)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
	}
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(check); err != nil {
		_ = sqlDB.Close()
		return nil, ErrConnection
	}
	if migrate {
		err = store.migrate(check)
	} else {
		err = checkSchema(gdb.WithContext(check))
	}
	if err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.SQL.Close() }

func prepareSQLiteFile(path string, create bool) error {
	path = filepath.Clean(path)
	dir := filepath.Dir(path)
	if create {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return errors.New("create sqlite directory failed")
		}
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("sqlite directory must be private and not a symlink")
	}
	if create {
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return f.Close()
		}
		if !errors.Is(err, os.ErrExist) {
			return errors.New("create sqlite file failed")
		}
	}
	info, err = os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("sqlite file must be private and regular")
	}
	return nil
}
