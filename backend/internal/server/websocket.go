package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/connection"
	"github.com/coder/websocket"
)

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/ws/connection-instances/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, 404, "not found")
		return
	}
	if _, err := s.auth.Authenticate(websocketToken(r)); err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	if err := s.terms.ReserveAttach(id); err != nil {
		if errors.Is(err, connection.ErrClientCapacity) {
			writeError(w, http.StatusTooManyRequests, "client capacity reached")
		} else if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "not found")
		} else {
			writeError(w, http.StatusConflict, "terminal unavailable")
		}
		return
	}
	reserved := true
	defer func() {
		if reserved {
			s.terms.ReleaseAttach(id)
		}
	}()
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"roaminal.v1"}})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	client, err := s.terms.AttachReserved(ctx, id)
	if err != nil {
		_ = conn.Close(websocket.StatusCode(1011), "attach failed")
		return
	}
	reserved = false
	defer s.terms.Detach(id, client)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		ticker := time.NewTicker(s.cfg.WebsocketPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-client.Done():
				code, reason := client.CloseReason()
				if code != 0 && code != 1000 {
					_ = conn.Close(websocket.StatusCode(code), reason)
				}
				cancel()
				return
			case <-ticker.C:
				if err := conn.Ping(ctx); err != nil {
					cancel()
					return
				}
			case data := <-client.Messages:
				if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
					cancel()
					return
				}
				client.Consumed(len(data))
			}
		}
	}()
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			cancel()
			break
		}
		if typ != websocket.MessageText || len(data) > 1024*1024 {
			_ = conn.Close(websocket.StatusMessageTooBig, "message too large")
			return
		}
		var msg map[string]json.RawMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			_ = conn.Close(websocket.StatusPolicyViolation, "invalid_message")
			return
		}
		var kind string
		_ = json.Unmarshal(msg["type"], &kind)
		if !validWSMessage(kind, msg) {
			_ = conn.Close(websocket.StatusPolicyViolation, "invalid_message")
			return
		}
		switch kind {
		case "input":
			var value string
			inputErr := json.Unmarshal(msg["data"], &value)
			if inputErr == nil {
				inputErr = s.terms.Input(id, client, value)
			}
			if errors.Is(inputErr, connection.ErrControlNotOwner) {
				continue
			}
			if inputErr != nil {
				_ = conn.Close(websocket.StatusPolicyViolation, "invalid_message")
				return
			}
		case "resize":
			var value struct {
				Cols int `json:"cols"`
				Rows int `json:"rows"`
			}
			if json.Unmarshal(data, &value) != nil {
				_ = conn.Close(websocket.StatusPolicyViolation, "invalid_message")
				return
			}
			if err := s.terms.Resize(id, client, value.Cols, value.Rows); errors.Is(err, connection.ErrControlNotOwner) {
				continue
			} else if err != nil {
				_ = conn.Close(websocket.StatusPolicyViolation, "invalid_message")
				return
			}
		case "ping":
			_ = client.EnqueueControl([]byte(`{"type":"pong"}`))
		case "claim_terminal_control":
			if err := s.terms.ClaimControl(id, client); err != nil {
				_ = conn.Close(websocket.StatusPolicyViolation, "claim_failed")
				return
			}
		default:
			_ = conn.Close(websocket.StatusPolicyViolation, "invalid_message")
			return
		}
	}
	<-writerDone
}

func validWSMessage(kind string, msg map[string]json.RawMessage) bool {
	allowed := map[string]map[string]bool{"input": {"type": true, "data": true}, "resize": {"type": true, "cols": true, "rows": true}, "claim_terminal_control": {"type": true}, "ping": {"type": true}}
	fields, ok := allowed[kind]
	if !ok {
		return false
	}
	for key := range msg {
		if !fields[key] {
			return false
		}
	}
	if kind == "input" || kind == "resize" {
		return msg["data"] != nil || (msg["cols"] != nil && msg["rows"] != nil)
	}
	return true
}

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
