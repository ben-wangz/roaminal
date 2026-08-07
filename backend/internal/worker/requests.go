package worker

import (
	"context"
	"errors"
)

func (c *Client) request(ctx context.Context, header map[string]any, payload []byte) (Result, error) {
	requestID := newID()
	header["requestId"] = requestID
	waiter := c.register(requestID)
	if err := c.send(header, payload); err != nil {
		c.unregister(requestID, waiter)
		return Result{}, err
	}
	select {
	case result := <-waiter:
		if stringField(result.Header, "op") == "error" {
			return Result{}, errors.New(stringField(result.Header, "message"))
		}
		return result, nil
	case <-ctx.Done():
		c.unregister(requestID, waiter)
		return Result{}, ctx.Err()
	case <-c.done:
		c.unregister(requestID, waiter)
		return Result{}, ErrUnavailable
	}
}
func (c *Client) Create(ctx context.Context, sessionID string, cols, rows, scrollback int) error {
	result, err := c.request(ctx, map[string]any{"op": "create", "protocol": Protocol, "sessionId": sessionID, "cols": cols, "rows": rows, "scrollbackLines": scrollback}, nil)
	if err != nil {
		return err
	}
	if stringField(result.Header, "requestOp") != "create" {
		return errors.New("worker create failed")
	}
	return nil
}
func (c *Client) Restore(ctx context.Context, sessionID string, cols, rows, scrollback int, sequence string, payload []byte) error {
	result, err := c.request(ctx, map[string]any{"op": "restore", "protocol": Protocol, "sessionId": sessionID, "cols": cols, "rows": rows, "scrollbackLines": scrollback, "throughSequence": sequence}, payload)
	if err != nil {
		return err
	}
	if stringField(result.Header, "requestOp") != "restore" || stringField(result.Header, "throughSequence") != sequence {
		return errors.New("worker restore barrier failed")
	}
	return nil
}
func (c *Client) Write(sessionID, sequence string, payload []byte) error {
	if len(payload) > 256*1024 {
		return errors.New("worker write frame too large")
	}
	if !validSequence(sequence) {
		return errors.New("invalid worker sequence")
	}
	return c.send(map[string]any{"op": "write", "protocol": Protocol, "sessionId": sessionID, "sequence": sequence}, payload)
}
func (c *Client) Resize(sessionID, sequence string, cols, rows int) error {
	if !validSequence(sequence) || cols < 2 || cols > 1000 || rows < 1 || rows > 1000 {
		return errors.New("invalid worker resize")
	}
	return c.send(map[string]any{"op": "resize", "protocol": Protocol, "sessionId": sessionID, "sequence": sequence, "cols": cols, "rows": rows}, nil)
}
func (c *Client) Snapshot(ctx context.Context, sessionID, sequence string) ([]byte, string, error) {
	result, err := c.request(ctx, map[string]any{"op": "snapshot", "protocol": Protocol, "sessionId": sessionID, "throughSequence": sequence}, nil)
	if err != nil {
		return nil, "", err
	}
	return result.Payload, stringField(result.Header, "throughSequence"), nil
}
func (c *Client) CloseSession(ctx context.Context, sessionID string) error {
	_, err := c.request(ctx, map[string]any{"op": "close", "protocol": Protocol, "sessionId": sessionID}, nil)
	return err
}
func (c *Client) Shutdown(ctx context.Context) error {
	if c.stdin == nil {
		return nil
	}
	c.stopping.Store(true)
	_, err := c.request(ctx, map[string]any{"op": "shutdown", "protocol": Protocol}, nil)
	_ = c.stdin.Close()
	_ = terminateProcessGroup(ctx, c.cmd)
	return err
}
func (c *Client) Available() bool {
	select {
	case <-c.done:
		return false
	default:
		return c.stdin != nil
	}
}
