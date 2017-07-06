package server

import (
	"fmt"

	"bitbucket.org/harfangapps/regis-companion/resp"
)

type pingCmd struct{}

func (c pingCmd) Validate(cmdName string, req []string, s *Server) error {
	// supports only the argument-less PING call
	if len(req) != 1 {
		return fmt.Errorf("ERR wrong number of arguments for %v", cmdName)
	}
	return nil
}

func (c pingCmd) Execute(cmdName string, req []string, s *Server) (interface{}, error) {
	return resp.Pong{}, nil
}
