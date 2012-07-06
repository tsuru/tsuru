// Package config provide configuration facilities, handling configuration
// files in yaml format.
package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"strings"
)

var Configs map[interface{}]interface{}

// ReadConfigBytes receives a slice of bytes and builds the internal
// configuration map.
//
// If the given slice is not a valid yaml file, ReadConfigBytes returns a
// non-nil error.
func ReadConfigBytes(data []byte) error {
	return goyaml.Unmarshal(data, &Configs)
}

// ReadConfigFile reads the content of a file and calls ReadConfigBytes to
// build the internal configuration map.
//
// It returns error if it can not read the given file or if the file contents
// is not valid yaml.
func ReadConfigFile(filePath string) error {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}
	return ReadConfigBytes(data)
}

// Get returns the value for the given key, or an eror if the key is undefined.
//
// The key is composed by all the key names separated by :, in case of nested
// keys. For example, suppose we have the following configuration yaml:
//
//   databases:
//     mysql:
//       host: localhost
//       port: 3306
//
// The key "databases:mysql:host" would return "localhost", while the key
// "port" would return an error.
func Get(key string) (interface{}, error) {
	keys := strings.Split(key, ":")
	conf, ok := Configs[keys[0]]
	if !ok {
		return nil, errors.New(fmt.Sprintf("key %s not found", key))
	}
	for _, k := range keys[1:] {
		conf, ok = conf.(map[interface{}]interface{})[k]
		if !ok {
			return nil, errors.New(fmt.Sprintf("key %s not found", key))
		}
	}
	return conf, nil
}

// GetString works like Get, but doing a string type assertion before return
// the value.
//
// It returns error if the key is undefined or if it is not a string.
func GetString(key string) (string, error) {
	value, err := Get(key)
	if err != nil {
		return "", err
	}
	if v, ok := value.(string); ok {
		return v, nil
	}
	return "", errors.New(fmt.Sprintf("key %s has non-string value", key))
}
