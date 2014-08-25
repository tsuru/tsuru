// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"time"

	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/testing"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

func (s *S) TestListAppDeploys(c *gocheck.C) {
	s.conn.Deploys().RemoveAll(nil)
	a := App{Name: "g1"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	insert := []interface{}{
		deploy{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second)},
		deploy{App: "g1", Timestamp: time.Now()},
	}
	s.conn.Deploys().Insert(insert...)
	defer s.conn.Deploys().RemoveAll(bson.M{"app": a.Name})
	expected := []deploy{insert[1].(deploy), insert[0].(deploy)}
	deploys, err := a.ListDeploys()
	c.Assert(err, gocheck.IsNil)
	for i := 0; i < 2; i++ {
		ts := expected[i].Timestamp
		expected[i].Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
		ts = deploys[i].Timestamp
		deploys[i].Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
		expected[i].ID = deploys[i].ID
	}
	c.Assert(deploys, gocheck.DeepEquals, expected)
}

func (s *S) TestListServiceDeploys(c *gocheck.C) {
	s.conn.Deploys().RemoveAll(nil)
	srv := service.Service{Name: "mysql"}
	instance := service.ServiceInstance{
		Name:        "myinstance",
		ServiceName: "mysql",
		Apps:        []string{"g1"},
	}
	err := s.conn.ServiceInstances().Insert(instance)
	err = s.conn.Services().Insert(srv)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"apps": instance.Apps})
	defer s.conn.Services().Remove(bson.M{"_id": srv.Name})
	insert := []interface{}{
		deploy{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second)},
		deploy{App: "g1", Timestamp: time.Now()},
	}
	s.conn.Deploys().Insert(insert...)
	defer s.conn.Deploys().RemoveAll(bson.M{"apps": instance.Apps})
	expected := []deploy{insert[1].(deploy), insert[0].(deploy)}
	deploys, err := ListDeploys(&srv)
	c.Assert(err, gocheck.IsNil)
	for i := 0; i < 2; i++ {
		ts := expected[i].Timestamp
		expected[i].Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
		ts = deploys[i].Timestamp
		deploys[i].Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
		expected[i].ID = deploys[i].ID
	}
	c.Assert(deploys, gocheck.DeepEquals, expected)
}

func (s *S) TestListAllDeploys(c *gocheck.C) {
	s.conn.Deploys().RemoveAll(nil)
	insert := []interface{}{
		deploy{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second)},
		deploy{App: "ge", Timestamp: time.Now()},
	}
	s.conn.Deploys().Insert(insert...)
	defer s.conn.Deploys().RemoveAll(nil)
	expected := []deploy{insert[1].(deploy), insert[0].(deploy)}
	deploys, err := ListDeploys(nil)
	c.Assert(err, gocheck.IsNil)
	for i := 0; i < 2; i++ {
		ts := expected[i].Timestamp
		expected[i].Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
		ts = deploys[i].Timestamp
		deploys[i].Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
		expected[i].ID = deploys[i].ID
	}
	c.Assert(deploys, gocheck.DeepEquals, expected)
}

func (s *S) TestGetDeploy(c *gocheck.C) {
	s.conn.Deploys().RemoveAll(nil)
	newDeploy := deploy{ID: bson.NewObjectId(), App: "g1", Timestamp: time.Now()}
	err := s.conn.Deploys().Insert(&newDeploy)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Deploys().Remove(bson.M{"name": newDeploy.App})
	lastDeploy, err := GetDeploy(newDeploy.ID.Hex())
	c.Assert(err, gocheck.IsNil)
	ts := lastDeploy.Timestamp
	lastDeploy.Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
	ts = newDeploy.Timestamp
	newDeploy.Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
	c.Assert(lastDeploy.ID, gocheck.Equals, newDeploy.ID)
	c.Assert(lastDeploy.App, gocheck.Equals, newDeploy.App)
	c.Assert(lastDeploy.Timestamp, gocheck.Equals, newDeploy.Timestamp)
}

func (s *S) TestGetDeployNotFound(c *gocheck.C) {
	idTest := bson.NewObjectId()
	deploy, err := GetDeploy(idTest.Hex())
	c.Assert(err.Error(), gocheck.Equals, "not found")
	c.Assert(deploy, gocheck.IsNil)
}

func (s *S) TestGetDiffInDeploys(c *gocheck.C) {
	s.conn.Deploys().RemoveAll(nil)
	lastDeploy := deploy{App: "g1", Timestamp: time.Now(), Commit: "1b970b076bbb30d708e262b402d4e31910e1dc10"}
	previousDeploy := deploy{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second), Commit: "545b1904af34458704e2aa06ff1aaffad5289f8f"}
	otherAppDeploy := deploy{App: "ge", Timestamp: time.Now(), Commit: "hwed834hf8y34h8fhn8rnr823nr238runh23x"}
	s.conn.Deploys().Insert(previousDeploy)
	s.conn.Deploys().Insert(lastDeploy)
	s.conn.Deploys().Insert(otherAppDeploy)
	defer s.conn.Deploys().RemoveAll(nil)
	expected := "test_diff"
	h := testHandler{content: expected}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	diffOutput, err := GetDiffInDeploys(&lastDeploy)
	c.Assert(err, gocheck.IsNil)
	c.Assert(diffOutput, gocheck.DeepEquals, expected)
}

