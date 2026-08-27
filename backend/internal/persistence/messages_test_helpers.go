package persistence

import (
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

func testMessageDraft(index int, receivedAt time.Time) domain.MessageDraft {
	return domain.MessageDraft{
		MessageID: "message-" + formatTestIndex(index), IdempotencyKey: "key-" + formatTestIndex(index),
		Kind: "codex_turn_completed", Severity: "success", AgentType: "codex", PresentationKey: "codex_turn_finished",
		OccurredAt: receivedAt, ReceivedAt: receivedAt, EndpointKey: "endpoint", FallbackLabel: "tmux:roaminal",
		TmuxSessionName: "roaminal", TmuxSessionID: "$0", TmuxSessionCreated: 1,
	}
}

func formatTestIndex(index int) string {
	if index < 10 {
		return "00" + string(rune('0'+index))
	}
	if index < 100 {
		return "0" + string(rune('0'+index/10)) + string(rune('0'+index%10))
	}
	return string(rune('0'+index/100)) + string(rune('0'+(index/10)%10)) + string(rune('0'+index%10))
}
