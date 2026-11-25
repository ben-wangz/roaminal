package db

import (
	"errors"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ ports.MessageRepository = (*Store)(nil)

func (s *Store) messageTransaction(run func(*gorm.DB) error) error {
	var operationErr error
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		operationErr = run(tx)
		return operationErr
	}, repositoryTxOptions(s.DB)...)
	if operationErr != nil {
		return operationErr
	}
	if err != nil {
		return errMessageRepository
	}
	return nil
}

// The singleton lock orders all mutations and makes state, unread counts and
// page rows a single snapshot. SQLite takes its writer lock at BEGIN IMMEDIATE.
func (s *Store) withMessageState(run func(*gorm.DB, messageStateRow) error) error {
	return s.messageTransaction(func(tx *gorm.DB) error {
		state, err := lockMessageState(tx)
		if err != nil {
			return err
		}
		return run(tx, state)
	})
}

func (s *Store) AppendMessage(draft domain.MessageDraft) (domain.MessageRecord, bool, error) {
	var record domain.MessageRecord
	var duplicate bool
	err := s.messageTransaction(func(tx *gorm.DB) error {
		var err error
		record, duplicate, err = appendMessage(tx, draft)
		return err
	})
	if err != nil {
		return domain.MessageRecord{}, false, err
	}
	return record, duplicate, nil
}

// appendMessage participates in the caller's transaction. Agent projections
// acquire the endpoint row lock first; other message operations only lock state.
func appendMessage(tx *gorm.DB, draft domain.MessageDraft) (domain.MessageRecord, bool, error) {
	if err := persistence.ValidateMessageDraft(draft); err != nil {
		return domain.MessageRecord{}, false, err
	}
	state, err := lockMessageState(tx)
	if err != nil {
		return domain.MessageRecord{}, false, err
	}
	cutoff := time.Now().UTC().Add(-persistence.MessageMaxAge)
	pruned, err := pruneExpiredMessages(tx, cutoff)
	if err != nil {
		return domain.MessageRecord{}, false, err
	}
	row, exists, err := findMessage(tx, "idempotency_digest", draft.IdempotencyKey)
	if err != nil {
		return domain.MessageRecord{}, false, err
	}
	if exists {
		if row.Draft.IdempotencyKey != draft.IdempotencyKey {
			return domain.MessageRecord{}, false, errMessageRepository
		}
		if pruned {
			err = reviseMessageState(tx, &state)
		}
		return row.record(), true, err
	}
	if state.LatestSequence == ^messageCounter(0) {
		return domain.MessageRecord{}, false, errors.New("message sequence exhausted")
	}
	state.LatestSequence++
	draft.ConnectionInstanceIDs = append([]string(nil), draft.ConnectionInstanceIDs...)
	draft.ConnectionDefinitionIDs = append([]string(nil), draft.ConnectionDefinitionIDs...)
	record := domain.MessageRecord{Sequence: uint64(state.LatestSequence), MessageDraft: draft}
	// Late arrivals still consume a sequence/revision and return a record,
	// but do not occupy retention space or an idempotency key.
	if !draft.ReceivedAt.Before(cutoff) {
		if err := trimMessages(tx); err != nil {
			return domain.MessageRecord{}, false, err
		}
		row := newMessageRow(record)
		if result := tx.Create(&row); result.Error != nil || result.RowsAffected != 1 {
			return domain.MessageRecord{}, false, errMessageRepository
		}
	}
	return record, false, reviseMessageState(tx, &state)
}

func (s *Store) ListMessages(limit int, before uint64) (domain.MessagePage, error) {
	var page domain.MessagePage
	err := s.withMessageState(func(tx *gorm.DB, state messageStateRow) error {
		if before > uint64(state.LatestSequence) {
			return ports.ErrMessageCursorInvalid
		}
		if limit < 1 || limit > 100 {
			return errors.New("invalid message limit")
		}
		snapshot, err := readMessageState(tx, state)
		if err != nil {
			return err
		}
		query := tx.Order(newestMessagesFirst).Limit(limit + 1)
		if before != 0 {
			query = query.Where(clause.Lt{Column: "sequence", Value: messageCounter(before)})
		}
		var rows []messageRow
		if err := query.Find(&rows).Error; err != nil {
			return errMessageRepository
		}
		page = domain.MessagePage{Revision: snapshot.Revision, LatestSequence: snapshot.LatestSequence,
			ReadThroughSequence: snapshot.ReadThroughSequence, UnreadCount: snapshot.UnreadCount}
		if len(rows) > limit {
			rows = rows[:limit]
			page.HasMore = true
			page.NextBefore = uint64(rows[limit-1].Sequence)
		}
		for _, row := range rows {
			page.Messages = append(page.Messages, row.record())
		}
		return nil
	})
	if err != nil {
		return domain.MessagePage{}, err
	}
	return page, nil
}

func (s *Store) MessageState() (domain.MessageState, error) {
	return s.changeMessageState(nil)
}

func (s *Store) MarkMessagesReadThrough(sequence uint64) (domain.MessageState, error) {
	return s.changeMessageState(func(_ *gorm.DB, state *messageStateRow) (bool, error) {
		read := min(messageCounter(sequence), state.LatestSequence)
		if read <= state.ReadThroughSequence {
			return false, nil
		}
		state.ReadThroughSequence = read
		return true, nil
	})
}

func (s *Store) DeleteMessage(messageID string) (domain.MessageState, bool, error) {
	var deleted bool
	state, err := s.changeMessageState(func(tx *gorm.DB, _ *messageStateRow) (bool, error) {
		row, exists, err := findMessage(tx, "id_digest", messageID)
		if err != nil || !exists {
			return false, err
		}
		if row.Draft.MessageID != messageID || tx.Delete(&row).Error != nil {
			return false, errMessageRepository
		}
		deleted = true
		return true, nil
	})
	return state, deleted && err == nil, err
}

func (s *Store) ClearMessages() (domain.MessageState, int, error) {
	var deleted int
	state, err := s.changeMessageState(func(tx *gorm.DB, state *messageStateRow) (bool, error) {
		result := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&messageRow{})
		if result.Error != nil {
			return false, errMessageRepository
		}
		deleted = int(result.RowsAffected)
		if deleted == 0 {
			return false, nil
		}
		state.ReadThroughSequence = state.LatestSequence
		return true, nil
	})
	if err != nil {
		return domain.MessageState{}, 0, err
	}
	return state, deleted, nil
}

func (s *Store) changeMessageState(change func(*gorm.DB, *messageStateRow) (bool, error)) (domain.MessageState, error) {
	var snapshot domain.MessageState
	err := s.withMessageState(func(tx *gorm.DB, state messageStateRow) error {
		if change != nil {
			changed, err := change(tx, &state)
			if err != nil {
				return err
			}
			if changed {
				if err := reviseMessageState(tx, &state); err != nil {
					return err
				}
			}
		}
		var err error
		snapshot, err = readMessageState(tx, state)
		return err
	})
	if err != nil {
		return domain.MessageState{}, err
	}
	return snapshot, nil
}
