package db

import (
	"context"
	"errors"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/agent"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const agentOptionsTimeout = 5 * time.Second

var errAgentRepository = errors.New("Agent repository unavailable")

// AgentRepository stores one independently updatable projection per endpoint.
type AgentRepository struct{ db *gorm.DB }

var (
	_ ports.AgentRepository           = (*AgentRepository)(nil)
	_ ports.AgentProjectionRepository = (*AgentRepository)(nil)
)

func (s *Store) AgentRepository() *AgentRepository { return &AgentRepository{db: s.DB} }

func (r *AgentRepository) Available() bool { return r.Err() == nil }

func (r *AgentRepository) Err() error {
	ctx, cancel := context.WithTimeout(context.Background(), agentOptionsTimeout)
	defer cancel()
	var keys []string
	if err := r.db.WithContext(ctx).Model(&agentEndpointRow{}).Limit(1).Pluck("key", &keys).Error; err != nil {
		return errAgentRepository
	}
	return nil
}

func (r *AgentRepository) Snapshot() map[string]domain.AgentEndpointRecord {
	ctx, cancel := context.WithTimeout(context.Background(), agentOptionsTimeout)
	defer cancel()
	var rows []agentEndpointRow
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil
	}
	out := make(map[string]domain.AgentEndpointRecord, len(rows))
	for _, row := range rows {
		value, err := row.record()
		if err != nil {
			return nil
		}
		out[row.Key] = value
	}
	return out
}

func (r *AgentRepository) Get(key string) (domain.AgentEndpointRecord, bool) {
	if !agent.ValidEndpointKey(key) {
		return domain.AgentEndpointRecord{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), agentOptionsTimeout)
	defer cancel()
	var row agentEndpointRow
	if err := r.db.WithContext(ctx).Where(&agentEndpointRow{Key: key}).Take(&row).Error; err != nil {
		return domain.AgentEndpointRecord{}, false
	}
	value, err := row.record()
	return value, err == nil
}

func (r *AgentRepository) Update(key string, fn func(*domain.AgentEndpointRecord) error) error {
	if fn == nil {
		return errAgentRecord
	}
	return r.updateEndpoint(key, func(_ *gorm.DB, record *domain.AgentEndpointRecord) error {
		return fn(record)
	})
}

func (r *AgentRepository) UpdateProjection(key string, update ports.AgentProjectionUpdate) (domain.MessageRecord, bool, error) {
	if update == nil {
		return domain.MessageRecord{}, false, errAgentRecord
	}
	var record domain.MessageRecord
	var duplicate bool
	err := r.updateEndpoint(key, func(tx *gorm.DB, endpoint *domain.AgentEndpointRecord) error {
		draft, err := update(endpoint)
		if err != nil || draft == nil {
			return err
		}
		record, duplicate, err = appendMessage(tx, *draft)
		return err
	})
	if err != nil {
		return domain.MessageRecord{}, false, err
	}
	return record, duplicate, nil
}

func (r *AgentRepository) updateEndpoint(key string, update func(*gorm.DB, *domain.AgentEndpointRecord) error) error {
	if !agent.ValidEndpointKey(key) {
		return errAgentRecord
	}
	ctx, cancel := context.WithTimeout(context.Background(), agentOptionsTimeout)
	defer cancel()
	var callbackErr error
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := lockAgentEndpoint(tx, key)
		if err != nil {
			return err
		}
		value, err := row.record()
		if err != nil {
			return err
		}
		if value.Targets == nil {
			value.Targets = make(map[string]domain.AgentTargetState)
		}
		if callbackErr = update(tx, &value); callbackErr != nil {
			return callbackErr
		}
		next, err := encodeAgentEndpoint(key, value)
		if err != nil {
			return err
		}
		if result := tx.Model(&row).Update("payload", next.Payload); result.Error != nil || result.RowsAffected != 1 {
			return errAgentRepository
		}
		return nil
	}, repositoryTxOptions(r.db)...)
	if callbackErr != nil {
		return callbackErr
	}
	if errors.Is(err, errAgentRecord) {
		return errAgentRecord
	}
	if err != nil {
		return errAgentRepository
	}
	return nil
}

// Insert before locking so competing writers also serialize on absent keys.
// SQLite transactions take a database write lock; PostgreSQL locks just the row.
func lockAgentEndpoint(tx *gorm.DB, key string) (agentEndpointRow, error) {
	initial := agentEndpointRow{Key: key, FormatVersion: agentEndpointVersion, Payload: []byte("{}")}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&initial).Error; err != nil {
		return agentEndpointRow{}, err
	}
	var row agentEndpointRow
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(&agentEndpointRow{Key: key}).Take(&row).Error
	return row, err
}

// ImportAgentEndpoint creates an offline-import record under its canonical key
// and never replaces an existing projection. It can run in the caller's transaction.
func (s *Store) ImportAgentEndpoint(ctx context.Context, endpoint domain.AgentEndpointRecord) error {
	identity, err := agent.NormalizeEndpoint(ports.EffectiveEndpoint{User: endpoint.User, Host: endpoint.Host, Port: endpoint.Port})
	if err != nil {
		return errAgentRecord
	}
	return s.ImportAgentEndpointWithKey(ctx, identity.Key, endpoint)
}

// ImportAgentEndpointWithKey retains the original binding even for a partial
// legacy record whose endpoint identity has not been populated yet.
func (s *Store) ImportAgentEndpointWithKey(ctx context.Context, key string, endpoint domain.AgentEndpointRecord) error {
	row, err := encodeAgentEndpoint(key, endpoint)
	if err != nil {
		return err
	}
	result := s.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if result.Error != nil {
		return errAgentRepository
	}
	if result.RowsAffected != 1 {
		return errors.New("Agent endpoint already exists")
	}
	return nil
}
