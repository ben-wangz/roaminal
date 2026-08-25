package server

import "testing"

func TestWebSocketRoleContract(t *testing.T) {
	if role, err := parseWebSocketRole(""); err != nil || role != websocketInteractive {
		t.Fatalf("default role=%q err=%v", role, err)
	}
	if role, err := parseWebSocketRole("observer"); err != nil || role != websocketObserver {
		t.Fatalf("observer role=%q err=%v", role, err)
	}
	if _, err := parseWebSocketRole("control"); err == nil {
		t.Fatal("expected invalid role")
	}
}
