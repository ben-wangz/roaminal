package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/api"
	"github.com/ben-wangz/roaminal/backend/internal/identity"
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

func writeSuccess(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, successResponse{Status: "ok"})
}

func writeError(w http.ResponseWriter, status int, message string, fields ...string) {
	value := api.NewApplicationError(api.ErrorCode(errorCode(message)), status, message)
	if len(fields) > 0 {
		value.Field = fields[0]
	}
	writeApplicationError(w, value)
}

func writeErrorDetails(w http.ResponseWriter, status int, message string, details any) {
	value := api.NewApplicationError(api.ErrorCode(errorCode(message)), status, message)
	value.Details = details
	writeApplicationError(w, value)
}

func writeApplicationError(w http.ResponseWriter, err error) {
	var value *api.ApplicationError
	if errors.As(err, &value) && value != nil {
		status := value.Status
		if status == 0 {
			status = http.StatusInternalServerError
		}
		code := string(value.Code)
		if code == "" {
			code = string(api.ErrorInternal)
		}
		message := value.Message
		if message == "" {
			message = "internal error"
		}
		writeCodedErrorWithRetry(w, status, message, code, value.Details, &value.Retryable, value.Field)
		return
	}
	writeError(w, http.StatusInternalServerError, "internal error")
}

func writeCodedError(w http.ResponseWriter, status int, message, code string, details any, fields ...string) {
	writeCodedErrorWithRetry(w, status, message, code, details, nil, fields...)
}

func writeCodedErrorWithRetry(w http.ResponseWriter, status int, message, code string, details any, retryableOverride *bool, fields ...string) {
	requestIDValue := ""
	if status >= 500 {
		requestIDValue = w.Header().Get("X-Roaminal-Request-ID")
		if requestIDValue == "" {
			requestIDValue = requestID()
		}
		w.Header().Set("X-Roaminal-Request-ID", requestIDValue)
		log.Printf("request_id=%s status=%d", requestIDValue, status)
	}
	retryable := status == http.StatusRequestTimeout || status == http.StatusGatewayTimeout || status == http.StatusServiceUnavailable || status == http.StatusTooManyRequests
	if retryableOverride != nil {
		retryable = *retryableOverride
	}
	body := api.ErrorResponse{Error: message, Code: code, Retryable: retryable, RequestID: requestIDValue, Details: details}
	if len(fields) > 0 && fields[0] != "" {
		body.Field = fields[0]
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
	value, err := (identity.UUIDGenerator{}).NewID()
	if err != nil {
		return "unknown"
	}
	return value
}
