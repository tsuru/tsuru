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

type templateService struct {
	storage routerTypes.RouterTemplateStorage
}

func TemplateService() (routerTypes.RouterTemplateService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &templateService{
		storage: dbDriver.RouterTemplateStorage,
	}, nil
}

func (s *templateService) Save(rt routerTypes.RouterTemplate) error {
	if rt.Name == "" || rt.Type == "" {
		return errors.New("router template name and type are required")
	}
	if _, ok := routers[rt.Type]; !ok {
		return errors.Errorf("router type %q is not registered", rt.Type)
	}
	if _, err := config.Get("routers:" + rt.Name); err == nil {
		return errors.Errorf("router named %q already exists in config", rt.Name)
	}
	return s.storage.Save(rt)
}

func (s *templateService) Get(name string) (*routerTypes.RouterTemplate, error) {
	return s.storage.Get(name)
}

func (s *templateService) List() ([]routerTypes.RouterTemplate, error) {
	return s.storage.List()
}

func (s *templateService) Remove(name string) error {
	return s.storage.Remove(name)
}
