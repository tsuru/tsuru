// Copyright 2015 Globo.com. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

var empty = make(map[interface{}]interface{}, 0)

var expected = map[interface{}]interface{}{
	"database": map[interface{}]interface{}{
		"host": "127.0.0.1",
		"user": "root",
		"port": 8080,
	},
	"auth": map[interface{}]interface{}{
		"salt": "xpto",
		"key":  "sometoken1234",
	},
	"xpto":           "ble",
	"istrue":         false,
	"fakebool":       "foo",
	"names":          []interface{}{"Mary", "John", "Anthony", "Gopher"},
	"multiple-types": []interface{}{"Mary", 50, 5.3, true},
	"negative":       -10,
	"myfloatvalue":   0.95,
}

func (s *S) TearDownTest(c *check.C) {
	configs.Store(empty)
}

func (s *S) TestConfig(c *check.C) {
	conf := `
database:
  host: 127.0.0.1
  user: root
  port: 8080
auth:
  salt: xpto
  key: sometoken1234
xpto: ble
istrue: false
fakebool: foo
names:
  - Mary
  - John
  - Anthony
  - Gopher
multiple-types:
  - Mary
  - 50
  - 5.3
  - true
negative: -10
myfloatvalue: 0.95
`
	err := ReadConfigBytes([]byte(conf))
	c.Assert(err, check.IsNil)
	c.Assert(configs.Data(), check.DeepEquals, expected)
}

func (s *S) TestConfigFile(c *check.C) {
	configFile := "testdata/config.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	c.Assert(configs.Data(), check.DeepEquals, expected)
}

func (s *S) TestConfigFileIsAllInOrNothing(c *check.C) {
	err := ReadConfigFile("testdata/config.yml")
	c.Assert(err, check.IsNil)
	c.Assert(configs.Data(), check.DeepEquals, expected)
	err = ReadConfigFile("testdata/invalid_config.yml")
	c.Assert(err, check.NotNil)
	c.Assert(configs.Data(), check.DeepEquals, expected)
}

func (s *S) TestConfigFileUnknownFile(c *check.C) {
	err := ReadConfigFile("/some/unknwon/file/path")
	c.Assert(err, check.NotNil)
}

func (s *S) TestWatchConfigFile(c *check.C) {
	err := exec.Command("cp", "testdata/config.yml", "/tmp/config-test.yml").Run()
	c.Assert(err, check.IsNil)
	err = ReadAndWatchConfigFile("/tmp/config-test.yml")
	c.Assert(err, check.IsNil)
	c.Check(configs.Data(), check.DeepEquals, expected)
	err = exec.Command("cp", "testdata/config2.yml", "/tmp/config-test.yml").Run()
	c.Assert(err, check.IsNil)
	time.Sleep(1e9)
	expectedAuth := map[interface{}]interface{}{
		"salt": "xpta",
		"key":  "sometoken1234",
	}
	c.Check(configs.Data()["auth"], check.DeepEquals, expectedAuth)
}

func (s *S) TestWatchConfigFileUnknownFile(c *check.C) {
	err := ReadAndWatchConfigFile("/some/unknwon/file/path")
	c.Assert(err, check.NotNil)
}

func (s *S) TestBytes(c *check.C) {
	Set("database:host", "127.0.0.1")
	Set("database:port", 3306)
	Set("database:user", "root")
	Set("database:password", "s3cr3t")
	Set("database:name", "mydatabase")
	Set("something", "otherthing")
	data, err := Bytes()
	c.Assert(err, check.IsNil)
	err = ReadConfigBytes(data)
	c.Assert(err, check.IsNil)
	v, err := Get("database:host")
	c.Assert(err, check.IsNil)
	c.Assert(v, check.Equals, "127.0.0.1")
	v, err = Get("database:port")
	c.Assert(err, check.IsNil)
	c.Assert(v, check.Equals, 3306)
	v, err = Get("database:user")
	c.Assert(err, check.IsNil)
	c.Assert(v, check.Equals, "root")
	v, err = Get("database:password")
	c.Assert(err, check.IsNil)
	c.Assert(v, check.Equals, "s3cr3t")
	v, err = Get("database:name")
	c.Assert(err, check.IsNil)
	c.Assert(v, check.Equals, "mydatabase")
	v, err = Get("something")
	c.Assert(err, check.IsNil)
	c.Assert(v, check.Equals, "otherthing")
}

