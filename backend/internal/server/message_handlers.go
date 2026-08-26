package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/messages"
)

type messageReadStateRequest struct {
	ReadThroughSequence *uint64 `json:"readThroughSequence"`
}

type messageReadStateResponse struct {
	Revision       uint64 `json:"revision"`
	LatestSequence uint64 `json:"latestSequence"`
	UnreadCount    int    `json:"unreadCount"`
}

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request, _ string) {
	if s.messages == nil {
		writeCodedError(w, http.StatusServiceUnavailable, "Message storage is unavailable.", "message_store_unavailable", nil)
		return
	}
	limit := 50
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100 {
			writeCodedError(w, http.StatusBadRequest, "The message limit is invalid.", "message_cursor_invalid", nil)
			return
		}
		limit = parsed
	}
	page, err := s.messages.List(limit, r.URL.Query().Get("before"))
	if err != nil {
		writeMessageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) markMessagesRead(w http.ResponseWriter, r *http.Request, _ string) {
	if s.messages == nil {
		writeCodedError(w, http.StatusServiceUnavailable, "Message storage is unavailable.", "message_store_unavailable", nil)
		return
	}
	var body messageReadStateRequest
	if err := decodeMessageReadState(w, r, &body); err != nil {
		return
	}
	if body.ReadThroughSequence == nil {
		writeCodedError(w, http.StatusBadRequest, "The message read state is invalid.", "message_read_state_invalid", nil)
		return
	}
	state, err := s.messages.MarkReadThrough(*body.ReadThroughSequence)
	if err != nil {
		writeMessageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, messageReadStateResponse{Revision: state.Revision, LatestSequence: state.LatestSequence, UnreadCount: state.UnreadCount})
}

func decodeMessageReadState(w http.ResponseWriter, r *http.Request, target *messageReadStateRequest) error {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeCodedError(w, http.StatusBadRequest, "The message read state is invalid.", "message_read_state_invalid", nil)
		return errors.New("message read state content type")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeCodedError(w, http.StatusRequestEntityTooLarge, "The message read state is invalid.", "message_read_state_invalid", nil)
		} else {
			writeCodedError(w, http.StatusBadRequest, "The message read state is invalid.", "message_read_state_invalid", nil)
		}
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeCodedError(w, http.StatusBadRequest, "The message read state is invalid.", "message_read_state_invalid", nil)
		return errors.New("multiple message read state values")
	}
	return nil
}

func writeMessageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, messages.ErrCursorInvalid):
		writeCodedError(w, http.StatusBadRequest, "The message cursor is invalid.", "message_cursor_invalid", nil)
	case errors.Is(err, messages.ErrReadStateInvalid):
		writeCodedError(w, http.StatusBadRequest, "The message read state is invalid.", "message_read_state_invalid", nil)
	case errors.Is(err, messages.ErrStoreUnavailable):
		writeCodedError(w, http.StatusServiceUnavailable, "Message storage is unavailable.", "message_store_unavailable", nil)
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
