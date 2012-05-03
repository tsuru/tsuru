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
	value, err := Get("xpto")
	c.Assert(err, IsNil)
	c.Assert(value, Equals, "ble")
	value, err = Get("database:host")
	c.Assert(err, IsNil)
	c.Assert(value, Equals, "127.0.0.1")
}

func (s *S) TestGetConfigReturnErrorsIfTheKeyIsNotFound(c *C) {
	configFile := "testdata/config.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, IsNil)
	value, err := Get("xpta")
	c.Assert(value, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^key xpta not found$")
	value, err = Get("database:hhh")
	c.Assert(value, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^key database:hhh not found$")
}

func (s *S) TestGetString(c *C) {
	configFile := "testdata/config.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, IsNil)
	c.Assert(GetString("xpto"), DeepEquals, "ble")
	c.Assert(GetString("database:host"), DeepEquals, "127.0.0.1")
}
