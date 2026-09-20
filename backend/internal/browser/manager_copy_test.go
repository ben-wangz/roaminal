package browser

import (
	"encoding/json"
	"testing"
)

func TestCopyCommandRequiresCurrentPageIdentity(t *testing.T) {
	writer := &recordingWriter{}
	runtime := &process{stdin: writer, generation: "generation-1", viewport: viewportSize{Width: 1280, Height: 720}, pageStatus: "ready", pageGeneration: "page-one", pageOperation: 4, pageRevision: 4}
	m := newTestManager(runtime)
	current := newTestViewer(runtime, "client-one")
	m.addViewer(current)
	readEvent(t, current)
	if err := m.command(current, []byte(`{"type":"copy","clientId":"client-one","pageGeneration":"page-one","requestId":"copy-current"}`)); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if writer.count() != 1 {
		t.Fatalf("copy writes = %d, want 1", writer.count())
	}
	writer.mu.Lock()
	line := writer.lines[0]
	writer.mu.Unlock()
	var forwarded map[string]any
	if err := json.Unmarshal([]byte(line), &forwarded); err != nil {
		t.Fatalf("decode forwarded copy: %v", err)
	}
	if forwarded["type"] != "copy" || forwarded["pageGeneration"] != "page-one" || forwarded["pageOperation"] != float64(4) {
		t.Fatalf("forwarded copy = %#v", forwarded)
	}
}
