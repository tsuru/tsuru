// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/safe"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	eventTypes "github.com/tsuru/tsuru/types/event"
	permTypes "github.com/tsuru/tsuru/types/permission"
	trackerTypes "github.com/tsuru/tsuru/types/tracker"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
	check "gopkg.in/check.v1"
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

	storagev2.Reset()
}

func (s *S) SetUpTest(c *check.C) {
	defaultAppRetryTimeout = 200 * time.Millisecond
	setBaseConfig()
	throttlingInfo = map[string]ThrottlingSpec{}
	err := storagev2.ClearAllCollections(nil)
	c.Assert(err, check.IsNil)
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	user := &auth.User{Email: "me@me.com", Password: "123456"}
	_, err = nativeScheme.Create(context.TODO(), user)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(context.TODO(), map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	servicemock.SetMockService(&servicemock.MockService{})
}

func (s *S) TestNewDone(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	c.Assert(evt.StartTime.IsZero(), check.Equals, false)
	c.Assert(evt.LockUpdateTime.IsZero(), check.Equals, false)
	expected := &Event{EventData: eventTypes.EventData{
		ID:             evt.ID,
		UniqueID:       evt.ID,
		Target:         eventTypes.Target{Type: "app", Value: "myapp"},
		Lock:           &eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:           eventTypes.Kind{Type: eventTypes.KindTypePermission, Name: "app.update.env.set"},
		Owner:          eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.token.GetUserName()},
		Running:        true,
		StartTime:      evt.StartTime,
		LockUpdateTime: evt.LockUpdateTime,
		Allowed:        Allowed(permission.PermAppReadEvents),
		Instance:       evt.Instance,
	}}

	c.Assert(evt, check.DeepEquals, expected)
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].LockUpdateTime.IsZero(), check.Equals, false)
	evts[0].StartTime = expected.StartTime
	evts[0].LockUpdateTime = expected.LockUpdateTime
	evts[0].Instance = expected.Instance
	c.Assert(evts[0], check.DeepEquals, expected)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	evts, err = All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].LockUpdateTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	evts[0].EndTime = time.Time{}
	evts[0].StartTime = expected.StartTime
	evts[0].LockUpdateTime = expected.LockUpdateTime
	evts[0].Instance = expected.Instance
	expected.Running = false
	expected.ID = evts[0].ID
	expected.Lock = nil
	c.Assert(evts[0], check.DeepEquals, expected)
}

func (s *S) TestNewExpirable(c *check.C) {
	expireAt := time.Now().UTC().Add(10 * time.Minute)
	evt, err := New(context.TODO(), &Opts{
		Target:   eventTypes.Target{Type: "job", Value: "myjob"},
		Kind:     permission.PermJobRun,
		Owner:    s.token,
		Allowed:  Allowed(permission.PermJobRun),
		ExpireAt: &expireAt,
	})
	c.Assert(err, check.IsNil)
	c.Assert(evt.ExpireAt, check.NotNil)
	c.Assert(evt.ExpireAt.IsZero(), check.Equals, false)
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].ExpireAt.IsZero(), check.Equals, false)
}

func (s *S) TestNewExpirableMissingShouldNotCreateTimestampInDB(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "job", Value: "myjob"},
		Kind:    permission.PermJobRun,
		Owner:   s.token,
		Allowed: Allowed(permission.PermJobRun),
	})
	c.Assert(err, check.IsNil)
	c.Assert(evt.ExpireAt.IsZero(), check.Equals, true)
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].ExpireAt.IsZero(), check.Equals, true)
}

func (s *S) TestNewCustomDataDone(c *check.C) {
	customData := struct{ A string }{A: "value"}
	evt, err := New(context.TODO(), &Opts{
		Target:     eventTypes.Target{Type: "app", Value: "myapp"},
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
	expected := &Event{EventData: eventTypes.EventData{
		ID:              evt.UniqueID,
		UniqueID:        evt.UniqueID,
		Target:          eventTypes.Target{Type: "app", Value: "myapp"},
		Lock:            &eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:            eventTypes.Kind{Type: eventTypes.KindTypePermission, Name: "app.update.env.set"},
		Owner:           eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.token.GetUserName()},
		Running:         true,
		StartTime:       evt.StartTime,
		LockUpdateTime:  evt.LockUpdateTime,
		StartCustomData: evt.StartCustomData,
		Allowed:         Allowed(permission.PermAppReadEvents),
		Instance:        evt.Instance,
	}}

	c.Assert(evt, check.DeepEquals, expected)
	customData = struct{ A string }{A: "other"}
	err = evt.DoneCustomData(context.TODO(), nil, customData)
	c.Assert(err, check.IsNil)
	evts, err := All(context.TODO())
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
	expected.ID = evts[0].UniqueID
	expected.EndCustomData = evts[0].EndCustomData
	expected.Lock = nil
	expected.Instance = evts[0].Instance
	c.Assert(evts[0], check.DeepEquals, expected)
}

func (s *S) TestNewLocks(c *check.C) {
	_, err := New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	_, err = New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvUnset,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.ErrorMatches, `event locked: app\(myapp\) running "app.update.env.set" start by user me@me.com at .+`)
}

