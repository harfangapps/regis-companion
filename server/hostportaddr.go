package server

import (
	"net"
	"strconv"
)

// HostPortAddr is a TCP-based net.Addr that contains
// the unresolved host name and port number.
type HostPortAddr struct {
	Host string
	Port int
}

// Network returns the network type for this address, which is
// always "tcp".
func (a HostPortAddr) Network() string {
	return "tcp"
}

// String returns the host:port form of the address.
func (a HostPortAddr) String() string {
	return net.JoinHostPort(a.Host, strconv.Itoa(a.Port))
}
