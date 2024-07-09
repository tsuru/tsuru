// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/servicemanager"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	eventTypes "github.com/tsuru/tsuru/types/event"
	check "gopkg.in/check.v1"
)

type BindSuite struct {
	conn        *db.Storage
	user        auth.User
	team        authTypes.Team
	mockService servicemock.MockService
}

var _ = check.Suite(&BindSuite{})

func (s *BindSuite) SetUpSuite(c *check.C) {
	var err error
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_service_bind_test")
	config.Set("routers:fake:type", "fake")
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)

	storagev2.Reset()

	app.AuthScheme = auth.Scheme(native.NativeScheme{})
}

func (s *BindSuite) SetUpTest(c *check.C) {
	provisiontest.ProvisionerInstance.Reset()
	routertest.FakeRouter.Reset()
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	s.user = auth.User{Email: "sad-but-true@metallica.com"}
	err := s.user.Create(context.TODO())
	c.Assert(err, check.IsNil)
	s.team = authTypes.Team{Name: "metallica"}
	opts := pool.AddPoolOptions{Name: "pool1", Default: true, Provisioner: "fake"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	servicemock.SetMockService(&s.mockService)
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{s.team}, nil
	}
	s.mockService.Team.OnFindByNames = func(names []string) ([]authTypes.Team, error) {
		return []authTypes.Team{s.team}, nil
	}
	s.mockService.Pool.OnServices = func(pool string) ([]string, error) {
		return []string{"mysql"}, nil
	}

	plan := appTypes.Plan{
		Name:    "default",
		Default: true,
	}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{plan}, nil
	}
	s.mockService.Plan.OnDefaultPlan = func() (*appTypes.Plan, error) {
		return &plan, nil
	}
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		if name == plan.Name {
			return &plan, nil
		}
		return nil, appTypes.ErrPlanNotFound
	}
}

func (s *BindSuite) TearDownSuite(c *check.C) {
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	s.conn.Close()
}

func createEvt(c *check.C) *event.Event {
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: eventTypes.TargetTypeServiceInstance, Value: "x"},
		Kind:     permission.PermServiceInstanceCreate,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: "my@user"},
		Allowed:  event.Allowed(permission.PermServiceInstanceReadEvents),
	})
	c.Assert(err, check.IsNil)
	return evt
}

func (s *BindSuite) TestBindAppFailsWhenEndpointIsDown(c *check.C) {
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": "wrong"}, Password: "s3cr3t", OwnerTeams: []string{s.team.Name}}
	err := service.Create(context.TODO(), srvc)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "painkiller", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, &s.user)
	c.Assert(err, check.IsNil)
	newVersionForApp(c, a)
	err = a.AddUnits(context.TODO(), 1, "", "", nil)
	c.Assert(err, check.IsNil)
	evt := createEvt(c)
	err = instance.BindApp(context.TODO(), a, nil, true, nil, evt, "")
	c.Assert(err, check.NotNil)
}

func (s *BindSuite) TestBindAddsAppToTheServiceInstance(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t", OwnerTeams: []string{s.team.Name}}
	err := service.Create(context.TODO(), srvc)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "painkiller", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, &s.user)
	c.Assert(err, check.IsNil)
	newVersionForApp(c, a)
	err = a.AddUnits(context.TODO(), 1, "", "", nil)
	c.Assert(err, check.IsNil)
	evt := createEvt(c)
	err = instance.BindApp(context.TODO(), a, nil, true, nil, evt, "")
	c.Assert(err, check.IsNil)
	s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(instance.Apps, check.DeepEquals, []string{a.GetName()})
}

