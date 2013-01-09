// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/queue"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
)

func (s *S) TestInsertAppForward(c *C) {
	app := App{Name: "conviction", Framework: "evergrey"}
	ctx := action.FWContext{
		Params: []interface{}{app},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app.Name})
	a, ok := r.(*App)
	c.Assert(ok, Equals, true)
	c.Assert(a.Name, Equals, app.Name)
	c.Assert(a.Framework, Equals, app.Framework)
	err = app.Get()
	c.Assert(err, IsNil)
	c.Assert(app.State, Equals, "pending")
}

func (s *S) TestInsertAppForwardAppPointer(c *C) {
	app := App{Name: "conviction", Framework: "evergrey"}
	ctx := action.FWContext{
		Params: []interface{}{&app},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app.Name})
	a, ok := r.(*App)
	c.Assert(ok, Equals, true)
	c.Assert(a.Name, Equals, app.Name)
	c.Assert(a.Framework, Equals, app.Framework)
	err = app.Get()
	c.Assert(err, IsNil)
}

func (s *S) TestInsertAppForwardInvalidValue(c *C) {
	ctx := action.FWContext{
		Params: []interface{}{"hello"},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(r, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "First parameter must be App or *App.")
}

func (s *S) TestInsertAppBackward(c *C) {
	app := App{Name: "conviction", Framework: "evergrey"}
	ctx := action.BWContext{
		Params:   []interface{}{app},
		FWResult: &app,
	}
	err := db.Session.Apps().Insert(app)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app.Name}) // sanity
	insertApp.Backward(ctx)
	n, err := db.Session.Apps().Find(bson.M{"name": app.Name}).Count()
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 0)
}

func (s *S) TestInsertAppMinimumParams(c *C) {
	c.Assert(insertApp.MinParams, Equals, 1)
}

func (s *S) TestCreateBucketForward(c *C) {
	patchRandomReader()
	defer unpatchRandomReader()
	app := App{
		Name:      "earthshine",
		Framework: "django",
		Units:     []Unit{{Machine: 3}},
	}
	err := db.Session.Apps().Insert(app)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app.Name})
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	ctx := action.FWContext{Params: []interface{}{app}, Previous: &app}
	r, err := createBucketIam.Forward(ctx)
	c.Assert(err, IsNil)
	bwCtx := action.BWContext{Params: ctx.Params, FWResult: r}
	defer createBucketIam.Backward(bwCtx)
	cbResult, ok := r.(*createBucketResult)
	c.Assert(ok, Equals, true)
	c.Assert(cbResult.env.bucket, Equals, "earthshinee3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3")
	c.Assert(cbResult.env.endpoint, Equals, s.t.S3Server.URL())
	c.Assert(cbResult.app.Name, Equals, "earthshine")
	err = app.Get()
	c.Assert(err, IsNil)
	appEnv := app.InstanceEnv(s3InstanceName)
	c.Assert(appEnv["TSURU_S3_ENDPOINT"].Value, Equals, s.t.S3Server.URL())
	c.Assert(appEnv["TSURU_S3_ENDPOINT"].Public, Equals, false)
	c.Assert(appEnv["TSURU_S3_LOCATIONCONSTRAINT"].Value, Equals, "true")
	c.Assert(appEnv["TSURU_S3_LOCATIONCONSTRAINT"].Public, Equals, false)
	e, ok := appEnv["TSURU_S3_ACCESS_KEY_ID"]
	c.Assert(ok, Equals, true)
	c.Assert(e.Public, Equals, false)
	e, ok = appEnv["TSURU_S3_SECRET_KEY"]
	c.Assert(ok, Equals, true)
	c.Assert(e.Public, Equals, false)
	c.Assert(appEnv["TSURU_S3_BUCKET"].Value, HasLen, maxBucketSize-1)
	c.Assert(appEnv["TSURU_S3_BUCKET"].Value, Equals, "earthshinee3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3")
	c.Assert(appEnv["TSURU_S3_BUCKET"].Public, Equals, false)
	appEnv = app.InstanceEnv("")
	c.Assert(appEnv["APPNAME"].Value, Equals, app.Name)
	c.Assert(appEnv["APPNAME"].Public, Equals, false)
	c.Assert(appEnv["TSURU_HOST"].Value, Equals, expectedHost)
	c.Assert(appEnv["TSURU_HOST"].Public, Equals, false)
	message, err := queue.Get(queueName, 2e9)
	c.Assert(err, IsNil)
	defer message.Delete()
	c.Assert(message.Action, Equals, regenerateApprc)
	c.Assert(message.Args, DeepEquals, []string{app.Name})
}

func (s *S) TestCreateBucketBackward(c *C) {
	source := patchRandomReader()
	defer unpatchRandomReader()
	app := App{
		Name:      "theirapp",
		Framework: "ruby",
		Units:     []Unit{{Machine: 1}},
	}
	err := db.Session.Apps().Insert(app)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app.Name})
	fwctx := action.FWContext{Params: []interface{}{app}, Previous: &app}
	result, err := createBucketIam.Forward(fwctx)
	c.Assert(err, IsNil)
	bwctx := action.BWContext{Params: fwctx.Params, FWResult: result}
	createBucketIam.Backward(bwctx)
	iam := getIAMEndpoint()
	_, err = iam.GetUser("theirapp")
	c.Assert(err, NotNil)
	s3 := getS3Endpoint()
	bucketName := fmt.Sprintf("%s%x", app.Name, source[:(maxBucketSize-len(app.Name)/2)])
	bucket := s3.Bucket(bucketName)
	_, err = bucket.Get("")
	c.Assert(err, NotNil)
}

func (s *S) TestCreateBucketMinParams(c *C) {
	c.Assert(createBucketIam.MinParams, Equals, 0)
}

