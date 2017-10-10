// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/url"
	"strings"
	"time"

	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/types"
	serviceTypes "github.com/tsuru/tsuru/types/service"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func insertDeploysAsEvents(data []DeployData, c *check.C) []*event.Event {
	evts := make([]*event.Event, len(data))
	for i, d := range data {
		evt, err := event.New(&event.Opts{
			Target:   event.Target{Type: "app", Value: d.App},
			Kind:     permission.PermAppDeploy,
			RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: d.User},
			Allowed:  event.Allowed(permission.PermApp),
			CustomData: DeployOptions{
				Commit: d.Commit,
				Origin: d.Origin,
			},
		})
		evt.StartTime = d.Timestamp
		c.Assert(err, check.IsNil)
		evt.Logf(d.Log)
		err = evt.SetOtherCustomData(map[string]string{"diff": d.Diff})
		c.Assert(err, check.IsNil)
		err = evt.DoneCustomData(nil, map[string]string{"image": d.Image})
		c.Assert(err, check.IsNil)
		evts[i] = evt
	}
	return evts
}

func (s *S) TestListAppDeploysMarshalJSON(c *check.C) {
	a := App{Name: "g1", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	insert := []DeployData{
		{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second), Log: "logs", Diff: "diff", Origin: "app-deploy"},
		{App: "g1", Timestamp: time.Now().Add(-1800 * time.Second), Log: "logs", Diff: "diff"},
		{App: "g1", Timestamp: time.Now(), Log: "logs", Diff: "diff", Commit: "abcdef1234567890"},
	}
	insertDeploysAsEvents(insert, c)
	deploys, err := ListDeploys(nil, 0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(deploys, check.HasLen, 3)
	data, err := json.Marshal(&deploys)
	c.Assert(err, check.IsNil)
	err = json.Unmarshal(data, &deploys)
	c.Assert(err, check.IsNil)
	origins := []string{"git", "", "app-deploy"}
	expected := []DeployData{insert[2], insert[1], insert[0]}
	for i := 0; i < 3; i++ {
		c.Assert(deploys[i].App, check.Equals, expected[i].App)
		c.Assert(deploys[i].Commit, check.Equals, expected[i].Commit)
		c.Assert(deploys[i].Error, check.Equals, expected[i].Error)
		c.Assert(deploys[i].Image, check.Equals, expected[i].Image)
		c.Assert(deploys[i].User, check.Equals, expected[i].User)
		c.Assert(deploys[i].CanRollback, check.Equals, expected[i].CanRollback)
		c.Assert(deploys[i].Log, check.Equals, "")
		c.Assert(deploys[i].Diff, check.Equals, "")
		c.Assert(deploys[i].Origin, check.Equals, origins[i])
	}
}

func (s *S) TestListAppDeploys(c *check.C) {
	a := App{Name: "g1", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	insert := []DeployData{
		{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second), Log: "logs", Diff: "diff"},
		{App: "g1", Timestamp: time.Now(), Log: "logs", Diff: "diff"},
	}
	insertDeploysAsEvents(insert, c)
	deploys, err := ListDeploys(nil, 0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(deploys, check.HasLen, 2)
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
	a := App{Name: "g1", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	insert := []DeployData{
		{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second), Image: "registry.somewhere/tsuru/app-example:v2"},
		{App: "g1", Timestamp: time.Now(), Image: "127.0.0.1:5000/tsuru/app-tsuru-dashboard:v1"},
	}
	expectedDeploy := []DeployData{
		{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second), Image: "v2"},
		{App: "g1", Timestamp: time.Now(), Image: "v1"},
	}
	insertDeploysAsEvents(insert, c)
	expected := []DeployData{expectedDeploy[1], expectedDeploy[0]}
	deploys, err := ListDeploys(nil, 0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(deploys, check.HasLen, 2)
	normalizeTS(deploys)
	normalizeTS(expected)
	for i := 0; i < 2; i++ {
		expected[i].ID = deploys[i].ID
	}
	c.Assert(deploys, check.DeepEquals, expected)
}

func (s *S) TestListFilteredDeploys(c *check.C) {
	team := types.Team{Name: "team"}
	err := serviceTypes.Team().Insert(team)
	c.Assert(err, check.IsNil)
	a := App{
		Name:      "g1",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	a = App{
		Name:      "ge",
		Platform:  "zend",
		TeamOwner: team.Name,
	}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName("ge", "app-image")
	c.Assert(err, check.IsNil)
	insert := []DeployData{
		{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second)},
		{App: "ge", Timestamp: time.Now(), Image: "app-image"},
	}
	insertDeploysAsEvents(insert, c)
	expected := []DeployData{insert[1], insert[0]}
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
	for i := range deploys {
		deploys[i].Timestamp = time.Unix(deploys[i].Timestamp.Unix(), 0)
		deploys[i].Duration = 0
		deploys[i].ID = "-ignored-"
	}
}

func (s *S) TestListAllDeploysSkipAndLimit(c *check.C) {
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	team := &types.Team{Name: "team"}
	c.Assert(err, check.IsNil)
	a := App{
		Name:      "app1",
		Platform:  "zend",
		Teams:     []string{team.Name},
		TeamOwner: s.team.Name,
	}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	insert := []DeployData{
		{App: "app1", Commit: "v1", Timestamp: time.Now().Add(-30 * time.Second)},
		{App: "app1", Commit: "v2", Timestamp: time.Now().Add(-20 * time.Second)},
		{App: "app1", Commit: "v3", Timestamp: time.Now().Add(-10 * time.Second)},
		{App: "app1", Commit: "v4", Timestamp: time.Now()},
	}
	insertDeploysAsEvents(insert, c)
	expected := []DeployData{insert[2], insert[1]}
	expected[0].Origin = "git"
	expected[1].Origin = "git"
	deploys, err := ListDeploys(nil, 1, 2)
	c.Assert(err, check.IsNil)
	c.Assert(deploys, check.HasLen, 2)
	normalizeTS(deploys)
	normalizeTS(expected)
	c.Assert(deploys, check.DeepEquals, expected)
}

func (s *S) TestGetDeploy(c *check.C) {
	a := App{
		Name:      "g1",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	newDeploy := DeployData{App: "g1", Timestamp: time.Now()}
	evts := insertDeploysAsEvents([]DeployData{newDeploy}, c)
	lastDeploy, err := GetDeploy(evts[0].UniqueID.Hex())
	c.Assert(err, check.IsNil)
	lastDeploy.Timestamp = time.Unix(lastDeploy.Timestamp.Unix(), 0)
	newDeploy.Timestamp = time.Unix(newDeploy.Timestamp.Unix(), 0)
	c.Assert(lastDeploy.ID, check.Equals, evts[0].UniqueID)
	c.Assert(lastDeploy.App, check.Equals, newDeploy.App)
	c.Assert(lastDeploy.Timestamp, check.Equals, newDeploy.Timestamp)
}

func (s *S) TestGetDeployNotFound(c *check.C) {
	idTest := bson.NewObjectId()
	deploy, err := GetDeploy(idTest.Hex())
	c.Assert(err, check.Equals, event.ErrEventNotFound)
	c.Assert(deploy, check.IsNil)
}

func (s *S) TestGetDeployInvalidHex(c *check.C) {
	lastDeploy, err := GetDeploy("abc123")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "id parameter is not ObjectId: abc123")
	c.Assert(lastDeploy, check.IsNil)
}

func (s *S) TestBuildApp(c *check.C) {
	a := App{
		Name:      "some-app",
		Platform:  "django",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	buf := strings.NewReader("my file")
	imgID, err := Build(DeployOptions{
		App:          &a,
		OutputStream: ioutil.Discard,
		File:         ioutil.NopCloser(buf),
		FileSize:     int64(buf.Len()),
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	c.Assert(imgID, check.Equals, "registry.somewhere/"+a.TeamOwner+"/app-some-app:v1-builder")
}

func (s *S) TestDeployAppUpload(c *check.C) {
	a := App{
		Name:      "some-app",
		Platform:  "django",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	buf := strings.NewReader("my file")
	writer := &bytes.Buffer{}
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(DeployOptions{
		App:          &a,
		File:         ioutil.NopCloser(buf),
		FileSize:     int64(buf.Len()),
		OutputStream: writer,
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	logs := writer.String()
	c.Assert(logs, check.Equals, "Builder deploy called")
	var updatedApp App
	s.conn.Apps().Find(bson.M{"name": "some-app"}).One(&updatedApp)
	c.Assert(updatedApp.UpdatePlatform, check.Equals, false)
}

func (s *S) TestDeployAppImage(c *check.C) {
	a := App{
		Name:      "some-app",
		Platform:  "django",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(DeployOptions{
		App:          &a,
		Image:        "myimage",
		Commit:       "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		OutputStream: writer,
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	logs := writer.String()
	c.Assert(logs, check.Equals, "Builder deploy called")
	var updatedApp App
	s.conn.Apps().Find(bson.M{"name": "some-app"}).One(&updatedApp)
	c.Assert(updatedApp.UpdatePlatform, check.Equals, true)
}

func (s *S) TestDeployAppWithUpdatedPlatform(c *check.C) {
	a := App{
		Name:           "some-app",
		Platform:       "django",
		Teams:          []string{s.team.Name},
		UpdatePlatform: true,
		TeamOwner:      s.team.Name,
		Router:         "fake",
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	buf := strings.NewReader("my file")
	writer := &bytes.Buffer{}
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(DeployOptions{
		App:          &a,
		File:         ioutil.NopCloser(buf),
		FileSize:     int64(buf.Len()),
		OutputStream: writer,
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	logs := writer.String()
	c.Assert(logs, check.Equals, "Builder deploy called")
	var updatedApp App
	s.conn.Apps().Find(bson.M{"name": "some-app"}).One(&updatedApp)
	c.Assert(updatedApp.UpdatePlatform, check.Equals, false)
}
func (s *S) TestDeployAppImageWithUpdatedPlatform(c *check.C) {
	a := App{
		Name:           "some-app",
		Platform:       "django",
		Teams:          []string{s.team.Name},
		UpdatePlatform: true,
		TeamOwner:      s.team.Name,
		Router:         "fake",
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(DeployOptions{
		App:          &a,
		Image:        "myimage",
		Commit:       "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		OutputStream: writer,
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	logs := writer.String()
	c.Assert(logs, check.Equals, "Builder deploy called")
	var updatedApp App
	s.conn.Apps().Find(bson.M{"name": "some-app"}).One(&updatedApp)
	c.Assert(updatedApp.UpdatePlatform, check.Equals, true)
}

func (s *S) TestDeployAppWithoutImageOrPlatform(c *check.C) {
	a := App{
		Name:           "some-app",
		Teams:          []string{s.team.Name},
		UpdatePlatform: true,
		TeamOwner:      s.team.Name,
		Router:         "fake",
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(DeployOptions{
		App:    &a,
		Commit: "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		Event:  evt,
	})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "can't deploy app without platform, if it's not an image or rollback")
}

func (s *S) TestDeployAppIncrementDeployNumber(c *check.C) {
	a := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(DeployOptions{
		App:          &a,
		Image:        "myimage",
		Commit:       "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		OutputStream: writer,
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	var updatedApp App
	s.conn.Apps().Find(bson.M{"name": a.Name}).One(&updatedApp)
	c.Assert(updatedApp.Deploys, check.Equals, uint(1))
}

func (s *S) TestDeployAppSaveDeployData(c *check.C) {
	a := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	commit := "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c"
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(DeployOptions{
		App:          &a,
		Image:        "myimage",
		Commit:       commit,
		OutputStream: writer,
		User:         "someone@themoon",
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	var updatedApp App
	s.conn.Apps().Find(bson.M{"name": a.Name}).One(&updatedApp)
	c.Assert(updatedApp.Deploys, check.Equals, uint(1))
}

func (s *S) TestDeployAppSaveDeployDataOriginRollback(c *check.C) {
	a := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(DeployOptions{
		App:          &a,
		OutputStream: writer,
		Image:        "some-image",
		Origin:       "rollback",
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	var updatedApp App
	s.conn.Apps().Find(bson.M{"name": a.Name}).One(&updatedApp)
	c.Assert(updatedApp.Deploys, check.Equals, uint(1))
}

func (s *S) TestDeployAppSaveDeployDataOriginAppDeploy(c *check.C) {
	a := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	buf := strings.NewReader("my file")
	_, err = Deploy(DeployOptions{
		App:          &a,
		OutputStream: writer,
		File:         ioutil.NopCloser(buf),
		FileSize:     int64(buf.Len()),
		Origin:       "app-deploy",
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	var updatedApp App
	s.conn.Apps().Find(bson.M{"name": a.Name}).One(&updatedApp)
	c.Assert(updatedApp.Deploys, check.Equals, uint(1))
}

func (s *S) TestDeployAppSaveDeployDataOriginDragAndDrop(c *check.C) {
	a := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	buf := strings.NewReader("my file")
	_, err = Deploy(DeployOptions{
		App:          &a,
		OutputStream: writer,
		File:         ioutil.NopCloser(buf),
		FileSize:     int64(buf.Len()),
		Origin:       "drag-and-drop",
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	var updatedApp App
	s.conn.Apps().Find(bson.M{"name": a.Name}).One(&updatedApp)
	c.Assert(updatedApp.Deploys, check.Equals, uint(1))
}

func (s *S) TestDeployAppSaveDeployErrorData(c *check.C) {
	s.provisioner.PrepareFailure("Deploy", errors.New("deploy error"))
	a := App{
		Name:      "testerrorapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(DeployOptions{
		App:          &a,
		Image:        "myimage",
		Commit:       "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		OutputStream: writer,
		Event:        evt,
	})
	c.Assert(err, check.NotNil)
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
		Name:      "some-app",
		Platform:  "django",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	writer := &bytes.Buffer{}
	err = a.Sleep(writer, "web", &url.URL{Scheme: "http", Host: "proxy:1234"})
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	for _, u := range units {
		c.Assert(u.Status, check.Not(check.Equals), provision.StatusStarted)
	}
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(DeployOptions{
		App:          &a,
		Image:        "myimage",
		Commit:       "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		OutputStream: writer,
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
}

func (s *S) TestIncrementDeploy(c *check.C) {
	a := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	incrementDeploy(&a)
	c.Assert(a.Deploys, check.Equals, uint(1))
	s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(a.Deploys, check.Equals, uint(1))
}

func (s *S) TestDeployToProvisioner(c *check.C) {
	a := App{
		Name:      "some-app",
		Platform:  "django",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	opts := DeployOptions{App: &a, Image: "myimage"}
	_, err = deployToProvisioner(&opts, evt)
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	c.Assert(evt.Log, check.Equals, "Builder deploy called")
}

func (s *S) TestDeployToProvisionerArchive(c *check.C) {
	a := App{
		Name:      "some-app",
		Platform:  "django",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	opts := DeployOptions{App: &a, ArchiveURL: "https://s3.amazonaws.com/smt/archive.tar.gz"}
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = deployToProvisioner(&opts, evt)
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	c.Assert(evt.Log, check.Equals, "Builder deploy called")
}

func (s *S) TestDeployToProvisionerUpload(c *check.C) {
	a := App{
		Name:      "some-app",
		Platform:  "django",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	buf := strings.NewReader("my file")
	opts := DeployOptions{App: &a, File: ioutil.NopCloser(buf), FileSize: int64(buf.Len())}
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = deployToProvisioner(&opts, evt)
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	c.Assert(evt.Log, check.Equals, "Builder deploy called")
}

func (s *S) TestDeployToProvisionerImage(c *check.C) {
	a := App{
		Name:      "some-app",
		Platform:  "django",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	opts := DeployOptions{App: &a, Image: "my-image-x"}
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = deployToProvisioner(&opts, evt)
	c.Assert(err, check.IsNil)
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	c.Assert(evt.Log, check.Equals, "Builder deploy called")
}

func (s *S) TestRollbackWithNameImage(c *check.C) {
	a := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	imgID, err := Deploy(DeployOptions{
		App:          &a,
		OutputStream: writer,
		Image:        "registry.somewhere/tsuru/app-example:v2",
		Rollback:     true,
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	c.Assert(writer.String(), check.Equals, "Rollback deploy called")
	c.Assert(imgID, check.Equals, "registry.somewhere/tsuru/app-example:v2")
	var updatedApp App
	s.conn.Apps().Find(bson.M{"name": "otherapp"}).One(&updatedApp)
	c.Assert(updatedApp.UpdatePlatform, check.Equals, true)
}

func (s *S) TestRollbackWithVersionImage(c *check.C) {
	a := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName("otherapp", "registry.somewhere/tsuru/app-otherapp:v2")
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	imgID, err := Deploy(DeployOptions{
		App:          &a,
		OutputStream: writer,
		Image:        "v2",
		Rollback:     true,
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	c.Assert(writer.String(), check.Equals, "Rollback deploy called")
	c.Assert(imgID, check.Equals, "registry.somewhere/tsuru/app-otherapp:v2")
	var updatedApp App
	s.conn.Apps().Find(bson.M{"name": "otherapp"}).One(&updatedApp)
	c.Assert(updatedApp.UpdatePlatform, check.Equals, true)
}

func (s *S) TestRollbackWithWrongVersionImage(c *check.C) {
	a := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName("otherapp", "registry.somewhere/tsuru/app-example:v1")
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	imgID, err := Deploy(DeployOptions{
		App:          &a,
		OutputStream: writer,
		Image:        "v20",
		Rollback:     true,
		Event:        evt,
	})
	c.Assert(err, check.NotNil)
	e, ok := err.(*image.InvalidVersionErr)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Image, check.Equals, "v20")
	c.Assert(imgID, check.Equals, "")
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
		c.Check(t.input.GetKind(), check.Equals, t.expected)
		c.Check(t.input.Kind, check.Equals, t.expected)
	}
}

func (s *S) TestMigrateDeploysToEvents(c *check.C) {
	a := App{Name: "g1", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	now := time.Unix(time.Now().Unix(), 0)
	insert := []DeployData{
		{
			App:       "g1",
			Timestamp: now.Add(-3600 * time.Second),
			Log:       "logs",
			Diff:      "diff",
			Duration:  10 * time.Second,
			Commit:    "c1",
			Error:     "e1",
			Origin:    "app-deploy",
			User:      "admin@example.com",
		},
		{
			App:       "g1",
			Timestamp: now,
			Log:       "logs",
			Diff:      "diff",
			Duration:  10 * time.Second,
			Commit:    "c2",
			Error:     "e2",
			Origin:    "app-deploy",
			User:      "admin@example.com",
		},
	}
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	oldDeploysColl := conn.Collection("deploys")
	for _, data := range insert {
		err = oldDeploysColl.Insert(data)
		c.Assert(err, check.IsNil)
	}
	err = MigrateDeploysToEvents()
	c.Assert(err, check.IsNil)
	deploys, err := ListDeploys(nil, 0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(deploys, check.HasLen, 2)
	for i := range deploys {
		id := deploys[i].ID
		var d *DeployData
		d, err = GetDeploy(id.Hex())
		c.Assert(err, check.IsNil)
		deploys[i] = *d
	}
	normalizeTS(deploys)
	normalizeTS(insert)
	c.Assert(deploys, check.DeepEquals, []DeployData{insert[1], insert[0]})
}

func (s *S) TestRebuild(c *check.C) {
	a := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	imgID, err := Deploy(DeployOptions{
		App:          &a,
		OutputStream: writer,
		Kind:         DeployRebuild,
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	c.Assert(writer.String(), check.Equals, "Builder deploy called")
	c.Assert(imgID, check.Equals, "app-image")
}

func (s *S) TestRollbackUpdate(c *check.C) {
	app := App{
		Name:      "myapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	data := image.ImageMetadata{
		Name: "tsuru/app-myapp:v1",
	}
	err = data.Save()
	c.Assert(err, check.IsNil)
	err = RollbackUpdate(app.Name, "v1", "my reason", true)
	c.Assert(err, check.IsNil)
	imgMD, err := image.GetImageMetaData("tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	c.Assert(imgMD.Reason, check.Equals, "my reason")
	c.Assert(imgMD.DisableRollback, check.Equals, true)
	err = RollbackUpdate(app.Name, "v1", "", false)
	c.Assert(err, check.IsNil)
	imgMD, err = image.GetImageMetaData("tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	c.Assert(imgMD.Reason, check.Equals, "")
	c.Assert(imgMD.DisableRollback, check.Equals, false)
	err = RollbackUpdate(app.Name, "v20", "", false)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Invalid version: v20")
}

func (s *S) TestRollbackUpdateInvalidApp(c *check.C) {
	InvalidApp := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err := RollbackUpdate(InvalidApp.Name, "v1", "", false)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Image v1 not found in app \"otherapp\"")
}
