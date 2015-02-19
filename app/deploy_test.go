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
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/service"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestListDeployByNonAdminUsers(c *check.C) {
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	defer nativeScheme.Remove(user)
	team := &auth.Team{Name: "someteam", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().RemoveId("someteam")
	s.conn.Deploys().RemoveAll(nil)
	a := App{Name: "g1", Teams: []string{team.Name}}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	a2 := App{Name: "ge"}
	err = s.conn.Apps().Insert(a2)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Apps().Remove(bson.M{"name": a2.Name})
	deploys := []DeployData{
		{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second)},
		{App: "ge", Timestamp: time.Now()},
	}
	for _, deploy := range deploys {
		s.conn.Deploys().Insert(deploy)
	}
	defer s.conn.Deploys().RemoveAll(bson.M{"app": a.Name})
	result, err := ListDeploys(nil, nil, user, 0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 1)
	c.Assert(result[0].App, check.Equals, "g1")
}

func (s *S) TestListDeployByAdminUsers(c *check.C) {
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	defer nativeScheme.Remove(user)
	team := &auth.Team{Name: "adminteam", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().RemoveId("adminteam")
	s.conn.Deploys().RemoveAll(nil)
	adminTeamName, err := config.GetString("admin-team")
	c.Assert(err, check.IsNil)
	config.Set("admin-team", "adminteam")
	defer config.Set("admin-team", adminTeamName)
	a := App{Name: "g1", Teams: []string{team.Name}}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	a2 := App{Name: "ge"}
	err = s.conn.Apps().Insert(a2)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Apps().Remove(bson.M{"name": a2.Name})
	deploys := []DeployData{
		{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second)},
		{App: "ge", Timestamp: time.Now()},
	}
	for _, deploy := range deploys {
		s.conn.Deploys().Insert(deploy)
	}
	defer s.conn.Deploys().RemoveAll(bson.M{"app": a.Name})
	result, err := ListDeploys(nil, nil, user, 0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 2)
	c.Assert(result[0].App, check.Equals, "ge")
	c.Assert(result[1].App, check.Equals, "g1")
}

func (s *S) TestListDeployByAppAndService(c *check.C) {
	s.conn.Deploys().RemoveAll(nil)
	srv := service.Service{Name: "mysql"}
	instance := service.ServiceInstance{
		Name:        "myinstance",
		ServiceName: "mysql",
		Apps:        []string{"g1"},
	}
	err := s.conn.ServiceInstances().Insert(instance)
	err = s.conn.Services().Insert(srv)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"apps": instance.Apps})
	defer s.conn.Services().Remove(bson.M{"_id": srv.Name})
	a := App{Name: "g1"}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	a2 := App{Name: "ge"}
	err = s.conn.Apps().Insert(a2)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Apps().Remove(bson.M{"name": a2.Name})
	deploys := []DeployData{
		{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second)},
		{App: "ge", Timestamp: time.Now()},
	}
	for _, deploy := range deploys {
		s.conn.Deploys().Insert(deploy)
	}
	defer s.conn.Deploys().RemoveAll(bson.M{"app": a.Name})
	result, err := ListDeploys(&a2, &srv, nil, 0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.IsNil)
}

func (s *S) TestListAppDeploys(c *check.C) {
	s.conn.Deploys().RemoveAll(nil)
	a := App{Name: "g1"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	insert := []interface{}{
		DeployData{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second)},
		DeployData{App: "g1", Timestamp: time.Now()},
	}
	s.conn.Deploys().Insert(insert...)
	defer s.conn.Deploys().RemoveAll(bson.M{"app": a.Name})
	expected := []DeployData{insert[1].(DeployData), insert[0].(DeployData)}
	deploys, err := a.ListDeploys(nil)
	c.Assert(err, check.IsNil)
	for i := 0; i < 2; i++ {
		ts := expected[i].Timestamp
		expected[i].Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
		ts = deploys[i].Timestamp
		deploys[i].Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
		expected[i].ID = deploys[i].ID
	}
	c.Assert(deploys, check.DeepEquals, expected)
}

