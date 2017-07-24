package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/harfangapps/regis-companion/resp"
)

type jsonDoer string

func (d jsonDoer) Do(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	rec.WriteString(string(d))
	return rec.Result(), nil
}

type errDoer string

func (d errDoer) Do(req *http.Request) (*http.Response, error) {
	return nil, errors.New(string(d))
}

func TestCheckUpdates(t *testing.T) {
	var filledDoer = jsonDoer(`{
  "url": "https://api.github.com/repos/octocat/Hello-World/releases/1",
  "html_url": "https://github.com/octocat/Hello-World/releases/v1.0.0",
  "assets_url": "https://api.github.com/repos/octocat/Hello-World/releases/1/assets",
  "upload_url": "https://uploads.github.com/repos/octocat/Hello-World/releases/1/assets{?name,label}",
  "tarball_url": "https://api.github.com/repos/octocat/Hello-World/tarball/v1.0.0",
  "zipball_url": "https://api.github.com/repos/octocat/Hello-World/zipball/v1.0.0",
  "id": 1,
  "tag_name": "v1.0.0",
  "target_commitish": "master",
  "name": "v1.0.0",
  "body": "Description of the release",
  "draft": false,
  "prerelease": false,
  "created_at": "2013-02-27T19:35:32Z",
  "published_at": "2013-02-27T19:35:32Z",
  "author": {
    "login": "octocat",
    "id": 1,
    "avatar_url": "https://github.com/images/error/octocat_happy.gif",
    "gravatar_id": "",
    "url": "https://api.github.com/users/octocat",
    "html_url": "https://github.com/octocat",
    "followers_url": "https://api.github.com/users/octocat/followers",
    "following_url": "https://api.github.com/users/octocat/following{/other_user}",
    "gists_url": "https://api.github.com/users/octocat/gists{/gist_id}",
    "starred_url": "https://api.github.com/users/octocat/starred{/owner}{/repo}",
    "subscriptions_url": "https://api.github.com/users/octocat/subscriptions",
    "organizations_url": "https://api.github.com/users/octocat/orgs",
    "repos_url": "https://api.github.com/users/octocat/repos",
    "events_url": "https://api.github.com/users/octocat/events{/privacy}",
    "received_events_url": "https://api.github.com/users/octocat/received_events",
    "type": "User",
    "site_admin": false
  },
  "assets": [
    {
      "url": "https://api.github.com/repos/octocat/Hello-World/releases/assets/1",
      "browser_download_url": "https://github.com/octocat/Hello-World/releases/download/v1.0.0/example.zip",
      "id": 1,
      "name": "example.zip",
      "label": "short description",
      "state": "uploaded",
      "content_type": "application/zip",
      "size": 1024,
      "download_count": 42,
      "created_at": "2013-02-27T19:35:32Z",
      "updated_at": "2013-02-27T19:35:32Z",
      "uploader": {
        "login": "octocat",
        "id": 1,
        "avatar_url": "https://github.com/images/error/octocat_happy.gif",
        "gravatar_id": "",
        "url": "https://api.github.com/users/octocat",
        "html_url": "https://github.com/octocat",
        "followers_url": "https://api.github.com/users/octocat/followers",
        "following_url": "https://api.github.com/users/octocat/following{/other_user}",
        "gists_url": "https://api.github.com/users/octocat/gists{/gist_id}",
        "starred_url": "https://api.github.com/users/octocat/starred{/owner}{/repo}",
        "subscriptions_url": "https://api.github.com/users/octocat/subscriptions",
        "organizations_url": "https://api.github.com/users/octocat/orgs",
        "repos_url": "https://api.github.com/users/octocat/repos",
        "events_url": "https://api.github.com/users/octocat/events{/privacy}",
        "received_events_url": "https://api.github.com/users/octocat/received_events",
        "type": "User",
        "site_admin": false
      }
    }
  ]
}`)

	var (
		emptyDoer    = jsonDoer(``)
		nilDoer      = jsonDoer(`null`)
		emptyObjDoer = jsonDoer(`{}`)
	)

	cases := []struct {
		Doer    doer
		Version string
		Want    bool
	}{
		{filledDoer, "", true},
		{filledDoer, "v0.0.1", true},
		{filledDoer, "v1.0.0", false},
		{filledDoer, "v1.1.0", true},

		{emptyDoer, "v1.0.0", false},
		{emptyDoer, "", false},

		{nilDoer, "v1.0.0", false},
		{nilDoer, "", false},

		{emptyObjDoer, "v1.0.0", false},
		{emptyObjDoer, "", false},
	}

	for _, c := range cases {
		cmd := checkUpdatesCmd{client: c.Doer}
		Version = c.Version
		got, err := cmd.Execute("checkupdates", []string{"checkupdates"}, nil)
		if err != nil {
			t.Errorf("%s: want no error, got %v", c.Version, err)
			continue
		}

		if got != c.Want {
			t.Errorf("%s: want %v, got %v", c.Version, c.Want, got)
		}
	}
}

func TestCheckUpdatesRequestError(t *testing.T) {
	cmd := checkUpdatesCmd{client: errDoer("eof")}
	Version = "v1.0.0"

	got, err := cmd.Execute("checkupdates", []string{"checkupdates"}, nil)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	if s, ok := got.(resp.Error); !ok {
		t.Fatalf("want resp.Error, got %T", got)
	} else if !strings.HasPrefix(string(s), "ERR ") {
		t.Errorf("want RESP error, got %v", s)
	}
}
