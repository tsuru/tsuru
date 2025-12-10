// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/version"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/servicemanager"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/types/bind"
	check "gopkg.in/check.v1"
)

type S struct {
	service     *Service
	team        *authTypes.Team
	user        *auth.User
	mockService servicemock.MockService
}

var _ = check.Suite(&S{})

func Test(t *testing.T) { check.TestingT(t) }

type hasAccessToChecker struct{}

func (c *hasAccessToChecker) Info() *check.CheckerInfo {
	return &check.CheckerInfo{Name: "HasAccessTo", Params: []string{"team", "service"}}
}

func (c *hasAccessToChecker) Check(params []interface{}, names []string) (bool, string) {
	if len(params) != 2 {
		return false, "you must provide two parameters"
	}
	team, ok := params[0].(authTypes.Team)
	if !ok {
		return false, "first parameter should be a team instance"
	}
	service, ok := params[1].(Service)
	if !ok {
		return false, "second parameter should be service instance"
	}
	return service.HasTeam(&team), ""
}

var HasAccessTo check.Checker = &hasAccessToChecker{}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_service_test")

	storagev2.Reset()
}

func (s *S) SetUpTest(c *check.C) {
	routertest.FakeRouter.Reset()
	storagev2.ClearAllCollections(nil)
	s.user = &auth.User{Email: "cidade@raul.com"}
	err := s.user.Create(context.TODO())
	c.Assert(err, check.IsNil)
	s.team = &authTypes.Team{Name: "raul"}
	servicemock.SetMockService(&s.mockService)
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		return s.team, nil
	}
	s.mockService.Team.OnFindByNames = func(names []string) ([]authTypes.Team, error) {
		return []authTypes.Team{*s.team}, nil
	}
	servicemanager.LogService = &appTypes.MockAppLogService{}
	servicemanager.AppVersion, err = version.AppVersionService()
	c.Assert(err, check.IsNil)

	s.mockService.App.OnGetAddresses = func(a *appTypes.App) ([]string, error) {
		return routertest.FakeRouter.Addresses(context.TODO(), a)
	}
	s.mockService.App.OnAddInstance = func(a *appTypes.App, instanceArgs bind.AddInstanceArgs) error {
		a.ServiceEnvs = append(a.ServiceEnvs, instanceArgs.Envs...)
		if instanceArgs.Writer != nil {
			instanceArgs.Writer.Write([]byte("add instance"))
		}

		return nil
	}
	s.mockService.App.OnRemoveInstance = func(a *appTypes.App, instanceArgs bind.RemoveInstanceArgs) error {
		lenBefore := len(a.ServiceEnvs)
		for i := 0; i < len(a.ServiceEnvs); i++ {
			se := a.ServiceEnvs[i]
			if se.ServiceName == instanceArgs.ServiceName && se.InstanceName == instanceArgs.InstanceName {
				a.ServiceEnvs = append(a.ServiceEnvs[:i], a.ServiceEnvs[i+1:]...)
				i--
			}
		}
		if len(a.ServiceEnvs) == lenBefore {
			return errors.New("instance not found")
		}
		if instanceArgs.Writer != nil {
			instanceArgs.Writer.Write([]byte("remove instance"))
		}
		return nil
	}

	s.mockService.App.OnGetInternalBindableAddresses = func(a *appTypes.App) ([]string, error) {
		return []string{}, nil
	}
}

func (s *S) TearDownSuite(c *check.C) {
	storagev2.ClearAllCollections(nil)
}
