package browser

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func commandURL(command map[string]json.RawMessage) string {
	var value string
	if err := json.Unmarshal(command["url"], &value); err != nil {
		return ""
	}
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return ""
	}
	return parsed.String()
}

func writeError(w http.ResponseWriter, status int, message string) {
	code := "browser_unavailable"
	if status == http.StatusConflict {
		code = "browser_viewer_conflict"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, "{\"error\":%q,\"code\":%q,\"retryable\":true}", message, code)
}
