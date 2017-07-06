package server

import (
	"fmt"

	"bitbucket.org/harfangapps/regis-companion/resp"
)

type commandCmd struct{}

// COMMAND
func (c commandCmd) Execute(cmdName string, req []string, s *Server) (interface{}, error) {
	// support only argument-less COMMAND
	if len(req) != 1 {
		return resp.Error(fmt.Sprintf("ERR wrong number of arguments for %v", cmdName)), nil
	}
	return commandNames, nil
}
