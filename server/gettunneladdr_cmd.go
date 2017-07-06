package server

import (
	"fmt"

	"bitbucket.org/harfangapps/regis-companion/resp"
)

type getTunnelAddrCmd struct{}

// GETTUNNELADDR ssh.server.host[:port] remote.server.host:port
func (c getTunnelAddrCmd) Execute(cmdName string, req []string, s *Server) (interface{}, error) {
	if len(req) != 3 {
		return resp.Error(fmt.Sprintf("ERR wrong number of arguments for %v", cmdName)), nil
	}

	// SSH server address, default port to 22

	// Remote server address, port required
	return nil, nil
}
