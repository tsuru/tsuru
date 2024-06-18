// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"fmt"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/version"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/servicemanager"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	serviceTypes "github.com/tsuru/tsuru/types/service"
	check "gopkg.in/check.v1"
)

type S struct {
	conn        *db.Storage
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
	var err error
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_service_test")
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)

	storagev2.Reset()
}

func (s *S) SetUpTest(c *check.C) {
	routertest.FakeRouter.Reset()
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	s.user = &auth.User{Email: "cidade@raul.com"}
	err := s.user.Create()
	c.Assert(err, check.IsNil)
	s.team = &authTypes.Team{Name: "raul"}
	servicemock.SetMockService(&s.mockService)
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		return s.team, nil
	}
	s.mockService.Team.OnFindByNames = func(names []string) ([]authTypes.Team, error) {
		return []authTypes.Team{*s.team}, nil
	}
	s.mockService.ServiceBrokerCatalogCache.OnLoad = func(_ string) (*serviceTypes.BrokerCatalog, error) {
		return nil, fmt.Errorf("not found")
	}
	servicemanager.LogService = &appTypes.MockAppLogService{}
	servicemanager.AppVersion, err = version.AppVersionService()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	dbtest.ClearAllCollections(s.conn.Services().Database)
	s.conn.Close()
}
