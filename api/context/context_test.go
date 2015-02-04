// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package context

import (
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/gorilla/context"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"launchpad.net/gocheck"
)

type S struct {
	token auth.Token
}

var _ = gocheck.Suite(&S{})

var nativeScheme = auth.ManagedScheme(native.NativeScheme{})

func Test(t *testing.T) { gocheck.TestingT(t) }

func (s *S) SetUpSuite(c *gocheck.C) {
	config.Set("database:name", "api_context_tests_s")
	user := &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	_, err := nativeScheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TearDownSuite(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	dbtest.ClearAllCollections(conn.Apps().Database)
}

func (s *S) TestClear(c *gocheck.C) {
	r, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	context.Set(r, "my-key", "value")
	Clear(r)
	val := context.Get(r, "my-key")
	c.Assert(val, gocheck.IsNil)
}

func (s *S) TestGetAuthToken(c *gocheck.C) {
	r, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	token := GetAuthToken(r)
	c.Assert(token, gocheck.IsNil)
	SetAuthToken(r, s.token)
	token = GetAuthToken(r)
	c.Assert(token, gocheck.Equals, s.token)
}

func (s *S) TestAddRequestError(c *gocheck.C) {
	r, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	err1 := errors.New("msg1")
	err2 := errors.New("msg2")
	myErr := GetRequestError(r)
	c.Assert(myErr, gocheck.IsNil)
	AddRequestError(r, err1)
	myErr = GetRequestError(r)
	c.Assert(myErr, gocheck.Equals, err1)
	AddRequestError(r, err2)
	otherErr := GetRequestError(r)
	c.Assert(otherErr.Error(), gocheck.Equals, "msg2 Caused by: msg1")
}

func (s *S) TestSetDelayedHandler(c *gocheck.C) {
	r, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	val := GetDelayedHandler(r)
	c.Assert(val, gocheck.IsNil)
	SetDelayedHandler(r, handler)
	val = GetDelayedHandler(r)
	v1 := reflect.ValueOf(val)
	v2 := reflect.ValueOf(handler)
	c.Assert(v1.Pointer(), gocheck.Equals, v2.Pointer())
}

func (s *S) TestSetPreventUnlock(c *gocheck.C) {
	r, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(IsPreventUnlock(r), gocheck.Equals, false)
	SetPreventUnlock(r)
	c.Assert(IsPreventUnlock(r), gocheck.Equals, true)
}
