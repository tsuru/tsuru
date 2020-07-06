// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	internalConfig "github.com/tsuru/tsuru/config"
	"github.com/tsuru/tsuru/types/router"
)

var ErrRouterConfigNotFound = errors.New("router config not found")

type ConfigGetter interface {
	GetString(string) (string, error)
	GetInt(string) (int, error)
	GetFloat(string) (float64, error)
	GetBool(string) (bool, error)
	Get(string) (interface{}, error)
	Hash() (string, error)
}

var _ ConfigGetter = &dynamicConfigGetter{}
var _ ConfigGetter = &StaticConfigGetter{}

type dynamicConfigGetter struct {
	router router.DynamicRouter
}

type StaticConfigGetter struct {
	Prefix string
}

func (g *dynamicConfigGetter) Get(name string) (interface{}, error) {
	v, ok := g.router.Config[name]
	if !ok {
		return "", ErrRouterConfigNotFound
	}
	return v, nil
}

func (g *dynamicConfigGetter) GetString(name string) (string, error) {
	v, ok := g.router.Config[name]
	if !ok {
		return "", ErrRouterConfigNotFound
	}
	return fmt.Sprint(v), nil
}

func (g *dynamicConfigGetter) GetInt(name string) (int, error) {
	value, ok := g.router.Config[name]
	if !ok {
		return 0, ErrRouterConfigNotFound
	}
	if v, ok := value.(int); ok {
		return v, nil
	}
	if i, err := strconv.ParseInt(fmt.Sprint(value), 10, 64); err == nil {
		return int(i), nil
	}
	if i, err := strconv.ParseFloat(fmt.Sprint(value), 64); err == nil {
		return int(i), nil
	}
	return 0, errors.New("not a number")
}

func (g *dynamicConfigGetter) GetBool(name string) (bool, error) {
	value, ok := g.router.Config[name]
	if !ok {
		return false, ErrRouterConfigNotFound
	}
	if v, ok := value.(bool); ok {
		return v, nil
	}
	if v, err := strconv.ParseBool(fmt.Sprint(value)); err == nil {
		return v, nil
	}
	return false, errors.New("not a boolean")
}

func (g *dynamicConfigGetter) GetFloat(name string) (float64, error) {
	value, ok := g.router.Config[name]
	if !ok {
		return 0, ErrRouterConfigNotFound
	}
	if v, ok := value.(float64); ok {
		return v, nil
	}
	if v, err := strconv.ParseFloat(fmt.Sprint(value), 64); err == nil {
		return v, nil
	}
	return 0, errors.New("not a float")
}

func (g *dynamicConfigGetter) Hash() (string, error) {
	cfg, err := json.Marshal(g.router.Config)
	if err != nil {
		return "", err
	}
	return string(cfg), nil
}

func (g *StaticConfigGetter) Get(name string) (interface{}, error) {
	v, err := config.Get(g.Prefix + ":" + name)
	if err != nil {
		return nil, err
	}
	return internalConfig.ConvertEntries(v), nil
}

func (g *StaticConfigGetter) GetString(name string) (string, error) {
	return config.GetString(g.Prefix + ":" + name)
}

func (g *StaticConfigGetter) GetInt(name string) (int, error) {
	return config.GetInt(g.Prefix + ":" + name)
}

func (g *StaticConfigGetter) GetBool(name string) (bool, error) {
	return config.GetBool(g.Prefix + ":" + name)
}

func (g *StaticConfigGetter) GetFloat(name string) (float64, error) {
	return config.GetFloat(g.Prefix + ":" + name)
}

func (g *StaticConfigGetter) Hash() (string, error) {
	return g.Prefix, nil
}
