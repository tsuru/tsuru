// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/url"
	"time"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestListAppDeploysMarshalJSON(c *check.C) {
	s.conn.Deploys().RemoveAll(nil)
	a := App{Name: "g1"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	insert := []DeployData{
		{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second), Log: "logs", Diff: "diff"},
		{App: "g1", Timestamp: time.Now(), Log: "logs", Diff: "diff"},
	}
	err = s.conn.Deploys().Insert(&insert[0])
	c.Assert(err, check.IsNil)
	err = s.conn.Deploys().Insert(&insert[0])
	c.Assert(err, check.IsNil)
	s.conn.Deploys().Insert(&insert)
	defer s.conn.Deploys().RemoveAll(bson.M{"app": a.Name})
	deploys, err := ListDeploys(nil, 0, 0)
	c.Assert(err, check.IsNil)
	data, err := json.Marshal(&deploys)
	c.Assert(err, check.IsNil)
	err = json.Unmarshal(data, &deploys)
	c.Assert(err, check.IsNil)
	expected := []DeployData{insert[1], insert[0]}
	for i := 0; i < 2; i++ {
		c.Assert(deploys[i].App, check.Equals, expected[i].App)
		c.Assert(deploys[i].Commit, check.Equals, expected[i].Commit)
		c.Assert(deploys[i].Error, check.Equals, expected[i].Error)
		c.Assert(deploys[i].Image, check.Equals, expected[i].Image)
		c.Assert(deploys[i].User, check.Equals, expected[i].User)
		c.Assert(deploys[i].CanRollback, check.Equals, expected[i].CanRollback)
		c.Assert(deploys[i].Origin, check.Equals, expected[i].Origin)
		c.Assert(deploys[i].Log, check.Equals, "")
		c.Assert(deploys[i].Diff, check.Equals, "")
	}
}

func (s *S) TestListAppDeploys(c *check.C) {
	s.conn.Deploys().RemoveAll(nil)
	a := App{Name: "g1"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	insert := []DeployData{
		{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second), Log: "logs", Diff: "diff"},
		{App: "g1", Timestamp: time.Now(), Log: "logs", Diff: "diff"},
	}
	err = s.conn.Deploys().Insert(&insert[0])
	c.Assert(err, check.IsNil)
	err = s.conn.Deploys().Insert(&insert[0])
	c.Assert(err, check.IsNil)
	s.conn.Deploys().Insert(&insert)
	defer s.conn.Deploys().RemoveAll(bson.M{"app": a.Name})
	deploys, err := ListDeploys(nil, 0, 0)
	c.Assert(err, check.IsNil)
	expected := []DeployData{insert[1], insert[0]}
	for i := 0; i < 2; i++ {
		c.Assert(deploys[i].App, check.Equals, expected[i].App)
		c.Assert(deploys[i].Commit, check.Equals, expected[i].Commit)
		c.Assert(deploys[i].Error, check.Equals, expected[i].Error)
		c.Assert(deploys[i].Image, check.Equals, expected[i].Image)
		c.Assert(deploys[i].User, check.Equals, expected[i].User)
		c.Assert(deploys[i].CanRollback, check.Equals, expected[i].CanRollback)
		c.Assert(deploys[i].Origin, check.Equals, expected[i].Origin)
		c.Assert(deploys[i].Log, check.Equals, "")
		c.Assert(deploys[i].Diff, check.Equals, "")
	}
}

func (s *S) TestListAppDeploysWithImage(c *check.C) {
	s.conn.Deploys().RemoveAll(nil)
	a := App{Name: "g1"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	insert := []interface{}{
		DeployData{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second), Image: "registry.tsuru.globoi.com/tsuru/app-example:v2"},
		DeployData{App: "g1", Timestamp: time.Now(), Image: "127.0.0.1:5000/tsuru/app-tsuru-dashboard:v1"},
	}
	expectedDeploy := []interface{}{
		DeployData{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second), Image: "v2"},
		DeployData{App: "g1", Timestamp: time.Now(), Image: "v1"},
	}
	s.conn.Deploys().Insert(insert...)
	defer s.conn.Deploys().RemoveAll(bson.M{"app": a.Name})
	expected := []DeployData{expectedDeploy[1].(DeployData), expectedDeploy[0].(DeployData)}
	deploys, err := ListDeploys(nil, 0, 0)
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

