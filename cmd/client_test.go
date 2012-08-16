package cmd

import (
	"github.com/timeredbull/tsuru/fs/testing"
	. "launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestShouldReturnBodyMessageOnError(c *C) {
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, IsNil)

	client := NewClient(&http.Client{Transport: &transport{msg: "You must be authenticated to execute this command.", status: http.StatusUnauthorized}})
	response, err := client.Do(request)
	c.Assert(response, IsNil)
	c.Assert(err.Error(), Equals, "You must be authenticated to execute this command.")
}

func (s *S) TestShouldReturnErrorWhenServerIsDown(c *C) {
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, IsNil)
	client := NewClient(&http.Client{})
	_, err = client.Do(request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^A problem occurred while trying to do the request. Original error message is: Get /: unsupported protocol scheme \"\" \n$")
}

func (s *S) TestShouldNotIncludeTheHeaderAuthorizationWhenTheTsuruTokenFileIsMissing(c *C) {
	fsystem = &testing.FailureFs{}
	defer func() {
		fsystem = nil
	}()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, IsNil)
	trans := &transport{msg: "", status: http.StatusOK}
	client := NewClient(&http.Client{Transport: trans})
	_, err = client.Do(request)
	c.Assert(err, IsNil)
	header := map[string][]string(request.Header)
	_, ok := header["Authorization"]
	c.Assert(ok, Equals, false)
}

func (s *S) TestShouldIncludeTheHeaderAuthorizationWhenTsuruTokenFileExists(c *C) {
	fsystem = &testing.RecordingFs{FileContent: "mytoken"}
	defer func() {
		fsystem = nil
	}()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, IsNil)
	trans := &transport{msg: "", status: http.StatusOK}
	client := NewClient(&http.Client{Transport: trans})
	_, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(request.Header.Get("Authorization"), Equals, "mytoken")
}