func (s *S) TestNewExtraTargetLocks(c *check.C) {
	tests := []struct {
		target1      eventTypes.Target
		extras1      []eventTypes.ExtraTarget
		target2      eventTypes.Target
		extras2      []eventTypes.ExtraTarget
		err          string
		disableLock1 bool
		disableLock2 bool
	}{
		{
			target1: eventTypes.Target{Type: "app", Value: "myapp"},
			target2: eventTypes.Target{Type: "container", Value: "x"},
			extras2: []eventTypes.ExtraTarget{{Target: eventTypes.Target{Type: "app", Value: "myapp"}, Lock: true}},
			err:     `event locked: app\(myapp\) running "app.update.env.set" start by user me@me.com at .+`,
		},
		{
			target1: eventTypes.Target{Type: "app", Value: "myapp"},
			extras1: []eventTypes.ExtraTarget{{Target: eventTypes.Target{Type: "app", Value: "myapp2"}, Lock: true}},
			target2: eventTypes.Target{Type: "app", Value: "myapp2"},
			err:     `event locked: app\(myapp\) running "app.update.env.set" start by user me@me.com at .+`,
		},
		{
			target1: eventTypes.Target{Type: "app", Value: "myapp"},
			target2: eventTypes.Target{Type: "container", Value: "x"},
			extras2: []eventTypes.ExtraTarget{{Target: eventTypes.Target{Type: "app", Value: "myapp"}, Lock: false}},
			err:     "",
		},
		{
			target1: eventTypes.Target{Type: "app", Value: "myapp"},
			extras1: []eventTypes.ExtraTarget{{Target: eventTypes.Target{Type: "app", Value: "myapp2"}, Lock: false}},
			target2: eventTypes.Target{Type: "app", Value: "myapp2"},
			err:     "",
		},
		{
			target1:      eventTypes.Target{Type: "app", Value: "myapp"},
			extras1:      []eventTypes.ExtraTarget{{Target: eventTypes.Target{Type: "app", Value: "myapp2"}, Lock: true}},
			target2:      eventTypes.Target{Type: "app", Value: "myapp2"},
			disableLock2: true,
			err:          "",
		},
		{
			target1:      eventTypes.Target{Type: "app", Value: "myapp"},
			extras1:      []eventTypes.ExtraTarget{{Target: eventTypes.Target{Type: "app", Value: "myapp2"}, Lock: true}},
			target2:      eventTypes.Target{Type: "app", Value: "myapp2"},
			disableLock2: true,
			err:          "",
		},
	}
	for i, tt := range tests {
		evt1, err := New(context.TODO(), &Opts{
			Target:       tt.target1,
			ExtraTargets: tt.extras1,
			Kind:         permission.PermAppUpdateEnvSet,
			Owner:        s.token,
			Allowed:      Allowed(permission.PermAppReadEvents),
			DisableLock:  tt.disableLock1,
		})
		c.Assert(err, check.IsNil)
		evt2, err := New(context.TODO(), &Opts{
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
		err = evt1.Done(context.TODO(), nil)
		c.Assert(err, check.IsNil)
		if evt2 != nil {
			err = evt2.Done(context.TODO(), nil)
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
			_, err := New(context.TODO(), &Opts{
				Target: eventTypes.Target{Type: "app", Value: fmt.Sprintf("myapp-%d", i)},
				ExtraTargets: []eventTypes.ExtraTarget{
					{Target: eventTypes.Target{Type: "app", Value: "myapp"}, Lock: true},
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
	evt, err := New(context.TODO(), &Opts{
		Target:      eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:        permission.PermAppUpdateEnvSet,
		Owner:       s.token,
		DisableLock: true,
		Allowed:     Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	c.Assert(evt.StartTime.IsZero(), check.Equals, false)
	c.Assert(evt.LockUpdateTime.IsZero(), check.Equals, false)
	expected := &Event{EventData: eventTypes.EventData{
		ID:             evt.UniqueID,
		UniqueID:       evt.UniqueID,
		Target:         eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:           eventTypes.Kind{Type: eventTypes.KindTypePermission, Name: "app.update.env.set"},
		Owner:          eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.token.GetUserName()},
		Running:        true,
		StartTime:      evt.StartTime,
		LockUpdateTime: evt.LockUpdateTime,
		Allowed:        Allowed(permission.PermAppReadEvents),
		Instance:       evt.Instance,
	}}

	c.Assert(evt, check.DeepEquals, expected)
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].LockUpdateTime.IsZero(), check.Equals, false)
	evts[0].StartTime = expected.StartTime
	evts[0].LockUpdateTime = expected.LockUpdateTime
	evts[0].Instance = expected.Instance
	c.Assert(evts[0], check.DeepEquals, expected)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	evts, err = All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].LockUpdateTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	evts[0].EndTime = time.Time{}
	evts[0].StartTime = expected.StartTime
	evts[0].LockUpdateTime = expected.LockUpdateTime
	evts[0].Instance = expected.Instance
	expected.Running = false
	expected.ID = evts[0].UniqueID
	c.Assert(evts[0], check.DeepEquals, expected)
}

func (s *S) TestNewLockExpired(c *check.C) {
	oldLockExpire := lockExpireTimeout
	lockExpireTimeout = time.Millisecond
	defer func() {
		lockExpireTimeout = oldLockExpire
	}()
	_, err := New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	updater.stop()
	time.Sleep(100 * time.Millisecond)
	_, err = New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvUnset,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 2)
	c.Assert(evts[0].Kind.Name, check.Equals, "app.update.env.unset")
	c.Assert(evts[1].Kind.Name, check.Equals, "app.update.env.set")
	c.Assert(evts[0].Running, check.Equals, true)
	c.Assert(evts[1].Running, check.Equals, false)
	c.Assert(evts[0].Error, check.Equals, "")
	c.Assert(evts[1].Error, check.Matches, `event expired, no update for [\d.]+\w+`)
}

func (s *S) TestNewLockRetry(c *check.C) {
	evt1, err := New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	doneCh := make(chan struct{})
	go func() {
		_, lockErr := New(context.TODO(), &Opts{
			Target:       eventTypes.Target{Type: "app", Value: "myapp"},
			Kind:         permission.PermAppUpdateEnvUnset,
			Owner:        s.token,
			Allowed:      Allowed(permission.PermAppReadEvents),
			RetryTimeout: 5 * time.Second,
		})
		c.Assert(lockErr, check.IsNil)
		close(doneCh)
	}()
	time.Sleep(500 * time.Millisecond)
	evt1.Done(context.TODO(), nil)
	select {
	case <-doneCh:
	case <-time.After(10 * time.Second):
		c.Fatal("timeout waiting for event to be created")
	}
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 2)
	c.Assert(evts[0].Kind.Name, check.Equals, "app.update.env.unset")
	c.Assert(evts[1].Kind.Name, check.Equals, "app.update.env.set")
	c.Assert(evts[0].Running, check.Equals, true)
	c.Assert(evts[1].Running, check.Equals, false)
	c.Assert(evts[0].Error, check.Equals, "")
	c.Assert(evts[1].Error, check.Equals, "")
}

func (s *S) TestNewLockRetryError(c *check.C) {
	_, err := New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	_, err = New(context.TODO(), &Opts{
		Target:       eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:         permission.PermAppUpdateEnvUnset,
		Owner:        s.token,
		Allowed:      Allowed(permission.PermAppReadEvents),
		RetryTimeout: time.Second,
	})
	c.Assert(err, check.ErrorMatches, `event locked: app\(myapp\) running "app.update.env.set".*`)
}

func (s *S) TestNewEventBlocked(c *check.C) {
	err := AddBlock(context.TODO(), &Block{KindName: "app.deploy", Reason: "you shall not pass"})
	c.Assert(err, check.IsNil)
	blocks, err := listBlocks(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	_, err = New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.NotNil)
	c.Assert(err.(ErrEventBlocked).block, check.DeepEquals, &blocks[0])
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Running, check.Equals, false)
	c.Assert(evts[0].Error, check.Matches, `.*block app.deploy by all users on all targets: you shall not pass$`)
}

func (s *S) TestEventAbort(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.Abort(context.TODO())
	c.Assert(err, check.IsNil)
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
}

func (s *S) TestEventDoneError(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.Done(context.TODO(), errors.New("myerr"))
	c.Assert(err, check.IsNil)
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].LockUpdateTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].Instance.Name, check.Not(check.Equals), "")
	expected := &Event{EventData: eventTypes.EventData{
		ID:             evts[0].UniqueID,
		UniqueID:       evts[0].UniqueID,
		Target:         eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:           eventTypes.Kind{Type: eventTypes.KindTypePermission, Name: "app.update.env.set"},
		Owner:          eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.token.GetUserName()},
		StartTime:      evts[0].StartTime,
		LockUpdateTime: evts[0].LockUpdateTime,
		EndTime:        evts[0].EndTime,
		Error:          "myerr",
		Allowed:        Allowed(permission.PermAppReadEvents),
		Instance:       evts[0].Instance,
	}}

	c.Assert(evts[0], check.DeepEquals, expected)
}

