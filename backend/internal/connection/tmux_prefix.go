package connection

import (
	"context"
	"strings"
	"time"
)

func probeTmuxPrefix(parent context.Context, manager *Manager, transport *Transport) (string, string) {
	ctx, cancel := withAuxiliaryTimeout(parent)
	defer cancel()
	var output []byte
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		output, err = manager.runAuxiliary(ctx, transport, "tmux", "show-options", "-gv", "prefix")
		if err == nil || ctx.Err() != nil {
			break
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
		}
	}
	if err != nil {
		return "a", "fallback"
	}
	key, ok := normalizeTmuxPrefix(string(output))
	if !ok {
		return "", "unsupported"
	}
	return key, "runtime"
}

func normalizeTmuxPrefix(value string) (string, bool) {
	value = strings.TrimSuffix(value, "\n")
	value = strings.TrimSuffix(value, "\r")
	if value == "" || strings.ContainsAny(value, "\r\n\t ") {
		return "", false
	}
	value = strings.ToLower(value)
	if !strings.HasPrefix(value, "c-") || len(value) != 3 {
		return "", false
	}
	key := value[2]
	if key < 'a' || key > 'z' {
		return "", false
	}
	return string(key), true
}
