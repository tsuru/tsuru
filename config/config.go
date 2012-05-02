package config

import (
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
	return goyaml.Unmarshal(data, &Configs)
}

func Get(key string) interface{} {
	keys := strings.Split(key, ":")
	conf := Configs[keys[0]]
	for _, k := range keys[1:] {
		conf = conf.(map[interface{}]interface{})[k]
	}
	return conf
}

func GetString(key string) string {
	return Get(key).(string)
}