func (s *S) TestListFilteredDeploys(c *check.C) {
	team := &auth.Team{Name: "team"}
	err := s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	a := App{
		Name:     "g1",
		Platform: "zend",
		Teams:    []string{s.team.Name},
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
	insert := []interface{}{
		DeployData{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second)},
		DeployData{App: "ge", Timestamp: time.Now(), Image: "app-image"},
	}
	s.conn.Deploys().Insert(insert...)
	defer s.conn.Deploys().RemoveAll(nil)
	expected := []DeployData{insert[1].(DeployData), insert[0].(DeployData)}
	expected[0].CanRollback = true
	normalizeTS(expected)
	f := &Filter{}
	f.ExtraIn("teams", team.Name)
	deploys, err := ListDeploys(f, 0, 0)
	c.Assert(err, check.IsNil)
	normalizeTS(deploys)
	c.Assert(deploys, check.DeepEquals, []DeployData{expected[0]})
	f = &Filter{}
	f.ExtraIn("name", "g1")
	deploys, err = ListDeploys(f, 0, 0)
	c.Assert(err, check.IsNil)
	normalizeTS(deploys)
	c.Assert(deploys, check.DeepEquals, []DeployData{expected[1]})
}

func normalizeTS(deploys []DeployData) {
	for i := 0; i < len(deploys); i++ {
		ts := deploys[i].Timestamp
		deploys[i].Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
		deploys[i].ID = "-ignored-"
	}
}

func (s *S) TestListAllDeploysSkipAndLimit(c *check.C) {
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	defer user.Delete()
	team := &auth.Team{Name: "team"}
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
	deploys, err := ListDeploys(nil, 1, 2)
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

func (s *S) TestGetDeploy(c *check.C) {
	a := App{
		Name:     "g1",
		Platform: "zend",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.conn.Deploys().RemoveAll(nil)
	newDeploy := DeployData{ID: bson.NewObjectId(), App: "g1", Timestamp: time.Now()}
	err = s.conn.Deploys().Insert(&newDeploy)
	c.Assert(err, check.IsNil)
	defer s.conn.Deploys().Remove(bson.M{"name": newDeploy.App})
	lastDeploy, err := GetDeploy(newDeploy.ID.Hex())
	c.Assert(err, check.IsNil)
	ts := lastDeploy.Timestamp
	lastDeploy.Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
	ts = newDeploy.Timestamp
	newDeploy.Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
	c.Assert(lastDeploy.ID, check.Equals, newDeploy.ID)
	c.Assert(lastDeploy.App, check.Equals, newDeploy.App)
	c.Assert(lastDeploy.Timestamp, check.Equals, newDeploy.Timestamp)
}

func (s *S) TestGetDeployNotFound(c *check.C) {
	idTest := bson.NewObjectId()
	deploy, err := GetDeploy(idTest.Hex())
	c.Assert(err.Error(), check.Equals, "not found")
	c.Assert(deploy, check.IsNil)
}

func (s *S) TestGetDeployInvalidHex(c *check.C) {
	lastDeploy, err := GetDeploy("abc123")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "id parameter is not ObjectId: abc123")
	c.Assert(lastDeploy, check.IsNil)
}

func (s *S) TestDeploySaveDataAndDiff(c *check.C) {
	a := App{Name: "someApp"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	opts := DeployOptions{
		App:    &a,
		Image:  "myimage",
		Commit: "",
	}
	err = saveDeployData(&opts, "diff", "", time.Second, nil)
	c.Assert(err, check.IsNil)
	deploys, err := ListDeploys(nil, 0, 0)
	c.Assert(err, check.IsNil)
	result, err := GetDeploy(deploys[0].ID.Hex())
	c.Assert(err, check.IsNil)
	c.Assert(result.Image, check.Equals, "diff")
	c.Assert(result.Log, check.Equals, "")
	c.Assert(result.Diff, check.DeepEquals, "")
	err = SaveDiffData("testDiff", a.Name)
	c.Assert(err, check.IsNil)
	deploys, err = ListDeploys(nil, 0, 0)
	c.Assert(err, check.IsNil)
	result, err = GetDeploy(deploys[0].ID.Hex())
	c.Assert(err, check.IsNil)
	c.Assert(result.Image, check.Equals, "diff")
	c.Assert(result.Log, check.Equals, "")
	c.Assert(result.Diff, check.DeepEquals, "testDiff")
	err = saveDeployData(&opts, "myid", "mylog", time.Second, nil)
	c.Assert(err, check.IsNil)
	deploys, err = ListDeploys(nil, 0, 0)
	c.Assert(err, check.IsNil)
	result, err = GetDeploy(deploys[0].ID.Hex())
	c.Assert(err, check.IsNil)
	c.Assert(result.Image, check.Equals, "myid")
	c.Assert(result.Log, check.Equals, "mylog")
	c.Assert(result.Diff, check.DeepEquals, "testDiff")
}

func (s *S) TestDeployApp(c *check.C) {
	a := App{
		Name:     "someApp",
		Plan:     Plan{Router: "fake"},
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
		Image:        "myimage",
		Commit:       "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		OutputStream: writer,
	})
	c.Assert(err, check.IsNil)
	logs := writer.String()
	c.Assert(logs, check.Equals, "Image deploy called")
}

func (s *S) TestDeployAppWithUpdatePlatform(c *check.C) {
	a := App{
		Name:           "someApp",
		Plan:           Plan{Router: "fake"},
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
		Image:        "myimage",
		Commit:       "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		OutputStream: writer,
	})
	c.Assert(err, check.IsNil)
	logs := writer.String()
	c.Assert(logs, check.Equals, "Image deploy called")
	var updatedApp App
	s.conn.Apps().Find(bson.M{"name": "someApp"}).One(&updatedApp)
	c.Assert(updatedApp.UpdatePlatform, check.Equals, false)
}

