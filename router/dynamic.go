// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/storage"
	routerTypes "github.com/tsuru/tsuru/types/router"
)

type dynamicRouterService struct {
	storage routerTypes.DynamicRouterStorage
}

func DynamicRouterService() (routerTypes.DynamicRouterService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &dynamicRouterService{
		storage: dbDriver.DynamicRouterStorage,
	}, nil
}

func (s *dynamicRouterService) Update(dr routerTypes.DynamicRouter) error {
	existing, err := s.storage.Get(dr.Name)
	if err != nil {
		return err
	}
	if dr.Type != "" {
		existing.Type = dr.Type
	}
	err = s.validate(*existing)
	if err != nil {
		return err
	}

	for k, v := range dr.Config {
		if v == nil {
			delete(existing.Config, k)
		} else {
			existing.Config[k] = v
		}
	}

	return s.storage.Save(*existing)
}

func (s *dynamicRouterService) Create(dr routerTypes.DynamicRouter) error {
	err := s.validate(dr)
	if err != nil {
		return err
	}
	return s.storage.Save(dr)
}

func (s *dynamicRouterService) validate(dr routerTypes.DynamicRouter) error {
	if dr.Name == "" || dr.Type == "" {
		return errors.New("dynamic router name and type are required")
	}
	if _, ok := routers[dr.Type]; !ok {
		return errors.Errorf("router type %q is not registered", dr.Type)
	}
	if _, err := config.Get("routers:" + dr.Name); err == nil {
		return errors.Errorf("router named %q already exists in config", dr.Name)
	}
	return nil
}

func (s *dynamicRouterService) Get(name string) (*routerTypes.DynamicRouter, error) {
	return s.storage.Get(name)
}

func (s *dynamicRouterService) List() ([]routerTypes.DynamicRouter, error) {
	return s.storage.List()
}

func (s *dynamicRouterService) Remove(name string) error {
	return s.storage.Remove(name)
}
