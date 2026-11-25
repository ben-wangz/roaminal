package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Set ROAMINAL_TEST_POSTGRES_DSN to a verify-full TLS URI for a disposable
// PostgreSQL database and ROAMINAL_TEST_POSTGRES_DISPOSABLE=1 to opt in. Only a
// randomly named test schema is created and dropped; no server version change.
func postgresStore(t *testing.T) (*Store, Config) {
	t.Helper()
	dsn := os.Getenv("ROAMINAL_TEST_POSTGRES_DSN")
	if dsn == "" || os.Getenv("ROAMINAL_TEST_POSTGRES_DISPOSABLE") != "1" {
		t.Skip("requires disposable PostgreSQL with verified TLS")
	}
	base := Config{Driver: Postgres, PostgresDSN: dsn}
	if err := base.Validate(); err != nil {
		t.Fatal("test PostgreSQL DSN must use verify-full TLS")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("connect to disposable PostgreSQL failed")
	}
	adminSQL, err := admin.DB()
	if err != nil {
		t.Fatal("obtain disposable PostgreSQL connection failed")
	}
	t.Cleanup(func() { _ = adminSQL.Close() })
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	name := "roaminal_test_" + hex.EncodeToString(suffix[:])
	if err := admin.Exec(`CREATE SCHEMA "` + name + `"`).Error; err != nil {
		t.Fatal("create disposable test schema failed")
	}
	t.Cleanup(func() {
		if err := admin.Exec(`DROP SCHEMA "` + name + `" CASCADE`).Error; err != nil {
			t.Error("remove disposable test schema failed")
		}
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal("invalid test DSN")
	}
	q := u.Query()
	q.Set("search_path", name)
	u.RawQuery = q.Encode()
	c := Config{Driver: Postgres, PostgresDSN: u.String()}
	s, err := Open(context.Background(), c)
	if err != nil {
		t.Fatal("open disposable PostgreSQL schema failed")
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, c
}

func TestPostgresRepositoriesAndSchema(t *testing.T) {
	s, c := postgresStore(t)
	testRepositoryContract(t, s)
	testWorkspaceUploadRepositories(t, s)
	testInstanceArchiveContract(t, s)
	testMessageRepository(t, s)
	testPushSubscriptionRepository(t, s)
	testNotificationPreferenceRepository(t, s)
	second, err := Open(context.Background(), c)
	if err != nil {
		t.Fatal("reopen PostgreSQL schema failed")
	}
	_ = second.Close()
}
