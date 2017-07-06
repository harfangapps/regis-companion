package server

import (
	"errors"
	"fmt"
	"net"
	"strconv"

	"bitbucket.org/harfangapps/regis-companion/resp"
)

type getTunnelAddrCmd struct{}

// GETTUNNELADDR ssh.server.host[:port] remote.server.host:port
func (c getTunnelAddrCmd) Execute(cmdName string, req []string, s *Server) (interface{}, error) {
	if len(req) != 3 {
		return resp.Error(fmt.Sprintf("ERR wrong number of arguments for %v", cmdName)), nil
	}

	// SSH server address, default port to 22
	serverAddr, err := parseAddr(req[1], 22)
	if err != nil {
		return resp.Error(fmt.Sprintf("ERR invalid SSH server address: %s", err)), nil
	}
	// remote address, port required
	remoteAddr, err := parseAddr(req[2], 0)
	if err != nil {
		return resp.Error(fmt.Sprintf("ERR invalid remote server address: %s", err)), nil
	}
	_, _ = serverAddr, remoteAddr

	return nil, nil
}

func parseAddr(s string, defaultPort int) (net.Addr, error) {
	host, port, err := net.SplitHostPort(s)
	if err != nil {
		// if port is required, return that error
		if defaultPort <= 0 {
			return nil, err
		}
		return HostPortAddr{Host: s, Port: defaultPort}, nil
	}

	nPort, err := strconv.Atoi(port)
	if err != nil {
		return nil, errors.New("invalid port number")
	}

	if nPort == 0 {
		nPort = defaultPort
	}
	return HostPortAddr{Host: host, Port: nPort}, nil
}
