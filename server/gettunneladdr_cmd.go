package server

import (
	"fmt"

	"bitbucket.org/harfangapps/regis-companion/addr"
	"bitbucket.org/harfangapps/regis-companion/resp"
)

type getTunnelAddrCmd struct{}

// GETTUNNELADDR [user@]ssh.server.host[:port] remote.server.host:port
func (c getTunnelAddrCmd) Execute(cmdName string, req []string, s *Server) (interface{}, error) {
	if len(req) != 3 {
		return resp.Error(fmt.Sprintf("ERR wrong number of arguments for %v", cmdName)), nil
	}

	user, serverAddr, err := addr.ParseSSHUserAddr(req[1])
	if err != nil {
		return resp.Error(fmt.Sprintf("ERR invalid SSH server address: %s", err)), nil
	}

	// remote address, port required
	remoteAddr, err := addr.ParseAddr(req[2], 0)
	if err != nil {
		return resp.Error(fmt.Sprintf("ERR invalid remote server address: %s", err)), nil
	}

	addr, err := s.getTunnelAddr(user, serverAddr, remoteAddr)
	if err != nil {
		return resp.Error(fmt.Sprintf("ERR failed to start tunnel: %v", err)), nil
	}
	return addr.String(), nil
}