func (s *S) TestWriteConfigFile(c *check.C) {
	Set("database:host", "127.0.0.1")
	Set("database:port", 3306)
	Set("database:user", "root")
	Set("database:password", "s3cr3t")
	Set("database:name", "mydatabase")
	Set("something", "otherthing")
	err := WriteConfigFile("/tmp/config-test.yaml", 0644)
	c.Assert(err, check.IsNil)
	defer os.Remove("/tmp/config-test.yaml")
	configs.Store(empty)
	err = ReadConfigFile("/tmp/config-test.yaml")
	c.Assert(err, check.IsNil)
	v, err := Get("database:host")
	c.Assert(err, check.IsNil)
	c.Assert(v, check.Equals, "127.0.0.1")
	v, err = Get("database:port")
	c.Assert(err, check.IsNil)
	c.Assert(v, check.Equals, 3306)
	v, err = Get("database:user")
	c.Assert(err, check.IsNil)
	c.Assert(v, check.Equals, "root")
	v, err = Get("database:password")
	c.Assert(err, check.IsNil)
	c.Assert(v, check.Equals, "s3cr3t")
	v, err = Get("database:name")
	c.Assert(err, check.IsNil)
	c.Assert(v, check.Equals, "mydatabase")
	v, err = Get("something")
	c.Assert(err, check.IsNil)
	c.Assert(v, check.Equals, "otherthing")
}

func (s *S) TestGetConfig(c *check.C) {
	configFile := "testdata/config.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := Get("xpto")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, "ble")
	value, err = Get("database:host")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, "127.0.0.1")
}

func (s *S) TestGetConfigReturnErrorsIfTheKeyIsNotFound(c *check.C) {
	configFile := "testdata/config.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := Get("xpta")
	c.Assert(value, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, `key "xpta" not found`)
	value, err = Get("database:hhh")
	c.Assert(value, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, `key "database:hhh" not found`)
	_, err = Get("fakebool:err")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, ErrMismatchConf)
	configFile = "testdata/config5.yml"
	err = ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	err = os.Setenv("DATABASE", "{\"host\":\"6.6.6.6\"}")
	defer os.Unsetenv("DATABASE")
	_, err = Get("database:port")
	c.Assert(err.Error(), check.Equals, `key "database:port" not found`)
	err = os.Setenv("DATABASE", "{\"mongo\": {\"host\":\"6.6.6.6\"}}")
	_, err = Get("database:mongo:port")
	c.Assert(err.Error(), check.Equals, `key "database:mongo:port" not found`)
}

func (s *S) TestGetConfigReturnErrorsIfMismatchConf(c *check.C) {
	configFile := "testdata/wrong-config.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	_, err = Get("mismatch:err")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, ErrMismatchConf)
}

func (s *S) TestGetConfigExpandVars(c *check.C) {
	configFile := "testdata/config3.yml"
	err := os.Setenv("DBHOST", "6.6.6.6")
	defer os.Setenv("DBHOST", "")
	c.Assert(err, check.IsNil)
	err = ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := Get("database:host")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, "6.6.6.6")
}

func (s *S) TestGetConfigExpandVarsJsonObject(c *check.C) {
	configFile := "testdata/config5.yml"
	err := os.Setenv("DATABASE", "{\"host\":\"6.6.6.6\", \"port\": 27017}")
	defer os.Unsetenv("DATABASE")
	c.Assert(err, check.IsNil)
	err = ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := Get("database")
	c.Assert(err, check.IsNil)
	valueMap := value.(map[interface{}]interface{})
	expectedMap := map[interface{}]interface{}{
		"host": "6.6.6.6",
		"port": float64(27017),
	}
	for k := range valueMap {
		c.Assert(valueMap[k], check.Equals, expectedMap[k])
	}
	err = os.Setenv("DATABASE", "{\"mongo\": {\"config\": {\"host\":\"6.6.6.6\", \"port\": 27017}}}")
	c.Assert(err, check.IsNil)
	value, err = Get("database:mongo:config")
	c.Assert(err, check.IsNil)
	valueMap = value.(map[interface{}]interface{})
	for k := range valueMap {
		c.Assert(valueMap[k], check.Equals, expectedMap[k])
	}
}

