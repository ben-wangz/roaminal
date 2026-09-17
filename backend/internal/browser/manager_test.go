package browser

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/config"
)

type recordingWriter struct {
	mu     sync.Mutex
	lines  []string
	closed bool
}

func (w *recordingWriter) Write(value []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lines = append(w.lines, string(value))
	return len(value), nil
}

func (w *recordingWriter) Close() error {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
	return nil
}

func (w *recordingWriter) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.lines)
}

func newTestManager(runtime *process) *Manager {
	m := New(config.Config{})
	m.process = runtime
	return m
}

func newTestViewer(runtime *process, clientID string) *viewer {
	return &viewer{clientID: clientID, runtime: runtime, events: make(chan []byte, 32), done: make(chan struct{})}
}

func newTestRequest(clientID string) *http.Request {
	return httptest.NewRequest("GET", "/api/ws/browser?clientId="+clientID, nil)
}

func readEvent(t *testing.T, current *viewer) map[string]any {
	t.Helper()
	select {
	case data := <-current.events:
		var event map[string]any
		if err := json.Unmarshal(data, &event); err != nil {
			t.Fatalf("decode browser event: %v", err)
		}
		return event
	default:
		t.Fatal("expected browser event")
		return nil
	}
}

func TestResizeOwnershipAndTakeover(t *testing.T) {
	writer := &recordingWriter{}
	runtime := &process{stdin: writer, generation: "generation-1", viewport: viewportSize{Width: 1280, Height: 720}}
	m := newTestManager(runtime)
	first := newTestViewer(runtime, "client-one")
	second := newTestViewer(runtime, "client-two")
	m.addViewer(first)
	m.addViewer(second)
	readEvent(t, first)
	readEvent(t, second)

	firstResize := `{"type":"resize","clientId":"client-one","generation":"generation-1","width":900,"height":600,"primaryIntent":true}`
	if err := m.command(first, []byte(firstResize)); err != nil {
		t.Fatalf("first resize: %v", err)
	}
	if m.primary != "client-one" {
		t.Fatalf("primary = %q, want client-one", m.primary)
	}
	if writer.count() != 1 {
		t.Fatalf("worker writes = %d, want 1", writer.count())
	}
	if err := m.command(first, []byte(firstResize)); err != nil {
		t.Fatalf("duplicate resize: %v", err)
	}
	if writer.count() != 1 {
		t.Fatalf("duplicate resize reached worker; writes = %d", writer.count())
	}
	for {
		event := readEvent(t, first)
		if event["type"] == "resize_accepted" {
			break
		}
	}
	for {
		event := readEvent(t, second)
		if event["type"] == "primary" {
			if event["primary"] != false {
				t.Fatalf("second primary event = %v, want false", event["primary"])
			}
			break
		}
	}

	secondResize := `{"type":"resize","clientId":"client-two","generation":"generation-1","width":1000,"height":700,"primaryIntent":true}`
	if err := m.command(second, []byte(secondResize)); err != nil {
		t.Fatalf("non-primary resize should be reported, got transport error: %v", err)
	}
	rejected := readEvent(t, second)
	for rejected["type"] != "resize_rejected" {
		rejected = readEvent(t, second)
	}
	if rejected["code"] != "not_primary_client" {
		t.Fatalf("rejection code = %v, want not_primary_client", rejected["code"])
	}
	if writer.count() != 1 {
		t.Fatalf("rejected resize reached worker; writes = %d", writer.count())
	}

	takeover := `{"type":"resize","clientId":"client-two","generation":"generation-1","width":1000,"height":700,"primaryIntent":true,"takeover":true}`
	if err := m.command(second, []byte(takeover)); err != nil {
		t.Fatalf("takeover resize: %v", err)
	}
	if m.primary != "client-two" {
		t.Fatalf("primary after takeover = %q, want client-two", m.primary)
	}
	if writer.count() != 2 {
		t.Fatalf("worker writes after takeover = %d, want 2", writer.count())
	}
	accepted := readEvent(t, second)
	for accepted["type"] != "resize_accepted" {
		accepted = readEvent(t, second)
	}
	if accepted["takeover"] != true || accepted["primary"] != true {
		t.Fatalf("takeover response = %#v", accepted)
	}
}

