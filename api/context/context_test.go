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
	"github.com/tsuru/tsuru/repository/repositorytest"
	"gopkg.in/check.v1"
)

type S struct {
	token auth.Token
}

var _ = check.Suite(&S{})

var nativeScheme = auth.ManagedScheme(native.NativeScheme{})

func Test(t *testing.T) { check.TestingT(t) }

func (s *S) SetUpSuite(c *check.C) {
	repositorytest.Reset()
	config.Set("database:name", "api_context_tests_s")
	config.Set("repo-manager", "fake")
	user := &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	dbtest.ClearAllCollections(conn.Apps().Database)
}

func (s *S) SetUpTest(c *check.C) {
	repositorytest.Reset()
}

func (s *S) TestClear(c *check.C) {
	r, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	context.Set(r, "my-key", "value")
	Clear(r)
	val := context.Get(r, "my-key")
	c.Assert(val, check.IsNil)
}

func (s *S) TestGetAuthToken(c *check.C) {
	r, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	token := GetAuthToken(r)
	c.Assert(token, check.IsNil)
	SetAuthToken(r, s.token)
	token = GetAuthToken(r)
	c.Assert(token, check.Equals, s.token)
}

func (s *S) TestAddRequestError(c *check.C) {
	r, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	err1 := errors.New("msg1")
	err2 := errors.New("msg2")
	myErr := GetRequestError(r)
	c.Assert(myErr, check.IsNil)
	AddRequestError(r, err1)
	myErr = GetRequestError(r)
	c.Assert(myErr, check.Equals, err1)
	AddRequestError(r, err2)
	otherErr := GetRequestError(r)
	c.Assert(otherErr.Error(), check.Equals, "msg2 Caused by: msg1")
}

func (s *S) TestSetDelayedHandler(c *check.C) {
	r, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	val := GetDelayedHandler(r)
	c.Assert(val, check.IsNil)
	SetDelayedHandler(r, handler)
	val = GetDelayedHandler(r)
	v1 := reflect.ValueOf(val)
	v2 := reflect.ValueOf(handler)
	c.Assert(v1.Pointer(), check.Equals, v2.Pointer())
}

func (s *S) TestSetPreventUnlock(c *check.C) {
	r, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	c.Assert(IsPreventUnlock(r), check.Equals, false)
	SetPreventUnlock(r)
	c.Assert(IsPreventUnlock(r), check.Equals, true)
}
