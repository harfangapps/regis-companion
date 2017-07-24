package server

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/harfangapps/regis-companion/resp"
)

type doer interface {
	Do(req *http.Request) (*http.Response, error)
}

type checkUpdatesCmd struct {
	client doer
}

const githubEndpoint = `https://api.github.com/repos/harfangapps/homebrew-harfangapps/releases/latest`

// CHECKUPDATES
func (c checkUpdatesCmd) Execute(cmdName string, req []string, s *Server) (interface{}, error) {
	if len(req) != 1 {
		return resp.Error(fmt.Sprintf("ERR wrong number of arguments for %v", cmdName)), nil
	}

	hreq, err := http.NewRequest("GET", githubEndpoint, nil)
	if err != nil {
		return resp.Error(fmt.Sprintf("ERR failed to create request: %v", err)), nil
	}

	res, err := c.client.Do(hreq)
	if err != nil {
		return resp.Error(fmt.Sprintf("ERR request failed: %v", err)), nil
	}
	defer res.Body.Close()

	v, err := readRelease(res.Body)
	if err != nil {
		return resp.Error(fmt.Sprintf("ERR failed to read version: %v", err)), nil
	}

	// return true if the release is different than the current version
	// (ideally, should be later than, but in practice different is enough)
	return v != Version, nil
}

func readRelease(r io.Reader) (string, error) {
	var obj struct {
		TagName string `json:"tag_name"`
	}

	data, err := ioutil.ReadAll(r)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return "", err
	}
	return obj.TagName, nil
}
