package db

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

func sqliteStore(t *testing.T) (*Store, Config) {
	t.Helper()
	c := Config{Driver: SQLite, SQLitePath: filepath.Join(t.TempDir(), "private", "roaminal.sqlite3")}
	s, err := Open(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, c
}

func sampleEnrollment(step int64) domain.TOTPEnrollmentRecord {
	return domain.TOTPEnrollmentRecord{FormatVersion: domain.TOTPFormatVersion,
		EnrollmentID: "11111111-1111-4111-8111-111111111111", Secret: "JBSWY3DPEHPK3PXP", LastAcceptedTimeStep: step}
}

func sampleAuth() domain.AuthSessionRecord {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return domain.AuthSessionRecord{ID: "22222222-2222-4222-8222-222222222222", PasswordFingerprint: strings.Repeat("a", 64), EnrollmentID: sampleEnrollment(0).EnrollmentID,
		RefreshTokenHash: strings.Repeat("b", 64), CreatedAt: now, LastSeenAt: now, RefreshExpiresAt: now.Add(time.Hour),
		RotatedAt: now, UserAgent: "test"}
}

func testRepositoryContract(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	if _, exists, err := s.LoadEnrollment(ctx); err != nil || exists {
		t.Fatalf("fresh TOTP: exists=%v err=%v", exists, err)
	}
	if err := s.SaveEnrollment(ctx, sampleEnrollment(1)); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveEnrollment(ctx, sampleEnrollment(1)); !errors.Is(err, ErrStaleEnrollment) {
		t.Fatalf("replay accepted: %v", err)
	}
	changed := sampleEnrollment(2)
	changed.Secret = "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"
	if err := s.SaveEnrollment(ctx, changed); !errors.Is(err, ErrStaleEnrollment) {
		t.Fatalf("secret changed without reset: %v", err)
	}
	if err := s.SaveEnrollment(ctx, sampleEnrollment(3)); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveEnrollment(ctx, sampleEnrollment(2)); !errors.Is(err, ErrStaleEnrollment) {
		t.Fatalf("older step accepted: %v", err)
	}
	got, exists, err := s.LoadEnrollment(ctx)
	if err != nil || !exists || got != sampleEnrollment(3) {
		t.Fatalf("TOTP round trip: exists=%v err=%v", exists, err)
	}
	auth := sampleAuth()
	if err := s.SaveAuth(ctx, []domain.AuthSessionRecord{auth}); err != nil {
		t.Fatal(err)
	}
	bad := sampleAuth()
	bad.EnrollmentID = ""
	if err := s.SaveAuth(ctx, []domain.AuthSessionRecord{bad}); err == nil {
		t.Fatal("accepted auth without enrollment binding")
	}
	rows, err := s.LoadAuth(ctx)
	if err != nil || len(rows) != 1 || rows[0].RefreshTokenHash != auth.RefreshTokenHash || !rows[0].CreatedAt.Equal(auth.CreatedAt) {
		t.Fatalf("auth round trip: count=%d err=%v", len(rows), err)
	}
	if err := s.SaveAuth(ctx, nil); err != nil {
		t.Fatal(err)
	}
	rows, err = s.LoadAuth(ctx)
	if err != nil || len(rows) != 0 {
		t.Fatalf("auth clear: count=%d err=%v", len(rows), err)
	}
}

func TestSQLiteRepositoriesAndSchema(t *testing.T) {
	s, c := sqliteStore(t)
	testRepositoryContract(t, s)
	for _, p := range []string{c.SQLitePath, filepath.Dir(c.SQLitePath)} {
		info, err := os.Stat(p)
		if err != nil || info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("unsafe SQLite permissions: %s (%v)", p, err)
		}
	}
	second, err := Open(context.Background(), c)
	if err != nil {
		t.Fatalf("reopen versioned schema: %v", err)
	}
	_ = second.Close()
	if err := s.DB.Exec("INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)", 99, time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), c); err == nil {
		t.Fatal("accepted newer schema")
	}
}

func TestSQLiteIncompleteSchemaFailsClosed(t *testing.T) {
	s, c := sqliteStore(t)
	if err := s.DB.Migrator().DropTable(&authRow{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), c); err == nil {
		t.Fatal("accepted missing versioned table")
	}
}

func TestSQLiteConcurrentTimeSteps(t *testing.T) {
	s, _ := sqliteStore(t)
	if err := s.SaveEnrollment(context.Background(), sampleEnrollment(1)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for step := int64(2); step <= 40; step++ {
		wg.Add(1)
		go func(step int64) {
			defer wg.Done()
			err := s.SaveEnrollment(context.Background(), sampleEnrollment(step))
			if err != nil && !errors.Is(err, ErrStaleEnrollment) {
				t.Errorf("step %d: %v", step, err)
			}
		}(step)
	}
	wg.Wait()
	got, _, err := s.LoadEnrollment(context.Background())
	if err != nil || got.LastAcceptedTimeStep != 40 {
		t.Fatalf("lost monotonic update: step=%d err=%v", got.LastAcceptedTimeStep, err)
	}
}

func TestSQLiteUnavailableAndUnsafeFile(t *testing.T) {
	s, _ := sqliteStore(t)
	_ = s.Close()
	if _, exists, err := s.LoadEnrollment(context.Background()); err == nil || exists {
		t.Fatalf("unavailable DB became unconfigured: exists=%v err=%v", exists, err)
	}
	root := t.TempDir()
	path := filepath.Join(root, "private", "roaminal.sqlite3")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "elsewhere"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), Config{Driver: SQLite, SQLitePath: path}); err == nil {
		t.Fatal("followed SQLite symlink")
	}
}

func TestDatabaseEnvValidation(t *testing.T) {
	for _, key := range []string{"ROAMINAL_DB_DRIVER", "ROAMINAL_DB_SQLITE_PATH", "ROAMINAL_DB_POSTGRES_DSN"} {
		t.Setenv(key, "")
	}
	c, err := FromEnv(t.TempDir())
	if err != nil || c.Driver != SQLite || !strings.HasSuffix(c.SQLitePath, "db/roaminal.sqlite3") {
		t.Fatalf("default config: %+v %v", c, err)
	}
	t.Setenv("ROAMINAL_DB_DRIVER", Postgres)
	if _, err := FromEnv(t.TempDir()); err == nil {
		t.Fatal("postgres without DSN accepted")
	}
	dsn := "postgres://user:secret@example.invalid/db?sslmode=disable"
	t.Setenv("ROAMINAL_DB_POSTGRES_DSN", dsn)
	if _, err := FromEnv(t.TempDir()); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("TLS or DSN redaction failure: %v", err)
	}
	t.Setenv("ROAMINAL_DB_POSTGRES_DSN", "postgres://user:secret@example.invalid/db?sslmode=verify-full")
	if _, err := FromEnv(t.TempDir()); err != nil {
		t.Fatalf("valid postgres config: %v", err)
	}
	t.Setenv("ROAMINAL_DB_SQLITE_PATH", "/tmp/inactive.sqlite3")
	if _, err := FromEnv(t.TempDir()); err == nil {
		t.Fatal("accepted inactive driver path")
	}
}