func TestRemovingPrimaryReleasesOwnership(t *testing.T) {
	writer := &recordingWriter{}
	runtime := &process{stdin: writer, generation: "generation-1", viewport: viewportSize{Width: 1280, Height: 720}}
	m := newTestManager(runtime)
	current := newTestViewer(runtime, "client-one")
	m.addViewer(current)
	readEvent(t, current)
	if err := m.command(current, []byte(`{"type":"resize","clientId":"client-one","generation":"generation-1","width":800,"height":500,"primaryIntent":true}`)); err != nil {
		t.Fatalf("resize: %v", err)
	}
	if m.primary != "client-one" {
		t.Fatalf("primary = %q, want client-one", m.primary)
	}
	m.removeViewer(current)
	if m.primary != "" {
		t.Fatalf("primary after disconnect = %q, want empty", m.primary)
	}
	second := newTestViewer(runtime, "client-two")
	m.addViewer(second)
	state := readEvent(t, second)
	if state["primary"] != false {
		t.Fatalf("new viewer primary state = %v, want false", state["primary"])
	}
	if err := m.command(second, []byte(`{"type":"resize","clientId":"client-two","generation":"generation-1","width":900,"height":600,"primaryIntent":true}`)); err != nil {
		t.Fatalf("released-owner resize should be reported, got transport error: %v", err)
	}
	rejected := readEvent(t, second)
	if rejected["code"] != "not_primary_client" {
		t.Fatalf("released-owner rejection code = %v", rejected["code"])
	}
	if writer.count() != 2 {
		t.Fatalf("released-owner resize reached worker; writes = %d", writer.count())
	}
}

func TestStaleGenerationDoesNotReachWorker(t *testing.T) {
	writer := &recordingWriter{}
	runtime := &process{stdin: writer, generation: "generation-current", viewport: viewportSize{Width: 1280, Height: 720}}
	m := newTestManager(runtime)
	current := newTestViewer(runtime, "client-one")
	m.addViewer(current)
	readEvent(t, current)
	if err := m.command(current, []byte(`{"type":"resize","clientId":"client-one","generation":"generation-old","width":800,"height":500,"primaryIntent":true}`)); err != nil {
		t.Fatalf("stale resize should be reported, got transport error: %v", err)
	}
	event := readEvent(t, current)
	for event["type"] != "resize_rejected" {
		event = readEvent(t, current)
	}
	if event["code"] != "stale_browser_generation" {
		t.Fatalf("stale rejection code = %v", event["code"])
	}
	if writer.count() != 0 {
		t.Fatalf("stale resize reached worker; writes = %d", writer.count())
	}
}

func TestResizeWithoutPrimaryIntentDoesNotReachWorker(t *testing.T) {
	writer := &recordingWriter{}
	runtime := &process{stdin: writer, generation: "generation-current", viewport: viewportSize{Width: 1280, Height: 720}}
	m := newTestManager(runtime)
	current := newTestViewer(runtime, "client-one")
	m.addViewer(current)
	readEvent(t, current)
	if err := m.command(current, []byte(`{"type":"resize","clientId":"client-one","generation":"generation-current","width":800,"height":500,"primaryIntent":false}`)); err != nil {
		t.Fatalf("non-primary resize should be reported, got transport error: %v", err)
	}
	event := readEvent(t, current)
	for event["type"] != "resize_rejected" {
		event = readEvent(t, current)
	}
	if event["code"] != "primary_client_required" {
		t.Fatalf("rejection code = %v", event["code"])
	}
	if writer.count() != 0 {
		t.Fatalf("resize without primary intent reached worker; writes = %d", writer.count())
	}
}

func TestBrowserClientIDValidation(t *testing.T) {
	request := newTestRequest("client-123")
	if got := browserClientID(request); got != "client-123" {
		t.Fatalf("browserClientID = %q", got)
	}
	request = newTestRequest(strings.Repeat("x", 129))
	if got := browserClientID(request); got == strings.Repeat("x", 129) {
		t.Fatal("oversized client ID was accepted")
	}
}

func TestNormalizeViewportBounds(t *testing.T) {
	if got := normalizeViewport(viewportSize{Width: -10, Height: 0}); got != (viewportSize{Width: 1, Height: 1}) {
		t.Fatalf("lower viewport bounds = %#v", got)
	}
	if got := normalizeViewport(viewportSize{Width: 5000, Height: 3000}); got != (viewportSize{Width: 3840, Height: 2160}) {
		t.Fatalf("upper viewport bounds = %#v", got)
	}
}