func (s *S) TestDeployAppIncrementDeployNumber(c *check.C) {
	a := App{
		Name:     "otherapp",
		Plan:     Plan{Router: "fake"},
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
		Image:        "myimage",
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
		Plan:     Plan{Router: "fake"},
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
		Image:        "myimage",
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
	c.Assert(result["image"], check.Equals, "myimage")
	c.Assert(result["log"], check.Equals, "Image deploy called")
	c.Assert(result["user"], check.Equals, "someone@themoon")
	c.Assert(result["origin"], check.Equals, "git")
}

func (s *S) TestDeployAppSaveDeployDataOriginRollback(c *check.C) {
	a := App{
		Name:     "otherapp",
		Plan:     Plan{Router: "fake"},
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
		Origin:       "rollback",
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
		Plan:     Plan{Router: "fake"},
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
		Origin:       "app-deploy",
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

func (s *S) TestDeployAppSaveDeployDataOriginDragAndDrop(c *check.C) {
	a := App{
		Name:     "otherapp",
		Plan:     Plan{Router: "fake"},
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
		Origin:       "drag-and-drop",
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
	c.Assert(result["origin"], check.Equals, "drag-and-drop")
}

func (s *S) TestDeployAppSaveDeployErrorData(c *check.C) {
	provisioner := provisiontest.NewFakeProvisioner()
	provisioner.PrepareFailure("ImageDeploy", errors.New("deploy error"))
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
		Image:        "myimage",
		Commit:       "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		OutputStream: writer,
	})
	c.Assert(err, check.NotNil)
	var result map[string]interface{}
	s.conn.Deploys().Find(bson.M{"app": a.Name}).One(&result)
	c.Assert(result["app"], check.Equals, a.Name)
	c.Assert(result["error"], check.NotNil)
}

func (s *S) TestValidateOrigin(c *check.C) {
	c.Assert(ValidateOrigin("app-deploy"), check.Equals, true)
	c.Assert(ValidateOrigin("git"), check.Equals, true)
	c.Assert(ValidateOrigin("rollback"), check.Equals, true)
	c.Assert(ValidateOrigin("drag-and-drop"), check.Equals, true)
	c.Assert(ValidateOrigin("image"), check.Equals, true)
	c.Assert(ValidateOrigin("invalid"), check.Equals, false)
}

func (s *S) TestDeployAsleepApp(c *check.C) {
	a := App{
		Name:     "someApp",
		Plan:     Plan{Router: "fake"},
		Platform: "django",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	writer := &bytes.Buffer{}
	err = a.Sleep(writer, "web", &url.URL{Scheme: "http", Host: "proxy:1234"})
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	for _, u := range units {
		c.Assert(u.Status, check.Not(check.Equals), provision.StatusStarted)
	}
	err = Deploy(DeployOptions{
		App:          &a,
		Commit:       "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		OutputStream: writer,
	})
	c.Assert(err, check.IsNil)
	routes, err := routertest.FakeRouter.Routes(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 1)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, "http://proxy:1234"), check.Equals, false)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[0].Address.String()), check.Equals, true)
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
	c.Assert(a.Deploys, check.Equals, uint(1))
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
	opts := DeployOptions{App: &a, Image: "myimage"}
	_, err = deployToProvisioner(&opts, writer)
	c.Assert(err, check.IsNil)
	logs := writer.String()
	c.Assert(logs, check.Equals, "Image deploy called")
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
	a := App{Name: "someApp"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	opts := DeployOptions{
		App:    &a,
		Image:  "myimage",
		Commit: "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
	}
	err = saveDeployData(&opts, "myid", "mylog", time.Second, nil)
	c.Assert(err, check.IsNil)
	c.Assert(err, check.IsNil)
	defer s.conn.Deploys().RemoveAll(bson.M{"app": a.Name})
	result, err := ListDeploys(nil, 0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 1)
	c.Assert(result[0].Image, check.Equals, "myid")
	err = markDeploysAsRemoved(a.Name)
	c.Assert(err, check.IsNil)
	result, err = ListDeploys(nil, 0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 0)
	var allDeploys []DeployData
	err = s.conn.Deploys().Find(nil).All(&allDeploys)
	c.Assert(err, check.IsNil)
	c.Assert(allDeploys, check.HasLen, 1)
	c.Assert(allDeploys[0].Image, check.Equals, "myid")
	c.Assert(allDeploys[0].RemoveDate.IsZero(), check.Equals, false)
}

