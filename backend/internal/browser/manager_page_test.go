package browser

import (
	"encoding/json"
	"testing"
)

func TestViewerReceivesAuthoritativePageSnapshotAndFrame(t *testing.T) {
	writer := &recordingWriter{}
	runtime := &process{stdin: writer, generation: "generation-1", viewport: viewportSize{Width: 1280, Height: 720}, pageStatus: "ready", pageGeneration: "page-one", pageRevision: 3, pageURL: "https://fixture.test/", pageTitle: "Fixture", latestFrame: []byte(`{"type":"frame","pageStatus":"ready","pageGeneration":"page-one","revision":3,"sequence":1,"width":1280,"height":720,"data":"AA=="}`)}
	m := newTestManager(runtime)
	first := newTestViewer(runtime, "client-one")
	second := newTestViewer(runtime, "client-two")
	m.addViewer(first)
	m.addViewer(second)
	for _, current := range []*viewer{first, second} {
		state := readEvent(t, current)
		if state["pageStatus"] != "ready" || state["pageGeneration"] != "page-one" || state["url"] != "https://fixture.test/" {
			t.Fatalf("snapshot = %#v", state)
		}
		frame := readEvent(t, current)
		if frame["type"] != "frame" || frame["pageGeneration"] != "page-one" {
			t.Fatalf("replayed frame = %#v", frame)
		}
	}
}

func TestCloseIsGlobalAndStaleCloseIsHarmless(t *testing.T) {
	writer := &recordingWriter{}
	runtime := &process{stdin: writer, generation: "generation-1", viewport: viewportSize{Width: 1280, Height: 720}, pageStatus: "ready", pageGeneration: "page-one", pageRevision: 1, pageURL: "https://fixture.test/"}
	m := newTestManager(runtime)
	first := newTestViewer(runtime, "client-one")
	second := newTestViewer(runtime, "client-two")
	m.addViewer(first)
	m.addViewer(second)
	readEvent(t, first)
	readEvent(t, second)
	if err := m.command(first, []byte(`{"type":"close","clientId":"client-one","pageGeneration":"page-one","requestId":"close-one"}`)); err != nil {
		t.Fatalf("close: %v", err)
	}
	if writer.count() != 1 {
		t.Fatalf("close writes = %d, want 1", writer.count())
	}
	for _, current := range []*viewer{first, second} {
		event := readEvent(t, current)
		if event["type"] != "state" || event["pageStatus"] != "closing" {
			t.Fatalf("closing event = %#v", event)
		}
	}
	m.broadcastEvent(runtime, []byte(`{"type":"closed","pageStatus":"closed","pageGeneration":"page-one","revision":2}`))
	for _, current := range []*viewer{first, second} {
		event := readEvent(t, current)
		if event["type"] != "closed" || event["pageStatus"] != "closed" {
			t.Fatalf("closed event = %#v", event)
		}
	}
	if runtime.latestFrame != nil || runtime.pageURL != "" {
		t.Fatalf("page cache was not cleared: %#v", runtime)
	}
	if err := m.command(second, []byte(`{"type":"close","clientId":"client-two","pageGeneration":"page-one","requestId":"close-duplicate"}`)); err != nil {
		t.Fatalf("duplicate close: %v", err)
	}
	if writer.count() != 1 {
		t.Fatalf("duplicate close reached worker; writes = %d", writer.count())
	}
	runtime.pageStatus = "ready"
	runtime.pageGeneration = "page-two"
	runtime.pageRevision = 3
	if err := m.command(second, []byte(`{"type":"close","clientId":"client-two","pageGeneration":"page-one","requestId":"close-stale"}`)); err != nil {
		t.Fatalf("stale close: %v", err)
	}
	if writer.count() != 1 {
		t.Fatalf("stale close reached worker; writes = %d", writer.count())
	}
}

