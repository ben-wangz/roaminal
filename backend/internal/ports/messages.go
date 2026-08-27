package ports

import (
	"errors"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

var ErrMessageCursorInvalid = errors.New("message cursor is invalid")

// MessageRepository is the durable message boundary. Implementations own
// serialization, retention, and synchronization of the global read cursor.
type MessageRepository interface {
	AppendMessage(domain.MessageDraft) (domain.MessageRecord, bool, error)
	ListMessages(limit int, before uint64) (domain.MessagePage, error)
	MarkMessagesReadThrough(uint64) (domain.MessageState, error)
	MessageState() (domain.MessageState, error)
	DeleteMessage(string) (domain.MessageState, bool, error)
	ClearMessages() (domain.MessageState, int, error)
}

// MessageAppender is the small dependency used by Agent telemetry. Keeping
// it separate from the query methods prevents Agent code from depending on
// HTTP or browser presentation concerns.
type MessageAppender interface {
	AppendMessage(domain.MessageDraft) (domain.MessageRecord, bool, error)
}

// MessageNotifier receives only newly persisted records. Implementations must
// return quickly and perform best-effort delivery asynchronously.
type MessageNotifier interface {
	Notify(domain.MessageRecord)
}
