// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	token auth.Token
}

var _ = check.Suite(&S{})

func (s *S) SetUpTest(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_events_tests")
	config.Set("auth:hash-cost", bcrypt.MinCost)
	throttlingInfo = map[string]ThrottlingSpec{}
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = dbtest.ClearAllCollections(conn.Events().Database)
	c.Assert(err, check.IsNil)
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	user := &auth.User{Email: "me@me.com", Password: "123456"}
	_, err = nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
}

func (s *S) TestNewDone(c *check.C) {
	evt, err := New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token})
	c.Assert(err, check.IsNil)
	c.Assert(evt.StartTime.IsZero(), check.Equals, false)
	c.Assert(evt.LockUpdateTime.IsZero(), check.Equals, false)
	expected := &Event{eventData: eventData{
		ID:             eventId{target: Target{Name: "app", Value: "myapp"}},
		Target:         Target{Name: "app", Value: "myapp"},
		Kind:           kind{Type: KindTypePermission, Name: "app.update.env.set"},
		Owner:          owner{Type: OwnerTypeUser, Name: s.token.GetUserName()},
		Running:        true,
		StartTime:      evt.StartTime,
		LockUpdateTime: evt.LockUpdateTime,
	}}
	c.Assert(evt, check.DeepEquals, expected)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].LockUpdateTime.IsZero(), check.Equals, false)
	evts[0].StartTime = expected.StartTime
	evts[0].LockUpdateTime = expected.LockUpdateTime
	c.Assert(evts, check.DeepEquals, []Event{*expected})
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	evts, err = All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].LockUpdateTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	evts[0].EndTime = time.Time{}
	evts[0].StartTime = expected.StartTime
	evts[0].LockUpdateTime = expected.LockUpdateTime
	expected.Running = false
	expected.ID = eventId{objId: evts[0].ID.objId}
	c.Assert(evts, check.DeepEquals, []Event{*expected})
}

func (s *S) TestNewCustomDataDone(c *check.C) {
	customData := struct{ A string }{A: "value"}
	evt, err := New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token, CustomData: customData})
	c.Assert(err, check.IsNil)
	c.Assert(evt.StartTime.IsZero(), check.Equals, false)
	c.Assert(evt.LockUpdateTime.IsZero(), check.Equals, false)
	expected := &Event{eventData: eventData{
		ID:              eventId{target: Target{Name: "app", Value: "myapp"}},
		Target:          Target{Name: "app", Value: "myapp"},
		Kind:            kind{Type: KindTypePermission, Name: "app.update.env.set"},
		Owner:           owner{Type: OwnerTypeUser, Name: s.token.GetUserName()},
		Running:         true,
		StartTime:       evt.StartTime,
		LockUpdateTime:  evt.LockUpdateTime,
		StartCustomData: customData,
	}}
	c.Assert(evt, check.DeepEquals, expected)
	customData = struct{ A string }{A: "other"}
	err = evt.DoneCustomData(nil, customData)
	c.Assert(err, check.IsNil)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].LockUpdateTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	evts[0].EndTime = time.Time{}
	evts[0].StartTime = expected.StartTime
	evts[0].LockUpdateTime = expected.LockUpdateTime
	expected.Running = false
	expected.ID = eventId{objId: evts[0].ID.objId}
	expected.StartCustomData = bson.M{"a": "value"}
	expected.EndCustomData = bson.M{"a": "other"}
	c.Assert(evts, check.DeepEquals, []Event{*expected})
}

func (s *S) TestNewLocks(c *check.C) {
	_, err := New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token})
	c.Assert(err, check.IsNil)
	_, err = New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvUnset, Owner: s.token})
	c.Assert(err, check.ErrorMatches, `event locked: app\(myapp\) running "app.update.env.set" start by user me@me.com at .+`)
}

func (s *S) TestNewLockExpired(c *check.C) {
	oldLockExpire := lockExpireTimeout
	lockExpireTimeout = time.Millisecond
	defer func() {
		lockExpireTimeout = oldLockExpire
	}()
	_, err := New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token})
	c.Assert(err, check.IsNil)
	updater.stop()
	time.Sleep(100 * time.Millisecond)
	_, err = New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvUnset, Owner: s.token})
	c.Assert(err, check.IsNil)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 2)
	c.Assert(evts[0].Kind.Name, check.Equals, "app.update.env.set")
	c.Assert(evts[1].Kind.Name, check.Equals, "app.update.env.unset")
	c.Assert(evts[0].Running, check.Equals, false)
	c.Assert(evts[1].Running, check.Equals, true)
	c.Assert(evts[0].Error, check.Matches, `event expired, no update for [\d.]+\w+`)
	c.Assert(evts[1].Error, check.Equals, "")
}

