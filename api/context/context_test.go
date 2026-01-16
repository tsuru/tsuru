// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package context

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db/storagev2"
	appTypes "github.com/tsuru/tsuru/types/app"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	check "gopkg.in/check.v1"
)

type S struct {
	token auth.Token
	app   *appTypes.App
}

var _ = check.Suite(&S{})

var nativeScheme = auth.ManagedScheme(native.NativeScheme{})

func Test(t *testing.T) { check.TestingT(t) }

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "api_context_tests_s")
}

func (s *S) SetUpTest(c *check.C) {
	storagev2.ClearAllCollections(nil)
	user := &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	_, err := nativeScheme.Create(context.TODO(), user)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(context.TODO(), map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	s.app = &appTypes.App{Name: "app"}
}

func (s *S) TearDownSuite(c *check.C) {
	storagev2.ClearAllCollections(nil)
}

func (s *S) TestClear(c *check.C) {
	r, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	SetRequestID(r, "X-RID", "xpto")
	val := GetRequestID(r, "X-RID")
	c.Assert(val, check.DeepEquals, "xpto")
	Clear(r)
	val = GetRequestID(r, "X-RID")
	c.Assert(val, check.DeepEquals, "")
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

	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	tr := otel.Tracer("test")

	ctx, span := tr.Start(r.Context(), "test")
	r = r.WithContext(ctx)

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

	c.Check(span.IsRecording(), check.Equals, true)

	span.End()
	tp.Shutdown(context.TODO())
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

func (s *S) TestGetApp(c *check.C) {
	r, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	a := GetApp(r)
	c.Assert(a, check.IsNil)
	SetApp(r, s.app)
	a = GetApp(r)
	c.Assert(a, check.DeepEquals, s.app)
}

func (s *S) TestRequestID(c *check.C) {
	r, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	id := GetRequestID(r, "Request-ID")
	c.Assert(id, check.Equals, "")
	SetRequestID(r, "Request-ID", "test")
	id = GetRequestID(r, "Request-ID")
	c.Assert(id, check.Equals, "test")
}
