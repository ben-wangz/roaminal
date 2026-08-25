package worker

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func (c *Client) Start(ctx context.Context) error {
	if filepath.Ext(c.path) == ".mjs" {
		c.cmd = exec.Command("node", c.path)
	} else {
		c.cmd = exec.Command(c.path)
	}
	c.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	stdin, err := c.cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}
	c.cmd.Stderr = os.Stderr
	c.stdin, c.stdout = stdin, stdout
	c.writeQueue = make(chan writeRequest, 128)
	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("start terminal worker: %w", err)
	}
	go c.writeLoop()
	go c.readLoop()
	go func() {
		err := c.cmd.Wait()
		if err == nil {
			err = ErrUnavailable
		}
		c.fail(err)
	}()
	correlationID := c.newID()
	waiter := c.register(correlationID)
	if err := c.send(requestHeader{Op: "hello", Protocol: Protocol, SchemaVersion: SchemaVersion, CorrelationID: correlationID}, nil); err != nil {
		c.unregister(correlationID, waiter)
		c.fail(err)
		return err
	}
	handshakeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	select {
	case err := <-c.ready:
		return err
	case result := <-waiter:
		if result.Header.Op == "ready" && result.Header.Protocol == Protocol && result.Header.Engine == "xterm-headless" {
			return nil
		}
		err := errors.New("terminal worker handshake failed")
		c.fail(err)
		return err
	case <-handshakeCtx.Done():
		c.unregister(correlationID, waiter)
		c.fail(handshakeCtx.Err())
		return handshakeCtx.Err()
	}
}

func (c *Client) register(requestID string) chan Result {
	ch := make(chan Result, 1)
	c.waitMu.Lock()
	c.waiters[requestID] = ch
	c.waitMu.Unlock()
	return ch
}
func (c *Client) unregister(requestID string, waiter chan Result) {
	c.waitMu.Lock()
	if current := c.waiters[requestID]; current == waiter {
		delete(c.waiters, requestID)
	}
	c.waitMu.Unlock()
}

func (c *Client) readLoop() {
	reader := bufio.NewReader(c.stdout)
	for {
		frame, err := readFrame(reader)
		if err != nil {
			c.fail(err)
			return
		}
		header, err := decodeResponseHeader(frame.Header)
		if err != nil {
			c.fail(err)
			return
		}
		correlationID := header.CorrelationID
		if header.Op == "error" && header.Fatal {
			c.fail(errors.New(header.Message))
			return
		}
		if correlationID == "" {
			continue
		}
		c.waitMu.Lock()
		waiter := c.waiters[correlationID]
		delete(c.waiters, correlationID)
		c.waitMu.Unlock()
		if waiter != nil {
			waiter <- Result{Header: header, Payload: frame.Payload}
		}
	}
}

func (c *Client) fail(err error) {
	c.closeOnce.Do(func() {
		close(c.done)
		c.waitMu.Lock()
		for id, waiter := range c.waiters {
			delete(c.waiters, id)
			waiter <- Result{Header: responseHeader{Op: "error", Protocol: Protocol, Message: err.Error(), SchemaVersion: SchemaVersion, CorrelationID: id, Sequence: "1", EventID: c.newID(), OccurredAt: c.now().UTC()}}
		}
		c.waitMu.Unlock()
		if c.cmd != nil && c.cmd.Process != nil && c.cmd.ProcessState == nil && !c.stopping.Load() {
			_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
		}
		if c.onFatal != nil && !c.stopping.Load() {
			c.onFatal(err)
		}
	})
}

func (c *Client) Wait(ctx context.Context) error {
	select {
	case <-c.done:
		return ErrUnavailable
	case <-ctx.Done():
		return ctx.Err()
	}
}
