package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/api"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/coder/websocket"
)

type websocketRole string

const (
	websocketInteractive websocketRole = "interactive"
	websocketObserver    websocketRole = "observer"
)

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("connectionInstanceId")
	pendingID := r.PathValue("launchId")
	pending := pendingID != ""
	if pending {
		id = pendingID
	}
	if id == "" {
		writeError(w, 404, "not found")
		return
	}
	role, roleErr := parseWebSocketRole(r.URL.Query().Get("role"))
	if roleErr != nil {
		writeError(w, http.StatusBadRequest, "invalid websocket role", "role")
		return
	}
	authSessionID, err := s.auth.Authenticate(websocketToken(r))
	if err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	if pending {
		owner := s.terms.PendingOwner(id)
		if owner != "" && owner != authSessionID {
			writeError(w, http.StatusForbidden, "launch belongs to another auth session")
			return
		}
	}
	reserve := s.terms.ReserveAttach
	release := s.terms.ReleaseAttach
	if pending {
		reserve = s.terms.ReservePendingAttach
		release = s.terms.ReleasePendingAttach
	}
	if err := reserve(id); err != nil {
		if errors.Is(err, ports.ErrClientCapacity) {
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
			release(id)
		}
	}()
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{api.WebSocketProtocol}})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	attach := s.terms.AttachReserved
	detach := s.terms.Detach
	input := s.terms.Input
	resize := s.terms.Resize
	claim := s.terms.ClaimControl
	touch := func(string) {}
	if pending {
		attach = s.terms.AttachPendingReserved
		detach = s.terms.DetachPending
		input = s.terms.InputPending
		resize = s.terms.ResizePending
		claim = s.terms.ClaimPendingControl
		touch = s.terms.TouchPending
	}
	client, err := attach(ctx, id)
	if err != nil {
		_ = conn.Close(websocket.StatusCode(1011), "attach failed")
		return
	}
	reserved = false
	defer detach(id, client)
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
			case data := <-client.Messages():
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
		command, err := decodeWebSocketCommand(data)
		if err != nil {
			if role == websocketObserver && isWebSocketControlCommand(command.Type) {
				_ = conn.Close(websocket.StatusPolicyViolation, "observer_cannot_control")
				return
			}
			_ = conn.Close(websocket.StatusPolicyViolation, "invalid_message")
			return
		}
		switch command.Type {
		case "input":
			if role == websocketObserver {
				_ = conn.Close(websocket.StatusPolicyViolation, "observer_cannot_control")
				return
			}
			inputErr := input(id, client, command.Data)
			if errors.Is(inputErr, ports.ErrControlNotOwner) {
				continue
			}
			if inputErr != nil {
				_ = conn.Close(websocket.StatusPolicyViolation, "invalid_message")
				return
			}
		case "resize":
			if role == websocketObserver {
				_ = conn.Close(websocket.StatusPolicyViolation, "observer_cannot_control")
				return
			}
			if err := resize(id, client, command.Cols, command.Rows); errors.Is(err, ports.ErrControlNotOwner) {
				continue
			} else if err != nil {
				_ = conn.Close(websocket.StatusPolicyViolation, "invalid_message")
				return
			}
		case "ping":
			touch(id)
			pong, _ := json.Marshal(websocketPong{Type: "pong", RequestID: command.RequestID})
			_ = client.EnqueueControl(pong)
		case "claim_terminal_control":
			if role == websocketObserver {
				_ = conn.Close(websocket.StatusPolicyViolation, "observer_cannot_control")
				return
			}
			if err := claim(id, client); err != nil {
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

func parseWebSocketRole(value string) (websocketRole, error) {
	role := websocketRole(strings.TrimSpace(value))
	if role == "" {
		return websocketInteractive, nil
	}
	if role != websocketInteractive && role != websocketObserver {
		return "", errors.New("invalid websocket role")
	}
	return role, nil
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
