package db

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

func reopenMessageNotificationStore(t *testing.T, c Config) *Store {
	t.Helper()
	s, err := Open(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func runConcurrentRepositoryCalls(count int, run func(int)) {
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range count {
		wg.Go(func() {
			<-start
			run(i)
		})
	}
	close(start)
	wg.Wait()
}

func TestSQLiteMessageRepositoryConcurrentStores(t *testing.T) {
	s, c := sqliteStore(t)
	stores := []*Store{s, reopenMessageNotificationStore(t, c)}
	testMessageConcurrency(t, stores)
}

func testMessageConcurrency(t *testing.T, stores []*Store) {
	t.Helper()
	now := time.Now().UTC()
	draft := messageDraft(1, now)
	var inserted atomic.Int32
	runConcurrentRepositoryCalls(16, func(i int) {
		got, duplicate, err := stores[i%len(stores)].AppendMessage(draft)
		if err != nil || got.Sequence != 1 {
			t.Errorf("racing duplicate append = %+v, %t, %v", got, duplicate, err)
		} else if !duplicate {
			inserted.Add(1)
		}
	})
	if got := inserted.Load(); got != 1 {
		t.Fatalf("new duplicate drafts = %d, want 1", got)
	}

	const appends = 30
	runConcurrentRepositoryCalls(appends+8, func(i int) {
		s := stores[i%len(stores)]
		switch i {
		case appends:
			if _, _, err := s.DeleteMessage(draft.MessageID); err != nil {
				t.Errorf("concurrent delete: %v", err)
			}
		case appends + 1:
			if _, _, err := s.ClearMessages(); err != nil {
				t.Errorf("concurrent clear: %v", err)
			}
		case appends + 2:
			if _, err := s.MarkMessagesReadThrough(1); err != nil {
				t.Errorf("concurrent read cursor: %v", err)
			}
		default:
			if i < appends {
				if _, duplicate, err := s.AppendMessage(messageDraft(i+2, now)); err != nil || duplicate {
					t.Errorf("concurrent append: duplicate = %t, err = %v", duplicate, err)
				}
			} else {
				checkConsistentMessagePage(t, s)
			}
		}
	})
	page := checkConsistentMessagePage(t, stores[0])
	if page.LatestSequence != appends+1 || page.Revision < page.LatestSequence || page.Revision > page.LatestSequence+3 {
		t.Fatalf("concurrent state = %+v", page)
	}
}

func checkConsistentMessagePage(t *testing.T, s *Store) domain.MessagePage {
	t.Helper()
	page, err := s.ListMessages(100, 0)
	if err != nil || page.ReadThroughSequence > page.LatestSequence {
		t.Errorf("concurrent list = %+v, %v", page, err)
		return domain.MessagePage{}
	}
	var unread int
	for i, record := range page.Messages {
		if record.Sequence > page.LatestSequence || (i > 0 && page.Messages[i-1].Sequence <= record.Sequence) {
			t.Errorf("message order disagrees with page: %+v", page)
		}
		if record.Sequence > page.ReadThroughSequence {
			unread++
		}
	}
	if unread != page.UnreadCount {
		t.Errorf("page unread = %d, counted %d", page.UnreadCount, unread)
	}
	return page
}
