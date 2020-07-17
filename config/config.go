// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
)

func ConvertEntries(initial interface{}) interface{} {
	switch initialType := initial.(type) {
	case []interface{}:
		for i := range initialType {
			initialType[i] = ConvertEntries(initialType[i])
		}
		return initialType
	case map[interface{}]interface{}:
		output := make(map[string]interface{}, len(initialType))
		for k, v := range initialType {
			output[fmt.Sprintf("%v", k)] = ConvertEntries(v)
		}
		return output
	default:
		return initialType
	}
}

func UnconvertEntries(initial interface{}) interface{} {
	switch initialType := initial.(type) {
	case []interface{}:
		for i := range initialType {
			initialType[i] = UnconvertEntries(initialType[i])
		}
		return initialType
	case map[string]interface{}:
		output := make(map[interface{}]interface{}, len(initialType))
		for k, v := range initialType {
			output[k] = UnconvertEntries(v)
		}
		return output
	default:
		return initialType
	}
}

func UnmarshalConfig(key string, result interface{}) error {
	data, err := config.Get(key)
	if err != nil {
		return errors.WithStack(err)
	}
	data = ConvertEntries(data)
	jsonData, err := json.Marshal(data)
	if err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(json.Unmarshal(jsonData, result))
}
