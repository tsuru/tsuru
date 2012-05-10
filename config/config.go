package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"strings"
)

var Configs map[interface{}]interface{}

func ReadConfigBytes(data []byte) error {
	return goyaml.Unmarshal(data, &Configs)
}

func ReadConfigFile(filePath string) error {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}
	return ReadConfigBytes(data)
}

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