func (s *S) TestRollbackWithNameImage(c *check.C) {
	a := App{
		Name:     "otherapp",
		Plan:     Plan{Router: "fake"},
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	timestamp := time.Date(2013, time.November, 1, 0, 0, 0, 0, time.Local)
	duration := time.Since(timestamp)
	deploys := []DeployData{
		{App: "otherapp", Timestamp: timestamp, Duration: duration, Image: "registry.tsuru.globoi.com/tsuru/app-example:v2", CanRollback: true},
		{App: "otherapp", Timestamp: timestamp, Duration: duration, Image: "127.0.0.1:5000/tsuru/app-tsuru-dashboard:v1", CanRollback: true},
	}
	for _, deploy := range deploys {
		err = s.conn.Deploys().Insert(deploy)
		c.Assert(err, check.IsNil)
	}
	defer s.conn.Deploys().RemoveAll(nil)
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	writer := &bytes.Buffer{}
	err = Rollback(DeployOptions{
		App:          &a,
		OutputStream: writer,
		Image:        "registry.tsuru.globoi.com/tsuru/app-example:v2",
	})
	c.Assert(err, check.IsNil)
	c.Assert(writer.String(), check.Equals, "Image deploy called")
}

func (s *S) TestRollbackWithVersionImage(c *check.C) {
	a := App{
		Name:     "otherapp",
		Plan:     Plan{Router: "fake"},
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	timestamp := time.Date(2013, time.November, 1, 0, 0, 0, 0, time.Local)
	duration := time.Since(timestamp)
	deploys := []DeployData{
		{App: "otherapp", Timestamp: timestamp, Duration: duration, Image: "registry.tsuru.globoi.com/tsuru/app-example:v2", CanRollback: true},
		{App: "otherapp", Timestamp: timestamp, Duration: duration, Image: "127.0.0.1:5000/tsuru/app-tsuru-dashboard:v1", CanRollback: true},
	}
	for _, deploy := range deploys {
		err = s.conn.Deploys().Insert(deploy)
		c.Assert(err, check.IsNil)
	}
	defer s.conn.Deploys().RemoveAll(nil)
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	writer := &bytes.Buffer{}
	err = Rollback(DeployOptions{
		App:          &a,
		OutputStream: writer,
		Image:        "v2",
	})
	c.Assert(err, check.IsNil)
	c.Assert(writer.String(), check.Equals, "Rollback deploy called")
}

func (s *S) TestRollbackWithWrongVersionImage(c *check.C) {
	a := App{
		Name:     "otherapp",
		Plan:     Plan{Router: "fake"},
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	timestamp := time.Date(2013, time.November, 1, 0, 0, 0, 0, time.Local)
	duration := time.Since(timestamp)
	deploys := []DeployData{
		{App: "otherapp", Timestamp: timestamp, Duration: duration, Image: "registry.tsuru.globoi.com/tsuru/app-example:v2", CanRollback: true},
		{App: "otherapp", Timestamp: timestamp, Duration: duration, Image: "127.0.0.1:5000/tsuru/app-tsuru-dashboard:v1", CanRollback: true},
	}
	for _, deploy := range deploys {
		err = s.conn.Deploys().Insert(deploy)
		c.Assert(err, check.IsNil)
	}
	defer s.conn.Deploys().RemoveAll(nil)
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	writer := &bytes.Buffer{}
	err = Rollback(DeployOptions{
		App:          &a,
		OutputStream: writer,
		Image:        "v20",
	})
	c.Assert(err, check.IsNil)
}

func (s *S) TestGetImageName(c *check.C) {
	a := App{
		Name:     "otherapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	timestamp := time.Date(2013, time.November, 1, 0, 0, 0, 0, time.Local)
	duration := time.Since(timestamp)
	deploys := []DeployData{
		{App: "otherapp", Timestamp: timestamp, Duration: duration, Image: "registry.tsuru.globoi.com/tsuru/app-example:v2", CanRollback: true},
		{App: "otherapp", Timestamp: timestamp, Duration: duration, Image: "127.0.0.1:5000/tsuru/app-tsuru-dashboard:v1", CanRollback: true},
	}
	for _, deploy := range deploys {
		err = s.conn.Deploys().Insert(deploy)
		c.Assert(err, check.IsNil)
	}
	defer s.conn.Deploys().RemoveAll(nil)
	img, err := getImage("otherapp", "v2")
	c.Assert(err, check.IsNil)
	c.Assert(img, check.Equals, deploys[0].Image)
}

func (s *S) TestGetImageNameInexistDeploy(c *check.C) {
	apps := []App{
		{Name: "otherapp", Platform: "zend", Teams: []string{s.team.Name}},
		{Name: "otherapp2", Platform: "zend", Teams: []string{s.team.Name}},
	}
	for _, a := range apps {
		err := s.conn.Apps().Insert(a)
		c.Assert(err, check.IsNil)
	}
	defer s.conn.Apps().RemoveAll(nil)
	timestamp := time.Date(2013, time.November, 1, 0, 0, 0, 0, time.Local)
	duration := time.Since(timestamp)
	deploys := []DeployData{
		{App: "otherapp", Timestamp: timestamp, Duration: duration, Image: "regiv3ry.tsuru.globoi.com/tsuru/app-example:v2", CanRollback: true},
		{App: "otherapp", Timestamp: timestamp, Duration: duration, Image: "127.0.0.1:5000/tsuru/app-tsuru-dashboard:v1", CanRollback: true},
		{App: "otherapp2", Timestamp: timestamp, Duration: duration, Image: "127.0.0.1:5000/tsuru/app-tsuru-dashboard:v3", CanRollback: true},
	}
	for _, deploy := range deploys {
		err := s.conn.Deploys().Insert(deploy)
		c.Assert(err, check.IsNil)
	}
	defer s.conn.Deploys().RemoveAll(nil)
	_, err := getImage("otherapp", "v3")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "not found")
}

func (s *S) TestDeployKind(c *check.C) {
	var tests = []struct {
		input    DeployOptions
		expected DeployKind
	}{
		{
			DeployOptions{Rollback: true},
			DeployRollback,
		},
		{
			DeployOptions{Image: "quay.io/tsuru/python"},
			DeployImage,
		},
		{
			DeployOptions{File: ioutil.NopCloser(bytes.NewBuffer(nil))},
			DeployUpload,
		},
		{
			DeployOptions{File: ioutil.NopCloser(bytes.NewBuffer(nil)), Build: true},
			DeployUploadBuild,
		},
		{
			DeployOptions{Commit: "abcef48439"},
			DeployGit,
		},
		{
			DeployOptions{},
			DeployArchiveURL,
		},
	}
	for _, t := range tests {
		c.Check(t.input.Kind(), check.Equals, t.expected)
	}
}
