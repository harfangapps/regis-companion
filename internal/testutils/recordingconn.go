package testutils

import (
	"bytes"
	"io"
	"net"
	"sync"
	"time"
)

// RecordingConn implements a net.Conn that records the bytes
// written to it. Deadline methods are no-ops.
type RecordingConn struct {
	// Reader to read bytes from when Read is called.
	ReadFrom io.Reader
	// Error to return when Close is called.
	CloseErr error
	// Local address to return when LocalAddr is called.
	LocalAddress net.Addr
	// Remote address to return when RemoteAddr is called.
	RemoteAddress net.Addr

	buf    bytes.Buffer
	mu     sync.Mutex
	closed bool
}

func (c *RecordingConn) Bytes() []byte {
	return c.buf.Bytes()
}

func (c *RecordingConn) String() string {
	return c.buf.String()
}

func (c *RecordingConn) Read(b []byte) (int, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return 0, io.EOF
	}
	c.mu.Unlock()
	return c.ReadFrom.Read(b)
}

func (c *RecordingConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return 0, io.EOF
	}
	c.mu.Unlock()
	return c.buf.Write(b)
}

func (c *RecordingConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return c.CloseErr
}

func (c *RecordingConn) LocalAddr() net.Addr {
	return c.LocalAddress
}

func (c *RecordingConn) RemoteAddr() net.Addr {
	return c.RemoteAddress
}

func (c *RecordingConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *RecordingConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *RecordingConn) SetWriteDeadline(t time.Time) error {
	return nil
}
