package testutils

import (
	"net"
	"sync"
)

var _ net.Listener = (*MockListener)(nil)

// MockListener is a net.Listener that calls AcceptFunc to
// return the connection and error.
type MockListener struct {
	// AcceptFunc is the function called whenever Accept is called.
	// The i parameter indicates the 0-based index of the call.
	AcceptFunc func(i int) (net.Conn, error)
	// Error to return when Close is called on the Listener.
	CloseErr error
	// If set, this channel is closed when Close is called.
	CloseChan chan struct{}
	// Address to return when Addr is called on the Listener.
	Address net.Addr

	mu          sync.Mutex // protects close(CloseChan) and the indices
	acceptIndex int
	closeIndex  int
}

// AcceptCalls returns the number of times Accept was called.
func (l *MockListener) AcceptCalls() int {
	l.mu.Lock()
	i := l.acceptIndex
	l.mu.Unlock()
	return i
}

// CloseCalls returns the number of times Close was called.
func (l *MockListener) CloseCalls() int {
	l.mu.Lock()
	i := l.closeIndex
	l.mu.Unlock()
	return i
}

// Accept accepts a connection.
func (l *MockListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	i := l.acceptIndex
	l.acceptIndex++
	l.mu.Unlock()

	return l.AcceptFunc(i)
}

// Close closes the Listener.
func (l *MockListener) Close() error {
	l.mu.Lock()
	if l.CloseChan != nil {
		select {
		case <-l.CloseChan:
			// already closed
		default:
			close(l.CloseChan)
		}
	}
	l.closeIndex++
	l.mu.Unlock()

	return l.CloseErr
}

// Addr returns the address the Listener listens on.
func (l *MockListener) Addr() net.Addr {
	return l.Address
}
