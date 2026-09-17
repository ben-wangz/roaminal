package browser

import "testing"

func TestWriteRuntimePreservesRawJSONCommands(t *testing.T) {
	writer := &recordingWriter{}
	runtime := &process{stdin: writer}
	raw := []byte(`{"type":"reload","clientId":"client-one"}`)
	if err := writeRuntime(runtime, raw); err != nil {
		t.Fatalf("write raw command: %v", err)
	}
	if writer.count() != 1 {
		t.Fatalf("worker writes = %d, want 1", writer.count())
	}
	writer.mu.Lock()
	line := writer.lines[0]
	writer.mu.Unlock()
	if line != string(raw)+"\n" {
		t.Fatalf("worker line = %q, want raw JSON", line)
	}
}
