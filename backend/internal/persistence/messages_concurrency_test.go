package persistence

import (
	"sync"
	"testing"
	"time"
)

func TestMessageRepositorySerializesConcurrentMutations(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository := NewRepositories(store).Messages
	now := time.Now().UTC()
	for index := 1; index <= 40; index++ {
		if _, _, err := repository.AppendMessage(testMessageDraft(index, now)); err != nil {
			t.Fatal(err)
		}
	}

	const appended = 40
	start := make(chan struct{})
	results := make(chan error, appended+2)
	var group sync.WaitGroup
	group.Add(appended + 2)
	for index := 41; index <= 80; index++ {
		go func(index int) {
			defer group.Done()
			<-start
			_, _, appendErr := repository.AppendMessage(testMessageDraft(index, now))
			results <- appendErr
		}(index)
	}
	go func() {
		defer group.Done()
		<-start
		_, _, deleteErr := repository.DeleteMessage("message-001")
		results <- deleteErr
	}()
	go func() {
		defer group.Done()
		<-start
		_, _, clearErr := repository.ClearMessages()
		results <- clearErr
	}()
	close(start)
	group.Wait()
	close(results)
	for mutationErr := range results {
		if mutationErr != nil {
			t.Fatalf("concurrent message mutation failed: %v", mutationErr)
		}
	}

	page, err := repository.ListMessages(100, 0)
	if err != nil {
		t.Fatal(err)
	}
	state, err := repository.MessageState()
	if err != nil {
		t.Fatal(err)
	}
	if page.LatestSequence != 80 || state.LatestSequence != 80 || state.ReadThroughSequence > state.LatestSequence {
		t.Fatalf("concurrent mutations broke monotonic state: page=%+v state=%+v", page, state)
	}
	if len(page.Messages) > 80 {
		t.Fatalf("concurrent mutations created duplicate records: %d", len(page.Messages))
	}
}
