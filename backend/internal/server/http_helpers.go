package server

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeError(w, 400, "content type must be application/json")
		return errors.New("content type")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeError(w, 400, "invalid JSON body")
		}
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeError(w, 400, "invalid JSON body")
		}
		return errors.New("multiple values")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, message string, fields ...string) {
	if status >= 500 {
		id := requestID()
		w.Header().Set("X-Roaminal-Request-ID", id)
		log.Printf("request_id=%s status=%d", id, status)
	}
	body := map[string]string{"error": message, "code": errorCode(message)}
	if len(fields) > 0 && fields[0] != "" {
		body["field"] = fields[0]
	}
	writeJSON(w, status, body)
}
func errorCode(message string) string {
	value := strings.ToLower(strings.TrimSpace(message))
	value = strings.NewReplacer(" ", "_", "-", "_", "/", "_").Replace(value)
	if value == "" {
		return "error"
	}
	return value
}
func requestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "unknown"
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:])
}
