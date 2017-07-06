package server

import "fmt"

type commandCmd struct{}

func (c commandCmd) Validate(cmdName string, req []string, s *Server) error {
	// support only argument-less COMMAND
	if len(req) != 1 {
		return fmt.Errorf("ERR wrong number of arguments for %v", cmdName)
	}
	return nil
}

func (c commandCmd) Execute(cmdName string, req []string, s *Server) (interface{}, error) {
	return commandNames, nil
}
