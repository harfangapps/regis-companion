package server

import (
	"fmt"

	"github.com/harfangapps/regis-companion/resp"
)

type pingCmd struct{}

// PING
func (c pingCmd) Execute(cmdName string, req []string, s *Server) (interface{}, error) {
	// supports only the argument-less PING call
	if len(req) != 1 {
		return resp.Error(fmt.Sprintf("ERR wrong number of arguments for %v", cmdName)), nil
	}
	return resp.Pong{}, nil
}