func (s *S) TestEventLogf(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	evt.Logf("%s %d", "hey", 42)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Log(), check.Matches, `(?s)\d{4}-\d{2}-\d{2}.*: hey 42`+"\n")
	asJson, err := json.Marshal(evts[0])
	c.Assert(err, check.IsNil)
	c.Assert(string(asJson), check.Matches, `(?s).*"Log":"\d{4}-\d{2}-\d{2}.*: hey 42\\n".*`)
}

func (s *S) TestEventLogfWithWriter(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	buf := bytes.Buffer{}
	evt.SetLogWriter(&buf)
	evt.Logf("%s %d", "hey", 42)
	c.Assert(buf.String(), check.Matches, `hey 42`+"\n")
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Log(), check.Matches, `(?s)\d{4}-\d{2}-\d{2}.*: hey 42`+"\n")
}

func (s *S) TestEventCancel(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:        eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:          permission.PermAppUpdateEnvSet,
		Owner:         s.token,
		Cancelable:    true,
		Allowed:       Allowed(permission.PermAppReadEvents),
		AllowedCancel: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	err = evts[0].TryCancel(context.TODO(), "because I want", "admin@admin.com")
	c.Assert(err, check.IsNil)
	c.Assert(evts[0].CancelInfo.StartTime.IsZero(), check.Equals, false)
	evts[0].CancelInfo.StartTime = time.Time{}
	c.Assert(evts[0].CancelInfo, check.DeepEquals, eventTypes.CancelInfo{
		Reason: "because I want",
		Owner:  "admin@admin.com",
		Asked:  true,
	})
	evts, err = All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].CancelInfo.StartTime.IsZero(), check.Equals, false)
	evts[0].CancelInfo.StartTime = time.Time{}
	c.Assert(evts[0].CancelInfo, check.DeepEquals, eventTypes.CancelInfo{
		Reason: "because I want",
		Owner:  "admin@admin.com",
		Asked:  true,
	})
	canceled, err := evt.AckCancel(context.TODO())
	c.Assert(canceled, check.Equals, true)
	c.Assert(err, check.IsNil)
	c.Assert(evt.CancelInfo.StartTime.IsZero(), check.Equals, false)
	c.Assert(evt.CancelInfo.AckTime.IsZero(), check.Equals, false)
	evt.CancelInfo.StartTime = time.Time{}
	evt.CancelInfo.AckTime = time.Time{}
	c.Assert(evt.CancelInfo, check.DeepEquals, eventTypes.CancelInfo{
		Reason:   "because I want",
		Owner:    "admin@admin.com",
		Asked:    true,
		Canceled: true,
	})
	evts, err = All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].CancelInfo.StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].CancelInfo.AckTime.IsZero(), check.Equals, false)
	evts[0].CancelInfo.StartTime = time.Time{}
	evts[0].CancelInfo.AckTime = time.Time{}
	c.Assert(evts[0].CancelInfo, check.DeepEquals, eventTypes.CancelInfo{
		Reason:   "because I want",
		Owner:    "admin@admin.com",
		Asked:    true,
		Canceled: true,
	})
}

