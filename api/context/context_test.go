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
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db/storagev2"
	appTypes "github.com/tsuru/tsuru/types/app"
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

	// Setup OpenTelemetry testing
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	tracer := tp.Tracer("test-tracer")

	spanCtx, span := tracer.Start(r.Context(), "test")
	r = r.WithContext(spanCtx)

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

	span.End() // End the span so it's flushed to the exporter

	spans := exporter.GetSpans()
	c.Assert(spans, check.HasLen, 1)
	roSpan := spans[0]

	// Check span status
	c.Check(roSpan.Status().Code, check.Equals, codes.Error)
	c.Check(roSpan.Status().Description, check.Equals, "msg2") // The last error set becomes the status description

	// Check recorded errors as events
	// OpenTelemetry's RecordError creates events with specific attributes.
	// We expect two error events, one for err1 and one for err2.
	// The last error (err2) also sets the span status.
	c.Assert(roSpan.Events(), check.HasLen, 2)

	// Check first error event (err1)
	event1 := roSpan.Events()[0]
	c.Check(event1.Name, check.Equals, "exception")
	var foundErr1Object bool
	for _, attr := range event1.Attributes {
		if attr.Key == "exception.message" && attr.Value.AsString() == "msg1" {
			foundErr1Object = true
			break
		}
	}
	c.Check(foundErr1Object, check.Equals, true, check.Commentf("Expected to find error.object for msg1"))

	// Check second error event (err2)
	event2 := roSpan.Events()[1]
	c.Check(event2.Name, check.Equals, "exception")
	var foundErr2Object bool
	for _, attr := range event2.Attributes {
		if attr.Key == "exception.message" && attr.Value.AsString() == "msg2" {
			foundErr2Object = true
			break
		}
	}
	c.Check(foundErr2Object, check.Equals, true, check.Commentf("Expected to find error.object for msg2"))
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
