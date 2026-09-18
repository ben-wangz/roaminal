package server

import (
	"net/http"
	"strings"
)

func bearer(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return strings.TrimSpace(value[7:])
	}
	return value
}

func websocketToken(r *http.Request) string {
	protocols := strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",")
	for _, protocol := range protocols {
		protocol = strings.TrimSpace(protocol)
		if strings.HasPrefix(protocol, "roaminal.auth.") {
			return strings.TrimPrefix(protocol, "roaminal.auth.")
		}
	}
	return bearer(r)
}