func TestVisibilityDoesNotCloseThePage(t *testing.T) {
	writer := &recordingWriter{}
	runtime := &process{stdin: writer, generation: "generation-1", viewport: viewportSize{Width: 1280, Height: 720}, pageStatus: "ready", pageGeneration: "page-one"}
	m := newTestManager(runtime)
	current := newTestViewer(runtime, "client-one")
	m.addViewer(current)
	readEvent(t, current)
	if err := m.command(current, []byte(`{"type":"visibility","clientId":"client-one","visible":false}`)); err != nil {
		t.Fatalf("hide: %v", err)
	}
	if writer.count() != 0 {
		t.Fatalf("hide wrote worker command: %d", writer.count())
	}
	if err := m.command(current, []byte(`{"type":"visibility","clientId":"client-one","visible":true}`)); err != nil {
		t.Fatalf("show: %v", err)
	}
	if writer.count() == 0 {
		t.Fatal("show did not request worker visibility/sync")
	}
	writer.mu.Lock()
	for _, line := range writer.lines {
		if eventType([]byte(line)) == "close" {
			t.Fatalf("visibility sent close: %q", line)
		}
	}
	writer.mu.Unlock()
}

func TestLateWorkerEventsFromPreviousPageOperationAreIgnored(t *testing.T) {
	writer := &recordingWriter{}
	runtime := &process{stdin: writer, generation: "generation-1", viewport: viewportSize{Width: 1280, Height: 720}, pageStatus: "none"}
	m := newTestManager(runtime)
	current := newTestViewer(runtime, "client-one")
	m.addViewer(current)
	readEvent(t, current)
	if err := m.command(current, []byte(`{"type":"open","clientId":"client-one","url":"https://one.test/","requestId":"open-one"}`)); err != nil {
		t.Fatalf("open: %v", err)
	}
	if runtime.pageOperation != 1 {
		t.Fatalf("page operation after open = %d, want 1", runtime.pageOperation)
	}
	readEvent(t, current)
	if err := m.command(current, []byte(`{"type":"close","clientId":"client-one","pageGeneration":"`+runtime.pageGeneration+`","requestId":"close-one"}`)); err != nil {
		t.Fatalf("close: %v", err)
	}
	if runtime.pageOperation != 2 || runtime.pageStatus != "closing" {
		t.Fatalf("closing state = operation %d, status %q", runtime.pageOperation, runtime.pageStatus)
	}
	readEvent(t, current)
	m.broadcastEvent(runtime, []byte(`{"type":"state","pageStatus":"ready","pageGeneration":"`+runtime.pageGeneration+`","pageOperation":1,"revision":99,"url":"https://one.test/"}`))
	select {
	case event := <-current.events:
		t.Fatalf("late old-operation event was delivered: %q", event)
	default:
	}
	pageGeneration := runtime.pageGeneration
	m.broadcastEvent(runtime, []byte(`{"type":"closed","pageStatus":"closed","pageGeneration":"`+pageGeneration+`","pageOperation":2,"revision":3}`))
	closed := readEvent(t, current)
	if closed["type"] != "closed" || closed["pageStatus"] != "closed" {
		t.Fatalf("closed event = %#v", closed)
	}
}

