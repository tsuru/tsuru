package main

import (
	"bytes"
	"github.com/timeredbull/tsuru/cmd"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"net/http"
	"testing"
)

type S struct{}

type transport struct {
	msg    string
	status int
}

var _ = Suite(&S{})
var manager cmd.Manager

func Test(t *testing.T) { TestingT(t) }

func (t *transport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	resp = &http.Response{
		Body:       ioutil.NopCloser(bytes.NewBufferString(t.msg)),
		StatusCode: t.status,
	}
	return resp, nil
}

func (s *S) SetUpTest(c *C) {
	var stdout, stderr bytes.Buffer
	manager = cmd.NewManager("glb", &stdout, &stderr)
}
