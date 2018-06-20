// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http/httptest"
	"sort"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	check "gopkg.in/check.v1"
)

type SyncSuite struct {
	conn            *db.Storage
	user            auth.User
	team            authTypes.Team
	mockTeamService *authTypes.MockTeamService
}

var _ = check.Suite(&SyncSuite{})

func (s *SyncSuite) SetUpSuite(c *check.C) {
	var err error
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
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
	opts := pool.AddPoolOptions{Name: "pool1", Default: true, Provisioner: "fake"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	s.mockTeamService = &authTypes.MockTeamService{
		OnList: func() ([]authTypes.Team, error) {
			return []authTypes.Team{s.team}, nil
		},
		OnFindByNames: func(names []string) ([]authTypes.Team, error) {
			return []authTypes.Team{s.team}, nil
		},
	}
	servicemanager.Team = s.mockTeamService
}

func (s *SyncSuite) TearDownSuite(c *check.C) {
	s.conn.Apps().Database.DropDatabase()
	s.conn.Close()
}

func (s *SyncSuite) TestBindSyncer(c *check.C) {
	h := service.TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t", OwnerTeams: []string{s.team.Name}}
	err := service.Create(srvc)
	c.Assert(err, check.IsNil)
	srvc = service.Service{Name: "mysql2", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t", OwnerTeams: []string{s.team.Name}}
	err = service.Create(srvc)
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
		BoundUnits:  []service.Unit{{AppName: a.Name, ID: units[0].GetID(), IP: units[0].GetIp()}, {AppName: "my-app", ID: "wrong", IP: "wrong"}},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	instance = &service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql2",
		Teams:       []string{s.team.Name},
		Apps:        []string{a.GetName()},
		BoundUnits:  []service.Unit{},
	}
	err = s.conn.ServiceInstances().Insert(instance)
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
		{AppName: a.Name, ID: units[0].GetID(), IP: units[0].GetIp()},
	})
	instance, err = service.GetServiceInstance("mysql2", "my-mysql")
	c.Assert(err, check.IsNil)
	c.Assert(instance.BoundUnits, check.DeepEquals, []service.Unit{
		{AppName: a.Name, ID: units[0].GetID(), IP: units[0].GetIp()},
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

func (s *SyncSuite) TestBindSyncerMultipleAppsBound(c *check.C) {
	h := service.TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t", OwnerTeams: []string{s.team.Name}}
	err := service.Create(srvc)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "my-app", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(a, &s.user)
	c.Assert(err, check.IsNil)
	a2 := &app.App{Name: "my-app2", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(a2, &s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(2, "", nil)
	c.Assert(err, check.IsNil)
	err = a2.AddUnits(2, "", nil)
	c.Assert(err, check.IsNil)
	units, err := a.GetUnits()
	c.Assert(err, check.IsNil)
	units2, err := a2.GetUnits()
	c.Assert(err, check.IsNil)
	instance := &service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{a.GetName(), a2.GetName()},
		BoundUnits: []service.Unit{
			{AppName: a.Name, ID: units[0].GetID(), IP: units[0].GetIp()},
			{AppName: a2.Name, ID: units2[0].GetID(), IP: units2[0].GetIp()},
		},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	callCh := make(chan struct{})
	err = service.InitializeSync(func() ([]bind.App, error) {
		return []bind.App{a, a2}, nil
	})
	c.Assert(err, check.IsNil)
	go func() {
		for {
			evts, evtErr := event.All()
			c.Assert(evtErr, check.IsNil)
			if len(evts) == 2 {
				callCh <- struct{}{}
				return
			}
			time.Sleep(time.Millisecond * 100)
		}
	}()
	select {
	case <-callCh:
	case <-time.After(time.Second * 5):
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	shutdown.Do(ctx, ioutil.Discard)
	cancel()
	c.Assert(err, check.IsNil)
	instance, err = service.GetServiceInstance("mysql", "my-mysql")
	c.Assert(err, check.IsNil)
	sort.Slice(instance.BoundUnits, func(i int, j int) bool {
		return instance.BoundUnits[i].ID < instance.BoundUnits[j].ID
	})
	c.Assert(instance.BoundUnits, check.DeepEquals, []service.Unit{
		{AppName: a.Name, ID: units[0].GetID(), IP: units[0].GetIp()},
		{AppName: a.Name, ID: units[1].GetID(), IP: units[1].GetIp()},
		{AppName: a2.Name, ID: units2[0].GetID(), IP: units2[0].GetIp()},
		{AppName: a2.Name, ID: units2[1].GetID(), IP: units2[1].GetIp()},
	})
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

func (s *SyncSuite) TestBindSyncerError(c *check.C) {
	h := service.TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	h.Err = errors.New("my awful error")
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t", OwnerTeams: []string{s.team.Name}}
	err := service.Create(srvc)
	c.Assert(err, check.IsNil)
	srvc = service.Service{Name: "mysql2", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t", OwnerTeams: []string{s.team.Name}}
	err = service.Create(srvc)
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
		BoundUnits:  []service.Unit{{AppName: a.Name, ID: units[0].GetID(), IP: units[0].GetIp()}, {AppName: a.Name, ID: "wrong", IP: "wrong"}},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	instance = &service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql2",
		Teams:       []string{s.team.Name},
		Apps:        []string{a.GetName()},
		BoundUnits:  []service.Unit{},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	callCh := make(chan struct{})
	buf := bytes.NewBuffer(nil)
	log.SetLogger(log.NewWriterLogger(buf, true))
	defer log.SetLogger(nil)
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
		{AppName: a.Name, ID: units[0].GetID(), IP: units[0].GetIp()},
		{AppName: a.Name, ID: "wrong", IP: "wrong"},
	})
	instance, err = service.GetServiceInstance("mysql2", "my-mysql")
	c.Assert(err, check.IsNil)
	c.Assert(instance.BoundUnits, check.DeepEquals, []service.Unit{})
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
		ErrorMatches: `.*invalid response: my awful error.*` +
			`Failed to unbind \("/resources/my-mysql/bind"\).*` +
			`failed to unbind unit "wrong" for mysql\(my-mysql\).*` +
			`invalid response: my awful error.*` +
			`Failed to bind the instance "mysql2/my-mysql" to the unit "10\.10\.10\.1".*` +
			`failed to bind unit "my-app-0" for mysql2\(my-mysql\)`,
	}, eventtest.HasEvent)
	c.Assert(buf.String(), check.Matches, `(?s).*\[bind-syncer\] error syncing app "my-app": multiple errors reported \(2\): `+
		`error 0: failed to unbind unit "wrong" for mysql\(my-mysql\): Failed to unbind \("/resources/my-mysql/bind"\): invalid response: my awful error.*`+
		`error 1: failed to bind unit "my-app-0" for mysql2\(my-mysql\): Failed to bind the instance "mysql2/my-mysql" to the unit "10.10.10.1": invalid response: my awful error.*`)
}
