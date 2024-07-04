// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event_test

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kr/pretty"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/event/webhook"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/servicemanager"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	eventTypes "github.com/tsuru/tsuru/types/event"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
	check "gopkg.in/check.v1"
)

type S struct {
	token auth.Token
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	var err error
	servicemanager.Webhook, err = webhook.WebhookService()
	c.Assert(err, check.IsNil)
}

func (s *S) SetUpTest(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_events_list_tests")
	config.Set("auth:hash-cost", bcrypt.MinCost)
	conn, err := db.Conn()

	storagev2.Reset()

	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = dbtest.ClearAllCollections(conn.Apps().Database)
	c.Assert(err, check.IsNil)
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	user := &auth.User{Email: "me@me.com", Password: "123456"}
	_, err = nativeScheme.Create(context.TODO(), user)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(context.TODO(), map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
}

func (s *S) TestListFilterMany(c *check.C) {
	var allEvts []*event.Event
	var create = func(opts *event.Opts) {
		evt, err := event.New(context.TODO(), opts)
		c.Assert(err, check.IsNil)
		allEvts = append(allEvts, evt)
	}
	var createi = func(opts *event.Opts) {
		evt, err := event.NewInternal(context.TODO(), opts)
		c.Assert(err, check.IsNil)
		allEvts = append(allEvts, evt)
	}
	var checkFilters = func(f *event.Filter, expected interface{}) {
		evts, err := event.List(context.TODO(), f)
		c.Assert(err, check.IsNil)
		c.Assert(evts, eventtest.EvtEquals, expected, check.Commentf("Diff:\n%s", strings.Join(pretty.Diff(evts, expected), "\n")))
	}
	create(&event.Opts{
		Target: eventTypes.Target{Type: "app", Value: "myapp"},
		ExtraTargets: []eventTypes.ExtraTarget{
			{Target: eventTypes.Target{Type: "app", Value: "xapp1"}},
			{Target: eventTypes.Target{Type: "app", Value: "xapp2"}},
		},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppReadEvents, permission.Context(permTypes.CtxApp, "myapp")),
	})
	time.Sleep(100 * time.Millisecond)
	t0 := time.Now().UTC()
	create(&event.Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp2"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppAdmin),
	})
	t05 := time.Now().UTC()
	time.Sleep(100 * time.Millisecond)
	create(&event.Opts{
		Target:  eventTypes.Target{Type: "app2", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppAdmin),
	})
	t1 := time.Now().UTC()
	time.Sleep(100 * time.Millisecond)
	createi(&event.Opts{
		Target:       eventTypes.Target{Type: "node", Value: "http://10.0.1.1"},
		InternalKind: "healer",
		Allowed:      event.Allowed(permission.PermAppAdmin),
	})
	createi(&event.Opts{
		Target:       eventTypes.Target{Type: "node", Value: "http://10.0.1.2"},
		InternalKind: "healer",
		Allowed:      event.Allowed(permission.PermAppAdmin),
	})
	allEvts[len(allEvts)-1].Done(context.TODO(), nil)
	allEvts[len(allEvts)-2].Done(context.TODO(), errors.New("my err"))
	checkFilters(&event.Filter{Sort: "_id"}, allEvts)
	checkFilters(&event.Filter{Running: boolPtr(false), Sort: "_id"}, allEvts[len(allEvts)-2:])
	checkFilters(&event.Filter{Running: boolPtr(true), Sort: "_id"}, allEvts[:len(allEvts)-2])
	checkFilters(&event.Filter{ErrorOnly: true, Sort: "_id"}, allEvts[len(allEvts)-2])
	checkFilters(&event.Filter{Target: eventTypes.Target{Type: "app"}, Sort: "_id"}, []*event.Event{allEvts[0], allEvts[1]})
	checkFilters(&event.Filter{Target: eventTypes.Target{Type: "app", Value: "myapp"}}, allEvts[0])
	checkFilters(&event.Filter{Target: eventTypes.Target{Type: "app", Value: "xapp1"}}, allEvts[0])
	checkFilters(&event.Filter{Target: eventTypes.Target{Type: "app", Value: "xapp2"}}, allEvts[0])
	checkFilters(&event.Filter{KindType: eventTypes.KindTypeInternal, Sort: "_id"}, allEvts[3:])
	checkFilters(&event.Filter{KindType: eventTypes.KindTypePermission, Sort: "_id"}, allEvts[:3])
	checkFilters(&event.Filter{KindType: eventTypes.KindTypePermission, KindNames: []string{"kind"}}, nil)
	checkFilters(&event.Filter{KindType: eventTypes.KindTypeInternal, KindNames: []string{"healer"}, Sort: "_id"}, allEvts[3:])
	checkFilters(&event.Filter{OwnerType: eventTypes.OwnerTypeUser, Sort: "_id"}, allEvts[:3])
	checkFilters(&event.Filter{OwnerType: eventTypes.OwnerTypeInternal, Sort: "_id"}, allEvts[3:])
	checkFilters(&event.Filter{OwnerType: eventTypes.OwnerTypeUser, OwnerName: s.token.GetUserName(), Sort: "_id"}, allEvts[:3])
	checkFilters(&event.Filter{Since: t0, Sort: "_id"}, allEvts[1:])
	checkFilters(&event.Filter{Until: t05, Sort: "_id"}, allEvts[:2])
	checkFilters(&event.Filter{Since: t0, Until: t1, Sort: "_id"}, allEvts[1:3])
	checkFilters(&event.Filter{Limit: 2, Sort: "_id"}, allEvts[:2])
	checkFilters(&event.Filter{Limit: 1, Sort: "-_id"}, allEvts[len(allEvts)-1])
	checkFilters(&event.Filter{AllowedTargets: []event.TargetFilter{}, Sort: "_id"}, allEvts[:0])
	checkFilters(&event.Filter{AllowedTargets: []event.TargetFilter{
		{Type: "app"},
	}, Sort: "_id"}, allEvts[:2])
	checkFilters(&event.Filter{AllowedTargets: []event.TargetFilter{
		{Type: "app", Values: []string{}},
	}, Sort: "_id"}, allEvts[:0])
	checkFilters(&event.Filter{AllowedTargets: []event.TargetFilter{
		{Type: "app", Values: []string{"myapp", "myapp2"}},
	}, Sort: "_id"}, allEvts[:2])
	checkFilters(&event.Filter{AllowedTargets: []event.TargetFilter{
		{Type: "app", Values: []string{"myapp"}},
		{Type: "node", Values: []string{"http://10.0.1.2"}},
	}, Sort: "_id"}, []*event.Event{allEvts[0], allEvts[4]})
	checkFilters(&event.Filter{AllowedTargets: []event.TargetFilter{
		{Type: "app", Values: []string{"xapp1", "myapp2"}},
	}, Sort: "_id"}, allEvts[:2])
	checkFilters(&event.Filter{AllowedTargets: []event.TargetFilter{
		{Type: "app", Values: []string{"xapp2"}},
	}, Sort: "_id"}, allEvts[0])
	checkFilters(&event.Filter{Permissions: []permission.Permission{
		{Scheme: permission.PermAll, Context: permission.Context(permTypes.CtxGlobal, "")},
	}, Sort: "_id"}, allEvts[:])
	checkFilters(&event.Filter{Permissions: []permission.Permission{
		{Scheme: permission.PermAll},
	}, Sort: "_id"}, allEvts[:0])
	checkFilters(&event.Filter{Permissions: []permission.Permission{
		{Scheme: permission.PermAppRead, Context: permission.Context(permTypes.CtxApp, "myapp")},
		{Scheme: permission.PermAppRead, Context: permission.Context(permTypes.CtxApp, "invalid-app")},
	}, Sort: "_id"}, allEvts[:1])
	checkFilters(&event.Filter{Permissions: []permission.Permission{
		{Scheme: permission.PermAppRead, Context: permission.Context(permTypes.CtxApp, "invalid-app")},
	}, Sort: "_id"}, allEvts[:0])
}

