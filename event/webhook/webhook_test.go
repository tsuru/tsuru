// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webhook

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	eventTypes "github.com/tsuru/tsuru/types/event"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	service *webHookService
}

var _ = check.Suite(&S{})

func (s *S) SetUpTest(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=150")
	config.Set("database:name", "tsuru_event_webhook_tests")
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = dbtest.ClearAllCollections(conn.Events().Database)
	c.Assert(err, check.IsNil)
	svc, err := WebHookService()
	c.Assert(err, check.IsNil)
	s.service = svc.(*webHookService)
}

func (s *S) TearDownTest(c *check.C) {
	err := s.service.Shutdown(context.Background())
	c.Assert(err, check.IsNil)
}

func (s *S) TestNotify(c *check.C) {
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: "app", Value: "myapp"},
		ExtraTargets: []event.ExtraTarget{
			{Target: event.Target{Type: "app", Value: "xapp1"}},
			{Target: event.Target{Type: "app", Value: "xapp2"}},
		},
		RawOwner: event.Owner{
			Type: "user",
			Name: "me@me.com",
		},
		Kind:    permission.PermAppUpdateEnvSet,
		Allowed: event.Allowed(permission.PermAppReadEvents, permission.Context(permission.CtxApp, "myapp")),
	})
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	called := make(chan struct{})
	var receivedReq *http.Request
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(called)
		receivedBody, _ = ioutil.ReadAll(r.Body)
		receivedReq = r
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	u, err := url.Parse(srv.URL)
	c.Assert(err, check.IsNil)
	u.Path = "/a/b/c"
	u.RawQuery = "a=b&c=d"
	err = s.service.storage.Insert(eventTypes.WebHook{
		Name: "xyz",
		URL:  *u,
		Body: "ahoy",
		Headers: http.Header{
			"X-Ahoy": []string{"Errrr"},
		},
	})
	c.Assert(err, check.IsNil)
	s.service.Notify(string(evt.UniqueID))
	<-called
	c.Assert(string(receivedBody), check.Equals, "ahoy")
	c.Assert(receivedReq.URL.Path, check.Equals, "/a/b/c")
	c.Assert(receivedReq.URL.Query(), check.DeepEquals, url.Values{
		"a": []string{"b"},
		"c": []string{"d"},
	})
	c.Assert(receivedReq.Header, check.DeepEquals, http.Header{
		"X-Ahoy":          []string{"Errrr"},
		"User-Agent":      []string{"tsuru-webhook-client/1.0"},
		"Accept-Encoding": []string{"gzip"},
		"Content-Length":  []string{"4"},
	})
}