func (s *S) TestUpdaterUpdatesAndStopsUpdating(c *check.C) {
	updater.stop()
	oldUpdateInterval := lockUpdateInterval
	lockUpdateInterval = time.Millisecond
	defer func() {
		updater.stop()
		lockUpdateInterval = oldUpdateInterval
	}()
	evt, err := New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token})
	c.Assert(err, check.IsNil)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	t0 := evts[0].LockUpdateTime
	time.Sleep(100 * time.Millisecond)
	evts, err = All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	t1 := evts[0].LockUpdateTime
	c.Assert(t0.Before(t1), check.Equals, true)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	time.Sleep(100 * time.Millisecond)
	evts, err = All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	t0 = evts[0].LockUpdateTime
	time.Sleep(100 * time.Millisecond)
	evts, err = All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	t1 = evts[0].LockUpdateTime
	c.Assert(t0, check.DeepEquals, t1)
}

func (s *S) TestEventAbort(c *check.C) {
	evt, err := New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token})
	c.Assert(err, check.IsNil)
	err = evt.Abort()
	c.Assert(err, check.IsNil)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
}

func (s *S) TestEventDoneError(c *check.C) {
	evt, err := New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token})
	c.Assert(err, check.IsNil)
	err = evt.Done(errors.New("myerr"))
	c.Assert(err, check.IsNil)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].LockUpdateTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	expected := &Event{eventData: eventData{
		ID:             eventId{objId: evts[0].ID.objId},
		Target:         Target{Name: "app", Value: "myapp"},
		Kind:           kind{Type: KindTypePermission, Name: "app.update.env.set"},
		Owner:          owner{Type: OwnerTypeUser, Name: s.token.GetUserName()},
		StartTime:      evts[0].StartTime,
		LockUpdateTime: evts[0].LockUpdateTime,
		EndTime:        evts[0].EndTime,
		Error:          "myerr",
	}}
	c.Assert(evts, check.DeepEquals, []Event{*expected})
}

func (s *S) TestEventLogf(c *check.C) {
	evt, err := New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token})
	c.Assert(err, check.IsNil)
	evt.Logf("%s %d", "hey", 42)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Log, check.Equals, "hey 42\n")
}

func (s *S) TestEventLogfWithWriter(c *check.C) {
	evt, err := New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token})
	c.Assert(err, check.IsNil)
	buf := bytes.Buffer{}
	evt.SetLogWriter(&buf)
	evt.Logf("%s %d", "hey", 42)
	c.Assert(buf.String(), check.Equals, "hey 42\n")
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Log, check.Equals, "hey 42\n")
}

func (s *S) TestEventCancel(c *check.C) {
	evt, err := New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token, Cancelable: true})
	c.Assert(err, check.IsNil)
	oldEvt := *evt
	err = evt.TryCancel("because I want", "admin@admin.com")
	c.Assert(err, check.IsNil)
	c.Assert(evt.CancelInfo.StartTime.IsZero(), check.Equals, false)
	evt.CancelInfo.StartTime = time.Time{}
	c.Assert(evt.CancelInfo, check.DeepEquals, cancelInfo{
		Reason: "because I want",
		Owner:  "admin@admin.com",
		Asked:  true,
	})
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].CancelInfo.StartTime.IsZero(), check.Equals, false)
	evts[0].CancelInfo.StartTime = time.Time{}
	c.Assert(evts[0].CancelInfo, check.DeepEquals, cancelInfo{
		Reason: "because I want",
		Owner:  "admin@admin.com",
		Asked:  true,
	})
	err = oldEvt.AckCancel()
	c.Assert(err, check.IsNil)
	c.Assert(oldEvt.CancelInfo.StartTime.IsZero(), check.Equals, false)
	c.Assert(oldEvt.CancelInfo.AckTime.IsZero(), check.Equals, false)
	oldEvt.CancelInfo.StartTime = time.Time{}
	oldEvt.CancelInfo.AckTime = time.Time{}
	c.Assert(oldEvt.CancelInfo, check.DeepEquals, cancelInfo{
		Reason:   "because I want",
		Owner:    "admin@admin.com",
		Asked:    true,
		Canceled: true,
	})
	evts, err = All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].CancelInfo.StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].CancelInfo.AckTime.IsZero(), check.Equals, false)
	evts[0].CancelInfo.StartTime = time.Time{}
	evts[0].CancelInfo.AckTime = time.Time{}
	c.Assert(evts[0].CancelInfo, check.DeepEquals, cancelInfo{
		Reason:   "because I want",
		Owner:    "admin@admin.com",
		Asked:    true,
		Canceled: true,
	})
}