func (s *S) TestEventCancelMultipleTimes(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:        eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:          permission.PermAppUpdateEnvSet,
		Owner:         s.token,
		Cancelable:    true,
		Allowed:       Allowed(permission.PermAppReadEvents),
		AllowedCancel: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.TryCancel(context.TODO(), "because I want", "admin@admin.com")
	c.Assert(err, check.IsNil)
	err = evt.TryCancel(context.TODO(), "because I still want", "admin@admin.com")
	c.Assert(err, check.DeepEquals, ErrCancelAlreadyRequested)
}

func (s *S) TestEventCancelNotCancelable(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.TryCancel(context.TODO(), "yes", "admin@admin.com")
	c.Assert(err, check.Equals, ErrNotCancelable)
	canceled, err := evt.AckCancel(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(canceled, check.Equals, false)
}

func (s *S) TestEventCancelNotAsked(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:        eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:          permission.PermAppUpdateEnvSet,
		Owner:         s.token,
		Cancelable:    true,
		Allowed:       Allowed(permission.PermAppReadEvents),
		AllowedCancel: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	canceled, err := evt.AckCancel(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(canceled, check.Equals, false)
}

func (s *S) TestEventCancelNotRunning(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:        eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:          permission.PermAppUpdateEnvSet,
		Owner:         s.token,
		Cancelable:    true,
		Allowed:       Allowed(permission.PermAppReadEvents),
		AllowedCancel: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	err = evt.TryCancel(context.TODO(), "yes", "admin@admin.com")
	c.Assert(err, check.Equals, ErrNotCancelable)
	canceled, err := evt.AckCancel(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(canceled, check.Equals, false)
}

func (s *S) TestEventCancelDoneNoError(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:        eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:          permission.PermAppUpdateEnvSet,
		Owner:         s.token,
		Cancelable:    true,
		Allowed:       Allowed(permission.PermAppReadEvents),
		AllowedCancel: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.TryCancel(context.TODO(), "yes", "admin@admin.com")
	c.Assert(err, check.IsNil)
	canceled, err := evt.AckCancel(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(canceled, check.Equals, true)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Error, check.Equals, "canceled by user request")
}

func (s *S) TestEventCancelDoneWithCanceledContext(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:        eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:          permission.PermAppUpdateEnvSet,
		Owner:         s.token,
		Cancelable:    true,
		Allowed:       Allowed(permission.PermAppReadEvents),
		AllowedCancel: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err = evt.Done(canceledCtx, nil)
	c.Assert(err, check.IsNil)
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].EndTime, check.Not(check.IsNil))
}

func (s *S) TestEventCancelDoneCustomError(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:        eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:          permission.PermAppUpdateEnvSet,
		Owner:         s.token,
		Cancelable:    true,
		Allowed:       Allowed(permission.PermAppReadEvents),
		AllowedCancel: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.TryCancel(context.TODO(), "yes", "admin@admin.com")
	c.Assert(err, check.IsNil)
	canceled, err := evt.AckCancel(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(canceled, check.Equals, true)
	err = evt.Done(context.TODO(), errors.New("my err"))
	c.Assert(err, check.IsNil)
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Error, check.Equals, "my err")
}

func (s *S) TestEventNewValidation(c *check.C) {
	_, err := New(context.TODO(), nil)
	c.Assert(err, check.Equals, ErrNoOpts)
	_, err = New(context.TODO(), &Opts{Kind: permission.PermAppCreate, Owner: s.token})
	c.Assert(err, check.Equals, ErrNoTarget)
	_, err = New(context.TODO(), &Opts{Target: eventTypes.Target{Type: "app", Value: "myapp"}, Owner: s.token})
	c.Assert(err, check.Equals, ErrNoKind)
	_, err = New(context.TODO(), &Opts{Target: eventTypes.Target{Type: "app", Value: "myapp"}, Kind: permission.PermAppCreate})
	c.Assert(err, check.Equals, ErrNoOwner)
}

func (s *S) TestEventDoneLogError(c *check.C) {
	logBuf := safe.NewBuffer(nil)
	log.SetLogger(log.NewWriterLogger(logBuf, false))
	defer log.SetLogger(nil)
	evt, err := New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	config.Set("database:url", "127.0.0.1:9999")
	storagev2.Reset()
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.ErrorMatches, ".*connection refused.*")
	c.Assert(logBuf.String(), check.Matches, `(?s).*connection refused.*`)
}

func (s *S) TestNewThrottledAllKinds(c *check.C) {
	SetThrottling(ThrottlingSpec{
		TargetType: eventTypes.TargetTypeApp,
		Time:       time.Hour,
		Max:        2,
	})
	evt, err := New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	evt, err = New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	_, err = New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.FitsTypeOf, ErrThrottled{})
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app \"myapp\" is 2 every 1h0m0s")
	_, err = New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvUnset,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.FitsTypeOf, ErrThrottled{})
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app \"myapp\" is 2 every 1h0m0s")
	evt, err = New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "otherapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
}

