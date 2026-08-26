package persistence

import (
	"errors"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func (a *repositoryAdapter) AppendMessage(draft domain.MessageDraft) (domain.MessageRecord, bool, error) {
	a.store.messagesMu.Lock()
	defer a.store.messagesMu.Unlock()
	file, err := a.store.loadMessages()
	if err != nil {
		return domain.MessageRecord{}, false, a.store.markError(err)
	}
	if err := validateMessageDraft(draft); err != nil {
		return domain.MessageRecord{}, false, err
	}
	now := time.Now().UTC()
	pruned := pruneMessageFile(&file, now)
	for _, record := range file.Messages {
		if record.IdempotencyKey == draft.IdempotencyKey {
			if pruned {
				file.Revision++
				if err := a.store.saveMessages(file); err != nil {
					return domain.MessageRecord{}, false, err
				}
			}
			return record, true, nil
		}
	}
	file.LatestSequence++
	record := domain.MessageRecord{MessageDraft: cloneMessageDraft(draft), Sequence: file.LatestSequence}
	file.Messages = append(file.Messages, record)
	pruneMessageFile(&file, now)
	file.Revision++
	if err := a.store.saveMessages(file); err != nil {
		return domain.MessageRecord{}, false, err
	}
	return record, false, nil
}

func (a *repositoryAdapter) ListMessages(limit int, before uint64) (domain.MessagePage, error) {
	a.store.messagesMu.Lock()
	defer a.store.messagesMu.Unlock()
	file, err := a.store.loadMessages()
	if err != nil {
		return domain.MessagePage{}, a.store.markError(err)
	}
	if before > file.LatestSequence {
		return domain.MessagePage{}, ports.ErrMessageCursorInvalid
	}
	if limit < 1 || limit > 100 {
		return domain.MessagePage{}, errors.New("invalid message limit")
	}
	page := domain.MessagePage{Revision: file.Revision, LatestSequence: file.LatestSequence, ReadThroughSequence: file.ReadThroughSequence, UnreadCount: unreadMessageCount(file)}
	for index := len(file.Messages) - 1; index >= 0; index-- {
		record := file.Messages[index]
		if before != 0 && record.Sequence >= before {
			continue
		}
		page.Messages = append(page.Messages, cloneMessageRecord(record))
		if len(page.Messages) == limit {
			for older := index - 1; older >= 0; older-- {
				if before == 0 || file.Messages[older].Sequence < record.Sequence {
					page.HasMore = true
					break
				}
			}
			if page.HasMore {
				page.NextBefore = record.Sequence
			}
			break
		}
	}
	return page, nil
}

func (a *repositoryAdapter) MarkMessagesReadThrough(sequence uint64) (domain.MessageState, error) {
	a.store.messagesMu.Lock()
	defer a.store.messagesMu.Unlock()
	file, err := a.store.loadMessages()
	if err != nil {
		return domain.MessageState{}, a.store.markError(err)
	}
	if sequence > file.LatestSequence {
		sequence = file.LatestSequence
	}
	if sequence > file.ReadThroughSequence {
		file.ReadThroughSequence = sequence
		file.Revision++
		if err := a.store.saveMessages(file); err != nil {
			return domain.MessageState{}, err
		}
	}
	return messageState(file), nil
}

func (a *repositoryAdapter) MessageState() (domain.MessageState, error) {
	a.store.messagesMu.Lock()
	defer a.store.messagesMu.Unlock()
	file, err := a.store.loadMessages()
	if err != nil {
		return domain.MessageState{}, a.store.markError(err)
	}
	return messageState(file), nil
}

func messageState(file messageFile) domain.MessageState {
	return domain.MessageState{Revision: file.Revision, LatestSequence: file.LatestSequence, ReadThroughSequence: file.ReadThroughSequence, UnreadCount: unreadMessageCount(file)}
}

func unreadMessageCount(file messageFile) int {
	count := 0
	for _, record := range file.Messages {
		if record.Sequence > file.ReadThroughSequence {
			count++
		}
	}
	return count
}

func cloneMessageDraft(draft domain.MessageDraft) domain.MessageDraft {
	draft.ConnectionInstanceIDs = append([]string(nil), draft.ConnectionInstanceIDs...)
	return draft
}

func cloneMessageRecord(record domain.MessageRecord) domain.MessageRecord {
	record.MessageDraft = cloneMessageDraft(record.MessageDraft)
	return record
}
