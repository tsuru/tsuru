// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/safe"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	token auth.Token
}

var _ = check.Suite(&S{})

func setBaseConfig() {
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=150")
	config.Set("database:name", "tsuru_events_tests")
	config.Set("auth:hash-cost", bcrypt.MinCost)
}

func (s *S) SetUpTest(c *check.C) {
	setBaseConfig()
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
	evt, err := New(&Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	c.Assert(evt.StartTime.IsZero(), check.Equals, false)
	c.Assert(evt.LockUpdateTime.IsZero(), check.Equals, false)
	expected := &Event{eventData: eventData{
		ID:             eventID{Target: Target{Type: "app", Value: "myapp"}},
		UniqueID:       evt.UniqueID,
		Target:         Target{Type: "app", Value: "myapp"},
		Kind:           Kind{Type: KindTypePermission, Name: "app.update.env.set"},
		Owner:          Owner{Type: OwnerTypeUser, Name: s.token.GetUserName()},
		Running:        true,
		StartTime:      evt.StartTime,
		LockUpdateTime: evt.LockUpdateTime,
		Allowed:        Allowed(permission.PermAppReadEvents),
	}}
	expected.Init()
	c.Assert(evt, check.DeepEquals, expected)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].LockUpdateTime.IsZero(), check.Equals, false)
	evts[0].StartTime = expected.StartTime
	evts[0].LockUpdateTime = expected.LockUpdateTime
	c.Assert(&evts[0], check.DeepEquals, expected)
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
	expected.ID = eventID{ObjId: evts[0].ID.ObjId}
	c.Assert(&evts[0], check.DeepEquals, expected)
}

