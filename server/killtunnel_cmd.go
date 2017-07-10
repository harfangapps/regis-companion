package server

import (
	"fmt"

	"bitbucket.org/harfangapps/regis-companion/addr"
	"bitbucket.org/harfangapps/regis-companion/resp"
)

type killTunnelCmd struct{}

// KILLTUNNEL [user@]ssh.server.host[:port] remote.server.host:port
func (c killTunnelCmd) Execute(cmdName string, req []string, s *Server) (interface{}, error) {
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

	if err := s.killTunnel(serverAddr, remoteAddr, user); err != nil {
		return resp.Error(fmt.Sprintf("ERR failed to kill tunnel: %v", err)), nil
	}
	return resp.OK{}, nil
}
