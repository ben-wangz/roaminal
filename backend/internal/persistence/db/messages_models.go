package db

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errMessageRepository = errors.New("message repository unavailable")

const messageStateID = 1

// Fixed-width decimal keeps unsigned counters sortable in both engines without
// reducing the domain's uint64 range to the signed SQL integer range.
type messageCounter uint64

func (n messageCounter) Value() (driver.Value, error) { return fmt.Sprintf("%020d", n), nil }

func (n *messageCounter) Scan(value any) error {
	var text string
	switch value := value.(type) {
	case string:
		text = value
	case []byte:
		text = string(value)
	}
	parsed, err := strconv.ParseUint(text, 10, 64)
	if err != nil || len(text) != 20 {
		return errMessageRepository
	}
	*n = messageCounter(parsed)
	return nil
}

type messageStateRow struct {
	ID                  int            `gorm:"primaryKey;autoIncrement:false"`
	LatestSequence      messageCounter `gorm:"size:20;not null"`
	Revision            messageCounter `gorm:"size:20;not null"`
	ReadThroughSequence messageCounter `gorm:"size:20;not null"`
}

func (messageStateRow) TableName() string { return "message_state" }

type messageRow struct {
	Sequence          messageCounter      `gorm:"primaryKey;autoIncrement:false;size:20;not null"`
	IDDigest          []byte              `gorm:"not null;uniqueIndex"`
	IdempotencyDigest []byte              `gorm:"not null;uniqueIndex"`
	ReceivedSecond    int64               `gorm:"not null;index:idx_messages_received"`
	ReceivedNano      int                 `gorm:"not null;index:idx_messages_received"`
	Draft             domain.MessageDraft `gorm:"serializer:json;not null"`
}

func (messageRow) TableName() string { return "messages" }

func newMessageRow(record domain.MessageRecord) messageRow {
	return messageRow{
		Sequence: messageCounter(record.Sequence), IDDigest: stringDigest(record.MessageID),
		IdempotencyDigest: stringDigest(record.IdempotencyKey), Draft: record.MessageDraft,
		ReceivedSecond: record.ReceivedAt.Unix(), ReceivedNano: record.ReceivedAt.Nanosecond(),
	}
}

func (row messageRow) record() domain.MessageRecord {
	return domain.MessageRecord{Sequence: uint64(row.Sequence), MessageDraft: row.Draft}
}

func lockMessageState(tx *gorm.DB) (messageStateRow, error) {
	var state messageStateRow
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&state, messageStateID).Error
	if err != nil || state.ReadThroughSequence > state.LatestSequence {
		return messageStateRow{}, errMessageRepository
	}
	return state, nil
}

func saveMessageState(tx *gorm.DB, state messageStateRow) error {
	result := tx.Model(&state).Select("LatestSequence", "Revision", "ReadThroughSequence").Updates(&state)
	if result.Error != nil || result.RowsAffected != 1 {
		return errMessageRepository
	}
	return nil
}

func reviseMessageState(tx *gorm.DB, state *messageStateRow) error {
	if state.Revision == ^messageCounter(0) {
		return errors.New("message revision exhausted")
	}
	state.Revision++
	return saveMessageState(tx, *state)
}

func readMessageState(tx *gorm.DB, state messageStateRow) (domain.MessageState, error) {
	var unread int64
	if err := tx.Model(&messageRow{}).Where(clause.Gt{Column: "sequence", Value: state.ReadThroughSequence}).Count(&unread).Error; err != nil {
		return domain.MessageState{}, errMessageRepository
	}
	return domain.MessageState{Revision: uint64(state.Revision), LatestSequence: uint64(state.LatestSequence),
		ReadThroughSequence: uint64(state.ReadThroughSequence), UnreadCount: int(unread)}, nil
}

func pruneExpiredMessages(tx *gorm.DB, cutoff time.Time) (bool, error) {
	expired := clause.Or(
		clause.Lt{Column: "received_second", Value: cutoff.Unix()},
		clause.And(clause.Eq{Column: "received_second", Value: cutoff.Unix()},
			clause.Lt{Column: "received_nano", Value: cutoff.Nanosecond()}),
	)
	result := tx.Where(expired).Delete(&messageRow{})
	if result.Error != nil {
		return false, errMessageRepository
	}
	return result.RowsAffected > 0, nil
}

var newestMessagesFirst = clause.OrderByColumn{Column: clause.Column{Name: "sequence"}, Desc: true}

func trimMessages(tx *gorm.DB) error {
	// Reserve room for a new message. The removed ID can be reused by the
	// incoming record, just as when the file prunes before checking uniqueness.
	var oldest messageRow
	err := tx.Select("sequence").Order(newestMessagesFirst).Offset(persistence.MaxStoredMessages - 1).Take(&oldest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil || tx.Where(clause.Lte{Column: "sequence", Value: oldest.Sequence}).Delete(&messageRow{}).Error != nil {
		return errMessageRepository
	}
	return nil
}

func findMessage(tx *gorm.DB, column, value string) (messageRow, bool, error) {
	var row messageRow
	err := tx.Where(clause.Eq{Column: column, Value: stringDigest(value)}).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return messageRow{}, false, nil
	}
	if err != nil {
		return messageRow{}, false, errMessageRepository
	}
	return row, true, nil
}

// importMessages creates an exact legacy snapshot; use the caller's transaction
// so a malformed snapshot or any other import failure rolls back everything.
func importMessages(tx *gorm.DB, state domain.MessageState, records []domain.MessageRecord) error {
	if state.ReadThroughSequence > state.LatestSequence || len(records) > persistence.MaxStoredMessages {
		return errors.New("invalid message snapshot")
	}
	current, err := lockMessageState(tx)
	if err != nil {
		return err
	}
	if current.LatestSequence != 0 || current.Revision != 0 || current.ReadThroughSequence != 0 {
		return errors.New("message repository is not empty")
	}
	if err := requireEmptyRepository(tx, &messageRow{}); err != nil {
		return err
	}
	rows := make([]messageRow, 0, len(records))
	var previous uint64
	for _, record := range records {
		if record.Sequence <= previous || record.Sequence > state.LatestSequence {
			return errors.New("message sequence is not monotonic")
		}
		if err := persistence.ValidateMessageDraft(record.MessageDraft); err != nil {
			return err
		}
		previous = record.Sequence
		rows = append(rows, newMessageRow(record))
	}
	if len(rows) > 0 && tx.CreateInBatches(&rows, 100).Error != nil {
		return errMessageRepository
	}
	current.LatestSequence = messageCounter(state.LatestSequence)
	current.Revision = messageCounter(state.Revision)
	current.ReadThroughSequence = messageCounter(state.ReadThroughSequence)
	return saveMessageState(tx, current)
}