func (s *S) TestNewCustomDataDone(c *check.C) {
	customData := struct{ A string }{A: "value"}
	evt, err := New(&Opts{
		Target:     Target{Type: "app", Value: "myapp"},
		Kind:       permission.PermAppUpdateEnvSet,
		Owner:      s.token,
		CustomData: customData,
		Allowed:    Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	c.Assert(evt.StartTime.IsZero(), check.Equals, false)
	c.Assert(evt.LockUpdateTime.IsZero(), check.Equals, false)
	var resultData struct{ A string }
	err = evt.StartData(&resultData)
	c.Assert(err, check.IsNil)
	c.Assert(resultData, check.DeepEquals, customData)
	expected := &Event{eventData: eventData{
		ID:              eventID{Target: Target{Type: "app", Value: "myapp"}},
		UniqueID:        evt.UniqueID,
		Target:          Target{Type: "app", Value: "myapp"},
		Kind:            Kind{Type: KindTypePermission, Name: "app.update.env.set"},
		Owner:           Owner{Type: OwnerTypeUser, Name: s.token.GetUserName()},
		Running:         true,
		StartTime:       evt.StartTime,
		LockUpdateTime:  evt.LockUpdateTime,
		StartCustomData: evt.StartCustomData,
		Allowed:         Allowed(permission.PermAppReadEvents),
	}}
	expected.Init()
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
	err = evts[0].StartData(&resultData)
	c.Assert(err, check.IsNil)
	c.Assert(resultData, check.DeepEquals, struct{ A string }{A: "value"})
	err = evts[0].EndData(&resultData)
	c.Assert(err, check.IsNil)
	c.Assert(resultData, check.DeepEquals, struct{ A string }{A: "other"})
	evts[0].EndTime = time.Time{}
	evts[0].StartTime = expected.StartTime
	evts[0].LockUpdateTime = expected.LockUpdateTime
	expected.Running = false
	expected.ID = eventID{ObjId: evts[0].ID.ObjId}
	expected.EndCustomData = evts[0].EndCustomData
	c.Assert(&evts[0], check.DeepEquals, expected)
}

func (s *S) TestNewLocks(c *check.C) {
	_, err := New(&Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	_, err = New(&Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvUnset,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.ErrorMatches, `event locked: app\(myapp\) running "app.update.env.set" start by user me@me.com at .+`)
}

func (s *S) TestNewExtraTargetLocks(c *check.C) {
	tests := []struct {
		target1      Target
		extras1      []ExtraTarget
		target2      Target
		extras2      []ExtraTarget
		err          string
		disableLock1 bool
		disableLock2 bool
	}{
		{
			target1: Target{Type: "app", Value: "myapp"},
			target2: Target{Type: "container", Value: "x"},
			extras2: []ExtraTarget{{Target: Target{Type: "app", Value: "myapp"}, Lock: true}},
			err:     `event locked: app\(myapp\) running "app.update.env.set" start by user me@me.com at .+`,
		},
		{
			target1: Target{Type: "app", Value: "myapp"},
			extras1: []ExtraTarget{{Target: Target{Type: "app", Value: "myapp2"}, Lock: true}},
			target2: Target{Type: "app", Value: "myapp2"},
			err:     `event locked: app\(myapp\) running "app.update.env.set" start by user me@me.com at .+`,
		},
		{
			target1: Target{Type: "app", Value: "myapp"},
			target2: Target{Type: "container", Value: "x"},
			extras2: []ExtraTarget{{Target: Target{Type: "app", Value: "myapp"}, Lock: false}},
			err:     "",
		},
		{
			target1: Target{Type: "app", Value: "myapp"},
			extras1: []ExtraTarget{{Target: Target{Type: "app", Value: "myapp2"}, Lock: false}},
			target2: Target{Type: "app", Value: "myapp2"},
			err:     "",
		},
		{
			target1:      Target{Type: "app", Value: "myapp"},
			extras1:      []ExtraTarget{{Target: Target{Type: "app", Value: "myapp2"}, Lock: true}},
			target2:      Target{Type: "app", Value: "myapp2"},
			disableLock2: true,
			err:          "",
		},
		{
			target1:      Target{Type: "app", Value: "myapp"},
			extras1:      []ExtraTarget{{Target: Target{Type: "app", Value: "myapp2"}, Lock: true}},
			target2:      Target{Type: "app", Value: "myapp2"},
			disableLock2: true,
			err:          "",
		},
	}
	for i, tt := range tests {
		evt1, err := New(&Opts{
			Target:       tt.target1,
			ExtraTargets: tt.extras1,
			Kind:         permission.PermAppUpdateEnvSet,
			Owner:        s.token,
			Allowed:      Allowed(permission.PermAppReadEvents),
			DisableLock:  tt.disableLock1,
		})
		c.Assert(err, check.IsNil)
		evt2, err := New(&Opts{
			Target:       tt.target2,
			ExtraTargets: tt.extras2,
			Kind:         permission.PermAppUpdateEnvUnset,
			Owner:        s.token,
			Allowed:      Allowed(permission.PermAppReadEvents),
			DisableLock:  tt.disableLock2,
		})
		if tt.err != "" {
			c.Assert(err, check.ErrorMatches, tt.err, check.Commentf("failed test case %d - %#v", i, tt))
		} else {
			c.Assert(err, check.IsNil, check.Commentf("failed test case %d - %#v", i, tt))
		}
		err = evt1.Done(nil)
		c.Assert(err, check.IsNil)
		if evt2 != nil {
			err = evt2.Done(nil)
			c.Assert(err, check.IsNil)
		}
	}
}

func (s *S) TestNewLockExtraTargetRace(c *check.C) {
	originalMaxProcs := runtime.GOMAXPROCS(10)
	defer runtime.GOMAXPROCS(originalMaxProcs)
	wg := sync.WaitGroup{}
	var countOK int32
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := New(&Opts{
				Target: Target{Type: "app", Value: fmt.Sprintf("myapp-%d", i)},
				ExtraTargets: []ExtraTarget{
					{Target: Target{Type: "app", Value: "myapp"}, Lock: true},
				},
				Kind:    permission.PermAppUpdateEnvSet,
				Owner:   s.token,
				Allowed: Allowed(permission.PermAppReadEvents),
			})
			if _, ok := err.(ErrEventLocked); ok {
				return
			}
			c.Assert(err, check.IsNil)
			atomic.AddInt32(&countOK, 1)
		}(i)
	}
	wg.Wait()
	c.Assert(countOK <= 1, check.Equals, true)
}

func (s *S) TestNewDoneDisableLock(c *check.C) {
	evt, err := New(&Opts{
		Target:      Target{Type: "app", Value: "myapp"},
		Kind:        permission.PermAppUpdateEnvSet,
		Owner:       s.token,
		DisableLock: true,
		Allowed:     Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	c.Assert(evt.StartTime.IsZero(), check.Equals, false)
	c.Assert(evt.LockUpdateTime.IsZero(), check.Equals, false)
	expected := &Event{eventData: eventData{
		ID:             eventID{ObjId: evt.UniqueID},
		UniqueID:       evt.UniqueID,
		Target:         Target{Type: "app", Value: "myapp"},
		Kind:           Kind{Type: KindTypePermission, Name: "app.update.env.set"},
		Owner:          Owner{Type: OwnerTypeUser, Name: s.token.GetUserName()},
		Running:        true,
		StartTime:      evt.StartTime,
		LockUpdateTime: evt.LockUpdateTime,
		Allowed:        Allowed(permission.PermAppReadEvents),
	}}
	expected.Init()
	c.Assert(evt, check.DeepEquals, expected)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].LockUpdateTime.IsZero(), check.Equals, false)
	evts[0].StartTime = expected.StartTime
	evts[0].LockUpdateTime = expected.LockUpdateTime
	c.Assert(&evts[0], check.DeepEquals, expected)
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
	expected.ID = eventID{ObjId: evts[0].ID.ObjId}
	c.Assert(&evts[0], check.DeepEquals, expected)
}

func (s *S) TestNewLockExpired(c *check.C) {
	oldLockExpire := lockExpireTimeout
	lockExpireTimeout = time.Millisecond
	defer func() {
		lockExpireTimeout = oldLockExpire
	}()
	_, err := New(&Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	updater.stop()
	time.Sleep(100 * time.Millisecond)
	_, err = New(&Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvUnset,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 2)
	c.Assert(evts[0].Kind.Name, check.Equals, "app.update.env.unset")
	c.Assert(evts[1].Kind.Name, check.Equals, "app.update.env.set")
	c.Assert(evts[0].Running, check.Equals, true)
	c.Assert(evts[1].Running, check.Equals, false)
	c.Assert(evts[0].Error, check.Equals, "")
	c.Assert(evts[1].Error, check.Matches, `event expired, no update for [\d.]+\w+`)
}

func (s *S) TestNewEventBlocked(c *check.C) {
	err := AddBlock(&Block{KindName: "app.deploy", Reason: "you shall not pass"})
	c.Assert(err, check.IsNil)
	blocks, err := listBlocks(nil)
	c.Assert(err, check.IsNil)
	_, err = New(&Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.NotNil)
	c.Assert(err.(ErrEventBlocked).block, check.DeepEquals, &blocks[0])
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Running, check.Equals, false)
	c.Assert(evts[0].Error, check.Matches, `.*block app.deploy by all users on all targets: you shall not pass$`)
}

func (s *S) TestEventAbort(c *check.C) {
	evt, err := New(&Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.Abort()
	c.Assert(err, check.IsNil)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
}

func (s *S) TestEventDoneError(c *check.C) {
	evt, err := New(&Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
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
		ID:             eventID{ObjId: evts[0].ID.ObjId},
		UniqueID:       evts[0].UniqueID,
		Target:         Target{Type: "app", Value: "myapp"},
		Kind:           Kind{Type: KindTypePermission, Name: "app.update.env.set"},
		Owner:          Owner{Type: OwnerTypeUser, Name: s.token.GetUserName()},
		StartTime:      evts[0].StartTime,
		LockUpdateTime: evts[0].LockUpdateTime,
		EndTime:        evts[0].EndTime,
		Error:          "myerr",
		Allowed:        Allowed(permission.PermAppReadEvents),
	}}
	expected.Init()
	c.Assert(&evts[0], check.DeepEquals, expected)
}

func (s *S) TestEventLogf(c *check.C) {
	evt, err := New(&Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
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
	evt, err := New(&Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
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
	evt, err := New(&Opts{
		Target:        Target{Type: "app", Value: "myapp"},
		Kind:          permission.PermAppUpdateEnvSet,
		Owner:         s.token,
		Cancelable:    true,
		Allowed:       Allowed(permission.PermAppReadEvents),
		AllowedCancel: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	err = evts[0].TryCancel("because I want", "admin@admin.com")
	c.Assert(err, check.IsNil)
	c.Assert(evts[0].CancelInfo.StartTime.IsZero(), check.Equals, false)
	evts[0].CancelInfo.StartTime = time.Time{}
	c.Assert(evts[0].CancelInfo, check.DeepEquals, cancelInfo{
		Reason: "because I want",
		Owner:  "admin@admin.com",
		Asked:  true,
	})
	evts, err = All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].CancelInfo.StartTime.IsZero(), check.Equals, false)
	evts[0].CancelInfo.StartTime = time.Time{}
	c.Assert(evts[0].CancelInfo, check.DeepEquals, cancelInfo{
		Reason: "because I want",
		Owner:  "admin@admin.com",
		Asked:  true,
	})
	canceled, err := evt.AckCancel()
	c.Assert(canceled, check.Equals, true)
	c.Assert(err, check.IsNil)
	c.Assert(evt.CancelInfo.StartTime.IsZero(), check.Equals, false)
	c.Assert(evt.CancelInfo.AckTime.IsZero(), check.Equals, false)
	evt.CancelInfo.StartTime = time.Time{}
	evt.CancelInfo.AckTime = time.Time{}
	c.Assert(evt.CancelInfo, check.DeepEquals, cancelInfo{
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

func (s *S) TestEventCancelMultipleTimes(c *check.C) {
	evt, err := New(&Opts{
		Target:        Target{Type: "app", Value: "myapp"},
		Kind:          permission.PermAppUpdateEnvSet,
		Owner:         s.token,
		Cancelable:    true,
		Allowed:       Allowed(permission.PermAppReadEvents),
		AllowedCancel: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.TryCancel("because I want", "admin@admin.com")
	c.Assert(err, check.IsNil)
	err = evt.TryCancel("because I still want", "admin@admin.com")
	c.Assert(err, check.DeepEquals, ErrCancelAlreadyRequested)
}

func (s *S) TestEventCancelNotCancelable(c *check.C) {
	evt, err := New(&Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.TryCancel("yes", "admin@admin.com")
	c.Assert(err, check.Equals, ErrNotCancelable)
	canceled, err := evt.AckCancel()
	c.Assert(err, check.IsNil)
	c.Assert(canceled, check.Equals, false)
}

func (s *S) TestEventCancelNotAsked(c *check.C) {
	evt, err := New(&Opts{
		Target:        Target{Type: "app", Value: "myapp"},
		Kind:          permission.PermAppUpdateEnvSet,
		Owner:         s.token,
		Cancelable:    true,
		Allowed:       Allowed(permission.PermAppReadEvents),
		AllowedCancel: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	canceled, err := evt.AckCancel()
	c.Assert(err, check.IsNil)
	c.Assert(canceled, check.Equals, false)
}

func (s *S) TestEventCancelNotRunning(c *check.C) {
	evt, err := New(&Opts{
		Target:        Target{Type: "app", Value: "myapp"},
		Kind:          permission.PermAppUpdateEnvSet,
		Owner:         s.token,
		Cancelable:    true,
		Allowed:       Allowed(permission.PermAppReadEvents),
		AllowedCancel: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	err = evt.TryCancel("yes", "admin@admin.com")
	c.Assert(err, check.Equals, ErrNotCancelable)
	canceled, err := evt.AckCancel()
	c.Assert(err, check.IsNil)
	c.Assert(canceled, check.Equals, false)
}

func (s *S) TestEventCancelDoneNoError(c *check.C) {
	evt, err := New(&Opts{
		Target:        Target{Type: "app", Value: "myapp"},
		Kind:          permission.PermAppUpdateEnvSet,
		Owner:         s.token,
		Cancelable:    true,
		Allowed:       Allowed(permission.PermAppReadEvents),
		AllowedCancel: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.TryCancel("yes", "admin@admin.com")
	c.Assert(err, check.IsNil)
	canceled, err := evt.AckCancel()
	c.Assert(err, check.IsNil)
	c.Assert(canceled, check.Equals, true)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Error, check.Equals, "canceled by user request")
}

func (s *S) TestEventCancelDoneCustomError(c *check.C) {
	evt, err := New(&Opts{
		Target:        Target{Type: "app", Value: "myapp"},
		Kind:          permission.PermAppUpdateEnvSet,
		Owner:         s.token,
		Cancelable:    true,
		Allowed:       Allowed(permission.PermAppReadEvents),
		AllowedCancel: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.TryCancel("yes", "admin@admin.com")
	c.Assert(err, check.IsNil)
	canceled, err := evt.AckCancel()
	c.Assert(err, check.IsNil)
	c.Assert(canceled, check.Equals, true)
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
	_, err = New(&Opts{Target: Target{Type: "app", Value: "myapp"}, Owner: s.token})
	c.Assert(err, check.Equals, ErrNoKind)
	_, err = New(&Opts{Target: Target{Type: "app", Value: "myapp"}, Kind: permission.PermAppCreate})
	c.Assert(err, check.Equals, ErrNoOwner)
}

func (s *S) TestEventDoneLogError(c *check.C) {
	logBuf := safe.NewBuffer(nil)
	log.SetLogger(log.NewWriterLogger(logBuf, false))
	defer log.SetLogger(nil)
	evt, err := New(&Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	config.Set("database:url", "127.0.0.1:99999")
	err = evt.Done(nil)
	c.Assert(err, check.ErrorMatches, "no reachable servers")
	c.Assert(logBuf.String(), check.Matches, `(?s).*\[events\] error marking event as done - .*: no reachable servers.*`)
}

func (s *S) TestNewThrottledAllKinds(c *check.C) {
	SetThrottling(ThrottlingSpec{
		TargetType: TargetTypeApp,
		Time:       time.Hour,
		Max:        2,
	})
	evt, err := New(&Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	evt, err = New(&Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	_, err = New(&Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.FitsTypeOf, ErrThrottled{})
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app \"myapp\" is 2 every 1h0m0s")
	_, err = New(&Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvUnset,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.FitsTypeOf, ErrThrottled{})
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app \"myapp\" is 2 every 1h0m0s")
	evt, err = New(&Opts{
		Target:  Target{Type: "app", Value: "otherapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
}

func (s *S) TestNewThrottledOneKind(c *check.C) {
	SetThrottling(ThrottlingSpec{
		TargetType: TargetTypeApp,
		KindName:   permission.PermAppUpdateEnvSet.FullName(),
		Time:       time.Hour,
		Max:        2,
	})
	evt, err := New(&Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	evt, err = New(&Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	_, err = New(&Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.FitsTypeOf, ErrThrottled{})
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app.update.env.set on app \"myapp\" is 2 every 1h0m0s")
	// A different target value is not throttled
	_, err = New(&Opts{
		Target:  Target{Type: "app", Value: "myapp2"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	// A different kind is not throttled
	evt, err = New(&Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvUnset,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
}

func (s *S) TestNewThrottledAllTargets(c *check.C) {
	SetThrottling(ThrottlingSpec{
		TargetType: TargetTypeApp,
		KindName:   permission.PermAppUpdateEnvSet.FullName(),
		Time:       time.Hour,
		Max:        1,
		AllTargets: true,
	})
	baseOpts := &Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	}
	evt, err := New(baseOpts)
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	_, err = New(baseOpts)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app.update.env.set on any app is 1 every 1h0m0s")
	baseOpts.Target.Value = "myapp2"
	_, err = New(baseOpts)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app.update.env.set on any app is 1 every 1h0m0s")
}

func (s *S) TestNewThrottledAllTargetsTwoRules(c *check.C) {
	SetThrottling(ThrottlingSpec{
		TargetType: TargetTypeApp,
		KindName:   permission.PermAppUpdateEnvSet.FullName(),
		Time:       time.Hour,
		Max:        2,
		AllTargets: true,
	})
	SetThrottling(ThrottlingSpec{
		TargetType: TargetTypeApp,
		KindName:   permission.PermAppUpdateEnvSet.FullName(),
		Time:       time.Hour,
		Max:        1,
	})
	baseOpts := &Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	}
	evt, err := New(baseOpts)
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	_, err = New(baseOpts)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app.update.env.set on app \"myapp\" is 1 every 1h0m0s")
	app2Opts := &Opts{
		Target:  Target{Type: "app", Value: "myapp2"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	}
	_, err = New(app2Opts)
	c.Assert(err, check.IsNil)
	app3Opts := &Opts{
		Target:  Target{Type: "app", Value: "myapp3"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	}
	_, err = New(app3Opts)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app.update.env.set on any app is 2 every 1h0m0s")
}

func (s *S) TestNewThrottledExpiration(c *check.C) {
	SetThrottling(ThrottlingSpec{
		TargetType: TargetTypeApp,
		KindName:   permission.PermAppUpdateEnvSet.FullName(),
		Time:       300 * time.Millisecond,
		Max:        1,
		AllTargets: true,
	})
	baseOpts := &Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	}
	_, err := New(baseOpts)
	c.Assert(err, check.IsNil)
	otherOpts := &Opts{
		Target:  Target{Type: "app", Value: "myapp2"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	}
	_, err = New(otherOpts)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app.update.env.set on any app is 1 every 300ms")
	time.Sleep(400 * time.Millisecond)
	_, err = New(otherOpts)
	c.Assert(err, check.IsNil)
}

func (s *S) TestNewThrottledExpirationWaitFinish(c *check.C) {
	SetThrottling(ThrottlingSpec{
		TargetType: TargetTypeApp,
		KindName:   permission.PermAppUpdateEnvSet.FullName(),
		Time:       300 * time.Millisecond,
		Max:        1,
		AllTargets: true,
		WaitFinish: true,
	})
	baseOpts := &Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	}
	evt, err := New(baseOpts)
	c.Assert(err, check.IsNil)
	otherOpts := &Opts{
		Target:  Target{Type: "app", Value: "myapp2"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	}
	_, err = New(otherOpts)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app.update.env.set on any app is 1 every 300ms")
	time.Sleep(400 * time.Millisecond)
	_, err = New(otherOpts)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app.update.env.set on any app is 1 every 300ms")
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	_, err = New(otherOpts)
	c.Assert(err, check.IsNil)
}

func (s *S) TestNewThrottledExpirationWaitFinishExpired(c *check.C) {
	oldLockExpire := lockExpireTimeout
	lockExpireTimeout = 200 * time.Millisecond
	defer func() {
		lockExpireTimeout = oldLockExpire
	}()
	SetThrottling(ThrottlingSpec{
		TargetType: TargetTypeApp,
		KindName:   permission.PermAppUpdateEnvSet.FullName(),
		Time:       300 * time.Millisecond,
		Max:        1,
		AllTargets: true,
		WaitFinish: true,
	})
	baseOpts := &Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	}
	_, err := New(baseOpts)
	c.Assert(err, check.IsNil)
	updater.stop()
	otherOpts := &Opts{
		Target:  Target{Type: "app", Value: "myapp2"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	}
	_, err = New(otherOpts)
	c.Assert(err, check.NotNil)
	updater.stop()
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app.update.env.set on any app is 1 every 300ms")
	time.Sleep(500 * time.Millisecond)
	_, err = New(otherOpts)
	c.Check(err, check.IsNil)
}

func (s *S) TestListWithFilters(c *check.C) {
	e1, err := New(&Opts{Owner: s.token, Kind: permission.PermAll, Allowed: Allowed(permission.PermNode), Target: Target{Type: "node"}})
	c.Assert(err, check.IsNil)
	err = e1.Done(nil)
	c.Assert(err, check.IsNil)
	e2, err := New(&Opts{Owner: s.token, Kind: permission.PermAll, Allowed: Allowed(permission.PermNode), Target: Target{Type: "container"}})
	c.Assert(err, check.IsNil)
	err = e2.Done(nil)
	c.Assert(err, check.IsNil)
	e3, err := New(&Opts{Owner: s.token, Kind: permission.PermAppCreate, Allowed: Allowed(permission.PermNode), Target: Target{Type: "container", Value: "1234"}})
	c.Assert(err, check.IsNil)
	err = e3.Done(nil)
	c.Assert(err, check.IsNil)

	evts, err := List(nil)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 3)

	evts, err = List(&Filter{Target: Target{Type: "container"}})
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 2)
	c.Assert(evts[0].Target.Type, check.Equals, TargetType("container"))
	c.Assert(evts[1].Target.Type, check.Equals, TargetType("container"))

	evts, err = List(&Filter{Target: Target{Type: "container", Value: "1234"}})
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Target.Type, check.Equals, TargetType("container"))
	c.Assert(evts[0].Target.Value, check.Equals, "1234")

	evts, err = List(&Filter{Target: Target{Type: "container", Value: "unknown"}})
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)

	evts, err = List(&Filter{Target: Target{Type: "node"}, Since: time.Now().Add(time.Duration(-1 * time.Hour))})
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Target.Type, check.Equals, TargetType("node"))
}

func (s *S) TestListFilterPruneUserValues(c *check.C) {
	t := true
	f := Filter{
		Target:         Target{Type: "app", Value: "myapp"},
		KindType:       KindTypePermission,
		KindNames:      []string{"a"},
		OwnerType:      OwnerTypeUser,
		OwnerName:      "u",
		Since:          time.Now(),
		Until:          time.Now(),
		Running:        &t,
		IncludeRemoved: true,
		Raw:            bson.M{"a": 1},
		AllowedTargets: []TargetFilter{{Type: TargetTypeApp, Values: []string{"a1"}}},
		Limit:          50,
		Skip:           10,
		Sort:           "id",
	}
	expectedFilter := f
	expectedFilter.Raw = nil
	expectedFilter.AllowedTargets = nil
	f.PruneUserValues()
	c.Assert(f, check.DeepEquals, expectedFilter)
	f.Limit = 110
	expectedFilter.Limit = 100
	f.PruneUserValues()
	c.Assert(f, check.DeepEquals, expectedFilter)
	f.Limit = 0
	expectedFilter.Limit = 100
	f.PruneUserValues()
	c.Assert(f, check.DeepEquals, expectedFilter)
	f.Limit = -10
	expectedFilter.Limit = 100
	f.PruneUserValues()
	c.Assert(f, check.DeepEquals, expectedFilter)
}

func (s *S) TestLoadKindNames(c *check.C) {
	f := &Filter{}
	form := map[string][]string{
		"kindname": {"a", "b", ""},
		"kindName": {"c", "d"},
		"KindName": {"e", "f"},
		"KINDNAME": {"g", "h"},
	}
	f.LoadKindNames(form)
	sort.Strings(f.KindNames)
	c.Assert(f.KindNames, check.DeepEquals, []string{"a", "b", "c", "d", "e", "f", "g", "h"})
}

func (s *S) TestLoadKindNamesOnlyEmptyValues(c *check.C) {
	f := &Filter{}
	form := map[string][]string{
		"kindname": {""},
	}
	f.LoadKindNames(form)
	c.Assert(f.KindNames, check.IsNil)
}

func (s *S) TestEventOtherCustomData(c *check.C) {
	_, err := New(&Opts{
		Target:     Target{Type: "app", Value: "myapp"},
		Kind:       permission.PermAppUpdateEnvSet,
		Owner:      s.token,
		CustomData: map[string]string{"x": "y"},
		Allowed:    Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	getEvt, err := GetRunning(Target{Type: "app", Value: "myapp"}, permission.PermAppUpdateEnvSet.FullName())
	c.Assert(err, check.IsNil)
	err = getEvt.SetOtherCustomData(map[string]string{"z": "h"})
	c.Assert(err, check.IsNil)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Owner, check.DeepEquals, Owner{Type: OwnerTypeUser, Name: s.token.GetUserName()})
	var data map[string]string
	err = evts[0].StartData(&data)
	c.Assert(err, check.IsNil)
	c.Assert(data, check.DeepEquals, map[string]string{"x": "y"})
	err = evts[0].OtherData(&data)
	c.Assert(err, check.IsNil)
	c.Assert(data, check.DeepEquals, map[string]string{"z": "h"})
}

func (s *S) TestEventAsWriter(c *check.C) {
	evt, err := New(&Opts{
		Target:     Target{Type: "app", Value: "myapp"},
		Kind:       permission.PermAppUpdateEnvSet,
		Owner:      s.token,
		CustomData: map[string]string{"x": "y"},
		Allowed:    Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	var writer io.Writer = evt
	n, err := writer.Write([]byte("hey"))
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 3)
	c.Assert(evt.logBuffer.String(), check.Equals, "hey")
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	c.Assert(evt.Log, check.Equals, "hey")
	evt2, err := New(&Opts{
		Target:     Target{Type: "app", Value: "myapp"},
		Kind:       permission.PermAppUpdateEnvSet,
		Owner:      s.token,
		CustomData: map[string]string{"x": "y"},
		Allowed:    Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	var otherWriter bytes.Buffer
	evt2.SetLogWriter(&otherWriter)
	evt2.Write([]byte("hey2"))
	c.Assert(evt2.logBuffer.String(), check.Equals, "hey2")
	c.Assert(otherWriter.String(), check.Equals, "hey2")
	err = evt2.Done(nil)
	c.Assert(err, check.IsNil)
	c.Assert(evt2.Log, check.Equals, "hey2")
}

func (s *S) TestGetTargetType(c *check.C) {
	var tests = []struct {
		input  string
		output TargetType
		err    error
	}{
		{"app", TargetTypeApp, nil},
		{"node", TargetTypeNode, nil},
		{"container", TargetTypeContainer, nil},
		{"pool", TargetTypePool, nil},
		{"service", TargetTypeService, nil},
		{"service-instance", TargetTypeServiceInstance, nil},
		{"team", TargetTypeTeam, nil},
		{"user", TargetTypeUser, nil},
		{"invalid", "", ErrInvalidTargetType},
	}
	for _, t := range tests {
		got, err := GetTargetType(t.input)
		c.Check(got, check.Equals, t.output)
		c.Check(err, check.Equals, t.err)
	}
}

func (s *S) TestEventRawInsert(c *check.C) {
	now := time.Unix(time.Now().Unix(), 0).UTC()
	evt := &Event{eventData: eventData{
		UniqueID:  bson.NewObjectId(),
		Target:    Target{Type: "app", Value: "myapp"},
		Owner:     Owner{Type: OwnerTypeUser, Name: s.token.GetUserName()},
		Kind:      Kind{Type: KindTypePermission, Name: "app.update.env.set"},
		StartTime: now,
		EndTime:   now.Add(10 * time.Second),
		Error:     "err x",
		Log:       "my log",
	}}
	evt.Init()
	err := evt.RawInsert(nil, nil, nil)
	c.Assert(err, check.IsNil)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	evt.ID = eventID{ObjId: evt.UniqueID}
	c.Assert(&evts[0], check.DeepEquals, evt)
}

func (s *S) TestNewWithPermission(c *check.C) {
	evt, err := New(&Opts{
		Target: Target{Type: "app", Value: "myapp"},
		Kind:   permission.PermAppUpdateEnvSet,
		Owner:  s.token,
		Allowed: Allowed(permission.PermAppReadEvents,
			permission.Context(permTypes.CtxApp, "myapp"), permission.Context(permTypes.CtxTeam, "myteam")),
	})
	c.Assert(err, check.IsNil)
	expected := &Event{eventData: eventData{
		ID:             eventID{Target: Target{Type: "app", Value: "myapp"}},
		UniqueID:       evt.UniqueID,
		Target:         Target{Type: "app", Value: "myapp"},
		Kind:           Kind{Type: KindTypePermission, Name: "app.update.env.set"},
		Owner:          Owner{Type: OwnerTypeUser, Name: s.token.GetUserName()},
		Running:        true,
		StartTime:      evt.StartTime,
		LockUpdateTime: evt.LockUpdateTime,
		Allowed: AllowedPermission{
			Scheme:   permission.PermAppReadEvents.FullName(),
			Contexts: []permTypes.PermissionContext{permission.Context(permTypes.CtxApp, "myapp"), permission.Context(permTypes.CtxTeam, "myteam")},
		},
	}}
	expected.Init()
	c.Assert(evt, check.DeepEquals, expected)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	evts[0].StartTime = expected.StartTime
	evts[0].LockUpdateTime = expected.LockUpdateTime
	c.Assert(&evts[0], check.DeepEquals, expected)
}

func (s *S) TestNewLockRetryRace(c *check.C) {
	originalMaxProcs := runtime.GOMAXPROCS(100)
	defer runtime.GOMAXPROCS(originalMaxProcs)
	wg := sync.WaitGroup{}
	var countOK int32
	for i := 0; i < 150; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			evt, err := New(&Opts{
				Target:  Target{Type: "app", Value: "myapp"},
				Kind:    permission.PermAppUpdateEnvSet,
				Owner:   s.token,
				Allowed: Allowed(permission.PermAppReadEvents),
			})
			if _, ok := err.(ErrEventLocked); ok {
				return
			}
			c.Assert(err, check.IsNil)
			atomic.AddInt32(&countOK, 1)
			err = evt.Done(nil)
			c.Assert(err, check.IsNil)
		}()
	}
	wg.Wait()
	c.Assert(countOK > 0, check.Equals, true)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, int(countOK))
}

func (s *S) TestNewCustomDataPtr(c *check.C) {
	customData := struct{ A string }{A: "value"}
	evt, err := New(&Opts{
		Target:     Target{Type: "app", Value: "myapp"},
		Kind:       permission.PermAppUpdateEnvSet,
		Owner:      s.token,
		CustomData: &customData,
		Allowed:    Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	c.Assert(evt.StartTime.IsZero(), check.Equals, false)
	c.Assert(evt.LockUpdateTime.IsZero(), check.Equals, false)
	var resultData struct{ A string }
	err = evt.StartData(&resultData)
	c.Assert(err, check.IsNil)
	c.Assert(resultData, check.DeepEquals, customData)
	expected := &Event{eventData: eventData{
		ID:              eventID{Target: Target{Type: "app", Value: "myapp"}},
		UniqueID:        evt.UniqueID,
		Target:          Target{Type: "app", Value: "myapp"},
		Kind:            Kind{Type: KindTypePermission, Name: "app.update.env.set"},
		Owner:           Owner{Type: OwnerTypeUser, Name: s.token.GetUserName()},
		Running:         true,
		StartTime:       evt.StartTime,
		LockUpdateTime:  evt.LockUpdateTime,
		StartCustomData: evt.StartCustomData,
		Allowed:         Allowed(permission.PermAppReadEvents),
	}}
	expected.Init()
	c.Assert(evt, check.DeepEquals, expected)
}

func (s *S) TestLoadThrottling(c *check.C) {
	defer config.Unset("event:throttling")
	err := loadThrottling()
	c.Assert(err, check.IsNil)
	c.Assert(throttlingInfo, check.DeepEquals, map[string]ThrottlingSpec{})
	err = config.ReadConfigBytes([]byte(`
event:
  throttling:
`))
	c.Assert(err, check.IsNil)
	setBaseConfig()
	err = loadThrottling()
	c.Assert(err, check.IsNil)
	c.Assert(throttlingInfo, check.DeepEquals, map[string]ThrottlingSpec{})
	err = config.ReadConfigBytes([]byte(`
event:
  throttling:
  - target-type: app
    kind-name: app.update.env.set
    limit: 1
    window: 300
    all-targets: true
    wait-finish: true
  - target-type: container
    kind-name: healer
    limit: 5
    window: 60
    all-targets: false
    wait-finish: false
`))
	c.Assert(err, check.IsNil)
	setBaseConfig()
	err = loadThrottling()
	c.Assert(err, check.IsNil)
	c.Assert(throttlingInfo, check.DeepEquals, map[string]ThrottlingSpec{
		"app_app.update.env.set_global": {
			TargetType: TargetTypeApp,
			KindName:   permission.PermAppUpdateEnvSet.FullName(),
			Time:       300 * time.Second,
			Max:        1,
			AllTargets: true,
			WaitFinish: true,
		},
		"container_healer": {
			TargetType: TargetTypeContainer,
			KindName:   "healer",
			Time:       time.Minute,
			Max:        5,
			AllTargets: false,
			WaitFinish: false,
		},
	})
}

func (s *S) TestLoadThrottlingInvalid(c *check.C) {
	defer config.Unset("event:throttling")
	err := loadThrottling()
	c.Assert(err, check.IsNil)
	c.Assert(throttlingInfo, check.DeepEquals, map[string]ThrottlingSpec{})
	err = config.ReadConfigBytes([]byte(`
event:
  throttling:
    a:
`))
	c.Assert(err, check.IsNil)
	setBaseConfig()
	err = loadThrottling()
	c.Assert(err, check.ErrorMatches, `json: cannot unmarshal object into Go value of type \[\]event.ThrottlingSpec`)
	c.Assert(throttlingInfo, check.DeepEquals, map[string]ThrottlingSpec{})
	err = config.ReadConfigBytes([]byte(`
event:
  throttling:
  - target-type: app
    kind-name: app.update.env.set
    limit: xxx
    window: 300
    all-targets: true
    wait-finish: true
`))
	c.Assert(err, check.IsNil)
	setBaseConfig()
	err = loadThrottling()
	c.Assert(err, check.ErrorMatches, `json: cannot unmarshal string into Go struct field throttlingSpecAlias.limit of type int`)
	c.Assert(throttlingInfo, check.DeepEquals, map[string]ThrottlingSpec{})
}

func (s *S) TestEventCancelableContext(c *check.C) {
	evt, err := New(&Opts{
		Target:        Target{Type: "app", Value: "myapp"},
		Kind:          permission.PermAppUpdateEnvSet,
		Owner:         s.token,
		Cancelable:    true,
		Allowed:       Allowed(permission.PermAppReadEvents),
		AllowedCancel: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	err = evts[0].TryCancel("because I want", "admin@admin.com")
	c.Assert(err, check.IsNil)
	c.Assert(evts[0].CancelInfo.StartTime.IsZero(), check.Equals, false)
	evts[0].CancelInfo.StartTime = time.Time{}
	c.Assert(evts[0].CancelInfo, check.DeepEquals, cancelInfo{
		Reason: "because I want",
		Owner:  "admin@admin.com",
		Asked:  true,
	})
	evts, err = All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].CancelInfo.StartTime.IsZero(), check.Equals, false)
	evts[0].CancelInfo.StartTime = time.Time{}
	c.Assert(evts[0].CancelInfo, check.DeepEquals, cancelInfo{
		Reason: "because I want",
		Owner:  "admin@admin.com",
		Asked:  true,
	})
	ctx, cancel := evt.CancelableContext(context.Background())
	defer cancel()
	<-ctx.Done()
	c.Assert(ctx.Err(), check.Equals, context.Canceled)
	c.Assert(evt.CancelInfo.StartTime.IsZero(), check.Equals, false)
	c.Assert(evt.CancelInfo.AckTime.IsZero(), check.Equals, false)
	evt.CancelInfo.StartTime = time.Time{}
	evt.CancelInfo.AckTime = time.Time{}
	c.Assert(evt.CancelInfo, check.DeepEquals, cancelInfo{
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

func (s *S) TestEventCancelableContextNotCancelable(c *check.C) {
	evt, err := New(&Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	evts, err := All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	nGoroutines := runtime.NumGoroutine()
	ctx, cancel := evt.CancelableContext(context.Background())
	c.Assert(runtime.NumGoroutine(), check.Equals, nGoroutines)
	defer cancel()
	select {
	case <-ctx.Done():
		c.Fatal("context should not be done")
	default:
	}
	c.Assert(ctx.Err(), check.IsNil)
}
