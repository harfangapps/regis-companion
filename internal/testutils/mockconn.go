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
	closedAt   time.Time
}

// CloseCalls returns the number of times Close was called.
func (c *MockConn) CloseCalls() int {
	c.mu.Lock()
	i := c.closeIndex
	c.mu.Unlock()
	return i
}

func (c *MockConn) ClosedAt() time.Time {
	c.mu.Lock()
	t := c.closedAt
	c.mu.Unlock()
	return t
}

// ReadCalls returns the number of times Read was called.
func (c *MockConn) ReadCalls() int {
	c.mu.Lock()
	i := c.readIndex
	c.mu.Unlock()
	return i
}

// WriteCalls returns the number of times Write was called.
func (c *MockConn) WriteCalls() int {
	c.mu.Lock()
	i := c.writeIndex
	c.mu.Unlock()
	return i
}

// Read implements io.Reader for MockConn.
func (c *MockConn) Read(b []byte) (int, error) {
	c.mu.Lock()
	i := c.readIndex
	c.readIndex++
	c.mu.Unlock()

	if c.ReadFunc == nil {
		return 0, nil
	}
	return c.ReadFunc(i, b)
}

// Write implements io.Writer for MockConn.
func (c *MockConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	i := c.writeIndex
	c.writeIndex++
	c.mu.Unlock()

	if c.WriteFunc == nil {
		return 0, nil
	}
	return c.WriteFunc(i, b)
}

// Close implements io.Closer for MockConn.
func (c *MockConn) Close() error {
	c.mu.Lock()
	if c.CloseChan != nil {
		select {
		case <-c.CloseChan:
			// already closed
		default:
			close(c.CloseChan)
			c.closedAt = time.Now()
		}
	}
	c.closeIndex++
	c.mu.Unlock()
	return c.CloseErr
}

// LocalAddr returns the local address of the connection.
func (c *MockConn) LocalAddr() net.Addr {
	return c.LocalAddress
}

// RemoteAddr returns the remote address of the connection.
func (c *MockConn) RemoteAddr() net.Addr {
	return c.RemoteAddress
}

// SetDeadline is a no-op for MockConn.
func (c *MockConn) SetDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline is a no-op for MockConn.
func (c *MockConn) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline is a no-op for MockConn.
func (c *MockConn) SetWriteDeadline(t time.Time) error {
	return nil
}
