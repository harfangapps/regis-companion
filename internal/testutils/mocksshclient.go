package testutils

import (
	"net"
	"sync"
)

// MockSSHClient implements an SSH client (more specifically, a dialCloser
// interface as defined in the server package) that can be used for
// tests.
type MockSSHClient struct {
	// Function to call when Dial is called.
	DialFunc func(i int, network, address string) (net.Conn, error)

	// Error to return when Close is called.
	CloseErr error
	// If set, the channel is closed when Close is called.
	CloseChan chan struct{}

	mu         sync.Mutex // protects close(CloseChan) and the indices
	dialIndex  int
	closeIndex int
}

// CloseCalls returns the number of times Close was called.
func (c *MockSSHClient) CloseCalls() int {
	c.mu.Lock()
	i := c.closeIndex
	c.mu.Unlock()
	return i
}

// DialCalls returns the number of times Dial was called.
func (c *MockSSHClient) DialCalls() int {
	c.mu.Lock()
	i := c.dialIndex
	c.mu.Unlock()
	return i
}

// Dial attempts a connection to the specified address.
func (c *MockSSHClient) Dial(n, addr string) (net.Conn, error) {
	c.mu.Lock()
	i := c.dialIndex
	c.dialIndex++
	c.mu.Unlock()
	return c.DialFunc(i, n, addr)
}

// Close closes the SSH client.
func (c *MockSSHClient) Close() error {
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
