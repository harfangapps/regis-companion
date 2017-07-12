package addr

import "net"

// ListenFunc is a variable that holds the reference to
// the Listen function to use, so that it can be mocked
// for tests.
var ListenFunc = Listen

// Listen creates a Listener listening on the specified address.
// It returns the listener, the port it uses (0 if not
// listening on a TCP address), or an error.
//
// The main purpose is to listen on port 0 and let the system
// select a free TCP port, and then get that port number back.
// The returned Listener should then be passed to a server's Serve
// method to start accepting connections.
func Listen(addr net.Addr) (l net.Listener, port int, err error) {
	l, err = net.Listen(addr.Network(), addr.String())
	if err != nil {
		return nil, 0, err
	}
	if addr, ok := l.Addr().(*net.TCPAddr); ok {
		port = addr.Port
	}
	return l, port, nil
}
