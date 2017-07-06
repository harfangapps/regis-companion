package testutils

import (
	"net"
	"sync"
	"time"
)

// MockConn implements a net.Conn that can be used for testing.
// Deadline methods are no-ops.
type MockConn struct {
	// Function to call when Read is called.
	ReadFunc func(i int, b []byte) (int, error)
	// Function to call when Write is called.
	WriteFunc func(i int, b []byte) (int, error)

	// Error to return when Close is called.
	CloseErr error
	// If set, the channel is closed when Close is called.
	CloseChan chan struct{}

	// Local address to return when LocalAddr is called.
	LocalAddress net.Addr
	// Remote address to return when RemoteAddr is called.
	RemoteAddress net.Addr

	mu         sync.Mutex // protects close(CloseChan) and the indices
	readIndex  int
	writeIndex int
	closeIndex int
}

func (c *MockConn) CloseCalls() int {
	c.mu.Lock()
	i := c.closeIndex
	c.mu.Unlock()
	return i
}

func (c *MockConn) ReadCalls() int {
	c.mu.Lock()
	i := c.readIndex
	c.mu.Unlock()
	return i
}

func (c *MockConn) WriteCalls() int {
	c.mu.Lock()
	i := c.writeIndex
	c.mu.Unlock()
	return i
}

func (c *MockConn) Read(b []byte) (int, error) {
	c.mu.Lock()
	i := c.readIndex
	c.readIndex++
	c.mu.Unlock()
	return c.ReadFunc(i, b)
}

func (c *MockConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	i := c.writeIndex
	c.writeIndex++
	c.mu.Unlock()
	return c.WriteFunc(i, b)
}

func (c *MockConn) Close() error {
	c.mu.Lock()
	if c.CloseChan != nil {
		select {
		case <-c.CloseChan:
			// already closed
		default:
			close(c.CloseChan)
		}
	}
	c.closeIndex++
	c.mu.Unlock()
	return c.CloseErr
}

func (c *MockConn) LocalAddr() net.Addr {
	return c.LocalAddress
}

func (c *MockConn) RemoteAddr() net.Addr {
	return c.RemoteAddress
}

func (c *MockConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *MockConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *MockConn) SetWriteDeadline(t time.Time) error {
	return nil
}
