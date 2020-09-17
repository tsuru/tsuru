// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migrate

import (
	"context"
	"testing"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	_ "github.com/tsuru/tsuru/router/routertest"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	user        *auth.User
	team        *authTypes.Team
	mockService servicemock.MockService
}

var _ = check.Suite(&S{})

func (s *S) SetUpTest(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_events_migrate_tests")
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = dbtest.ClearAllCollections(conn.Events().Database)
	c.Assert(err, check.IsNil)
	config.Set("routers:fake:type", "fake")
	config.Set("routers:fake:default", true)
	servicemock.SetMockService(&s.mockService)
	plan := appTypes.Plan{Name: "default", CpuShare: 100, Default: true}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{plan}, nil
	}
	s.mockService.Plan.OnDefaultPlan = func() (*appTypes.Plan, error) {
		return &plan, nil
	}
	c.Assert(err, check.IsNil)
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	s.user = &auth.User{Email: "me@me.com", Password: "123456"}
	_, err = nativeScheme.Create(s.user)
	c.Assert(err, check.IsNil)
	s.team = &authTypes.Team{Name: "angra"}
	provision.DefaultProvisioner = "fake"
	provisiontest.ProvisionerInstance.Reset()
	opts := pool.AddPoolOptions{Name: "test1", Default: true}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{*s.team}, nil
	}
	s.mockService.Team.OnFindByName = func(_ string) (*authTypes.Team, error) {
		return s.team, nil
	}
}

func (s *S) TestMigrateRCEventsNoApp(c *check.C) {
	now := time.Unix(time.Now().Unix(), 0).UTC()
	id := bson.NewObjectId()
	var expected event.Event

	expected.UniqueID = id
	expected.Target = event.Target{Type: event.TargetTypeApp, Value: "a1"}
	expected.Owner = event.Owner{Type: event.OwnerTypeUser, Name: "u1"}
	expected.Kind = event.Kind{Type: event.KindTypePermission, Name: permission.PermAppDeploy.FullName()}
	expected.StartTime = now
	expected.EndTime = now.Add(time.Minute)
	expected.Error = "err1"
	expected.Allowed = event.Allowed(permission.PermAppReadEvents)
	s.checkEvtMatch(&expected, c)
}

func (s *S) TestMigrateRCEventsWithApp(c *check.C) {
	a := app.App{Name: "a1", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	now := time.Unix(time.Now().Unix(), 0).UTC()
	id := bson.NewObjectId()
	var expected event.Event

	expected.UniqueID = id
	expected.Target = event.Target{Type: event.TargetTypeApp, Value: "a1"}
	expected.Owner = event.Owner{Type: event.OwnerTypeUser, Name: "u1"}
	expected.Kind = event.Kind{Type: event.KindTypePermission, Name: permission.PermAppDeploy.FullName()}
	expected.StartTime = now
	expected.EndTime = now.Add(time.Minute)
	expected.Error = "err1"
	expected.Allowed = event.Allowed(permission.PermAppReadEvents,
		append(permission.Contexts(permTypes.CtxTeam, a.Teams),
			permission.Context(permTypes.CtxApp, a.Name),
			permission.Context(permTypes.CtxPool, a.Pool),
		)...,
	)
	s.checkEvtMatch(&expected, c)
}

func (s *S) TestMigrateRCEventsInvalidTarget(c *check.C) {
	now := time.Unix(time.Now().Unix(), 0).UTC()
	id := bson.NewObjectId()
	var expected event.Event

	expected.UniqueID = id
	expected.Target = event.Target{Type: "some-invalid-target", Value: "a1"}
	expected.Owner = event.Owner{Type: event.OwnerTypeUser, Name: "u1"}
	expected.Kind = event.Kind{Type: event.KindTypePermission, Name: permission.PermAppDeploy.FullName()}
	expected.StartTime = now
	expected.EndTime = now.Add(time.Minute)
	expected.Error = "err1"
	expected.Allowed = event.Allowed(permission.PermDebug)
	s.checkEvtMatch(&expected, c)
}

func (s *S) checkEvtMatch(evt *event.Event, c *check.C) {
	rawEvt := bson.M{
		"_id":        evt.UniqueID,
		"uniqueid":   evt.UniqueID,
		"starttime":  evt.StartTime,
		"endtime":    evt.EndTime,
		"target":     evt.Target,
		"kind":       evt.Kind,
		"owner":      evt.Owner,
		"error":      evt.Error,
		"cancelable": evt.Cancelable,
		"running":    evt.Running,
	}
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Events().Insert(rawEvt)
	c.Assert(err, check.IsNil)
	err = MigrateRCEvents()
	c.Assert(err, check.IsNil)
	evts, err := event.All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	evt.ID = evts[0].ID
	evt.Instance = evts[0].Instance
	c.Assert(evts[0], check.DeepEquals, evt)

}
