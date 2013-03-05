package local

import (
	"github.com/globocom/config"
	"github.com/globocom/tsuru/fs/testing"
	"io/ioutil"
	. "launchpad.net/gocheck"
)

func (s *S) TestAddRoute(c *C) {
	config.Set("local:domain", "andrewzito.com")
	config.Set("local:routes-path", "testdata")
	rfs := &testing.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	err := AddRoute("name", "127.0.0.1")
	c.Assert(err, IsNil)
	file, _ := rfs.Open("testdata/name")
	data, err := ioutil.ReadAll(file)
	c.Assert(err, IsNil)
	expected := `server {
	listen 80;
	name.andrewzito.com;
	location / {
		proxy_pass http://127.0.0.1;
	}
}`
	c.Assert(string(data), Equals, expected)
}
