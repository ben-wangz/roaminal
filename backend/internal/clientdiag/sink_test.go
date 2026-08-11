package clientdiag

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type testLogger struct{ bytes.Buffer }

func (l *testLogger) Printf(format string, args ...any) {
	_, _ = fmt.Fprintf(&l.Buffer, format, args...)
}

func TestSinkLogsRedactedRecordsAndDeduplicatesIDs(t *testing.T) {
	logger := &testLogger{}
	sink := New("", "0.2.11", "boot", logger)
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	batch := Batch{SchemaVersion: 1, PageID: testUUID(4), DroppedCount: 2, Events: []Event{{EventID: testUUID(5), OccurredAt: now.Format(time.RFC3339Nano), Kind: "console_error", Message: "password=secret\nnext"}}}
	if err := sink.acceptAt(now, "auth-session", "Bearer user-agent", batch); err != nil {
		t.Fatal(err)
	}
	if err := sink.acceptAt(now, "auth-session", "Bearer user-agent", batch); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(logger.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("log lines = %d, want 1: %q", len(lines), logger.String())
	}
	if strings.Contains(logger.String(), "secret") || strings.Contains(logger.String(), "Bearer user-agent") {
		t.Fatalf("log contains unredacted data: %q", logger.String())
	}
	var record StoredRecord
	if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[0], "client_diagnostic=")), &record); err != nil {
		t.Fatal(err)
	}
	if record.DroppedCount != 2 || record.AuthSessionID != "auth-session" || record.RuntimeVersion != "0.2.11" {
		t.Fatalf("unexpected record: %+v", record)
	}
}

func TestSinkRateLimitsNewEventsPerSession(t *testing.T) {
	sink := New("", "v", "b", &testLogger{})
	now := time.Now().UTC()
	for start := 0; start < 120; start += MaxEventsPerBatch {
		count := MaxEventsPerBatch
		if remaining := 120 - start; remaining < count {
			count = remaining
		}
		if err := sink.acceptAt(now, "session", "", batchWithEvents(now, start, count)); err != nil {
			t.Fatalf("event batch %d: %v", start, err)
		}
	}
	if err := sink.acceptAt(now, "session", "", batchWithEvents(now, 1000, 1)); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected rate limit, got %v", err)
	}
	if err := sink.acceptAt(now, "other-session", "", batchWithEvents(now, 1000, 1)); err != nil {
		t.Fatalf("other session should have an independent budget: %v", err)
	}
}

func batchWithEvents(now time.Time, start, count int) Batch {
	events := make([]Event, 0, count)
	for index := 0; index < count; index++ {
		events = append(events, Event{EventID: uuidForIndex(start + index), OccurredAt: now.Format(time.RFC3339Nano), Kind: "console_error", Message: "error"})
	}
	return Batch{SchemaVersion: 1, PageID: testUUID(4), Events: events}
}

func uuidForIndex(index int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012x", index+1)
}