func (s *S) TestGetConfigExpandVarsJsonList(c *check.C) {
	configFile := "testdata/config5.yml"
	err := os.Setenv("TYPES", "[10, 5, \"abc\"]")
	defer os.Unsetenv("TYPES")
	c.Assert(err, check.IsNil)
	err = ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := Get("multiple-types")
	c.Assert(err, check.IsNil)
	expectedList := []interface{}{float64(10), float64(5), "abc"}
	for i := range expectedList {
		c.Assert(expectedList[i], check.Equals, value.([]interface{})[i])
	}
	err = os.Setenv("TYPES", "{\"obj1\": {\"obj2\": {\"list\": [10, 5, \"abc\"]}}}")
	c.Assert(err, check.IsNil)
	value, err = Get("multiple-types:obj1:obj2:list")
	c.Assert(err, check.IsNil)
	for i := range expectedList {
		c.Assert(expectedList[i], check.Equals, value.([]interface{})[i])
	}
}

func (s *S) TestGetStringExpandVars(c *check.C) {
	configFile := "testdata/config3.yml"
	err := os.Setenv("DBHOST", "6.6.6.6")
	defer os.Setenv("DBHOST", "")
	c.Assert(err, check.IsNil)
	err = ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := GetString("database:host")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, "6.6.6.6")
}

func (s *S) TestGetConfigStringExpandVarsJsonObject(c *check.C) {
	configFile := "testdata/config5.yml"
	err := os.Setenv("DATABASE", "{\"host\":\"6.6.6.6\", \"port\": 27017}")
	defer os.Unsetenv("DATABASE")
	c.Assert(err, check.IsNil)
	err = ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := GetString("database:host")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, "6.6.6.6")
}

func (s *S) TestGetIntExpandVars(c *check.C) {
	configFile := "testdata/config3.yml"
	err := os.Setenv("DBPORT", "6680")
	defer os.Setenv("DBPORT", "")
	c.Assert(err, check.IsNil)
	err = ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := GetInt("database:port")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, 6680)
}

func (s *S) TestGetString(c *check.C) {
	configFile := "testdata/config.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	Set("some-key", "some-value")
	value, err := GetString("xpto")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, "ble")
	value, err = GetString("database:host")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, "127.0.0.1")
	value, err = GetString("some-key")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, "some-value")
	value, err = GetString("database:port")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, "8080")
}

func (s *S) TestGetInt(c *check.C) {
	configFile := "testdata/config.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := GetInt("database:port")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, 8080)
	value, err = GetInt("xpto")
	c.Assert(err, check.NotNil)
	value, err = GetInt("something-unknown")
	c.Assert(err, check.NotNil)
	c.Assert(value, check.Equals, 0)
}

func (s *S) TestGetIntExpandVarsJsonObject(c *check.C) {
	configFile := "testdata/config5.yml"
	err := os.Setenv("DATABASE", "{\"mongo\": {\"host\":\"6.6.6.6\", \"port\": 27017}}")
	defer os.Unsetenv("DATABASE")
	c.Assert(err, check.IsNil)
	err = ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := GetInt("database:mongo:port")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, 27017)
}

func (s *S) TestGetFloat(c *check.C) {
	configFile := "testdata/config.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := GetFloat("myfloatvalue")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, 0.95)
	value, err = GetFloat("database:port")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, 8080.0)
	value, err = GetFloat("xpto")
	c.Assert(err, check.NotNil)
	c.Assert(value, check.Equals, 0.0)
}

