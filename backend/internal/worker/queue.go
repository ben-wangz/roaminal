package worker

import (
	"io"
	"time"
)

func (c *Client) send(header requestHeader, payload []byte) error {
	if header.SchemaVersion == 0 {
		header.SchemaVersion = SchemaVersion
	}
	if header.CorrelationID == "" {
		header.CorrelationID = c.newID()
	}
	data, err := frame(header, payload)
	if err != nil {
		return err
	}
	if c.stdin == nil || !c.Available() {
		return ErrUnavailable
	}
	queueSize := 0
	if header.Op == "write" || header.Op == "resize" {
		queueSize = len(payload)
	}
	deadline := time.NewTimer(WriterStallLimit)
	defer deadline.Stop()
	for {
		c.queueMu.Lock()
		if c.queuedBytes+int64(queueSize) <= WriterQueueLimit {
			c.queuedBytes += int64(queueSize)
			c.queueMu.Unlock()
			break
		}
		c.queueMu.Unlock()
		select {
		case <-deadline.C:
			c.fail(ErrWriterQueueFull)
			return ErrWriterQueueFull
		case <-c.done:
			return ErrUnavailable
		case <-time.After(5 * time.Millisecond):
		}
	}
	req := writeRequest{data: data, queueSize: queueSize, done: make(chan error, 1)}
	select {
	case c.writeQueue <- req:
	case <-deadline.C:
		c.releaseQueued(queueSize)
		c.fail(ErrWriterStalled)
		return ErrWriterStalled
	case <-c.done:
		c.releaseQueued(queueSize)
		return ErrUnavailable
	}
	select {
	case err := <-req.done:
		return err
	case <-c.done:
		return ErrUnavailable
	}
}

func (c *Client) releaseQueued(size int) {
	c.queueMu.Lock()
	c.queuedBytes -= int64(size)
	if c.queuedBytes < 0 {
		c.queuedBytes = 0
	}
	c.queueMu.Unlock()
}
func (c *Client) writeLoop() {
	for {
		select {
		case <-c.done:
			return
		case req := <-c.writeQueue:
			err := c.writeAll(req.data)
			c.releaseQueued(req.queueSize)
			req.done <- err
			if err != nil {
				c.fail(err)
				return
			}
		}
	}
}
func (c *Client) writeAll(data []byte) error {
	if deadlineWriter, ok := c.stdin.(interface{ SetWriteDeadline(time.Time) error }); ok {
		_ = deadlineWriter.SetWriteDeadline(c.now().Add(WriterStallLimit))
		defer deadlineWriter.SetWriteDeadline(time.Time{})
	}
	for len(data) > 0 {
		written, err := c.stdin.Write(data)
		if written > 0 {
			data = data[written:]
		}
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}