func (s *S) TestNewThrottledOneKind(c *check.C) {
	SetThrottling(ThrottlingSpec{
		TargetType: eventTypes.TargetTypeApp,
		KindName:   permission.PermAppUpdateEnvSet.FullName(),
		Time:       time.Hour,
		Max:        2,
	})
	evt, err := New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	evt, err = New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	_, err = New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.FitsTypeOf, ErrThrottled{})
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app.update.env.set on app \"myapp\" is 2 every 1h0m0s")
	// A different target value is not throttled
	_, err = New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp2"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	// A different kind is not throttled
	evt, err = New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvUnset,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
}

func (s *S) TestNewThrottledAllTargets(c *check.C) {
	SetThrottling(ThrottlingSpec{
		TargetType: eventTypes.TargetTypeApp,
		KindName:   permission.PermAppUpdateEnvSet.FullName(),
		Time:       time.Hour,
		Max:        1,
		AllTargets: true,
	})
	baseOpts := &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	}
	evt, err := New(context.TODO(), baseOpts)
	c.Assert(err, check.IsNil)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	_, err = New(context.TODO(), baseOpts)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app.update.env.set on any app is 1 every 1h0m0s")
	baseOpts.Target.Value = "myapp2"
	_, err = New(context.TODO(), baseOpts)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app.update.env.set on any app is 1 every 1h0m0s")
}

func (s *S) TestNewThrottledAllTargetsTwoRules(c *check.C) {
	SetThrottling(ThrottlingSpec{
		TargetType: eventTypes.TargetTypeApp,
		KindName:   permission.PermAppUpdateEnvSet.FullName(),
		Time:       time.Hour,
		Max:        2,
		AllTargets: true,
	})
	SetThrottling(ThrottlingSpec{
		TargetType: eventTypes.TargetTypeApp,
		KindName:   permission.PermAppUpdateEnvSet.FullName(),
		Time:       time.Hour,
		Max:        1,
	})
	baseOpts := &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	}
	evt, err := New(context.TODO(), baseOpts)
	c.Assert(err, check.IsNil)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	_, err = New(context.TODO(), baseOpts)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app.update.env.set on app \"myapp\" is 1 every 1h0m0s")
	app2Opts := &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp2"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	}
	_, err = New(context.TODO(), app2Opts)
	c.Assert(err, check.IsNil)
	app3Opts := &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp3"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	}
	_, err = New(context.TODO(), app3Opts)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app.update.env.set on any app is 2 every 1h0m0s")
}

func (s *S) TestNewThrottledExpiration(c *check.C) {
	SetThrottling(ThrottlingSpec{
		TargetType: eventTypes.TargetTypeApp,
		KindName:   permission.PermAppUpdateEnvSet.FullName(),
		Time:       300 * time.Millisecond,
		Max:        1,
		AllTargets: true,
	})
	baseOpts := &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	}
	_, err := New(context.TODO(), baseOpts)
	c.Assert(err, check.IsNil)
	otherOpts := &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp2"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	}
	_, err = New(context.TODO(), otherOpts)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app.update.env.set on any app is 1 every 300ms")
	time.Sleep(400 * time.Millisecond)
	_, err = New(context.TODO(), otherOpts)
	c.Assert(err, check.IsNil)
}

func (s *S) TestNewThrottledExpirationWaitFinish(c *check.C) {
	SetThrottling(ThrottlingSpec{
		TargetType: eventTypes.TargetTypeApp,
		KindName:   permission.PermAppUpdateEnvSet.FullName(),
		Time:       300 * time.Millisecond,
		Max:        1,
		AllTargets: true,
		WaitFinish: true,
	})
	baseOpts := &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	}
	evt, err := New(context.TODO(), baseOpts)
	c.Assert(err, check.IsNil)
	otherOpts := &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp2"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	}
	_, err = New(context.TODO(), otherOpts)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app.update.env.set on any app is 1 every 300ms")
	time.Sleep(400 * time.Millisecond)
	_, err = New(context.TODO(), otherOpts)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app.update.env.set on any app is 1 every 300ms")
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	_, err = New(context.TODO(), otherOpts)
	c.Assert(err, check.IsNil)
}

func (s *S) TestNewThrottledExpirationWaitFinishExpired(c *check.C) {
	oldLockExpire := lockExpireTimeout
	lockExpireTimeout = 200 * time.Millisecond
	defer func() {
		lockExpireTimeout = oldLockExpire
	}()
	SetThrottling(ThrottlingSpec{
		TargetType: eventTypes.TargetTypeApp,
		KindName:   permission.PermAppUpdateEnvSet.FullName(),
		Time:       300 * time.Millisecond,
		Max:        1,
		AllTargets: true,
		WaitFinish: true,
	})
	baseOpts := &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	}
	_, err := New(context.TODO(), baseOpts)
	c.Assert(err, check.IsNil)
	updater.stop()
	otherOpts := &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp2"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	}
	_, err = New(context.TODO(), otherOpts)
	c.Assert(err, check.NotNil)
	updater.stop()
	c.Assert(err, check.ErrorMatches, "event throttled, limit for app.update.env.set on any app is 1 every 300ms")
	time.Sleep(500 * time.Millisecond)
	_, err = New(context.TODO(), otherOpts)
	c.Check(err, check.IsNil)
}