func (s *S) TestGetFloatExpandVarsJsonObject(c *check.C) {
	configFile := "testdata/config5.yml"
	err := os.Setenv("DATABASE", "{\"mongo\": {\"host\":\"6.6.6.6\", \"port\": 27017}}")
	defer os.Unsetenv("DATABASE")
	c.Assert(err, check.IsNil)
	err = ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := GetFloat("database:mongo:port")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, float64(27017))
}

func (s *S) TestGetUint(c *check.C) {
	configFile := "testdata/config.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := GetUint("database:port")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, uint(8080))
	_, err = GetUint("negative")
	c.Assert(err, check.NotNil)
	_, err = GetUint("auth:salt")
	c.Assert(err, check.NotNil)
	_, err = GetUint("Unknown")
	c.Assert(err, check.NotNil)
}

func (s *S) TestGetUintExpandVarsJsonObject(c *check.C) {
	configFile := "testdata/config5.yml"
	err := os.Setenv("DATABASE", "{\"mongo\": {\"host\":\"6.6.6.6\", \"port\": 27017}}")
	defer os.Unsetenv("DATABASE")
	c.Assert(err, check.IsNil)
	err = ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := GetUint("database:mongo:port")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, uint(27017))
}

func (s *S) TestGetStringShouldReturnErrorIfTheKeyDoesNotRepresentAStringOrInt(c *check.C) {
	configFile := "testdata/config.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := GetString("myfloatvalue")
	c.Assert(value, check.Equals, "")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `value for the key "myfloatvalue" is not a string|int|int64`)
}

func (s *S) TestGetStringShouldReturnErrorIfTheKeyDoesNotExist(c *check.C) {
	configFile := "testdata/config.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := GetString("xpta")
	c.Assert(value, check.Equals, "")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `key "xpta" not found`)
}

func (s *S) TestGetDuration(c *check.C) {
	configFile := "testdata/config2.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := GetDuration("interval")
	c.Check(err, check.IsNil)
	c.Check(value, check.Equals, time.Duration(1e9))
	value, err = GetDuration("superinterval")
	c.Check(err, check.IsNil)
	c.Check(value, check.Equals, time.Duration(1e9))
	value, err = GetDuration("wait")
	c.Check(err, check.IsNil)
	c.Check(value, check.Equals, time.Duration(1e6))
	value, err = GetDuration("one_year")
	c.Check(err, check.IsNil)
	c.Check(value, check.Equals, time.Duration(365*24*time.Hour))
	value, err = GetDuration("nano")
	c.Check(err, check.IsNil)
	c.Check(value, check.Equals, time.Duration(1))
	value, err = GetDuration("human-interval")
	c.Check(err, check.IsNil)
	c.Check(value, check.Equals, time.Duration(10e9))
}

func (s *S) TestGetDurationUnknown(c *check.C) {
	configFile := "testdata/config2.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := GetDuration("intervalll")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `key "intervalll" not found`)
	c.Assert(value, check.Equals, time.Duration(0))
}

func (s *S) TestGetDurationInvalid(c *check.C) {
	configFile := "testdata/config.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := GetDuration("auth:key")
	c.Assert(value, check.Equals, time.Duration(0))
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `value for the key "auth:key" is not a duration`)
}

func (s *S) TestGetBool(c *check.C) {
	configFile := "testdata/config.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := GetBool("istrue")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, false)
}

func (s *S) TestGetBoolExpandVarsJsonObject(c *check.C) {
	configFile := "testdata/config5.yml"
	err := os.Setenv("ISTRUE", "{\"obj\": {\"istrue\": true}}")
	defer os.Unsetenv("ISTRUE")
	c.Assert(err, check.IsNil)
	err = ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := GetBool("istrue:obj:istrue")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, true)
}

func (s *S) TestGetBoolWithNonBoolConfValue(c *check.C) {
	configFile := "testdata/config.yml"
	err := ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := GetBool("fakebool")
	c.Assert(value, check.Equals, false)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `value for the key "fakebool" is not a boolean`)
}

