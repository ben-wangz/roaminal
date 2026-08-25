package worker

import "time"

const (
	Protocol         = "roaminal-terminal-worker/4"
	SchemaVersion    = 1
	HeaderLimit      = 64 * 1024
	PayloadLimit     = 256 * 1024 * 1024
	WriterQueueLimit = 16 * 1024 * 1024
	WriterStallLimit = 10 * time.Second
)