func (s *S) TestListWithFilters(c *check.C) {
	e1, err := New(context.TODO(), &Opts{Owner: s.token, Kind: permission.PermAll, Allowed: Allowed(permission.PermApp), Target: eventTypes.Target{Type: "node"}})
	c.Assert(err, check.IsNil)
	err = e1.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	e2, err := New(context.TODO(), &Opts{Owner: s.token, Kind: permission.PermAll, Allowed: Allowed(permission.PermApp), Target: eventTypes.Target{Type: "container"}})
	c.Assert(err, check.IsNil)
	err = e2.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	e3, err := New(context.TODO(), &Opts{Owner: s.token, Kind: permission.PermAppCreate, Allowed: Allowed(permission.PermApp), Target: eventTypes.Target{Type: "container", Value: "1234"}})
	c.Assert(err, check.IsNil)
	err = e3.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)

	evts, err := List(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 3)

	evts, err = List(context.TODO(), &Filter{Target: eventTypes.Target{Type: "container"}})
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 2)
	c.Assert(evts[0].Target.Type, check.Equals, eventTypes.TargetType("container"))
	c.Assert(evts[1].Target.Type, check.Equals, eventTypes.TargetType("container"))

	evts, err = List(context.TODO(), &Filter{Target: eventTypes.Target{Type: "container", Value: "1234"}})
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Target.Type, check.Equals, eventTypes.TargetType("container"))
	c.Assert(evts[0].Target.Value, check.Equals, "1234")

	evts, err = List(context.TODO(), &Filter{Target: eventTypes.Target{Type: "container", Value: "unknown"}})
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)

	evts, err = List(context.TODO(), &Filter{Target: eventTypes.Target{Type: "node"}, Since: time.Now().Add(time.Duration(-1 * time.Hour))})
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Target.Type, check.Equals, eventTypes.TargetType("node"))
}

func (s *S) TestListFilterPruneUserValues(c *check.C) {
	t := true
	f := Filter{
		Target:         eventTypes.Target{Type: "app", Value: "myapp"},
		KindType:       eventTypes.KindTypePermission,
		KindNames:      []string{"a"},
		OwnerType:      eventTypes.OwnerTypeUser,
		OwnerName:      "u",
		Since:          time.Now(),
		Until:          time.Now(),
		Running:        &t,
		Raw:            mongoBSON.M{"a": 1},
		AllowedTargets: []TargetFilter{{Type: eventTypes.TargetTypeApp, Values: []string{"a1"}}},
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
	_, err := New(context.TODO(), &Opts{
		Target:     eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:       permission.PermAppUpdateEnvSet,
		Owner:      s.token,
		CustomData: map[string]string{"x": "y"},
		Allowed:    Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	getEvt, err := GetRunning(context.TODO(), eventTypes.Target{Type: "app", Value: "myapp"}, permission.PermAppUpdateEnvSet.FullName())
	c.Assert(err, check.IsNil)
	err = getEvt.SetOtherCustomData(context.TODO(), map[string]string{"z": "h"})
	c.Assert(err, check.IsNil)
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Owner, check.DeepEquals, eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.token.GetUserName()})
	data := map[string]string{}
	err = evts[0].StartData(&data)
	c.Assert(err, check.IsNil)
	c.Assert(data, check.DeepEquals, map[string]string{"x": "y"})

	data = map[string]string{}
	err = evts[0].OtherData(&data)
	c.Assert(err, check.IsNil)
	c.Assert(data, check.DeepEquals, map[string]string{"z": "h"})
}

func (s *S) TestEventAsWriter(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:     eventTypes.Target{Type: "app", Value: "myapp"},
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
	c.Assert(evt.StructuredLog, check.HasLen, 1)
	c.Assert(evt.StructuredLog[0].Message, check.Equals, "hey")
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	evt2, err := New(context.TODO(), &Opts{
		Target:     eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:       permission.PermAppUpdateEnvSet,
		Owner:      s.token,
		CustomData: map[string]string{"x": "y"},
		Allowed:    Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	var otherWriter bytes.Buffer
	evt2.SetLogWriter(&otherWriter)
	evt2.Write([]byte("hey2"))
	c.Assert(evt2.StructuredLog, check.HasLen, 1)
	c.Assert(evt2.StructuredLog[0].Message, check.Equals, "hey2")
	err = evt2.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
}

func (s *S) TestGetTargetType(c *check.C) {
	var tests = []struct {
		input  string
		output eventTypes.TargetType
		err    error
	}{
		{"app", eventTypes.TargetTypeApp, nil},
		{"node", eventTypes.TargetTypeNode, nil},
		{"container", eventTypes.TargetTypeContainer, nil},
		{"pool", eventTypes.TargetTypePool, nil},
		{"service", eventTypes.TargetTypeService, nil},
		{"service-instance", eventTypes.TargetTypeServiceInstance, nil},
		{"team", eventTypes.TargetTypeTeam, nil},
		{"user", eventTypes.TargetTypeUser, nil},
		{"invalid", "", eventTypes.ErrInvalidTargetType},
	}
	for _, t := range tests {
		got, err := eventTypes.GetTargetType(t.input)
		c.Check(got, check.Equals, t.output)
		c.Check(err, check.Equals, t.err)
	}
}

func (s *S) TestEventRawInsert(c *check.C) {
	now := time.Unix(time.Now().Unix(), 0).UTC()
	id := primitive.NewObjectID()
	evt := &Event{EventData: eventTypes.EventData{
		ID:        id,
		UniqueID:  id,
		Target:    eventTypes.Target{Type: "app", Value: "myapp"},
		Owner:     eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.token.GetUserName()},
		Kind:      eventTypes.Kind{Type: eventTypes.KindTypePermission, Name: "app.update.env.set"},
		StartTime: now,
		EndTime:   now.Add(10 * time.Second),
		Error:     "err x",
		Log:       "my log",
		Instance:  trackerTypes.TrackedInstance{Addresses: []string{}},
	}}

	err := evt.RawInsert(context.TODO(), nil, nil, nil)
	c.Assert(err, check.IsNil)
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0], check.DeepEquals, evt)
}

