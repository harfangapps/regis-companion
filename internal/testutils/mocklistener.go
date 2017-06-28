package testutils

import "net"

var _ net.Listener = (*MockListener)(nil)

// MockListener is a net.Listener that calls AcceptFunc to
// return the connection and error.
type MockListener struct {
	// AcceptFunc is the function called whenever Accept is called.
	// The i parameter indicates the 0-based index of the call.
	AcceptFunc func(i int) (net.Conn, error)
	// Error to return when Close is called on the Listener.
	CloseErr error
	// Address to return when Addr is called on the Listener.
	Address net.Addr

	acceptIndex int
}

func (l *MockListener) AcceptCalls() int {
	return l.acceptIndex
}

func (l *MockListener) Accept() (net.Conn, error) {
	defer func() { l.acceptIndex += 1 }()
	return l.AcceptFunc(l.acceptIndex)
}

func (l *MockListener) Close() error {
	return l.CloseErr
}

func (l *MockListener) Addr() net.Addr {
	return l.Address
}
