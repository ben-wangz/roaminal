package worker

import (
	"context"
	"errors"
)

func (c *Client) request(ctx context.Context, header requestHeader, payload []byte) (Result, error) {
	correlationID := c.newID()
	header.SchemaVersion = SchemaVersion
	header.CorrelationID = correlationID
	waiter := c.register(correlationID)
	if err := c.send(header, payload); err != nil {
		c.unregister(correlationID, waiter)
		return Result{}, err
	}
	select {
	case result := <-waiter:
		if result.Header.Op == "error" {
			return Result{}, errors.New(result.Header.Message)
		}
		return result, nil
	case <-ctx.Done():
		c.unregister(correlationID, waiter)
		return Result{}, ctx.Err()
	case <-c.done:
		c.unregister(correlationID, waiter)
		return Result{}, ErrUnavailable
	}
}
func (c *Client) Create(ctx context.Context, terminalID string, cols, rows, scrollback int) error {
	result, err := c.request(ctx, requestHeader{Op: "create", Protocol: Protocol, TerminalID: terminalID, Cols: intValue(cols), Rows: intValue(rows), ScrollbackLines: intValue(scrollback)}, nil)
	if err != nil {
		return err
	}
	if result.Header.RequestOp != "create" {
		return errors.New("worker create failed")
	}
	return nil
}
func (c *Client) Restore(ctx context.Context, terminalID string, cols, rows, scrollback int, sequence string, payload []byte) error {
	result, err := c.request(ctx, requestHeader{Op: "restore", Protocol: Protocol, TerminalID: terminalID, Cols: intValue(cols), Rows: intValue(rows), ScrollbackLines: intValue(scrollback), ThroughSequence: sequence}, payload)
	if err != nil {
		return err
	}
	if result.Header.RequestOp != "restore" || result.Header.ThroughSequence != sequence {
		return errors.New("worker restore barrier failed")
	}
	return nil
}
func (c *Client) Write(terminalID, sequence string, payload []byte) error {
	if len(payload) > 256*1024 {
		return errors.New("worker write frame too large")
	}
	if !validSequence(sequence) {
		return errors.New("invalid worker sequence")
	}
	return c.send(requestHeader{Op: "write", Protocol: Protocol, TerminalID: terminalID, Sequence: sequence}, payload)
}
func (c *Client) Resize(terminalID, sequence string, cols, rows int) error {
	if !validSequence(sequence) || cols < 2 || cols > 1000 || rows < 1 || rows > 1000 {
		return errors.New("invalid worker resize")
	}
	return c.send(requestHeader{Op: "resize", Protocol: Protocol, TerminalID: terminalID, Sequence: sequence, Cols: intValue(cols), Rows: intValue(rows)}, nil)
}
func (c *Client) Snapshot(ctx context.Context, terminalID, sequence string) ([]byte, string, error) {
	result, err := c.request(ctx, requestHeader{Op: "snapshot", Protocol: Protocol, TerminalID: terminalID, ThroughSequence: sequence}, nil)
	if err != nil {
		return nil, "", err
	}
	return result.Payload, result.Header.ThroughSequence, nil
}
func (c *Client) CloseSession(ctx context.Context, terminalID string) error {
	_, err := c.request(ctx, requestHeader{Op: "close", Protocol: Protocol, TerminalID: terminalID}, nil)
	return err
}
func (c *Client) Shutdown(ctx context.Context) error {
	if c.stdin == nil {
		return nil
	}
	c.stopping.Store(true)
	_, err := c.request(ctx, requestHeader{Op: "shutdown", Protocol: Protocol}, nil)
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