func (s *S) TestNewWithPermission(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:     eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:       permission.PermAppUpdateEnvSet,
		Owner:      s.token,
		RemoteAddr: "1.1.1.1:30031",
		Allowed: Allowed(permission.PermAppReadEvents,
			permission.Context(permTypes.CtxApp, "myapp"), permission.Context(permTypes.CtxTeam, "myteam")),
	})
	c.Assert(err, check.IsNil)
	expected := &Event{EventData: eventTypes.EventData{
		ID:             evt.UniqueID,
		UniqueID:       evt.UniqueID,
		Target:         eventTypes.Target{Type: "app", Value: "myapp"},
		Lock:           &eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:           eventTypes.Kind{Type: eventTypes.KindTypePermission, Name: "app.update.env.set"},
		Owner:          eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.token.GetUserName()},
		Running:        true,
		SourceIP:       "1.1.1.1",
		StartTime:      evt.StartTime,
		LockUpdateTime: evt.LockUpdateTime,
		Allowed: eventTypes.AllowedPermission{
			Scheme:   permission.PermAppReadEvents.FullName(),
			Contexts: []permTypes.PermissionContext{permission.Context(permTypes.CtxApp, "myapp"), permission.Context(permTypes.CtxTeam, "myteam")},
		},
		Instance: evt.Instance,
	}}

	c.Assert(evt, check.DeepEquals, expected)
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	evts[0].StartTime = expected.StartTime
	evts[0].LockUpdateTime = expected.LockUpdateTime
	evts[0].Instance = expected.Instance
	c.Assert(evts[0], check.DeepEquals, expected)
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
			evt, err := New(context.TODO(), &Opts{
				Target:  eventTypes.Target{Type: "app", Value: "myapp"},
				Kind:    permission.PermAppUpdateEnvSet,
				Owner:   s.token,
				Allowed: Allowed(permission.PermAppReadEvents),
			})
			if _, ok := err.(ErrEventLocked); ok {
				return
			}
			c.Assert(err, check.IsNil)
			atomic.AddInt32(&countOK, 1)
			err = evt.Done(context.TODO(), nil)
			c.Assert(err, check.IsNil)
		}()
	}
	wg.Wait()
	c.Assert(countOK > 0, check.Equals, true)
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, int(countOK))
}

func (s *S) TestNewCustomDataPtr(c *check.C) {
	customData := struct{ A string }{A: "value"}
	evt, err := New(context.TODO(), &Opts{
		Target:     eventTypes.Target{Type: "app", Value: "myapp"},
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
	expected := &Event{EventData: eventTypes.EventData{
		ID:              evt.UniqueID,
		UniqueID:        evt.UniqueID,
		Target:          eventTypes.Target{Type: "app", Value: "myapp"},
		Lock:            &eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:            eventTypes.Kind{Type: eventTypes.KindTypePermission, Name: "app.update.env.set"},
		Owner:           eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.token.GetUserName()},
		Running:         true,
		StartTime:       evt.StartTime,
		LockUpdateTime:  evt.LockUpdateTime,
		StartCustomData: evt.StartCustomData,
		Allowed:         Allowed(permission.PermAppReadEvents),
		Instance:        evt.Instance,
	}}

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
			TargetType: eventTypes.TargetTypeApp,
			KindName:   permission.PermAppUpdateEnvSet.FullName(),
			Time:       300 * time.Second,
			Max:        1,
			AllTargets: true,
			WaitFinish: true,
		},
		"container_healer": {
			TargetType: eventTypes.TargetTypeContainer,
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
	evt, err := New(context.TODO(), &Opts{
		Target:        eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:          permission.PermAppUpdateEnvSet,
		Owner:         s.token,
		Cancelable:    true,
		Allowed:       Allowed(permission.PermAppReadEvents),
		AllowedCancel: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	err = evts[0].TryCancel(context.TODO(), "because I want", "admin@admin.com")
	c.Assert(err, check.IsNil)
	c.Assert(evts[0].CancelInfo.StartTime.IsZero(), check.Equals, false)
	evts[0].CancelInfo.StartTime = time.Time{}
	c.Assert(evts[0].CancelInfo, check.DeepEquals, eventTypes.CancelInfo{
		Reason: "because I want",
		Owner:  "admin@admin.com",
		Asked:  true,
	})
	evts, err = All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].CancelInfo.StartTime.IsZero(), check.Equals, false)
	evts[0].CancelInfo.StartTime = time.Time{}
	c.Assert(evts[0].CancelInfo, check.DeepEquals, eventTypes.CancelInfo{
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
	c.Assert(evt.CancelInfo, check.DeepEquals, eventTypes.CancelInfo{
		Reason:   "because I want",
		Owner:    "admin@admin.com",
		Asked:    true,
		Canceled: true,
	})
	evts, err = All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].CancelInfo.StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].CancelInfo.AckTime.IsZero(), check.Equals, false)
	evts[0].CancelInfo.StartTime = time.Time{}
	evts[0].CancelInfo.AckTime = time.Time{}
	c.Assert(evts[0].CancelInfo, check.DeepEquals, eventTypes.CancelInfo{
		Reason:   "because I want",
		Owner:    "admin@admin.com",
		Asked:    true,
		Canceled: true,
	})
}