func (s *S) TestGetBoolUndeclaredValue(c *check.C) {
	value, err := GetBool("something-unknown")
	c.Assert(value, check.Equals, false)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, `key "something-unknown" not found`)
}

func (s *S) TestGetList(c *check.C) {
	var tests = []struct {
		key      string
		expected []string
		err      error
	}{
		{
			key:      "names",
			expected: []string{"Mary", "John", "Anthony", "Gopher"},
			err:      nil,
		},
		{
			key:      "multiple-types",
			expected: []string{"Mary", "50", "5.3", "true"},
			err:      nil,
		},
		{
			key:      "fakebool",
			expected: nil,
			err:      &InvalidValue{"fakebool", "list"},
		},
		{
			key:      "dynamic",
			expected: []string{"Mary", "Petter"},
			err:      nil,
		},
	}
	err := ReadConfigFile("testdata/config.yml")
	c.Assert(err, check.IsNil)
	Set("dynamic", []string{"Mary", "Petter"})
	for _, t := range tests {
		values, err := GetList(t.key)
		c.Check(err, check.DeepEquals, t.err)
		c.Check(values, check.DeepEquals, t.expected)
	}
}

func (s *S) TestGetListUndeclaredValue(c *check.C) {
	value, err := GetList("something-unknown")
	c.Assert(value, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, `key "something-unknown" not found`)
}

func (s *S) TestGetListWithStringers(c *check.C) {
	err := errors.New("failure")
	Set("what", []interface{}{err})
	value, err := GetList("what")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.DeepEquals, []string{"failure"})
}

func (s *S) TestGetListExpandedVarsJsonList(c *check.C) {
	configFile := "testdata/config5.yml"
	err := os.Setenv("TYPES", "[\"a\", \"abc\"]")
	defer os.Unsetenv("TYPES")
	c.Assert(err, check.IsNil)
	err = ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := GetList("multiple-types")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.DeepEquals, []string{"a", "abc"})
}

func (s *S) TestGetListExpandedVarsJsonObject(c *check.C) {
	configFile := "testdata/config5.yml"
	err := os.Setenv("TYPES", "{\"obj\": [\"a\", \"abc\"]}")
	defer os.Unsetenv("TYPES")
	c.Assert(err, check.IsNil)
	err = ReadConfigFile(configFile)
	c.Assert(err, check.IsNil)
	value, err := GetList("multiple-types:obj")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.DeepEquals, []string{"a", "abc"})
}

func (s *S) TestSet(c *check.C) {
	err := ReadConfigFile("testdata/config.yml")
	c.Assert(err, check.IsNil)
	Set("xpto", "bla")
	value, err := GetString("xpto")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, "bla")
}

func (s *S) TestSetChildren(c *check.C) {
	err := ReadConfigFile("testdata/config.yml")
	c.Assert(err, check.IsNil)
	Set("database:host", "database.com")
	value, err := GetString("database:host")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, "database.com")
}

func (s *S) TestSetChildrenDoesNotImpactOtherChild(c *check.C) {
	err := ReadConfigFile("testdata/config.yml")
	c.Assert(err, check.IsNil)
	Set("database:host", "database.com")
	value, err := Get("database:port")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, 8080)
}

func (s *S) TestSetMap(c *check.C) {
	err := ReadConfigFile("testdata/config.yml")
	c.Assert(err, check.IsNil)
	Set("database", map[interface{}]interface{}{"host": "database.com", "port": 3306})
	host, err := GetString("database:host")
	c.Assert(err, check.IsNil)
	c.Assert(host, check.Equals, "database.com")
	port, err := Get("database:port")
	c.Assert(err, check.IsNil)
	c.Assert(port, check.Equals, 3306)
}

func (s *S) TestSetCallback(c *check.C) {
	err := ReadConfigFile("testdata/config.yml")
	c.Assert(err, check.IsNil)
	Set("xpto", func() interface{} {
		return "bla"
	})
	value, err := GetString("xpto")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, "bla")
}