func (s *S) TestListServiceDeploys(c *check.C) {
	s.conn.Deploys().RemoveAll(nil)
	srv := service.Service{Name: "mysql"}
	instance := service.ServiceInstance{
		Name:        "myinstance",
		ServiceName: "mysql",
		Apps:        []string{"g1"},
	}
	err := s.conn.ServiceInstances().Insert(instance)
	err = s.conn.Services().Insert(srv)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"apps": instance.Apps})
	defer s.conn.Services().Remove(bson.M{"_id": srv.Name})
	insert := []interface{}{
		DeployData{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second)},
		DeployData{App: "g1", Timestamp: time.Now()},
	}
	s.conn.Deploys().Insert(insert...)
	defer s.conn.Deploys().RemoveAll(bson.M{"apps": instance.Apps})
	expected := []DeployData{insert[1].(DeployData), insert[0].(DeployData)}
	deploys, err := ListDeploys(nil, &srv, nil, 0, 0)
	c.Assert(err, check.IsNil)
	for i := 0; i < 2; i++ {
		ts := expected[i].Timestamp
		expected[i].Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
		ts = deploys[i].Timestamp
		deploys[i].Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
		expected[i].ID = deploys[i].ID
	}
	c.Assert(deploys, check.DeepEquals, expected)
}

func (s *S) TestListAllDeploys(c *check.C) {
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	defer user.Delete()
	team := &auth.Team{Name: "team", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().Remove(team)
	a := App{
		Name:     "g1",
		Platform: "zend",
		Teams:    []string{team.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	a = App{
		Name:     "ge",
		Platform: "zend",
		Teams:    []string{team.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.conn.Deploys().RemoveAll(nil)
	insert := []interface{}{
		DeployData{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second)},
		DeployData{App: "ge", Timestamp: time.Now(), Image: "app-image"},
	}
	s.conn.Deploys().Insert(insert...)
	defer s.conn.Deploys().RemoveAll(nil)
	expected := []DeployData{insert[1].(DeployData), insert[0].(DeployData)}
	expected[0].CanRollback = true
	deploys, err := ListDeploys(nil, nil, user, 0, 0)
	c.Assert(err, check.IsNil)
	for i := 0; i < 2; i++ {
		ts := expected[i].Timestamp
		expected[i].Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
		ts = deploys[i].Timestamp
		deploys[i].Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
		expected[i].ID = deploys[i].ID
	}
	c.Assert(deploys, check.DeepEquals, expected)
}

func (s *S) TestListAllDeploysSkipAndLimit(c *check.C) {
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	defer user.Delete()
	team := &auth.Team{Name: "team", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().Remove(team)
	a := App{
		Name:     "app1",
		Platform: "zend",
		Teams:    []string{team.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.conn.Deploys().RemoveAll(nil)
	insert := []interface{}{
		DeployData{App: "app1", Commit: "v1", Timestamp: time.Now().Add(-30 * time.Second)},
		DeployData{App: "app1", Commit: "v2", Timestamp: time.Now().Add(-20 * time.Second)},
		DeployData{App: "app1", Commit: "v3", Timestamp: time.Now().Add(-10 * time.Second)},
		DeployData{App: "app1", Commit: "v4", Timestamp: time.Now()},
	}
	s.conn.Deploys().Insert(insert...)
	defer s.conn.Deploys().RemoveAll(nil)
	expected := []DeployData{insert[2].(DeployData), insert[1].(DeployData)}
	deploys, err := ListDeploys(nil, nil, user, 1, 2)
	c.Assert(err, check.IsNil)
	c.Assert(deploys, check.HasLen, 2)
	for i := 0; i < len(deploys); i++ {
		ts := expected[i].Timestamp
		newTs := time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
		expected[i].Timestamp = newTs
		ts = deploys[i].Timestamp
		deploys[i].Timestamp = newTs
		expected[i].ID = deploys[i].ID
	}
	c.Assert(deploys, check.DeepEquals, expected)
}

func (s *S) TestListDeployByAppAndUser(c *check.C) {
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	defer user.Delete()
	team := &auth.Team{Name: "team", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().Remove(team)
	a := App{
		Name:     "g1",
		Platform: "zend",
		Teams:    []string{team.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	a = App{
		Name:     "ge",
		Platform: "zend",
		Teams:    []string{team.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.conn.Deploys().RemoveAll(nil)
	insert := []interface{}{
		DeployData{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second)},
		DeployData{App: "ge", Timestamp: time.Now()},
	}
	s.conn.Deploys().Insert(insert...)
	defer s.conn.Deploys().RemoveAll(nil)
	expected := []DeployData{insert[1].(DeployData)}
	deploys, err := ListDeploys(&a, nil, user, 0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(expected[0].App, check.DeepEquals, deploys[0].App)
	c.Assert(len(expected), check.Equals, len(deploys))
}

func (s *S) TestGetDeploy(c *check.C) {
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	defer user.Delete()
	team := &auth.Team{Name: "team", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().Remove(team)
	a := App{
		Name:     "g1",
		Platform: "zend",
		Teams:    []string{team.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.conn.Deploys().RemoveAll(nil)
	newDeploy := DeployData{ID: bson.NewObjectId(), App: "g1", Timestamp: time.Now()}
	err = s.conn.Deploys().Insert(&newDeploy)
	c.Assert(err, check.IsNil)
	defer s.conn.Deploys().Remove(bson.M{"name": newDeploy.App})
	lastDeploy, err := GetDeploy(newDeploy.ID.Hex(), user)
	c.Assert(err, check.IsNil)
	ts := lastDeploy.Timestamp
	lastDeploy.Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
	ts = newDeploy.Timestamp
	newDeploy.Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
	c.Assert(lastDeploy.ID, check.Equals, newDeploy.ID)
	c.Assert(lastDeploy.App, check.Equals, newDeploy.App)
	c.Assert(lastDeploy.Timestamp, check.Equals, newDeploy.Timestamp)
}

func (s *S) TestGetDeployWithoutAccess(c *check.C) {
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	defer user.Delete()
	a := App{
		Name:     "g1",
		Platform: "zend",
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.conn.Deploys().RemoveAll(nil)
	newDeploy := DeployData{ID: bson.NewObjectId(), App: "g1", Timestamp: time.Now()}
	err = s.conn.Deploys().Insert(&newDeploy)
	c.Assert(err, check.IsNil)
	defer s.conn.Deploys().Remove(bson.M{"name": newDeploy.App})
	result, err := GetDeploy(newDeploy.ID.Hex(), user)
	c.Assert(err.Error(), check.Equals, "Deploy not found.")
	c.Assert(result, check.IsNil)
}

func (s *S) TestGetDeployNotFound(c *check.C) {
	idTest := bson.NewObjectId()
	deploy, err := GetDeploy(idTest.Hex(), nil)
	c.Assert(err.Error(), check.Equals, "not found")
	c.Assert(deploy, check.IsNil)
}

func (s *S) TestGetDiffInDeploys(c *check.C) {
	s.conn.Deploys().RemoveAll(nil)
	myDeploy := DeployData{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second), Commit: "545b1904af34458704e2aa06ff1aaffad5289f8g"}
	deploys := []DeployData{
		{App: "ge", Timestamp: time.Now(), Commit: "hwed834hf8y34h8fhn8rnr823nr238runh23x"},
		{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second * 2), Commit: "545b1904af34458704e2aa06ff1aaffad5289f8f"},
		myDeploy,
		{App: "g1", Timestamp: time.Now(), Commit: "1b970b076bbb30d708e262b402d4e31910e1dc10"},
	}
	for _, d := range deploys {
		s.conn.Deploys().Insert(d)
	}
	defer s.conn.Deploys().RemoveAll(nil)
	err := s.conn.Deploys().Find(bson.M{"commit": myDeploy.Commit}).One(&myDeploy)
	c.Assert(err, check.IsNil)
	repository.Manager().CreateRepository("g1", nil)
	diffOutput, err := GetDiffInDeploys(&myDeploy)
	c.Assert(err, check.IsNil)
	c.Assert(diffOutput, check.Equals, "")
}

func (s *S) TestGetDiffInDeploysWithOneCommit(c *check.C) {
	s.conn.Deploys().RemoveAll(nil)
	lastDeploy := DeployData{App: "g1", Timestamp: time.Now(), Commit: "1b970b076bbb30d708e262b402d4e31910e1dc10"}
	s.conn.Deploys().Insert(lastDeploy)
	defer s.conn.Deploys().RemoveAll(nil)
	err := s.conn.Deploys().Find(bson.M{"commit": lastDeploy.Commit}).One(&lastDeploy)
	c.Assert(err, check.IsNil)
	diffOutput, err := GetDiffInDeploys(&lastDeploy)
	c.Assert(err, check.IsNil)
	c.Assert(diffOutput, check.Equals, "The deployment must have at least two commits for the diff.")
}

func (s *S) TestDeployApp(c *check.C) {
	a := App{
		Name:     "someApp",
		Platform: "django",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
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
	c.Assert(err, check.IsNil)
	logs := writer.String()
	c.Assert(logs, check.Equals, "Git deploy called")
}

func (s *S) TestDeployAppWithUpdatePlatform(c *check.C) {
	a := App{
		Name:           "someApp",
		Platform:       "django",
		Teams:          []string{s.team.Name},
		UpdatePlatform: true,
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
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
	c.Assert(err, check.IsNil)
	logs := writer.String()
	c.Assert(logs, check.Equals, "Git deploy called")
	var updatedApp App
	s.conn.Apps().Find(bson.M{"name": "someApp"}).One(&updatedApp)
	c.Assert(updatedApp.UpdatePlatform, check.Equals, false)
}

func (s *S) TestDeployAppIncrementDeployNumber(c *check.C) {
	a := App{
		Name:     "otherapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
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
	c.Assert(err, check.IsNil)
	s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(a.Deploys, check.Equals, uint(1))
}

func (s *S) TestDeployAppSaveDeployData(c *check.C) {
	a := App{
		Name:     "otherapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
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
	c.Assert(err, check.IsNil)
	s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(a.Deploys, check.Equals, uint(1))
	var result map[string]interface{}
	s.conn.Deploys().Find(bson.M{"app": a.Name}).One(&result)
	c.Assert(result["app"], check.Equals, a.Name)
	now := time.Now()
	diff := now.Sub(result["timestamp"].(time.Time))
	c.Assert(diff < 60*time.Second, check.Equals, true)
	c.Assert(result["duration"], check.Not(check.Equals), 0)
	c.Assert(result["commit"], check.Equals, commit)
	c.Assert(result["image"], check.Equals, "app-image")
	c.Assert(result["log"], check.Equals, "Git deploy called")
	c.Assert(result["user"], check.Equals, "someone@themoon")
	c.Assert(result["origin"], check.Equals, "git")
}

func (s *S) TestDeployAppSaveDeployDataOriginRollback(c *check.C) {
	a := App{
		Name:     "otherapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	writer := &bytes.Buffer{}
	err = Deploy(DeployOptions{
		App:          &a,
		OutputStream: writer,
		Image:        "some-image",
	})
	c.Assert(err, check.IsNil)
	s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(a.Deploys, check.Equals, uint(1))
	var result map[string]interface{}
	s.conn.Deploys().Find(bson.M{"app": a.Name}).One(&result)
	c.Assert(result["app"], check.Equals, a.Name)
	now := time.Now()
	diff := now.Sub(result["timestamp"].(time.Time))
	c.Assert(diff < 60*time.Second, check.Equals, true)
	c.Assert(result["duration"], check.Not(check.Equals), 0)
	c.Assert(result["image"], check.Equals, "some-image")
	c.Assert(result["log"], check.Equals, "Image deploy called")
	c.Assert(result["origin"], check.Equals, "rollback")
}

func (s *S) TestDeployAppSaveDeployDataOriginAppDeploy(c *check.C) {
	a := App{
		Name:     "otherapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	writer := &bytes.Buffer{}
	err = Deploy(DeployOptions{
		App:          &a,
		OutputStream: writer,
		File:         ioutil.NopCloser(bytes.NewBuffer([]byte("my file"))),
	})
	c.Assert(err, check.IsNil)
	s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(a.Deploys, check.Equals, uint(1))
	var result map[string]interface{}
	s.conn.Deploys().Find(bson.M{"app": a.Name}).One(&result)
	c.Assert(result["app"], check.Equals, a.Name)
	now := time.Now()
	diff := now.Sub(result["timestamp"].(time.Time))
	c.Assert(diff < 60*time.Second, check.Equals, true)
	c.Assert(result["duration"], check.Not(check.Equals), 0)
	c.Assert(result["image"], check.Equals, "app-image")
	c.Assert(result["log"], check.Equals, "Upload deploy called")
	c.Assert(result["origin"], check.Equals, "app-deploy")
}

func (s *S) TestDeployAppSaveDeployErrorData(c *check.C) {
	provisioner := provisiontest.NewFakeProvisioner()
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
	c.Assert(err, check.IsNil)
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
	c.Assert(err, check.NotNil)
	var result map[string]interface{}
	s.conn.Deploys().Find(bson.M{"app": a.Name}).One(&result)
	c.Assert(result["app"], check.Equals, a.Name)
	c.Assert(result["error"], check.NotNil)
}

func (s *S) TestUserHasPermission(c *check.C) {
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	defer user.Delete()
	team := &auth.Team{Name: "team", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().Remove(team)
	a := App{
		Name:     "g1",
		Platform: "zend",
		Teams:    []string{team.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	hasPermission := userHasPermission(user, a.Name)
	c.Assert(hasPermission, check.Equals, true)
}

func (s *S) TestUserHasNoPermission(c *check.C) {
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	defer user.Delete()
	team := &auth.Team{Name: "team", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().Remove(team)
	a := App{
		Name:     "g1",
		Platform: "zend",
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	hasPermission := userHasPermission(user, a.Name)
	c.Assert(hasPermission, check.Equals, false)
}

func (s *S) TestIncrementDeploy(c *check.C) {
	a := App{
		Name:     "otherapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	incrementDeploy(&a)
	s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(a.Deploys, check.Equals, uint(1))
}

func (s *S) TestDeployToProvisioner(c *check.C) {
	a := App{
		Name:     "someApp",
		Platform: "django",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	writer := &bytes.Buffer{}
	opts := DeployOptions{App: &a, Version: "version"}
	_, err = deployToProvisioner(&opts, writer)
	c.Assert(err, check.IsNil)
	logs := writer.String()
	c.Assert(logs, check.Equals, "Git deploy called")
}

func (s *S) TestDeployToProvisionerArchive(c *check.C) {
	a := App{
		Name:     "someApp",
		Platform: "django",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	writer := &bytes.Buffer{}
	opts := DeployOptions{App: &a, ArchiveURL: "https://s3.amazonaws.com/smt/archive.tar.gz"}
	_, err = deployToProvisioner(&opts, writer)
	c.Assert(err, check.IsNil)
	logs := writer.String()
	c.Assert(logs, check.Equals, "Archive deploy called")
}

func (s *S) TestDeployToProvisionerUpload(c *check.C) {
	a := App{
		Name:     "someApp",
		Platform: "django",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	writer := &bytes.Buffer{}
	opts := DeployOptions{App: &a, File: ioutil.NopCloser(bytes.NewBuffer([]byte("my file")))}
	_, err = deployToProvisioner(&opts, writer)
	c.Assert(err, check.IsNil)
	logs := writer.String()
	c.Assert(logs, check.Equals, "Upload deploy called")
}

func (s *S) TestDeployToProvisionerImage(c *check.C) {
	a := App{
		Name:     "someApp",
		Platform: "django",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	writer := &bytes.Buffer{}
	opts := DeployOptions{App: &a, Image: "my-image-x"}
	_, err = deployToProvisioner(&opts, writer)
	c.Assert(err, check.IsNil)
	logs := writer.String()
	c.Assert(logs, check.Equals, "Image deploy called")
}

func (s *S) TestMarkDeploysAsRemoved(c *check.C) {
	s.createAdminUserAndTeam(c)
	a := App{Name: "someApp"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	opts := DeployOptions{
		App:     &a,
		Version: "version",
		Commit:  "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
	}
	err = saveDeployData(&opts, "myid", "mylog", time.Second, nil)
	c.Assert(err, check.IsNil)
	c.Assert(err, check.IsNil)
	defer s.conn.Deploys().RemoveAll(bson.M{"app": a.Name})
	result, err := ListDeploys(nil, nil, s.admin, 0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 1)
	c.Assert(result[0].Image, check.Equals, "myid")
	err = markDeploysAsRemoved(a.Name)
	c.Assert(err, check.IsNil)
	result, err = ListDeploys(nil, nil, s.admin, 0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 0)
	var allDeploys []DeployData
	err = s.conn.Deploys().Find(nil).All(&allDeploys)
	c.Assert(err, check.IsNil)
	c.Assert(allDeploys, check.HasLen, 1)
	c.Assert(allDeploys[0].Image, check.Equals, "myid")
	c.Assert(allDeploys[0].RemoveDate.IsZero(), check.Equals, false)
}
