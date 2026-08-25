package server

import (
	"github.com/ben-wangz/roaminal/backend/internal/auth"
	"github.com/ben-wangz/roaminal/backend/internal/clock"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/identity"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/random"
)

func newServerTestAuth(cfg config.Config, store *persistence.Store) (*auth.Manager, error) {
	repositories := persistence.NewRepositories(store)
	return auth.NewWithRepositories(cfg, repositories.Auth, auth.Dependencies{Clock: clock.System{}, IDs: identity.UUIDGenerator{Random: random.CryptoSource{}}, Random: random.CryptoSource{}})
}
