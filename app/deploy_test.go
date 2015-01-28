// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"errors"
	"io/ioutil"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/testing"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

func (s *S) TestListDeployByNonAdminUsers(c *gocheck.C) {
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	defer nativeScheme.Remove(user)
	team := &auth.Team{Name: "someteam", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Teams().RemoveId("someteam")
	s.conn.Deploys().RemoveAll(nil)
	a := App{Name: "g1", Teams: []string{team.Name}}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	a2 := App{Name: "ge"}
	err = s.conn.Apps().Insert(a2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Apps().Remove(bson.M{"name": a2.Name})
	deploys := []deploy{
		{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second)},
		{App: "ge", Timestamp: time.Now()},
	}
	for _, deploy := range deploys {
		s.conn.Deploys().Insert(deploy)
	}
	defer s.conn.Deploys().RemoveAll(bson.M{"app": a.Name})
	result, err := ListDeploys(nil, nil, user)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.HasLen, 1)
	c.Assert(result[0].App, gocheck.Equals, "g1")
}

func (s *S) TestListDeployByAdminUsers(c *gocheck.C) {
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	defer nativeScheme.Remove(user)
	team := &auth.Team{Name: "adminteam", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Teams().RemoveId("adminteam")
	s.conn.Deploys().RemoveAll(nil)
	adminTeamName, err := config.GetString("admin-team")
	c.Assert(err, gocheck.IsNil)
	config.Set("admin-team", "adminteam")
	defer config.Set("admin-team", adminTeamName)
	a := App{Name: "g1", Teams: []string{team.Name}}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	a2 := App{Name: "ge"}
	err = s.conn.Apps().Insert(a2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Apps().Remove(bson.M{"name": a2.Name})
	deploys := []deploy{
		{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second)},
		{App: "ge", Timestamp: time.Now()},
	}
	for _, deploy := range deploys {
		s.conn.Deploys().Insert(deploy)
	}
	defer s.conn.Deploys().RemoveAll(bson.M{"app": a.Name})
	result, err := ListDeploys(nil, nil, user)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.HasLen, 2)
	c.Assert(result[0].App, gocheck.Equals, "ge")
	c.Assert(result[1].App, gocheck.Equals, "g1")
}

