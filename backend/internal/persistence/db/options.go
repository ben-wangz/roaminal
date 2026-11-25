package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	errOptionsRepository = errors.New("connection options repository unavailable")
	errOptionsRecord     = errors.New("invalid connection options repository record")
)

type connectionOptionRow struct {
	Key         string `gorm:"primaryKey;size:64"`
	Alias       []byte `gorm:"not null"`
	Enabled     bool   `gorm:"not null"`
	SessionName []byte `gorm:"not null"`
	Pwd         string `gorm:"not null"`
}

func (connectionOptionRow) TableName() string { return "connection_options" }

// ConnectionOptions keeps a row per alias, independently of which aliases the
// current SSH config can read. Only explicit mutations change these rows.
type ConnectionOptions struct{ db *gorm.DB }

var _ connectionoptions.Repository = (*ConnectionOptions)(nil)

func (s *Store) ConnectionOptions() *ConnectionOptions { return &ConnectionOptions{db: s.DB} }

func (r *ConnectionOptions) Load(aliases map[string]bool) (connectionoptions.Collection, error) {
	ctx, cancel := context.WithTimeout(r.db.Statement.Context, agentOptionsTimeout)
	defer cancel()
	result := connectionoptions.Collection{Options: make(map[string]connectionoptions.Tmux)}
	var rows []connectionOptionRow
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		result.Source = connectionoptions.Source{Status: "unavailable", Reason: errOptionsRepository.Error()}
		return result, errOptionsRepository
	}
	if len(rows) == 0 {
		result.Source = connectionoptions.Source{Status: "missing"}
		return result, nil
	}
	result.Source = connectionoptions.Source{Status: "available", Readable: true, Writable: true}
	for _, row := range rows {
		alias := string(row.Alias)
		value, err := row.option()
		if err != nil {
			return connectionoptions.Collection{Source: connectionoptions.Source{Status: "invalid", Reason: errOptionsRecord.Error()}}, err
		}
		if aliases == nil || aliases[alias] {
			result.Options[alias] = value
		}
	}
	return result, nil
}

func (r *ConnectionOptions) UpdateAlias(alias string, value connectionoptions.Tmux, present bool) error {
	if !present {
		return r.RemoveAlias(alias)
	}
	if !value.Enabled {
		value.SessionName = ""
	}
	row, err := newConnectionOptionRow(alias, value)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.db.Statement.Context, agentOptionsTimeout)
	defer cancel()
	return writeConnectionOption(r.db.WithContext(ctx), row)
}

func (r *ConnectionOptions) RemoveAlias(alias string) error {
	ctx, cancel := context.WithTimeout(r.db.Statement.Context, agentOptionsTimeout)
	defer cancel()
	return removeConnectionOption(r.db.WithContext(ctx), alias)
}

func (r *ConnectionOptions) MoveAlias(oldAlias, newAlias string) error {
	return r.transferAlias(oldAlias, newAlias, true)
}

func (r *ConnectionOptions) CopyAlias(oldAlias, newAlias string) error {
	return r.transferAlias(oldAlias, newAlias, false)
}

func (r *ConnectionOptions) transferAlias(oldAlias, newAlias string, move bool) error {
	if oldAlias == newAlias {
		return nil
	}
	ctx, cancel := context.WithTimeout(r.db.Statement.Context, agentOptionsTimeout)
	defer cancel()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var source connectionOptionRow
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(&connectionOptionRow{Key: optionKey(oldAlias)}).Take(&source).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return errOptionsRepository
		}
		value, err := source.option()
		if err != nil {
			return err
		}
		row, err := newConnectionOptionRow(newAlias, value)
		if err != nil {
			return err
		}
		if err := writeConnectionOption(tx, row); err != nil || !move {
			return err
		}
		return removeConnectionOption(tx, oldAlias)
	})
	if err != nil && !errors.Is(err, errOptionsRecord) && !errors.Is(err, connectionoptions.ErrInvalidSessionName) {
		return errOptionsRepository
	}
	return err
}

// Replace is for verified offline import. The caller supplies all aliases,
// including stale entries; a nil map clears the options atomically.
func (r *ConnectionOptions) Replace(ctx context.Context, values map[string]connectionoptions.Tmux) error {
	rows := make([]connectionOptionRow, 0, len(values))
	for alias, value := range values {
		row, err := newConnectionOptionRow(alias, value)
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&connectionOptionRow{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.CreateInBatches(rows, 100).Error
	})
	if err != nil {
		return errOptionsRepository
	}
	return nil
}

func optionKey(alias string) string {
	digest := sha256.Sum256([]byte(alias))
	return hex.EncodeToString(digest[:])
}

func newConnectionOptionRow(alias string, value connectionoptions.Tmux) (connectionOptionRow, error) {
	value, err := connectionoptions.NormalizeAliasOption(alias, value)
	if err != nil {
		return connectionOptionRow{}, err
	}
	return connectionOptionRow{Key: optionKey(alias), Alias: []byte(alias), Enabled: value.Enabled, SessionName: []byte(value.SessionName), Pwd: value.Pwd}, nil
}

func (row connectionOptionRow) option() (connectionoptions.Tmux, error) {
	value, err := connectionoptions.NormalizeAliasOption(string(row.Alias), connectionoptions.Tmux{
		Enabled: row.Enabled, SessionName: string(row.SessionName), Pwd: row.Pwd,
	})
	if err != nil || row.Key != optionKey(string(row.Alias)) || row.Pwd != value.Pwd {
		return connectionoptions.Tmux{}, errOptionsRecord
	}
	return value, nil
}

func writeConnectionOption(tx *gorm.DB, row connectionOptionRow) error {
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, UpdateAll: true}).Create(&row).Error; err != nil {
		return errOptionsRepository
	}
	return nil
}

func removeConnectionOption(tx *gorm.DB, alias string) error {
	if err := tx.Delete(&connectionOptionRow{Key: optionKey(alias)}).Error; err != nil {
		return errOptionsRepository
	}
	return nil
}
