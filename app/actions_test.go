// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/queue"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
)

func (s *S) TestOldInsertAppForward(c *C) {
	action := new(oldInsertApp)
	a := App{
		Name:      "appname",
		Framework: "django",
		Units:     []Unit{{Machine: 3}},
	}
	err := action.forward(&a)
	defer action.backward(&a)
	c.Assert(err, IsNil)
	c.Assert(a.State, Equals, "pending")
	var retrievedApp App
	err = db.Session.Apps().Find(bson.M{"name": a.Name}).One(&retrievedApp)
	c.Assert(err, IsNil)
	c.Assert(retrievedApp.Name, Equals, a.Name)
	c.Assert(retrievedApp.Framework, Equals, a.Framework)
	c.Assert(retrievedApp.State, Equals, a.State)
}

func (s *S) TestOldInsertAppBackward(c *C) {
	action := new(oldInsertApp)
	a := App{
		Name:      "appname",
		Framework: "django",
		Units:     []Unit{{Machine: 3}},
	}
	err := action.forward(&a)
	c.Assert(err, IsNil)
	action.backward(&a)
	qt, err := db.Session.Apps().Find(bson.M{"name": a.Name}).Count()
	c.Assert(err, IsNil)
	c.Assert(qt, Equals, 0)
}

func (s *S) TestOldInsertAppRollbackItself(c *C) {
	action := new(oldInsertApp)
	c.Assert(action.rollbackItself(), Equals, false)
}

func (s *S) TestOldCreateBucketForward(c *C) {
	patchRandomReader()
	defer unpatchRandomReader()
	a := App{
		Name:      "appname",
		Framework: "django",
		Units:     []Unit{{Machine: 3}},
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	insert := new(oldInsertApp)
	err := insert.forward(&a)
	c.Assert(err, IsNil)
	defer insert.backward(&a)
	bucket := new(oldCreateBucketIam)
	err = bucket.forward(&a)
	c.Assert(err, IsNil)
	defer bucket.backward(&a)
	err = a.Get()
	c.Assert(err, IsNil)
	env := a.InstanceEnv(s3InstanceName)
	c.Assert(env["TSURU_S3_ENDPOINT"].Value, Equals, s.t.S3Server.URL())
	c.Assert(env["TSURU_S3_ENDPOINT"].Public, Equals, false)
	c.Assert(env["TSURU_S3_LOCATIONCONSTRAINT"].Value, Equals, "true")
	c.Assert(env["TSURU_S3_LOCATIONCONSTRAINT"].Public, Equals, false)
	e, ok := env["TSURU_S3_ACCESS_KEY_ID"]
	c.Assert(ok, Equals, true)
	c.Assert(e.Public, Equals, false)
	e, ok = env["TSURU_S3_SECRET_KEY"]
	c.Assert(ok, Equals, true)
	c.Assert(e.Public, Equals, false)
	c.Assert(env["TSURU_S3_BUCKET"].Value, HasLen, maxBucketSize)
	c.Assert(env["TSURU_S3_BUCKET"].Value, Equals, "appnamee3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3")
	c.Assert(env["TSURU_S3_BUCKET"].Public, Equals, false)
	env = a.InstanceEnv("")
	c.Assert(env["APPNAME"].Value, Equals, a.Name)
	c.Assert(env["APPNAME"].Public, Equals, false)
	c.Assert(env["TSURU_HOST"].Value, Equals, expectedHost)
	c.Assert(env["TSURU_HOST"].Public, Equals, false)
	expected := queue.Message{
		Action: regenerateApprc,
		Args:   []string{a.Name},
	}
	message, err := queue.Get(queueName, 2e9)
	c.Assert(err, IsNil)
	defer message.Delete()
	c.Assert(message.Action, Equals, expected.Action)
	c.Assert(message.Args, DeepEquals, expected.Args)
}

func (s *S) TestOldCreateBucketBackward(c *C) {
	source := patchRandomReader()
	defer unpatchRandomReader()
	a := App{
		Name:      "theirapp",
		Framework: "ruby",
		Units:     []Unit{{Machine: 1}},
	}
	action := new(oldCreateBucketIam)
	err := action.forward(&a)
	c.Assert(err, IsNil)
	action.backward(&a)
	iam := getIAMEndpoint()
	_, err = iam.GetUser("theirapp")
	c.Assert(err, NotNil)
	s3 := getS3Endpoint()
	bucketName := fmt.Sprintf("%s%x", a.Name, source[:(maxBucketSize-len(a.Name)/2)])
	bucket := s3.Bucket(bucketName)
	_, err = bucket.Get("")
	c.Assert(err, NotNil)
}

func (s *S) TestOldCreateBucketRollbackItself(c *C) {
	action := new(oldCreateBucketIam)
	c.Assert(action.rollbackItself(), Equals, true)
}

func (s *S) TestDeployForward(c *C) {
	action := new(provisionApp)
	a := App{
		Name:      "appname",
		Framework: "django",
		Units:     []Unit{{Machine: 3}},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = action.forward(&a, 4)
	defer s.provisioner.Destroy(&a)
	c.Assert(err, IsNil)
	index := s.provisioner.FindApp(&a)
	c.Assert(index, Equals, 0)
	units := s.provisioner.GetUnits(&a)
	c.Assert(units, HasLen, 4)
}

func (s *S) TestDeployRollbackItself(c *C) {
	action := new(provisionApp)
	c.Assert(action.rollbackItself(), Equals, false)
}

func (s *S) TestOldCreateRepositoryForward(c *C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	a := App{Name: "someapp"}
	var teams []auth.Team
	err := db.Session.Teams().Find(bson.M{"users": s.user.Email}).All(&teams)
	c.Assert(err, IsNil)
	a.SetTeams(teams)
	action := new(oldCreateRepository)
	err = action.forward(&a)
	c.Assert(err, IsNil)
	defer action.backward(&a)
	c.Assert(h.url[0], Equals, "/repository")
	c.Assert(h.method[0], Equals, "POST")
	expected := fmt.Sprintf(`{"name":"someapp","users":["%s"],"ispublic":false}`, s.user.Email)
	c.Assert(string(h.body[0]), Equals, expected)
}

func (s *S) TestOldCreateRepositoryBackward(c *C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	a := App{Name: "someapp"}
	action := new(oldCreateRepository)
	action.backward(&a)
	c.Assert(h.url[0], Equals, "/repository/someapp")
	c.Assert(h.method[0], Equals, "DELETE")
	c.Assert(string(h.body[0]), Equals, "null")
}