func TestPageCommandsRequireCurrentPageIdentity(t *testing.T) {
	writer := &recordingWriter{}
	runtime := &process{stdin: writer, generation: "generation-1", viewport: viewportSize{Width: 1280, Height: 720}, pageStatus: "ready", pageGeneration: "page-one", pageOperation: 4, pageRevision: 4}
	m := newTestManager(runtime)
	current := newTestViewer(runtime, "client-one")
	m.addViewer(current)
	readEvent(t, current)
	if err := m.command(current, []byte(`{"type":"input","clientId":"client-one","pageGeneration":"page-old","requestId":"input-old","event":{"kind":"text","text":"stale"}}`)); err != nil {
		t.Fatalf("stale input: %v", err)
	}
	result := readEvent(t, current)
	if result["type"] != "command_result" || result["code"] != "stale_browser_page" {
		t.Fatalf("stale input result = %#v", result)
	}
	readEvent(t, current)
	if writer.count() != 0 {
		t.Fatalf("stale input reached worker: %d writes", writer.count())
	}
	if err := m.command(current, []byte(`{"type":"input","clientId":"client-one","pageGeneration":"page-one","requestId":"input-current","event":{"kind":"text","text":"current"}}`)); err != nil {
		t.Fatalf("current input: %v", err)
	}
	if writer.count() != 1 {
		t.Fatalf("current input writes = %d, want 1", writer.count())
	}
	writer.mu.Lock()
	line := writer.lines[0]
	writer.mu.Unlock()
	var forwarded map[string]any
	if err := json.Unmarshal([]byte(line), &forwarded); err != nil {
		t.Fatalf("decode forwarded input: %v", err)
	}
	if forwarded["pageOperation"] != float64(4) {
		t.Fatalf("forwarded operation = %#v, want 4", forwarded["pageOperation"])
	}
}

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

func TestPageCommandsRejectStaleOperation(t *testing.T) {
	writer := &recordingWriter{}
	runtime := &process{stdin: writer, generation: "generation-1", viewport: viewportSize{Width: 1280, Height: 720}, pageStatus: "ready", pageGeneration: "page-one", pageOperation: 4, pageRevision: 4}
	m := newTestManager(runtime)
	current := newTestViewer(runtime, "client-one")
	m.addViewer(current)
	readEvent(t, current)
	if err := m.command(current, []byte(`{"type":"input","clientId":"client-one","pageGeneration":"page-one","pageOperation":3,"requestId":"input-old","event":{"kind":"text","text":"stale"}}`)); err != nil {
		t.Fatalf("stale operation: %v", err)
	}
	result := readEvent(t, current)
	if result["type"] != "command_result" || result["code"] != "stale_browser_page" {
		t.Fatalf("stale operation result = %#v", result)
	}
	if writer.count() != 0 {
		t.Fatalf("stale operation reached worker: %d writes", writer.count())
	}
}

func TestWorkerCommandResultsRouteToOriginatingViewer(t *testing.T) {
	writer := &recordingWriter{}
	runtime := &process{stdin: writer, generation: "generation-1", viewport: viewportSize{Width: 1280, Height: 720}, pageStatus: "ready", pageGeneration: "page-one", pageOperation: 4, pageRevision: 4}
	m := newTestManager(runtime)
	first := newTestViewer(runtime, "client-one")
	second := newTestViewer(runtime, "client-two")
	m.addViewer(first)
	m.addViewer(second)
	readEvent(t, first)
	readEvent(t, second)
	if err := m.command(first, []byte(`{"type":"input","clientId":"client-one","pageGeneration":"page-one","requestId":"request-one","event":{"kind":"text","text":"hello"}}`)); err != nil {
		t.Fatalf("input: %v", err)
	}
	writer.mu.Lock()
	line := writer.lines[0]
	writer.mu.Unlock()
	var forwarded map[string]any
	if err := json.Unmarshal([]byte(line), &forwarded); err != nil {
		t.Fatalf("decode forwarded input: %v", err)
	}
	internalClientID, _ := forwarded["clientId"].(string)
	internalRequestID, _ := forwarded["requestId"].(string)
	if internalClientID == "client-one" || internalRequestID == "request-one" || internalClientID == "" || internalRequestID == "" {
		t.Fatalf("worker routing identifiers were not internalized: %#v", forwarded)
	}
	event, err := json.Marshal(map[string]any{"type": "command_result", "clientId": internalClientID, "requestId": internalRequestID, "success": true, "pageGeneration": "page-one", "pageOperation": 4, "revision": 4})
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	m.broadcastEvent(runtime, event)
	result := readEvent(t, first)
	if result["type"] != "command_result" || result["requestId"] != "request-one" {
		t.Fatalf("originating result = %#v", result)
	}
	select {
	case event := <-second.events:
		t.Fatalf("result leaked to second viewer: %q", event)
	default:
	}
}
