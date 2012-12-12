// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"fmt"
	"github.com/globocom/commandmocker"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/queue"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	stdlog "log"
	"strings"
	/* "time" */
)

func (s *S) TestInsertAppForward(c *C) {
	action := new(insertApp)
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

func (s *S) TestInsertAppBackward(c *C) {
	action := new(insertApp)
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

func (s *S) TestCreateBucketForward(c *C) {
	patchRandomReader()
	defer unpatchRandomReader()
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	server := FakeQueueServer{}
	server.Start("127.0.0.1:0")
	defer server.Stop()
	a := App{
		Name:      "appname",
		Framework: "django",
		Units:     []Unit{{Machine: 3}},
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	old, err := config.Get("queue-server")
	if err != nil {
		defer config.Set("queue-server", old)
	}
	config.Set("queue-server", server.listener.Addr().String())
	insert := new(insertApp)
	err = insert.forward(&a)
	c.Assert(err, IsNil)
	defer insert.backward(&a)
	bucket := new(createBucketIam)
	err = bucket.forward(&a)
	c.Assert(err, IsNil)
	defer bucket.backward(&a)
	de := new(deploy)
	err = de.forward(&a)
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
	expectedMessage := queue.Message{
		Action: RegenerateApprc,
		Args:   []string{a.Name},
	}
	server.Lock()
	defer server.Unlock()
	c.Assert(server.messages, DeepEquals, []queue.Message{expectedMessage})
}

func (s *S) TestCreateBucketBackward(c *C) {
	source := patchRandomReader()
	defer unpatchRandomReader()
	a := App{
		Name:      "theirapp",
		Framework: "ruby",
		Units:     []Unit{{Machine: 1}},
	}
	action := new(createBucketIam)
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

func (s *S) TestDeployForward(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	action := new(deploy)
	a := App{
		Name:      "appname",
		Framework: "django",
		Units:     []Unit{{Machine: 3}},
	}
	err = action.forward(&a)
	c.Assert(err, IsNil)
	str := strings.Replace(w.String(), "\n", "", -1)
	c.Assert(str, Matches, ".*deploy --repository=/home/charms local:django appname.*")
}
