package worker

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
)

const (
	Protocol     = "roaminal-terminal-worker/1"
	HeaderLimit  = 64 * 1024
	PayloadLimit = 256 * 1024 * 1024
)

var ErrUnavailable = errors.New("terminal worker unavailable")

type Frame struct {
	Header  map[string]json.RawMessage
	Payload []byte
}

type Result struct {
	Header  map[string]json.RawMessage
	Payload []byte
}

type Client struct {
	path       string
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	writeMu    sync.Mutex
	waitMu     sync.Mutex
	waiters    map[string]chan Result
	ready      chan error
	done       chan struct{}
	closeOnce  sync.Once
	callbackMu sync.Mutex
	onFatal    func(error)
	stopping   atomic.Bool
}

func New(path string, onFatal func(error)) *Client {
	return &Client{path: path, waiters: make(map[string]chan Result), ready: make(chan error, 1), done: make(chan struct{}), onFatal: onFatal}
}

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
	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("start terminal worker: %w", err)
	}
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
	select {
	case err := <-c.ready:
		return err
	case result := <-waiter:
		if op := stringField(result.Header, "op"); op == "ready" && stringField(result.Header, "protocol") == Protocol {
			return nil
		}
		err := errors.New("terminal worker handshake failed")
		c.fail(err)
		return err
	case <-ctx.Done():
		c.unregister(requestID, waiter)
		c.fail(ctx.Err())
		return ctx.Err()
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

func raw(value string) json.RawMessage { data, _ := json.Marshal(value); return data }

func (c *Client) send(header map[string]any, payload []byte) error {
	data, err := frame(header, payload)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.stdin == nil || !c.Available() {
		return ErrUnavailable
	}
	for len(data) > 0 {
		written, err := c.stdin.Write(data)
		if written > 0 {
			data = data[written:]
		}
		if err != nil {
			c.fail(err)
			return err
		}
		if written == 0 {
			err := io.ErrShortWrite
			c.fail(err)
			return err
		}
	}
	return nil
}

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
	if err != nil && c.cmd != nil && c.cmd.Process != nil && c.cmd.ProcessState == nil {
		_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
	}
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

func frame(header map[string]any, payload []byte) ([]byte, error) {
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return nil, err
	}
	if len(headerBytes) > HeaderLimit || len(payload) > PayloadLimit {
		return nil, errors.New("worker frame exceeds limit")
	}
	result := make([]byte, 8+len(headerBytes)+len(payload))
	result[0] = byte(len(headerBytes) >> 24)
	result[1] = byte(len(headerBytes) >> 16)
	result[2] = byte(len(headerBytes) >> 8)
	result[3] = byte(len(headerBytes))
	result[4] = byte(len(payload) >> 24)
	result[5] = byte(len(payload) >> 16)
	result[6] = byte(len(payload) >> 8)
	result[7] = byte(len(payload))
	copy(result[8:], headerBytes)
	copy(result[8+len(headerBytes):], payload)
	return result, nil
}

func readFrame(reader *bufio.Reader) (Frame, error) {
	var prefix [8]byte
	if _, err := io.ReadFull(reader, prefix[:]); err != nil {
		return Frame{}, err
	}
	headerLength := int(prefix[0])<<24 | int(prefix[1])<<16 | int(prefix[2])<<8 | int(prefix[3])
	payloadLength := int(prefix[4])<<24 | int(prefix[5])<<16 | int(prefix[6])<<8 | int(prefix[7])
	if headerLength < 1 || headerLength > HeaderLimit || payloadLength < 0 || payloadLength > PayloadLimit {
		return Frame{}, errors.New("invalid worker frame lengths")
	}
	headerBytes := make([]byte, headerLength)
	if _, err := io.ReadFull(reader, headerBytes); err != nil {
		return Frame{}, err
	}
	payload := make([]byte, payloadLength)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return Frame{}, err
	}
	var header map[string]json.RawMessage
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return Frame{}, errors.New("invalid worker JSON header")
	}
	return Frame{Header: header, Payload: payload}, nil
}

func stringField(header map[string]json.RawMessage, key string) string {
	var value string
	_ = json.Unmarshal(header[key], &value)
	return value
}
func boolField(header map[string]json.RawMessage, key string) bool {
	var value bool
	_ = json.Unmarshal(header[key], &value)
	return value
}

func newID() string {
	var rawBytes [16]byte
	if _, err := rand.Read(rawBytes[:]); err != nil {
		return hex.EncodeToString(rawBytes[:])
	}
	rawBytes[6] = rawBytes[6]&0x0f | 0x40
	rawBytes[8] = rawBytes[8]&0x3f | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", rawBytes[0:4], rawBytes[4:6], rawBytes[6:8], rawBytes[8:10], rawBytes[10:])
}

func validSequence(value string) bool {
	if value == "" || (len(value) > 1 && value[0] == '0') || strings.Trim(value, "0123456789") != "" {
		return false
	}
	_, err := strconv.ParseUint(value, 10, 64)
	return err == nil
}

func (c *Client) Wait(ctx context.Context) error {
	select {
	case <-c.done:
		return ErrUnavailable
	case <-ctx.Done():
		return ctx.Err()
	}
}