func (s *S) TestGetDiffInDeploysWithOneCommit(c *gocheck.C) {
	s.conn.Deploys().RemoveAll(nil)
	lastDeploy := deploy{App: "g1", Timestamp: time.Now(), Commit: "1b970b076bbb30d708e262b402d4e31910e1dc10"}
	s.conn.Deploys().Insert(lastDeploy)
	defer s.conn.Deploys().RemoveAll(nil)
	expected := "test_diff"
	h := testHandler{content: expected}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	diffOutput, err := GetDiffInDeploys(&lastDeploy)
	c.Assert(err, gocheck.IsNil)
	c.Assert(diffOutput, gocheck.Equals, "The deployment must have at least two commits for the diff.")
}

func (s *S) TestDeployApp(c *gocheck.C) {
	a := App{
		Name:     "someApp",
		Platform: "django",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	writer := &bytes.Buffer{}
	err = Deploy(DeployOptions{
		App:          &a,
		Version:      "version",
		Commit:       "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		OutputStream: writer,
	})
	c.Assert(err, gocheck.IsNil)
	logs := writer.String()
	c.Assert(logs, gocheck.Equals, "Git deploy called")
}

func (s *S) TestDeployAppWithUpdatePlatform(c *gocheck.C) {
	a := App{
		Name:           "someApp",
		Platform:       "django",
		Teams:          []string{s.team.Name},
		UpdatePlatform: true,
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	writer := &bytes.Buffer{}
	err = Deploy(DeployOptions{
		App:          &a,
		Version:      "version",
		Commit:       "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		OutputStream: writer,
	})
	c.Assert(err, gocheck.IsNil)
	logs := writer.String()
	c.Assert(logs, gocheck.Equals, "Git deploy called")
	var updatedApp App
	s.conn.Apps().Find(bson.M{"name": "someApp"}).One(&updatedApp)
	c.Assert(updatedApp.UpdatePlatform, gocheck.Equals, false)
}

func (s *S) TestDeployAppIncrementDeployNumber(c *gocheck.C) {
	a := App{
		Name:     "otherapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	writer := &bytes.Buffer{}
	err = Deploy(DeployOptions{
		App:          &a,
		Version:      "version",
		Commit:       "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		OutputStream: writer,
	})
	c.Assert(err, gocheck.IsNil)
	s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(a.Deploys, gocheck.Equals, uint(1))
}

func (s *S) TestDeployAppSaveDeployData(c *gocheck.C) {
	a := App{
		Name:     "otherapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	writer := &bytes.Buffer{}
	commit := "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c"
	err = Deploy(DeployOptions{
		App:          &a,
		Version:      "version",
		Commit:       commit,
		OutputStream: writer,
	})
	c.Assert(err, gocheck.IsNil)
	s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(a.Deploys, gocheck.Equals, uint(1))
	var result map[string]interface{}
	s.conn.Deploys().Find(bson.M{"app": a.Name}).One(&result)
	c.Assert(result["app"], gocheck.Equals, a.Name)
	now := time.Now()
	diff := now.Sub(result["timestamp"].(time.Time))
	c.Assert(diff < 60*time.Second, gocheck.Equals, true)
	c.Assert(result["duration"], gocheck.Not(gocheck.Equals), 0)
	c.Assert(result["commit"], gocheck.Equals, commit)
}

func (s *S) TestDeployCustomPipeline(c *gocheck.C) {
	provisioner := testing.PipelineFakeProvisioner{
		FakeProvisioner: testing.NewFakeProvisioner(),
	}
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	a := App{
		Name:     "otherapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	provisioner.Provision(&a)
	defer provisioner.Destroy(&a)
	writer := &bytes.Buffer{}
	err = Deploy(DeployOptions{
		App:          &a,
		Version:      "version",
		Commit:       "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		OutputStream: writer,
	})
	c.Assert(err, gocheck.IsNil)
	c.Assert(provisioner.ExecutedPipeline(), gocheck.Equals, true)
}

func (s *S) TestDeployAppSaveDeployErrorData(c *gocheck.C) {
	provisioner := testing.PipelineErrorFakeProvisioner{
		FakeProvisioner: testing.NewFakeProvisioner(),
	}
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	a := App{
		Name:     "testErrorApp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	provisioner.Provision(&a)
	defer provisioner.Destroy(&a)
	writer := &bytes.Buffer{}
	err = Deploy(DeployOptions{
		App:          &a,
		Version:      "version",
		Commit:       "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		OutputStream: writer,
	})
	c.Assert(err, gocheck.NotNil)
	var result map[string]interface{}
	s.conn.Deploys().Find(bson.M{"app": a.Name}).One(&result)
	c.Assert(result["app"], gocheck.Equals, a.Name)
	c.Assert(result["error"], gocheck.NotNil)
}