func (s *BindSuite) TestBindUnbindAppDuplicatedInstanceNames(c *check.C) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Write([]byte(`{"SRV1":"val1"}`))
		} else {
			w.Write([]byte(`{"SRV2":"val2"}`))
		}
	}))
	defer ts.Close()
	srvc1 := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t", OwnerTeams: []string{s.team.Name}}
	err := service.Create(context.TODO(), srvc1)
	c.Assert(err, check.IsNil)
	srvc2 := service.Service{Name: "postgres", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t", OwnerTeams: []string{s.team.Name}}
	err = service.Create(context.TODO(), srvc2)
	c.Assert(err, check.IsNil)
	instance1 := service.ServiceInstance{
		Name:        "my-db",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = s.conn.ServiceInstances().Insert(instance1)
	c.Assert(err, check.IsNil)
	instance2 := service.ServiceInstance{
		Name:        "my-db",
		ServiceName: "postgres",
		Teams:       []string{s.team.Name},
	}
	err = s.conn.ServiceInstances().Insert(instance2)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "painkiller", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, &s.user)
	c.Assert(err, check.IsNil)
	evt := createEvt(c)
	err = instance1.BindApp(context.TODO(), a, nil, true, nil, evt, "")
	c.Assert(err, check.IsNil)
	err = instance2.BindApp(context.TODO(), a, nil, true, nil, evt, "")
	c.Assert(err, check.IsNil)
	envs := a.Envs()
	c.Assert(envs["SRV1"], check.DeepEquals, bindTypes.EnvVar{
		Name:      "SRV1",
		Value:     "val1",
		Public:    false,
		ManagedBy: "mysql/my-db",
	})
	c.Assert(envs["SRV2"], check.DeepEquals, bindTypes.EnvVar{
		Name:      "SRV2",
		Value:     "val2",
		Public:    false,
		ManagedBy: "postgres/my-db",
	})
	dbI1, err := service.GetServiceInstance(context.TODO(), instance1.ServiceName, instance1.Name)
	c.Assert(err, check.IsNil)
	err = dbI1.UnbindApp(context.TODO(), service.UnbindAppArgs{
		App:     a,
		Restart: true,
		Event:   evt,
	})
	c.Assert(err, check.IsNil)
	envs = a.Envs()
	c.Assert(envs["SRV1"], check.DeepEquals, bindTypes.EnvVar{})
	c.Assert(envs["SRV2"], check.DeepEquals, bindTypes.EnvVar{
		Name:      "SRV2",
		Value:     "val2",
		Public:    false,
		ManagedBy: "postgres/my-db",
	})
}

func (s *BindSuite) TestBindReturnConflictIfTheAppIsAlreadyBound(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	srvc := service.Service{Name: "mysql", Password: "s3cr3t", Endpoint: map[string]string{"production": ts.URL}, OwnerTeams: []string{s.team.Name}}
	err := service.Create(context.TODO(), srvc)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "painkiller", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, &s.user)
	c.Assert(err, check.IsNil)
	newVersionForApp(c, a)
	err = a.AddUnits(context.TODO(), 1, "", "", nil)
	c.Assert(err, check.IsNil)
	evt := createEvt(c)
	err = instance.BindApp(context.TODO(), a, nil, true, nil, evt, "")
	c.Assert(err, check.Equals, service.ErrAppAlreadyBound)
}

func (s *BindSuite) TestBindAppWithNoUnits(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t", OwnerTeams: []string{s.team.Name}}
	err := service.Create(context.TODO(), srvc)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "painkiller", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, &s.user)
	c.Assert(err, check.IsNil)
	evt := createEvt(c)
	err = instance.BindApp(context.TODO(), a, nil, true, nil, evt, "")
	c.Assert(err, check.IsNil)
	envs := a.Envs()
	c.Assert(envs["DATABASE_USER"], check.DeepEquals, bindTypes.EnvVar{
		Name:      "DATABASE_USER",
		Value:     "root",
		Public:    false,
		ManagedBy: "mysql/my-mysql",
	})
	c.Assert(envs["DATABASE_PASSWORD"], check.DeepEquals, bindTypes.EnvVar{
		Name:      "DATABASE_PASSWORD",
		Value:     "s3cr3t",
		Public:    false,
		ManagedBy: "mysql/my-mysql",
	})
}

func (s *BindSuite) TestUnbindRemovesAppFromServiceInstance(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t", OwnerTeams: []string{s.team.Name}}
	err := service.Create(context.TODO(), srvc)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "painkiller", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, &s.user)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(context.TODO(), bind.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "ENV1", Value: "VAL1"}, ServiceName: "mysql", InstanceName: "my-mysql"},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	evt := createEvt(c)
	err = instance.UnbindApp(context.TODO(), service.UnbindAppArgs{
		App:     a,
		Restart: true,
		Event:   evt,
	})
	c.Assert(err, check.IsNil)
	s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(instance.Apps, check.DeepEquals, []string{})
}

func (s *BindSuite) TestUnbindReturnsPreconditionFailedIfTheAppIsNotBoundToTheInstance(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t", OwnerTeams: []string{s.team.Name}}
	err := service.Create(context.TODO(), srvc)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "painkiller", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, &s.user)
	c.Assert(err, check.IsNil)
	evt := createEvt(c)
	err = instance.UnbindApp(context.TODO(), service.UnbindAppArgs{
		App:     a,
		Restart: true,
		Event:   evt,
	})
	c.Assert(err, check.Equals, service.ErrAppNotBound)
}

func newVersionForApp(c *check.C, a appTypes.AppInterface) appTypes.AppVersion {
	version, err := servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
		App: a,
	})
	c.Assert(err, check.IsNil)
	err = version.CommitBaseImage()
	c.Assert(err, check.IsNil)
	err = version.CommitSuccessful()
	c.Assert(err, check.IsNil)
	return version
}
