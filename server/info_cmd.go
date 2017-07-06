package server

import "fmt"

type infoCmd struct{}

func (c infoCmd) Validate(cmdName string, req []string) error {
	if l := len(req); l < 1 || l > 2 {
		return fmt.Errorf("ERR wrong number of arguments for %v", cmdName)
	}
	return nil
}

func (c infoCmd) Execute(cmdName string, req []string) (interface{}, error) {
	return nil, nil
}
