// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"github.com/tsuru/config"
	internalConfig "github.com/tsuru/tsuru/config"
)

type ConfigGetter interface {
	GetString(string) (string, error)
	GetList(string) ([]string, error)
	GetInt(string) (int, error)
	GetFloat(string) (float64, error)
	GetBool(string) (bool, error)
	Get(string) (interface{}, error)
	Hash() (string, error)
}

var _ ConfigGetter = &delegateConfigGetter{}

type delegateConfigGetter struct {
	*config.Configuration
}

func (g *delegateConfigGetter) Hash() (string, error) {
	serialized, err := g.Configuration.Bytes()
	if err != nil {
		return "", err
	}
	return string(serialized), nil
}

var ConfigGetterFromData = configGetterFromData

func configGetterFromData(data map[string]interface{}) ConfigGetter {
	cfg := config.Configuration{}
	unconverted, _ := internalConfig.UnconvertEntries(data).(map[interface{}]interface{})
	cfg.Store(unconverted)
	return &delegateConfigGetter{Configuration: &cfg}
}

func ConfigGetterFromPrefix(prefix string) ConfigGetter {
	data, _ := config.Get(prefix)
	cfg := config.Configuration{}
	unconverted, _ := data.(map[interface{}]interface{})
	cfg.Store(unconverted)
	return &delegateConfigGetter{Configuration: &cfg}
}
