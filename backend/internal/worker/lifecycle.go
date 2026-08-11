package worker

import (
	"bufio"
	"context"
	"encoding/json"
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
	requestID := newID()
	waiter := c.register(requestID)
	if err := c.send(map[string]any{"op": "hello", "protocol": Protocol, "requestId": requestID}, nil); err != nil {
		c.unregister(requestID, waiter)
		c.fail(err)
		return err
	}
	handshakeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	select {
	case err := <-c.ready:
		return err
	case result := <-waiter:
		if stringField(result.Header, "op") == "ready" && stringField(result.Header, "protocol") == Protocol && stringField(result.Header, "engine") == "xterm-headless" {
			return nil
		}
		err := errors.New("terminal worker handshake failed")
		c.fail(err)
		return err
	case <-handshakeCtx.Done():
		c.unregister(requestID, waiter)
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
		requestID := stringField(frame.Header, "requestId")
		if stringField(frame.Header, "op") == "error" && boolField(frame.Header, "fatal") {
			c.fail(errors.New(stringField(frame.Header, "message")))
			return
		}
		if requestID == "" {
			continue
		}
		c.waitMu.Lock()
		waiter := c.waiters[requestID]
		delete(c.waiters, requestID)
		c.waitMu.Unlock()
		if waiter != nil {
			waiter <- Result{Header: frame.Header, Payload: frame.Payload}
		}
	}
}

func (c *Client) fail(err error) {
	c.closeOnce.Do(func() {
		close(c.done)
		c.waitMu.Lock()
		for id, waiter := range c.waiters {
			delete(c.waiters, id)
			waiter <- Result{Header: map[string]json.RawMessage{"op": raw("error"), "message": raw(err.Error())}}
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
