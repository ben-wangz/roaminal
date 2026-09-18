package server

import "testing"

func TestDecodeWebSocketCommandUsesTypedValidation(t *testing.T) {
	command, err := decodeWebSocketCommand([]byte(`{"type":"input","requestId":"request-1","data":"echo \u4f60\u597d\n"}`))
	if err != nil || command.Type != "input" || command.Data != "echo 你好\n" {
		t.Fatalf("command=%+v err=%v", command, err)
	}
	if _, err := decodeWebSocketCommand([]byte(`{"type":"resize","cols":1,"rows":24}`)); err == nil {
		t.Fatal("expected invalid dimensions")
	}
	if _, err := decodeWebSocketCommand([]byte(`{"type":"ping","extra":true}`)); err == nil {
		t.Fatal("expected unknown field rejection")
	}
	invalidObserverInput, err := decodeWebSocketCommand([]byte(`{"type":"input","data":"observer-must-not-write"}`))
	if err == nil || invalidObserverInput.Type != "input" || !isWebSocketControlCommand(invalidObserverInput.Type) {
		t.Fatalf("invalid observer command=%+v err=%v", invalidObserverInput, err)
	}
	if isWebSocketControlCommand("ping") || isWebSocketControlCommand("unknown") {
		t.Fatal("non-control websocket commands must not be classified as control")
	}
}