func (s *S) TestCreateRepositoryForward(c *C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	app := App{Name: "someapp", Teams: []string{s.team.Name}}
	ctx := action.FWContext{Params: []interface{}{app}}
	result, err := createRepository.Forward(ctx)
	a, ok := result.(*App)
	c.Assert(ok, Equals, true)
	c.Assert(a.Name, Equals, app.Name)
	c.Assert(err, IsNil)
	c.Assert(h.url[0], Equals, "/repository")
	c.Assert(h.method[0], Equals, "POST")
	expected := fmt.Sprintf(`{"name":"someapp","users":["%s"],"ispublic":false}`, s.user.Email)
	c.Assert(string(h.body[0]), Equals, expected)
}

func (s *S) TestCreateRepositoryForwardAppPointer(c *C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	app := App{Name: "someapp", Teams: []string{s.team.Name}}
	ctx := action.FWContext{Params: []interface{}{&app}}
	result, err := createRepository.Forward(ctx)
	a, ok := result.(*App)
	c.Assert(ok, Equals, true)
	c.Assert(a.Name, Equals, app.Name)
	c.Assert(err, IsNil)
	c.Assert(h.url[0], Equals, "/repository")
	c.Assert(h.method[0], Equals, "POST")
	expected := fmt.Sprintf(`{"name":"someapp","users":["%s"],"ispublic":false}`, s.user.Email)
	c.Assert(string(h.body[0]), Equals, expected)
}

func (s *S) TestCreateRepositoryForwardInvalidType(c *C) {
	ctx := action.FWContext{Params: []interface{}{"something"}}
	_, err := createRepository.Forward(ctx)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "First parameter must be App or *App.")
}

func (s *S) TestCreateRepositoryBackward(c *C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	app := App{Name: "someapp"}
	ctx := action.BWContext{FWResult: &app, Params: []interface{}{app}}
	createRepository.Backward(ctx)
	c.Assert(h.url[0], Equals, "/repository/someapp")
	c.Assert(h.method[0], Equals, "DELETE")
	c.Assert(string(h.body[0]), Equals, "null")
}

func (s *S) TestCreateRepositoryMinParams(c *C) {
	c.Assert(createRepository.MinParams, Equals, 1)
}

func (s *S) TestProvisionAppForward(c *C) {
	app := App{
		Name:      "earthshine",
		Framework: "django",
		Units:     []Unit{{Machine: 3}},
	}
	err := db.Session.Apps().Insert(app)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app.Name})
	ctx := action.FWContext{Params: []interface{}{app, 4}}
	result, err := provisionApp.Forward(ctx)
	defer s.provisioner.Destroy(&app)
	c.Assert(err, IsNil)
	a, ok := result.(*App)
	c.Assert(ok, Equals, true)
	c.Assert(a.Name, Equals, app.Name)
	index := s.provisioner.FindApp(&app)
	c.Assert(index, Equals, 0)
}

func (s *S) TestProvisionAppForwardAppPointer(c *C) {
	app := App{
		Name:      "earthshine",
		Framework: "django",
		Units:     []Unit{{Machine: 3}},
	}
	err := db.Session.Apps().Insert(app)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app.Name})
	ctx := action.FWContext{Params: []interface{}{&app, 4}}
	result, err := provisionApp.Forward(ctx)
	defer s.provisioner.Destroy(&app)
	c.Assert(err, IsNil)
	a, ok := result.(*App)
	c.Assert(ok, Equals, true)
	c.Assert(a.Name, Equals, app.Name)
	index := s.provisioner.FindApp(&app)
	c.Assert(index, Equals, 0)
}

func (s *S) TestProvisionAppForwardInvalidApp(c *C) {
	ctx := action.FWContext{Params: []interface{}{"something", 1}}
	_, err := provisionApp.Forward(ctx)
	c.Assert(err, NotNil)
}

func (s *S) TestProvisionAppBackward(c *C) {
	app := App{
		Name:      "earthshine",
		Framework: "django",
		Units:     []Unit{{Machine: 3}},
	}
	err := db.Session.Apps().Insert(app)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app.Name})
	fwctx := action.FWContext{Params: []interface{}{&app, 4}}
	result, err := provisionApp.Forward(fwctx)
	c.Assert(err, IsNil)
	bwctx := action.BWContext{Params: []interface{}{&app, 4}, FWResult: result}
	provisionApp.Backward(bwctx)
	index := s.provisioner.FindApp(&app)
	c.Assert(index, Equals, -1)
}

func (s *S) TestProvisionAppMinParams(c *C) {
	c.Assert(provisionApp.MinParams, Equals, 2)
}

func (s *S) TestProvisionAddUnitsForward(c *C) {
	app := App{
		Name:      "castle",
		Framework: "heavens",
		Units:     []Unit{{Machine: 2}},
	}
	err := db.Session.Apps().Insert(app)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app.Name})
	previous, err := provisionApp.Forward(action.FWContext{Params: []interface{}{&app, 4}})
	c.Assert(err, IsNil)
	defer provisionApp.Backward(action.BWContext{Params: []interface{}{&app, 4}, FWResult: previous})
	ctx := action.FWContext{Params: []interface{}{&app, 4}, Previous: previous}
	result, err := provisionAddUnits.Forward(ctx)
	c.Assert(err, IsNil)
	c.Assert(result, IsNil)
	units := s.provisioner.GetUnits(&app)
	c.Assert(units, HasLen, 4)
}

func (s *S) TestProvisionAddUnitsBackward(c *C) {
	c.Assert(provisionAddUnits.Backward, IsNil)
}

func (s *S) TestProvisionAddUnitsMinParams(c *C) {
	c.Assert(provisionAddUnits.MinParams, Equals, 2)
}
