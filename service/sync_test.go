// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service_test

import (
	"context"
	"io/ioutil"
	"net/http/httptest"
	"time"

	"github.com/tsuru/tsuru/event/eventtest"

	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/event"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/service"
	authTypes "github.com/tsuru/tsuru/types/auth"
	check "gopkg.in/check.v1"
)

type SyncSuite struct {
	conn *db.Storage
	user auth.User
	team authTypes.Team
}

var _ = check.Suite(&SyncSuite{})

func (s *SyncSuite) SetUpSuite(c *check.C) {
	var err error
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_service_bind_test")
	config.Set("routers:fake:type", "fake")
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	app.AuthScheme = auth.Scheme(native.NativeScheme{})
}

func (s *SyncSuite) SetUpTest(c *check.C) {
	provisiontest.ProvisionerInstance.Reset()
	routertest.FakeRouter.Reset()
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	s.user = auth.User{Email: "sad-but-true@metallica.com"}
	err := s.user.Create()
	c.Assert(err, check.IsNil)
	s.team = authTypes.Team{Name: "metallica"}
	err = auth.TeamService().Insert(s.team)
	c.Assert(err, check.IsNil)
	opts := pool.AddPoolOptions{Name: "pool1", Default: true, Provisioner: "fake"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
}

func (s *SyncSuite) TearDownSuite(c *check.C) {
	s.conn.Apps().Database.DropDatabase()
	s.conn.Close()
}

func (s *SyncSuite) TestBindSyncer(c *check.C) {
	h := service.TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	srvc = service.Service{Name: "mysql2", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	err = srvc.Create()
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "my-app", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(a, &s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "", nil)
	c.Assert(err, check.IsNil)
	units, err := a.GetUnits()
	c.Assert(err, check.IsNil)
	instance := &service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{a.GetName()},
		BoundUnits:  []service.Unit{{ID: units[0].GetID(), IP: units[0].GetIp()}, {ID: "wrong", IP: "wrong"}},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	instance = &service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql2",
		Teams:       []string{s.team.Name},
		Apps:        []string{a.GetName()},
		BoundUnits:  []service.Unit{},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	callCh := make(chan struct{})
	err = service.InitializeSync(func() ([]bind.App, error) {
		callCh <- struct{}{}
		return []bind.App{a}, nil
	})
	c.Assert(err, check.IsNil)
	<-callCh
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	shutdown.Do(ctx, ioutil.Discard)
	cancel()
	c.Assert(err, check.IsNil)
	instance, err = service.GetServiceInstance("mysql", "my-mysql")
	c.Assert(err, check.IsNil)
	c.Assert(instance.BoundUnits, check.DeepEquals, []service.Unit{
		{ID: units[0].GetID(), IP: units[0].GetIp()},
	})
	instance, err = service.GetServiceInstance("mysql2", "my-mysql")
	c.Assert(err, check.IsNil)
	c.Assert(instance.BoundUnits, check.DeepEquals, []service.Unit{
		{ID: units[0].GetID(), IP: units[0].GetIp()},
	})
	evts, err := event.All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{
			Type:  event.TargetTypeApp,
			Value: "my-app",
		},
		Kind: "bindsyncer",
		EndCustomData: map[string]interface{}{
			"binds": map[string][]interface{}{
				"my-mysql": {"my-app-0"},
			},
			"unbinds": map[string][]interface{}{
				"my-mysql": {"wrong"},
			},
		},
	}, eventtest.HasEvent)
}

func (s *SyncSuite) TestBindSyncerNoOp(c *check.C) {
	a := &app.App{Name: "my-app", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(a, &s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "", nil)
	c.Assert(err, check.IsNil)
	callCh := make(chan struct{})
	err = service.InitializeSync(func() ([]bind.App, error) {
		callCh <- struct{}{}
		return []bind.App{a}, nil
	})
	c.Assert(err, check.IsNil)
	<-callCh
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	shutdown.Do(ctx, ioutil.Discard)
	cancel()
	c.Assert(err, check.IsNil)
	evts, err := event.All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
}
