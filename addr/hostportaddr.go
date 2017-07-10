package addr

import (
	"net"
	"strconv"
	"strings"
)

// HostPortAddr is a TCP-based net.Addr that contains
// the unresolved host name and port number.
type HostPortAddr struct {
	Host string
	Port int
}

// Network returns the network type for this address, which is
// always "tcp".
func (a *HostPortAddr) Network() string {
	return "tcp"
}

// String returns the host:port form of the address.
func (a *HostPortAddr) String() string {
	return net.JoinHostPort(a.Host, strconv.Itoa(a.Port))
}

// ParseSSHUserAddr parses s into a HostPortAddr using the default SSH
// port if no port is provided. It returns the user specified in s
// as well as the parsed address, or an error. The string should have
// the format [user@]host[:port].
func ParseSSHUserAddr(s string) (user string, addr *HostPortAddr, err error) {
	if i := strings.Index(s, "@"); i > 0 {
		user = s[:i]
		s = s[i+1:]
	}

	// SSH server address, default port to 22
	addr, err = ParseAddr(s, 22)
	if err != nil {
		return "", nil, err
	}
	return user, addr, nil
}

// ParseAddr parses s into a HostPortAddr, using defaultPort if no port
// is specified in s. The string should have the format host:port
// or just host.
func ParseAddr(s string, defaultPort int) (*HostPortAddr, error) {
	host, port, err := net.SplitHostPort(s)
	if err != nil {
		// if port is required, return that error
		if defaultPort <= 0 {
			return nil, err
		}
		return &HostPortAddr{Host: strings.ToLower(s), Port: defaultPort}, nil
	}

	nPort, err := strconv.Atoi(port)
	if err != nil {
		return nil, err
	}

	if nPort == 0 {
		nPort = defaultPort
	}
	return &HostPortAddr{Host: strings.ToLower(host), Port: nPort}, nil
}