func (s *S) TestEventCancelError(c *check.C) {
	evt, err := New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token})
	c.Assert(err, check.IsNil)
	err = evt.TryCancel("yes", "admin@admin.com")
	c.Assert(err, check.Equals, ErrNotCancelable)
	err = evt.AckCancel()
	c.Assert(err, check.Equals, ErrNotCancelable)
}

func (s *S) TestEventCancelNotAsked(c *check.C) {
	evt, err := New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token, Cancelable: true})
	c.Assert(err, check.IsNil)
	err = evt.AckCancel()
	c.Assert(err, check.Equals, ErrEventNotFound)
}

func (s *S) TestEventCancelNotRunning(c *check.C) {
	evt, err := New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token, Cancelable: true})
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	err = evt.TryCancel("yes", "admin@admin.com")
	c.Assert(err, check.Equals, ErrNotCancelable)
	err = evt.AckCancel()
	c.Assert(err, check.Equals, ErrNotCancelable)
}

func (s *S) TestEventCancelDoneNoError(c *check.C) {
	evt, err := New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token, Cancelable: true})
	c.Assert(err, check.IsNil)
	err = evt.TryCancel("yes", "admin@admin.com")
	c.Assert(err, check.IsNil)
	err = evt.AckCancel()
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Error, check.Equals, "canceled by user request")
}

func (s *S) TestEventCancelDoneCustomError(c *check.C) {
	evt, err := New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token, Cancelable: true})
	c.Assert(err, check.IsNil)
	err = evt.TryCancel("yes", "admin@admin.com")
	c.Assert(err, check.IsNil)
	err = evt.AckCancel()
	c.Assert(err, check.IsNil)
	err = evt.Done(errors.New("my err"))
	c.Assert(err, check.IsNil)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Error, check.Equals, "my err")
}

func (s *S) TestEventNewValidation(c *check.C) {
	_, err := New(nil)
	c.Assert(err, check.Equals, ErrNoOpts)
	_, err = New(&Opts{Kind: permission.PermAppCreate, Owner: s.token})
	c.Assert(err, check.Equals, ErrNoTarget)
	_, err = New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Owner: s.token})
	c.Assert(err, check.Equals, ErrNoKind)
	_, err = New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppCreate})
	c.Assert(err, check.Equals, ErrNoOwner)
}

func (s *S) TestEventDoneLogError(c *check.C) {
	logBuf := bytes.NewBuffer(nil)
	log.SetLogger(log.NewWriterLogger(logBuf, false))
	defer log.SetLogger(nil)
	evt, err := New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token})
	c.Assert(err, check.IsNil)
	config.Set("database:url", "127.0.0.1:99999")
	err = evt.Done(nil)
	c.Assert(err, check.ErrorMatches, "no reachable servers")
	c.Assert(logBuf.String(), check.Matches, `(?s).*\[events\] error marking event as done - .*: no reachable servers.*`)
}

func (s *S) TestNewThrottledAllKinds(c *check.C) {
	SetThrottling(ThrottlingSpec{
		TargetName: "app",
		Time:       time.Hour,
		Max:        2,
	})
	evt, err := New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token})
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	evt, err = New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token})
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	_, err = New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token})
	c.Assert(err, check.FitsTypeOf, ErrThrottled{})
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app \"myapp\" is 2 every 1h0m0s")
	_, err = New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvUnset, Owner: s.token})
	c.Assert(err, check.FitsTypeOf, ErrThrottled{})
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app \"myapp\" is 2 every 1h0m0s")
	evt, err = New(&Opts{Target: Target{Name: "app", Value: "otherapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token})
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
}

func (s *S) TestNewThrottledOneKind(c *check.C) {
	SetThrottling(ThrottlingSpec{
		TargetName: "app",
		KindName:   permission.PermAppUpdateEnvSet.FullName(),
		Time:       time.Hour,
		Max:        2,
	})
	evt, err := New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token})
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	evt, err = New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token})
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	_, err = New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvSet, Owner: s.token})
	c.Assert(err, check.FitsTypeOf, ErrThrottled{})
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app.update.env.set on app \"myapp\" is 2 every 1h0m0s")
	evt, err = New(&Opts{Target: Target{Name: "app", Value: "myapp"}, Kind: permission.PermAppUpdateEnvUnset, Owner: s.token})
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
}
