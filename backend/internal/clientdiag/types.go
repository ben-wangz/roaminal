package clientdiag

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	SchemaVersion       = 1
	MaxBodyBytes        = 256 * 1024
	MaxEventsPerBatch   = 20
	MaxMessageBytes     = 4096
	MaxStackBytes       = 16384
	MaxPathBytes        = 1024
	MaxPageAge          = 24 * time.Hour
	MaxFutureSkew       = 5 * time.Minute
	MaxStoredFiles      = 5
	MaxStoredBytes      = 10 * 1024 * 1024
	MaxStoredFileBytes  = 2 * 1024 * 1024
	Retention           = 7 * 24 * time.Hour
	MaxUserAgentBytes   = 512
	MaxRepeatCount      = 1_000_000
	MaxDroppedCount     = 1_000_000
	MaxDeduplicationIDs = 4096
)

var (
	ErrInvalid     = errors.New("invalid client diagnostics")
	ErrRateLimited = errors.New("client diagnostics rate limited")
)

var uuidV4Pattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type Batch struct {
	SchemaVersion int     `json:"schemaVersion"`
	PageID        string  `json:"pageId"`
	DroppedCount  int     `json:"droppedCount"`
	Events        []Event `json:"events"`
}

type Event struct {
	EventID    string     `json:"eventId"`
	OccurredAt string     `json:"occurredAt"`
	Kind       string     `json:"kind"`
	Message    string     `json:"message"`
	Stack      string     `json:"stack,omitempty"`
	PagePath   string     `json:"pagePath,omitempty"`
	SourcePath string     `json:"sourcePath,omitempty"`
	Line       int        `json:"line,omitempty"`
	Column     int        `json:"column,omitempty"`
	Repeat     int        `json:"repeatCount,omitempty"`
	Operation  *Operation `json:"operation,omitempty"`
}

type Operation struct {
	Protocol             string `json:"protocol"`
	Endpoint             string `json:"endpoint,omitempty"`
	ConnectionInstanceID string `json:"connectionInstanceId,omitempty"`
	Phase                string `json:"phase,omitempty"`
	DurationMs           int64  `json:"durationMs,omitempty"`
	CloseCode            int    `json:"closeCode,omitempty"`
	WasClean             bool   `json:"wasClean,omitempty"`
	Online               bool   `json:"online,omitempty"`
	ElementType          string `json:"elementType,omitempty"`
}

type StoredRecord struct {
	ReceivedAt     time.Time `json:"receivedAt"`
	RuntimeVersion string    `json:"runtimeVersion"`
	BootID         string    `json:"bootId"`
	AuthSessionID  string    `json:"authSessionId"`
	UserAgent      string    `json:"userAgent,omitempty"`
	PageID         string    `json:"pageId"`
	DroppedCount   int       `json:"droppedCount,omitempty"`
	Event          Event     `json:"event"`
}

func (b Batch) validate(now time.Time) ([]Event, error) {
	if b.SchemaVersion != SchemaVersion || !isUUIDv4(b.PageID) || len(b.Events) == 0 || len(b.Events) > MaxEventsPerBatch {
		return nil, ErrInvalid
	}
	if b.DroppedCount < 0 || b.DroppedCount > MaxDroppedCount {
		return nil, ErrInvalid
	}
	clean := make([]Event, 0, len(b.Events))
	for _, event := range b.Events {
		value, err := sanitizeEvent(event, now)
		if err != nil {
			return nil, err
		}
		clean = append(clean, value)
	}
	return clean, nil
}

func sanitizeEvent(event Event, now time.Time) (Event, error) {
	if !isUUIDv4(event.EventID) || !validKind(event.Kind) {
		return Event{}, ErrInvalid
	}
	when, err := time.Parse(time.RFC3339Nano, event.OccurredAt)
	if err != nil || when.Before(now.Add(-MaxPageAge)) || when.After(now.Add(MaxFutureSkew)) {
		return Event{}, ErrInvalid
	}
	event.OccurredAt = when.UTC().Format(time.RFC3339Nano)
	event.Message = RedactText(event.Message, MaxMessageBytes)
	event.Stack = RedactText(event.Stack, MaxStackBytes)
	if len(event.Message) == 0 || len(event.Message) > MaxMessageBytes || len(event.Stack) > MaxStackBytes {
		return Event{}, ErrInvalid
	}
	var ok bool
	if event.PagePath, ok = validatePath(event.PagePath); !ok {
		return Event{}, ErrInvalid
	}
	if event.SourcePath, ok = validatePath(event.SourcePath); !ok {
		return Event{}, ErrInvalid
	}
	if event.Line < 0 || event.Line > 0x7fffffff || event.Column < 0 || event.Column > 0x7fffffff || event.Repeat < 0 || event.Repeat > MaxRepeatCount {
		return Event{}, ErrInvalid
	}
	if event.Operation != nil {
		if err := validateOperation(event.Operation); err != nil {
			return Event{}, err
		}
	}
	return event, nil
}

func validateOperation(operation *Operation) error {
	if operation.Protocol != "websocket" && operation.Protocol != "resource" {
		return ErrInvalid
	}
	if operation.Protocol == "websocket" {
		if operation.ElementType != "" {
			return ErrInvalid
		}
		if operation.Endpoint != "connection-instances" && operation.Endpoint != "connection-launches" {
			return ErrInvalid
		}
		if operation.ConnectionInstanceID != "" && !isUUIDv4(operation.ConnectionInstanceID) {
			return ErrInvalid
		}
		if operation.Phase != "" && operation.Phase != "construct" && operation.Phase != "handshake" && operation.Phase != "open" && operation.Phase != "close" {
			return ErrInvalid
		}
	} else {
		if operation.Endpoint != "" || operation.ConnectionInstanceID != "" || operation.Phase != "" || operation.DurationMs != 0 || operation.CloseCode != 0 || operation.WasClean {
			return ErrInvalid
		}
		if operation.ElementType == "" || len(operation.ElementType) > 128 {
			return ErrInvalid
		}
	}
	if operation.DurationMs < 0 || operation.DurationMs > 3_600_000 || !validCloseCode(operation.CloseCode) {
		return ErrInvalid
	}
	return nil
}

func validCloseCode(code int) bool {
	if code == 0 || code >= 3000 && code <= 4999 {
		return true
	}
	switch code {
	case 1000, 1001, 1002, 1003, 1007, 1008, 1009, 1010, 1011, 1012, 1013, 1014, 1006:
		return true
	default:
		return false
	}
}

func validatePath(value string) (string, bool) {
	if value == "" {
		return "", true
	}
	if len(value) > MaxPathBytes || !utf8.ValidString(value) || strings.ContainsAny(value, "?#") || !strings.HasPrefix(value, "/") {
		return "", false
	}
	if parsed, err := url.Parse(value); err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.User != nil {
		return "", false
	}
	return value, true
}

func isUUIDv4(value string) bool { return uuidV4Pattern.MatchString(value) }

func validKind(value string) bool {
	switch value {
	case "console_error", "uncaught_error", "unhandled_rejection", "resource_error", "websocket_error":
		return true
	default:
		return false
	}
}

func truncateUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func cleanControlCharacters(value string) string {
	var builder strings.Builder
	for _, runeValue := range value {
		if runeValue == '\n' || runeValue == '\t' || !unicode.IsControl(runeValue) {
			builder.WriteRune(runeValue)
		} else {
			builder.WriteByte(' ')
		}
	}
	return builder.String()
}
