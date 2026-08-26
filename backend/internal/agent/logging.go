package agent

import (
	"log"
	"time"
)

// Agent initialization is intentionally logged with stable identifiers and
// phase names. Payloads, tokens, webhook URLs, and remote command output are
// never included in these records.
func logAgentInfo(event, operationID, instanceID, fields string, values ...any) {
	args := []any{event, operationID, instanceID}
	args = append(args, values...)
	if fields == "" {
		log.Printf("level=INFO event=%s operation_id=%q connection_instance_id=%q", args...)
		return
	}
	log.Printf("level=INFO event=%s operation_id=%q connection_instance_id=%q "+fields, args...)
}

func durationMillis(start time.Time) int64 {
	if start.IsZero() {
		return 0
	}
	return time.Since(start).Milliseconds()
}
