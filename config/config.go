// Package config provide configuration facilities, handling configuration
// files in yaml format.
package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"strings"
	"sync"
)

var configs map[interface{}]interface{}

// ReadConfigBytes receives a slice of bytes and builds the internal
// configuration object.
//
// If the given slice is not a valid yaml file, ReadConfigBytes returns a
// non-nil error.
func ReadConfigBytes(data []byte) error {
	return goyaml.Unmarshal(data, &configs)
}

// ReadConfigFile reads the content of a file and calls ReadConfigBytes to
// build the internal configuration object.
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
	conf, ok := configs[keys[0]]
	if !ok {
		return nil, fmt.Errorf("key %s not found", key)
	}
	for _, k := range keys[1:] {
		conf, ok = conf.(map[interface{}]interface{})[k]
		if !ok {
			return nil, fmt.Errorf("key %s not found", key)
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
	return "", fmt.Errorf("key %s has non-string value", key)
}

// GetBool does a type assertion before returning the requested value
func GetBool(key string) (bool, error) {
	value, err := Get(key)
	if err != nil {
		return false, err
	}
	if v, ok := value.(bool); ok {
		return v, nil
	}
	return false, fmt.Errorf("key %s has non-boolean value", key)
}

// mergeMaps takes two maps and merge its keys and values.
//
// In case of conflicts, the function picks value from map2.
func mergeMaps(map1, map2 map[interface{}]interface{}) map[interface{}]interface{} {
	var wg sync.WaitGroup
	wg.Add(2)
	result := make(map[interface{}]interface{})
	go func() {
		for k, v2 := range map2 {
			if v1, ok := map1[k]; !ok {
				result[k] = v2
			} else {
				map1, ok1 := v1.(map[interface{}]interface{})
				map2, ok2 := v2.(map[interface{}]interface{})
				if ok1 && ok2 {
					result[k] = mergeMaps(map1, map2)
				} else {
					result[k] = v2
				}
			}
		}
		wg.Done()
	}()
	go func() {
		for k, v := range map1 {
			if v2, ok := map2[k]; !ok {
				result[k] = v
			} else {
				map1, ok1 := v.(map[interface{}]interface{})
				map2, ok2 := v2.(map[interface{}]interface{})
				if ok1 && ok2 {
					result[k] = mergeMaps(map1, map2)
				}
			}
		}
		wg.Done()
	}()
	wg.Wait()
	return result
}

// Set redefines or defines a value for a key. The key has the same format that
// it has in Get and GetString.
//
// Values defined by this function affects only runtime informatin, nothing
// defined by Set is persisted in the filesystem or any database.
func Set(key string, value interface{}) {
	parts := strings.Split(key, ":")
	last := map[interface{}]interface{}{
		parts[len(parts)-1]: value,
	}
	for i := len(parts) - 2; i >= 0; i-- {
		last = map[interface{}]interface{}{
			parts[i]: last,
		}
	}
	configs = mergeMaps(configs, last)
}

// Unset removes a key from the configuration map. It returns error if the key
// is not defined.
//
// Calling this function does not remove a key from a configuration file, only
// from the in-memory configuration object.
func Unset(key string) error {
	var i int
	var part string
	m := configs
	parts := strings.Split(key, ":")
	for i, part = range parts {
		if item, ok := m[part]; ok {
			if nm, ok := item.(map[interface{}]interface{}); ok && i < len(parts)-1 {
				m = nm
			} else {
				break
			}
		} else {
			return errors.New("Key " + key + " not found")
		}
	}
	delete(m, part)
	return nil
}
