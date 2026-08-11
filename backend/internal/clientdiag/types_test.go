package clientdiag

import (
	"fmt"
	"testing"
	"time"
)

func TestBatchValidationAndRedaction(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	batch := Batch{SchemaVersion: 1, PageID: testUUID(4), Events: []Event{{
		EventID: testUUID(5), OccurredAt: now.Format(time.RFC3339Nano), Kind: "console_error", Message: "password=secret",
		Operation: &Operation{Protocol: "websocket", Endpoint: "connection-instances", ConnectionInstanceID: testUUID(6), Phase: "handshake", DurationMs: 10, CloseCode: 1006},
	}}}
	events, err := batch.validate(now)
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Message != "password=[REDACTED]" {
		t.Fatalf("message = %q", events[0].Message)
	}
}

func TestBatchValidationRejectsUnknownOrStaleValues(t *testing.T) {
	now := time.Now().UTC()
	valid := Batch{SchemaVersion: 1, PageID: testUUID(4), Events: []Event{{EventID: testUUID(5), OccurredAt: now.Format(time.RFC3339Nano), Kind: "console_error", Message: "x"}}}
	for name, batch := range map[string]Batch{
		"schema":              {SchemaVersion: 2, PageID: valid.PageID, Events: valid.Events},
		"page":                {SchemaVersion: 1, PageID: "bad", Events: valid.Events},
		"event":               {SchemaVersion: 1, PageID: valid.PageID, Events: []Event{{EventID: "bad", OccurredAt: now.Format(time.RFC3339Nano), Kind: "console_error", Message: "x"}}},
		"stale":               {SchemaVersion: 1, PageID: valid.PageID, Events: []Event{{EventID: testUUID(5), OccurredAt: now.Add(-MaxPageAge - time.Second).Format(time.RFC3339Nano), Kind: "console_error", Message: "x"}}},
		"kind":                {SchemaVersion: 1, PageID: valid.PageID, Events: []Event{{EventID: testUUID(5), OccurredAt: now.Format(time.RFC3339Nano), Kind: "warning", Message: "x"}}},
		"path":                {SchemaVersion: 1, PageID: valid.PageID, Events: []Event{{EventID: testUUID(5), OccurredAt: now.Format(time.RFC3339Nano), Kind: "console_error", Message: "x", PagePath: "/?secret=1"}}},
		"operation":           {SchemaVersion: 1, PageID: valid.PageID, Events: []Event{{EventID: testUUID(5), OccurredAt: now.Format(time.RFC3339Nano), Kind: "console_error", Message: "x", Operation: &Operation{Protocol: "websocket", Endpoint: "bad"}}}},
		"resource-operation":  {SchemaVersion: 1, PageID: valid.PageID, Events: []Event{{EventID: testUUID(5), OccurredAt: now.Format(time.RFC3339Nano), Kind: "resource_error", Message: "x", Operation: &Operation{Protocol: "resource"}}}},
		"reserved-close-code": {SchemaVersion: 1, PageID: valid.PageID, Events: []Event{{EventID: testUUID(5), OccurredAt: now.Format(time.RFC3339Nano), Kind: "websocket_error", Message: "x", Operation: &Operation{Protocol: "websocket", Endpoint: "connection-instances", CloseCode: 1004}}}},
		"line-overflow":       {SchemaVersion: 1, PageID: valid.PageID, Events: []Event{{EventID: testUUID(5), OccurredAt: now.Format(time.RFC3339Nano), Kind: "console_error", Message: "x", Line: 0x80000000}}},
	} {
		if _, err := batch.validate(now); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}
}

func testUUID(value byte) string {
	return fmt.Sprintf("11111111-1111-4000-8000-%012x", value)
}