func (s *S) TestGetByID(c *check.C) {
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)

	otherEvt, err := event.GetByID(context.TODO(), evt.ID)
	c.Assert(err, check.IsNil)
	c.Assert(evt, eventtest.EvtEquals, otherEvt)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	otherEvt, err = event.GetByID(context.TODO(), evt.ID)
	c.Assert(err, check.IsNil)
	c.Assert(evt, eventtest.EvtEquals, otherEvt)
	otherEvt, err = event.GetByID(context.TODO(), primitive.NewObjectID())
	c.Assert(otherEvt, check.IsNil)
	c.Assert(err, check.Equals, event.ErrEventNotFound)
}

func (s *S) TestGetRunning(c *check.C) {
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	getEvt, err := event.GetRunning(context.TODO(), eventTypes.Target{Type: "app", Value: "myapp"}, permission.PermAppUpdateEnvSet.FullName())
	c.Assert(err, check.IsNil)
	c.Assert(evt, eventtest.EvtEquals, getEvt)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	_, err = event.GetRunning(context.TODO(), eventTypes.Target{Type: "app", Value: "myapp"}, permission.PermAppUpdateEnvSet.FullName())
	c.Assert(err, check.Equals, event.ErrEventNotFound)
}

func boolPtr(b bool) *bool {
	return &b
}

func (s *S) TestGetKinds(c *check.C) {
	_, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	kinds, err := event.GetKinds(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(kinds, check.HasLen, 1)
	expected := []eventTypes.Kind{
		{Type: "permission", Name: "app.update.env.set"},
	}
	c.Assert(kinds, check.DeepEquals, expected)
}
