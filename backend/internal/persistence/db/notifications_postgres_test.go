package db

import "testing"

func TestPostgresMessageRepositoryConcurrentStores(t *testing.T) {
	s, c := postgresStore(t)
	testMessageConcurrency(t, []*Store{s, reopenMessageNotificationStore(t, c)})
}
