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

func resetConfig() {
	Configs = nil
}

func (s *S) TestConfig(c *C) {
	defer resetConfig()
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
	defer resetConfig()
	configFile := "testdata/config.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, IsNil)
	c.Assert(Configs, DeepEquals, expected)
}

func (s *S) TestGetConfig(c *C) {
	defer func() {
		Configs = nil
	}()
	configFile := "testdata/config.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, IsNil)
	c.Assert(Get("xpto"), DeepEquals, "ble")
	c.Assert(Get("database:host"), DeepEquals, "127.0.0.1")
}
