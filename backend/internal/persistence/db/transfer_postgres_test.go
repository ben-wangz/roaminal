package db

import "testing"

func TestPostgresDatabaseTransfer(t *testing.T) {
	t.Run("SQLiteToPostgreSQL", func(t *testing.T) {
		postgres, _ := postgresStore(t)
		sqlite, _ := sqliteStore(t)
		testDatabaseTransfer(t, sqlite, postgres)
	})
	t.Run("PostgreSQLToSQLite", func(t *testing.T) {
		postgres, _ := postgresStore(t)
		sqlite, _ := sqliteStore(t)
		testDatabaseTransfer(t, postgres, sqlite)
	})
}
