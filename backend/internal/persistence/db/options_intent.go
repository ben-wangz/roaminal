package db

import (
	"context"
	"errors"

	"github.com/ben-wangz/roaminal/backend/internal/definition"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const mutationKey = "active"

var (
	errMutationPending       = errors.New("connection-definition mutation already pending")
	errMutationMissing       = errors.New("connection-definition mutation is no longer pending")
	errMutationRepository    = errors.New("connection-definition mutation repository unavailable")
	errInvalidMutationIntent = errors.New("invalid connection-definition mutation intent")
)

// DefinitionMutations resolves a single local SSH config's pending writes.
type DefinitionMutations struct{ db *gorm.DB }

var _ definition.MutationRepository = (*DefinitionMutations)(nil)

func (s *Store) DefinitionMutations() *DefinitionMutations { return &DefinitionMutations{db: s.DB} }

func agentOptionsModels() []any {
	return []any{&agentEndpointRow{}, &connectionOptionRow{}, &definitionMutationRow{}}
}

func (r *DefinitionMutations) Pending() (definition.MutationIntent, bool, error) {
	ctx, cancel := context.WithTimeout(r.db.Statement.Context, agentOptionsTimeout)
	defer cancel()
	var row definitionMutationRow
	err := r.db.WithContext(ctx).Where(&definitionMutationRow{Key: mutationKey}).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return definition.MutationIntent{}, false, nil
	}
	if err != nil {
		return definition.MutationIntent{}, false, errMutationRepository
	}
	intent, err := row.intent()
	return intent, err == nil, err
}

func (r *DefinitionMutations) Prepare(intent definition.MutationIntent) error {
	row, err := encodeDefinitionMutation(intent)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.db.Statement.Context, agentOptionsTimeout)
	defer cancel()
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if result.Error != nil {
		return errMutationRepository
	}
	if result.RowsAffected != 1 {
		return errMutationPending
	}
	return nil
}

func (r *DefinitionMutations) Resolve(id string, decide func() (bool, error)) error {
	if !validMutationID(id) {
		return errInvalidMutationIntent
	}
	ctx, cancel := context.WithTimeout(r.db.Statement.Context, agentOptionsTimeout)
	defer cancel()
	var decisionErr error
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		key := &definitionMutationRow{Key: mutationKey, ID: id}
		var row definitionMutationRow
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(key).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errMutationMissing
		}
		if err != nil {
			return err
		}
		intent, err := row.intent()
		if err != nil {
			return err
		}
		if decide != nil {
			apply, err := decide()
			if err != nil {
				decisionErr = err
				return err
			}
			if apply {
				if err := applyOptionUpdates(tx, intent.Updates); err != nil {
					return err
				}
			}
		}
		return tx.Where(key).Delete(&definitionMutationRow{}).Error
	})
	if decisionErr != nil {
		return decisionErr
	}
	if errors.Is(err, errMutationMissing) || errors.Is(err, errInvalidMutationIntent) {
		return err
	}
	if err != nil {
		return errMutationRepository
	}
	return nil
}

func applyOptionUpdates(tx *gorm.DB, updates []definition.OptionUpdate) error {
	for _, update := range updates {
		if update.Value == nil {
			if err := removeConnectionOption(tx, update.Alias); err != nil {
				return err
			}
			continue
		}
		row, err := newConnectionOptionRow(update.Alias, *update.Value)
		if err != nil {
			return err
		}
		if err := writeConnectionOption(tx, row); err != nil {
			return err
		}
	}
	return nil
}