func (s *S) TestUnset(c *check.C) {
	err := ReadConfigFile("testdata/config.yml")
	c.Assert(err, check.IsNil)
	err = Unset("xpto")
	c.Assert(err, check.IsNil)
	_, err = Get("xpto")
	c.Assert(err, check.NotNil)
}

func (s *S) TestUnsetChildren(c *check.C) {
	err := ReadConfigFile("testdata/config.yml")
	c.Assert(err, check.IsNil)
	err = Unset("database:host")
	c.Assert(err, check.IsNil)
	_, err = Get("database:host")
	c.Assert(err, check.NotNil)
	user, err := GetString("database:user")
	c.Assert(err, check.IsNil)
	c.Assert(user, check.Equals, "root")
}

func (s *S) TestUnsetWithUndefinedKey(c *check.C) {
	err := ReadConfigFile("testdata/config.yml")
	c.Assert(err, check.IsNil)
	err = Unset("database:hoster")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, `key "database:hoster" not found`)
}

func (s *S) TestUnsetMap(c *check.C) {
	err := ReadConfigFile("testdata/config.yml")
	c.Assert(err, check.IsNil)
	err = Unset("database")
	c.Assert(err, check.IsNil)
	_, err = Get("database:host")
	c.Assert(err, check.NotNil)
	_, err = Get("database:port")
	c.Assert(err, check.NotNil)
}

func (s *S) TestMergeMaps(c *check.C) {
	m1 := map[interface{}]interface{}{
		"database": map[interface{}]interface{}{
			"host": "localhost",
			"port": 3306,
		},
	}
	m2 := map[interface{}]interface{}{
		"database": map[interface{}]interface{}{
			"host": "remotehost",
		},
		"memcached": []string{"mymemcached"},
	}
	expected := map[interface{}]interface{}{
		"database": map[interface{}]interface{}{
			"host": "remotehost",
			"port": 3306,
		},
		"memcached": []string{"mymemcached"},
	}
	c.Assert(mergeMaps(m1, m2), check.DeepEquals, expected)
}

func (s *S) TestMergeMapsMultipleProcs(c *check.C) {
	old := runtime.GOMAXPROCS(16)
	defer runtime.GOMAXPROCS(old)
	m1 := map[interface{}]interface{}{
		"database": map[interface{}]interface{}{
			"host": "localhost",
			"port": 3306,
		},
	}
	m2 := map[interface{}]interface{}{
		"database": map[interface{}]interface{}{
			"host": "remotehost",
		},
		"memcached": []string{"mymemcached"},
	}
	expected := map[interface{}]interface{}{
		"database": map[interface{}]interface{}{
			"host": "remotehost",
			"port": 3306,
		},
		"memcached": []string{"mymemcached"},
	}
	c.Assert(mergeMaps(m1, m2), check.DeepEquals, expected)
}

func (s *S) TestMergeMapsWithDiffingMaps(c *check.C) {
	m1 := map[interface{}]interface{}{
		"database": map[interface{}]interface{}{
			"host": "localhost",
			"port": 3306,
		},
	}
	m2 := map[interface{}]interface{}{
		"auth": map[interface{}]interface{}{
			"user":     "root",
			"password": "123",
		},
	}
	expected := map[interface{}]interface{}{
		"auth": map[interface{}]interface{}{
			"user":     "root",
			"password": "123",
		},
		"database": map[interface{}]interface{}{
			"host": "localhost",
			"port": 3306,
		},
	}
	c.Assert(mergeMaps(m1, m2), check.DeepEquals, expected)
}

func (s *S) TestConfigFileResetsOldValues(c *check.C) {
	err := ReadConfigFile("testdata/config.yml")
	c.Assert(err, check.IsNil)
	err = ReadConfigFile("testdata/config4.yml")
	c.Assert(err, check.IsNil)
	expected := map[interface{}]interface{}{
		"xpto":       "changed",
		"my-new-key": "new",
	}
	c.Assert(configs.Data(), check.DeepEquals, expected)
}