func (s *S) TestEventCancelableContextNotCancelable(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:  eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	evts, err := All(context.TODO())
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

func (s *S) TestEventCancelableContextBaseCanceled(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:        eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:          permission.PermAppUpdateEnvSet,
		Owner:         s.token,
		Allowed:       Allowed(permission.PermAppReadEvents),
		AllowedCancel: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)

	cancelCtx, cancel := context.WithCancel(context.Background())
	_, evtCancel := evt.CancelableContext(cancelCtx)
	evtCancel()
	cancel()

	err = evt.Done(context.TODO(), cancelCtx.Err())
	c.Assert(err, check.IsNil)

	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].CancelInfo.StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].CancelInfo.AckTime.IsZero(), check.Equals, false)
	evts[0].CancelInfo.StartTime = time.Time{}
	evts[0].CancelInfo.AckTime = time.Time{}
	c.Assert(evts[0].CancelInfo, check.DeepEquals, eventTypes.CancelInfo{
		Reason:   "context canceled",
		Owner:    "user me@me.com",
		Canceled: true,
	})
}

func (s *S) TestEventCancelableContextNotCanceled(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:        eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:          permission.PermAppUpdateEnvSet,
		Owner:         s.token,
		Allowed:       Allowed(permission.PermAppReadEvents),
		AllowedCancel: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)

	cancelCtx, cancel := context.WithCancel(context.Background())
	_, evtCancel := evt.CancelableContext(cancelCtx)
	evtCancel()

	err = evt.Done(context.TODO(), cancelCtx.Err())
	c.Assert(err, check.IsNil)
	cancel()

	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].CancelInfo, check.DeepEquals, eventTypes.CancelInfo{})
}

func (s *S) TestEventInfo(c *check.C) {
	customData := struct{ A string }{A: "value"}
	evt, err := New(context.TODO(), &Opts{
		Target:     eventTypes.Target{Type: "app", Value: "myapp"},
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

	customData = struct{ A string }{A: "other"}
	err = evt.DoneCustomData(context.TODO(), nil, customData)
	c.Assert(err, check.IsNil)

	evt, err = GetByHexID(context.TODO(), evt.UniqueID.Hex())
	c.Assert(err, check.IsNil)

	evtInfo, err := EventInfo(evt)
	c.Assert(err, check.IsNil)

	c.Assert(evtInfo.StartCustomData, check.DeepEquals, eventTypes.LegacyBSONRaw{Kind: 0x3, Data: []uint8{0x12, 0x0, 0x0, 0x0, 0x2, 0x61, 0x0, 0x6, 0x0, 0x0, 0x0, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x0, 0x0}})
	c.Assert(evtInfo.EndCustomData, check.DeepEquals, eventTypes.LegacyBSONRaw{Kind: 0x3, Data: []uint8{0x12, 0x0, 0x0, 0x0, 0x2, 0x61, 0x0, 0x6, 0x0, 0x0, 0x0, 0x6f, 0x74, 0x68, 0x65, 0x72, 0x0, 0x0}})
	c.Assert(evtInfo.OtherCustomData, check.DeepEquals, eventTypes.LegacyBSONRaw{})

	c.Assert(evtInfo.CustomData, check.DeepEquals, eventTypes.EventInfoCustomData{
		Start: map[string]interface{}{
			"a": "value",
		},
		End: map[string]interface{}{
			"a": "other",
		},
	})
}

func (s *S) TestEventInfoWithGenericCustomData(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:     eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:       permission.PermAppUpdateEnvSet,
		Owner:      s.token,
		CustomData: "start",
		Allowed:    Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	c.Assert(evt.StartTime.IsZero(), check.Equals, false)
	c.Assert(evt.LockUpdateTime.IsZero(), check.Equals, false)

	err = evt.DoneCustomData(context.TODO(), nil, "end")
	c.Assert(err, check.IsNil)
	evtInfo, err := EventInfo(evt)
	c.Assert(err, check.IsNil)

	c.Assert(evtInfo.StartCustomData, check.DeepEquals, eventTypes.LegacyBSONRaw{Kind: 0x2, Data: []uint8{0x6, 0x0, 0x0, 0x0, 0x73, 0x74, 0x61, 0x72, 0x74, 0x0}})
	c.Assert(evtInfo.EndCustomData, check.DeepEquals, eventTypes.LegacyBSONRaw{Kind: 0x2, Data: []uint8{0x4, 0x0, 0x0, 0x0, 0x65, 0x6e, 0x64, 0x0}})
	c.Assert(evtInfo.OtherCustomData, check.DeepEquals, eventTypes.LegacyBSONRaw{})

	c.Assert(evtInfo.CustomData, check.DeepEquals, eventTypes.EventInfoCustomData{
		Start: "start",
		End:   "end",
	})
}

func (s *S) TestEventOwnerEmailWithUserOwner(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:   eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:     permission.PermAppUpdateEnvSet,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: "my@user.com"},
		Allowed:  Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	c.Assert(evt.OwnerEmail(), check.Equals, "my@user.com")
}

func (s *S) TestEventOwnerEmailWithTokenOwner(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:   eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:     permission.PermAppUpdateEnvSet,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeToken, Name: "my-team-token"},
		Allowed:  Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	c.Assert(evt.OwnerEmail(), check.Equals, "my-team-token@tsuru-team-token")
}

func (s *S) TestEventOwnerEmailWithInternalOwner(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:   eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:     permission.PermAppUpdateEnvSet,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeInternal, Name: "internal-process"},
		Allowed:  Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	c.Assert(evt.OwnerEmail(), check.Equals, "")
}

func (s *S) TestEventOwnerEmailWithAppOwner(c *check.C) {
	evt, err := New(context.TODO(), &Opts{
		Target:   eventTypes.Target{Type: "app", Value: "myapp"},
		Kind:     permission.PermAppUpdateEnvSet,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeApp, Name: "myapp"},
		Allowed:  Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	c.Assert(evt.OwnerEmail(), check.Equals, "")
}
