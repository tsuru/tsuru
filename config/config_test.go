package config

import (
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})

var expected = map[interface{}]interface{}{
	"database": map[interface{}]interface{}{
		"host": "127.0.0.1",
		"port": 8080,
	},
	"auth": map[interface{}]interface{}{
		"salt": "xpto",
		"key":  "sometoken1234",
	},
	"xpto": "ble",
}

func (s *S) TearDownTest(c *C) {
	Configs = nil
}

func (s *S) TestConfig(c *C) {
	conf := `
database:
  host: 127.0.0.1
  port: 8080
auth:
  salt: xpto
  key: sometoken1234
xpto: ble
`
	err := ReadConfigBytes([]byte(conf))
	c.Assert(err, IsNil)
	c.Assert(Configs, DeepEquals, expected)
}

func (s *S) TestConfigFile(c *C) {
	configFile := "testdata/config.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, IsNil)
	c.Assert(Configs, DeepEquals, expected)
}

func (s *S) TestGetConfig(c *C) {
	configFile := "testdata/config.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, IsNil)
	c.Assert(Get("xpto"), DeepEquals, "ble")
	c.Assert(Get("database:host"), DeepEquals, "127.0.0.1")
}

func (s *S) TestGetString(c *C) {
	configFile := "testdata/config.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, IsNil)
	c.Assert(GetString("xpto"), DeepEquals, "ble")
	c.Assert(GetString("database:host"), DeepEquals, "127.0.0.1")
}