func (s *S) TestListDeployByAppAndService(c *gocheck.C) {
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
	a := App{Name: "g1"}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	a2 := App{Name: "ge"}
	err = s.conn.Apps().Insert(a2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Apps().Remove(bson.M{"name": a2.Name})
	deploys := []deploy{
		{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second)},
		{App: "ge", Timestamp: time.Now()},
	}
	for _, deploy := range deploys {
		s.conn.Deploys().Insert(deploy)
	}
	defer s.conn.Deploys().RemoveAll(bson.M{"app": a.Name})
	result, err := ListDeploys(&a2, &srv, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.IsNil)
}

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
	deploys, err := a.ListDeploys(nil)
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
	deploys, err := ListDeploys(nil, &srv, nil)
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
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	defer user.Delete()
	team := &auth.Team{Name: "team", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Teams().Remove(team)
	a := App{
		Name:     "g1",
		Platform: "zend",
		Teams:    []string{team.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	a = App{
		Name:     "ge",
		Platform: "zend",
		Teams:    []string{team.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.conn.Deploys().RemoveAll(nil)
	insert := []interface{}{
		deploy{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second)},
		deploy{App: "ge", Timestamp: time.Now()},
	}
	s.conn.Deploys().Insert(insert...)
	defer s.conn.Deploys().RemoveAll(nil)
	expected := []deploy{insert[1].(deploy), insert[0].(deploy)}
	deploys, err := ListDeploys(nil, nil, user)
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

func (s *S) TestListDeployByAppAndUser(c *gocheck.C) {
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	defer user.Delete()
	team := &auth.Team{Name: "team", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Teams().Remove(team)
	a := App{
		Name:     "g1",
		Platform: "zend",
		Teams:    []string{team.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	a = App{
		Name:     "ge",
		Platform: "zend",
		Teams:    []string{team.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.conn.Deploys().RemoveAll(nil)
	insert := []interface{}{
		deploy{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second)},
		deploy{App: "ge", Timestamp: time.Now()},
	}
	s.conn.Deploys().Insert(insert...)
	defer s.conn.Deploys().RemoveAll(nil)
	expected := []deploy{insert[1].(deploy)}
	deploys, err := ListDeploys(&a, nil, user)
	c.Assert(err, gocheck.IsNil)
	c.Assert(expected[0].App, gocheck.DeepEquals, deploys[0].App)
	c.Assert(len(expected), gocheck.Equals, len(deploys))
}

func (s *S) TestGetDeploy(c *gocheck.C) {
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	defer user.Delete()
	team := &auth.Team{Name: "team", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Teams().Remove(team)
	a := App{
		Name:     "g1",
		Platform: "zend",
		Teams:    []string{team.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.conn.Deploys().RemoveAll(nil)
	newDeploy := deploy{ID: bson.NewObjectId(), App: "g1", Timestamp: time.Now()}
	err = s.conn.Deploys().Insert(&newDeploy)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Deploys().Remove(bson.M{"name": newDeploy.App})
	lastDeploy, err := GetDeploy(newDeploy.ID.Hex(), user)
	c.Assert(err, gocheck.IsNil)
	ts := lastDeploy.Timestamp
	lastDeploy.Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
	ts = newDeploy.Timestamp
	newDeploy.Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
	c.Assert(lastDeploy.ID, gocheck.Equals, newDeploy.ID)
	c.Assert(lastDeploy.App, gocheck.Equals, newDeploy.App)
	c.Assert(lastDeploy.Timestamp, gocheck.Equals, newDeploy.Timestamp)
}

func (s *S) TestGetDeployWithoutAccess(c *gocheck.C) {
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	defer user.Delete()
	a := App{
		Name:     "g1",
		Platform: "zend",
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.conn.Deploys().RemoveAll(nil)
	newDeploy := deploy{ID: bson.NewObjectId(), App: "g1", Timestamp: time.Now()}
	err = s.conn.Deploys().Insert(&newDeploy)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Deploys().Remove(bson.M{"name": newDeploy.App})
	result, err := GetDeploy(newDeploy.ID.Hex(), user)
	c.Assert(err.Error(), gocheck.Equals, "Deploy not found.")
	c.Assert(result, gocheck.IsNil)
}

func (s *S) TestGetDeployNotFound(c *gocheck.C) {
	idTest := bson.NewObjectId()
	deploy, err := GetDeploy(idTest.Hex(), nil)
	c.Assert(err.Error(), gocheck.Equals, "not found")
	c.Assert(deploy, gocheck.IsNil)
}

func (s *S) TestGetDiffInDeploys(c *gocheck.C) {
	s.conn.Deploys().RemoveAll(nil)
	myDeploy := deploy{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second), Commit: "545b1904af34458704e2aa06ff1aaffad5289f8g"}
	deploys := []deploy{
		{App: "ge", Timestamp: time.Now(), Commit: "hwed834hf8y34h8fhn8rnr823nr238runh23x"},
		{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second * 2), Commit: "545b1904af34458704e2aa06ff1aaffad5289f8f"},
		myDeploy,
		{App: "g1", Timestamp: time.Now(), Commit: "1b970b076bbb30d708e262b402d4e31910e1dc10"},
	}
	for _, d := range deploys {
		s.conn.Deploys().Insert(d)
	}
	defer s.conn.Deploys().RemoveAll(nil)
	expected := "test_diff"
	h := testHandler{content: expected}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	err := s.conn.Deploys().Find(bson.M{"commit": myDeploy.Commit}).One(&myDeploy)
	c.Assert(err, gocheck.IsNil)
	diffOutput, err := GetDiffInDeploys(&myDeploy)
	c.Assert(err, gocheck.IsNil)
	c.Assert(diffOutput, gocheck.DeepEquals, expected)
	c.Assert(h.request.URL.Query().Get("last_commit"), gocheck.Equals, "545b1904af34458704e2aa06ff1aaffad5289f8g")
	c.Assert(h.request.URL.Query().Get("previous_commit"), gocheck.Equals, "545b1904af34458704e2aa06ff1aaffad5289f8f")
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
	err := s.conn.Deploys().Find(bson.M{"commit": lastDeploy.Commit}).One(&lastDeploy)
	c.Assert(err, gocheck.IsNil)
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
		User:         "someone@themoon",
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
	c.Assert(result["image"], gocheck.Equals, "app-image")
	c.Assert(result["log"], gocheck.Equals, "Git deploy called")
	c.Assert(result["user"], gocheck.Equals, "someone@themoon")
}

func (s *S) TestDeployAppSaveDeployErrorData(c *gocheck.C) {
	provisioner := testing.NewFakeProvisioner()
	provisioner.PrepareFailure("GitDeploy", errors.New("deploy error"))
	Provisioner = provisioner
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

func (s *S) TestUserHasPermission(c *gocheck.C) {
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	defer user.Delete()
	team := &auth.Team{Name: "team", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Teams().Remove(team)
	a := App{
		Name:     "g1",
		Platform: "zend",
		Teams:    []string{team.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	hasPermission := userHasPermission(user, a.Name)
	c.Assert(hasPermission, gocheck.Equals, true)
}

func (s *S) TestUserHasNoPermission(c *gocheck.C) {
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	defer user.Delete()
	team := &auth.Team{Name: "team", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Teams().Remove(team)
	a := App{
		Name:     "g1",
		Platform: "zend",
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	hasPermission := userHasPermission(user, a.Name)
	c.Assert(hasPermission, gocheck.Equals, false)
}

func (s *S) TestIncrementDeploy(c *gocheck.C) {
	a := App{
		Name:     "otherapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	incrementDeploy(&a)
	s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(a.Deploys, gocheck.Equals, uint(1))
}

func (s *S) TestDeployToProvisioner(c *gocheck.C) {
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
	opts := DeployOptions{App: &a, Version: "version"}
	_, err = deployToProvisioner(&opts, writer)
	c.Assert(err, gocheck.IsNil)
	logs := writer.String()
	c.Assert(logs, gocheck.Equals, "Git deploy called")
}

func (s *S) TestDeployToProvisionerArchive(c *gocheck.C) {
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
	opts := DeployOptions{App: &a, ArchiveURL: "https://s3.amazonaws.com/smt/archive.tar.gz"}
	_, err = deployToProvisioner(&opts, writer)
	c.Assert(err, gocheck.IsNil)
	logs := writer.String()
	c.Assert(logs, gocheck.Equals, "Archive deploy called")
}

func (s *S) TestDeployToProvisionerUpload(c *gocheck.C) {
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
	opts := DeployOptions{App: &a, File: ioutil.NopCloser(bytes.NewBuffer([]byte("my file")))}
	_, err = deployToProvisioner(&opts, writer)
	c.Assert(err, gocheck.IsNil)
	logs := writer.String()
	c.Assert(logs, gocheck.Equals, "Upload deploy called")
}
