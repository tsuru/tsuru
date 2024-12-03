// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	eventTypes "github.com/tsuru/tsuru/types/event"
	provisionTypes "github.com/tsuru/tsuru/types/provision"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	check "gopkg.in/check.v1"
)

func insertDeploysAsEvents(data []DeployData, c *check.C) []*event.Event {
	evts := make([]*event.Event, len(data))
	for i, d := range data {
		evt, err := event.New(context.TODO(), &event.Opts{
			Target:   eventTypes.Target{Type: "app", Value: d.App},
			Kind:     permission.PermAppDeploy,
			RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: d.User},
			Allowed:  event.Allowed(permission.PermApp),
			CustomData: DeployOptions{
				Commit:  d.Commit,
				Origin:  d.Origin,
				Message: d.Message,
			},
		})
		evt.StartTime = d.Timestamp
		c.Assert(err, check.IsNil)
		evt.Logf(d.Log)
		err = evt.SetOtherCustomData(context.TODO(), map[string]string{"diff": d.Diff})
		c.Assert(err, check.IsNil)
		err = evt.DoneCustomData(context.TODO(), nil, map[string]string{"image": d.Image})
		c.Assert(err, check.IsNil)
		evts[i] = evt
	}
	return evts
}

func (s *S) TestListAppDeploysMarshalJSON(c *check.C) {
	a := App{Name: "g1", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	insert := []DeployData{
		{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second), Log: "logs", Diff: "diff", Origin: "app-deploy"},
		{App: "g1", Timestamp: time.Now().Add(-1800 * time.Second), Log: "logs", Diff: "diff"},
		{App: "g1", Timestamp: time.Now(), Log: "logs", Diff: "diff", Commit: "abcdef1234567890", Message: "my awesome commit..."},
	}
	insertDeploysAsEvents(insert, c)
	deploys, err := ListDeploys(context.TODO(), nil, 0, 0)
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
		c.Assert(deploys[i].Message, check.Equals, expected[i].Message)
	}
}

func (s *S) TestListAppDeploys(c *check.C) {
	a := App{Name: "g1", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	insert := []DeployData{
		{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second), Log: "logs", Diff: "diff"},
		{App: "g1", Timestamp: time.Now(), Log: "logs", Diff: "diff"},
	}
	insertDeploysAsEvents(insert, c)
	deploys, err := ListDeploys(context.TODO(), nil, 0, 0)
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
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	insert := []DeployData{
		{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second), Image: "registry.somewhere/tsuru/app-example:v2"},
		{App: "g1", Timestamp: time.Now(), Image: "127.0.0.1:5000/tsuru/app-tsuru-dashboard:v1"},
	}
	expectedDeploy := []DeployData{
		{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second), Image: "registry.somewhere/tsuru/app-example:v2", Version: 2},
		{App: "g1", Timestamp: time.Now(), Image: "127.0.0.1:5000/tsuru/app-tsuru-dashboard:v1", Version: 1},
	}
	insertDeploysAsEvents(insert, c)
	expected := []DeployData{expectedDeploy[1], expectedDeploy[0]}
	deploys, err := ListDeploys(context.TODO(), nil, 0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(deploys, check.HasLen, 2)
	normalizeTS(deploys)
	normalizeTS(expected)
	for i := 0; i < 2; i++ {
		expected[i].ID = deploys[i].ID
	}
	c.Assert(deploys, check.DeepEquals, expected)
}

func newSuccessfulAppVersion(c *check.C, app *appTypes.App) appTypes.AppVersion {
	version, err := servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
		App: app,
	})
	c.Assert(err, check.IsNil)
	err = version.CommitBuildImage()
	c.Assert(err, check.IsNil)
	err = version.CommitBaseImage()
	c.Assert(err, check.IsNil)
	err = version.CommitSuccessful()
	c.Assert(err, check.IsNil)
	return version
}

func newUnsuccessfulAppVersion(c *check.C, app *appTypes.App) appTypes.AppVersion {
	version, err := servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
		App: app,
	})
	c.Assert(err, check.IsNil)
	err = version.CommitBuildImage()
	c.Assert(err, check.IsNil)
	err = version.CommitBaseImage()
	c.Assert(err, check.IsNil)
	err = version.MarkToRemoval()
	c.Assert(err, check.IsNil)
	return version
}

func (s *S) TestListFilteredDeploys(c *check.C) {
	a := App{
		Name:      "g1",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	team := authTypes.Team{Name: "team"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{team, {Name: s.team.Name}}, nil
	}
	s.mockService.Team.OnFindByName = func(_ string) (*authTypes.Team, error) {
		return &team, nil
	}
	a = App{
		Name:      "ge",
		Platform:  "zend",
		TeamOwner: team.Name,
	}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	version := newSuccessfulAppVersion(c, &a)
	testBaseImage, err := version.BaseImageName()
	c.Check(err, check.IsNil)
	insert := []DeployData{
		{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second)},
		{App: "ge", Timestamp: time.Now(), Image: testBaseImage},
	}
	insertDeploysAsEvents(insert, c)
	expected := []DeployData{insert[1], insert[0]}
	expected[0].CanRollback = true
	expected[0].Image = "registry.somewhere/tsuru/app-ge:v1"
	expected[0].Version = version.Version()
	normalizeTS(expected)
	f := &Filter{}
	f.ExtraIn("teams", team.Name)
	deploys, err := ListDeploys(context.TODO(), f, 0, 0)
	c.Assert(err, check.IsNil)
	normalizeTS(deploys)
	c.Assert(deploys, check.DeepEquals, []DeployData{expected[0]})
	f = &Filter{}
	f.ExtraIn("name", "g1")
	deploys, err = ListDeploys(context.TODO(), f, 0, 0)
	c.Assert(err, check.IsNil)
	normalizeTS(deploys)
	c.Assert(deploys, check.DeepEquals, []DeployData{expected[1]})
	f = &Filter{}
	deploys, err = ListDeploys(context.TODO(), f, 0, 0)
	c.Assert(err, check.IsNil)
	normalizeTS(deploys)
	c.Assert(deploys, check.DeepEquals, []DeployData{expected[0], expected[1]})
}

func (s *S) TestListFilteredDeploysWithDisabledRollback(c *check.C) {
	a := App{
		Name:      "g1",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	team := authTypes.Team{Name: "team"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{team, {Name: s.team.Name}}, nil
	}
	s.mockService.Team.OnFindByName = func(_ string) (*authTypes.Team, error) {
		return &team, nil
	}
	a = App{
		Name:      "ge",
		Platform:  "zend",
		TeamOwner: team.Name,
	}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	version := newSuccessfulAppVersion(c, &a)
	version.ToggleEnabled(false, "test") // disable rollback
	testBaseImage, err := version.BaseImageName()
	c.Check(err, check.IsNil)
	insert := []DeployData{
		{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second)},
		{App: "ge", Timestamp: time.Now(), Image: testBaseImage},
	}
	insertDeploysAsEvents(insert, c)
	expected := []DeployData{insert[1], insert[0]}
	expected[0].CanRollback = false
	expected[0].Image = "registry.somewhere/tsuru/app-ge:v1"
	expected[0].Version = version.Version()
	normalizeTS(expected)
	f := &Filter{}
	f.ExtraIn("teams", team.Name)
	deploys, err := ListDeploys(context.TODO(), f, 0, 0)
	c.Assert(err, check.IsNil)
	normalizeTS(deploys)
	c.Assert(deploys, check.HasLen, 1)
	c.Assert(deploys, check.DeepEquals, []DeployData{expected[0]})
	f = &Filter{}
	f.ExtraIn("name", "g1")
	deploys, err = ListDeploys(context.TODO(), f, 0, 0)
	c.Assert(err, check.IsNil)
	normalizeTS(deploys)
	c.Assert(deploys, check.DeepEquals, []DeployData{expected[1]})
	f = &Filter{}
	deploys, err = ListDeploys(context.TODO(), f, 0, 0)
	c.Assert(err, check.IsNil)
	normalizeTS(deploys)
	c.Assert(deploys, check.DeepEquals, []DeployData{expected[0], expected[1]})
}

func normalizeTS(deploys []DeployData) {
	for i := range deploys {
		deploys[i].Timestamp = time.Unix(deploys[i].Timestamp.Unix(), 0)
		deploys[i].Duration = 0
		deploys[i].ID = primitive.ObjectID{}
	}
}

func (s *S) TestListAllDeploysSkipAndLimit(c *check.C) {
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	AuthScheme = nativeScheme
	_, err := nativeScheme.Create(context.TODO(), user)
	c.Assert(err, check.IsNil)
	team := &authTypes.Team{Name: "team"}
	c.Assert(err, check.IsNil)
	a := App{
		Name:      "app1",
		Platform:  "zend",
		Teams:     []string{team.Name},
		TeamOwner: s.team.Name,
	}
	err = CreateApp(context.TODO(), &a, s.user)
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
	deploys, err := ListDeploys(context.TODO(), nil, 1, 2)
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
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newDeploy := DeployData{App: "g1", Timestamp: time.Now()}
	evts := insertDeploysAsEvents([]DeployData{newDeploy}, c)
	lastDeploy, err := GetDeploy(context.TODO(), evts[0].UniqueID.Hex())
	c.Assert(err, check.IsNil)
	lastDeploy.Timestamp = time.Unix(lastDeploy.Timestamp.Unix(), 0)
	newDeploy.Timestamp = time.Unix(newDeploy.Timestamp.Unix(), 0)
	c.Assert(lastDeploy.ID, check.Equals, evts[0].UniqueID)
	c.Assert(lastDeploy.App, check.Equals, newDeploy.App)
	c.Assert(lastDeploy.Timestamp, check.Equals, newDeploy.Timestamp)
}

func (s *S) TestGetDeployNotFound(c *check.C) {
	idTest := primitive.NewObjectID()
	deploy, err := GetDeploy(context.TODO(), idTest.Hex())
	c.Assert(err, check.Equals, event.ErrEventNotFound)
	c.Assert(deploy, check.IsNil)
}

func (s *S) TestGetDeployInvalidHex(c *check.C) {
	lastDeploy, err := GetDeploy(context.TODO(), "abc123")
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
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	buf := strings.NewReader("my file")
	imgID, err := Build(context.TODO(), DeployOptions{
		App:          &a,
		OutputStream: io.Discard,
		File:         io.NopCloser(buf),
		FileSize:     int64(buf.Len()),
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	c.Assert(imgID, check.Equals, "registry.somewhere/"+a.TeamOwner+"/app-some-app:v1-builder")
}

func (s *S) TestDeployAppUpload(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := App{
		Name:      "some-app",
		Platform:  "django",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	buf := strings.NewReader("my file")
	writer := &bytes.Buffer{}
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(context.TODO(), DeployOptions{
		App:          &a,
		File:         io.NopCloser(buf),
		FileSize:     int64(buf.Len()),
		OutputStream: writer,
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	logs := writer.String()
	c.Assert(logs, check.Matches, "(?s).*Builder deploy called.*")
	var updatedApp App
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "some-app"}).Decode(&updatedApp)
	c.Assert(err, check.IsNil)
	c.Assert(updatedApp.UpdatePlatform, check.Equals, false)
}

func (s *S) TestDeployAppImage(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := App{
		Name:      "some-app",
		Platform:  "django",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(context.TODO(), DeployOptions{
		App:          &a,
		Image:        "myimage",
		Commit:       "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		OutputStream: writer,
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	logs := writer.String()
	c.Assert(logs, check.Matches, "(?s).*Builder deploy called.*")
	var updatedApp App
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "some-app"}).Decode(&updatedApp)
	c.Assert(err, check.IsNil)
	c.Assert(updatedApp.UpdatePlatform, check.Equals, true)
}

func (s *S) TestDeployAppWithUpdatedPlatform(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := App{
		Name:           "some-app",
		Platform:       "django",
		Teams:          []string{s.team.Name},
		UpdatePlatform: true,
		TeamOwner:      s.team.Name,
		Router:         "fake",
	}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	buf := strings.NewReader("my file")
	writer := &bytes.Buffer{}
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(context.TODO(), DeployOptions{
		App:          &a,
		File:         io.NopCloser(buf),
		FileSize:     int64(buf.Len()),
		OutputStream: writer,
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	logs := writer.String()
	c.Assert(logs, check.Matches, "(?s).*Builder deploy called.*")
	var updatedApp App
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "some-app"}).Decode(&updatedApp)
	c.Assert(err, check.IsNil)
	c.Assert(updatedApp.UpdatePlatform, check.Equals, false)
}

func (s *S) TestDeployAppImageWithUpdatedPlatform(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := App{
		Name:           "some-app",
		Platform:       "django",
		Teams:          []string{s.team.Name},
		UpdatePlatform: true,
		TeamOwner:      s.team.Name,
		Router:         "fake",
	}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(context.TODO(), DeployOptions{
		App:          &a,
		Image:        "myimage",
		Commit:       "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		OutputStream: writer,
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	logs := writer.String()
	c.Assert(logs, check.Matches, "(?s).*Builder deploy called.*")
	var updatedApp App
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "some-app"}).Decode(&updatedApp)
	c.Assert(err, check.IsNil)
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
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(context.TODO(), DeployOptions{
		App:          &a,
		Commit:       "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		Event:        evt,
		OutputStream: io.Discard,
	})
	c.Assert(err, check.ErrorMatches, "(?s).*can't deploy app without platform, if it's not an image, dockerfile or rollback.*")
}

func (s *S) TestDeployAppIncrementDeployNumber(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(context.TODO(), DeployOptions{
		App:          &a,
		Image:        "myimage",
		Commit:       "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		OutputStream: writer,
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	var updatedApp App
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": a.Name}).Decode(&updatedApp)
	c.Assert(err, check.IsNil)
	c.Assert(updatedApp.Deploys, check.Equals, uint(1))
}

func (s *S) TestDeployAppSaveDeployData(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	commit := "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c"
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(context.TODO(), DeployOptions{
		App:          &a,
		Image:        "myimage",
		Commit:       commit,
		OutputStream: writer,
		User:         "someone@themoon",
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	var updatedApp App
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": a.Name}).Decode(&updatedApp)
	c.Assert(err, check.IsNil)
	c.Assert(updatedApp.Deploys, check.Equals, uint(1))
}

func (s *S) TestDeployAppSaveDeployDataOriginRollback(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(context.TODO(), DeployOptions{
		App:          &a,
		OutputStream: writer,
		Image:        "some-image",
		Origin:       "rollback",
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	var updatedApp App
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": a.Name}).Decode(&updatedApp)
	c.Assert(err, check.IsNil)
	c.Assert(updatedApp.Deploys, check.Equals, uint(1))
}

func (s *S) TestDeployAppSaveDeployDataOriginAppDeploy(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	buf := strings.NewReader("my file")
	_, err = Deploy(context.TODO(), DeployOptions{
		App:          &a,
		OutputStream: writer,
		File:         io.NopCloser(buf),
		FileSize:     int64(buf.Len()),
		Origin:       "app-deploy",
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	var updatedApp App
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": a.Name}).Decode(&updatedApp)
	c.Assert(err, check.IsNil)
	c.Assert(updatedApp.Deploys, check.Equals, uint(1))
}

func (s *S) TestDeployAppSaveDeployDataOriginDragAndDrop(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	buf := strings.NewReader("my file")
	_, err = Deploy(context.TODO(), DeployOptions{
		App:          &a,
		OutputStream: writer,
		File:         io.NopCloser(buf),
		FileSize:     int64(buf.Len()),
		Origin:       "drag-and-drop",
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	var updatedApp App
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": a.Name}).Decode(&updatedApp)
	c.Assert(err, check.IsNil)
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
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(context.TODO(), DeployOptions{
		App:          &a,
		Image:        "myimage",
		Commit:       "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		OutputStream: writer,
		Event:        evt,
	})
	c.Assert(err, check.NotNil)
}

func (s *S) TestDeployAppShowLogLinesOnStartupError(c *check.C) {
	s.provisioner.PrepareFailure("Deploy", provision.ErrUnitStartup{Err: errors.New("deploy error")})
	a := App{
		Name:      "testerrorapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = servicemanager.LogService.Add(a.Name, "msg1", "src1", "unit1")
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(context.TODO(), DeployOptions{
		App:          &a,
		Image:        "myimage",
		Commit:       "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		OutputStream: writer,
		Event:        evt,
	})
	c.Assert(err, check.ErrorMatches, `(?s).*---- ERROR during deploy: ----.*deploy error.*---- Last 1 log messages: ----.*\[src1\]\[unit1\]: msg1.*`)
}

func (s *S) TestDeployAppShowLogEmbeddedLinesOnStartupError(c *check.C) {
	s.provisioner.PrepareFailure("Deploy", provision.ErrUnitStartup{
		Err: errors.New("deploy error"),
		CrashedUnitsLogs: []appTypes.Applog{
			{
				Name:    "myapp",
				Source:  "web",
				Unit:    "myapp-web-001",
				Message: "embedded log message",
			},
		},
	})
	a := App{
		Name:      "testerrorapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = servicemanager.LogService.Add(a.Name, "msg1", "src1", "unit1")
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(context.TODO(), DeployOptions{
		App:          &a,
		Image:        "myimage",
		Commit:       "1ee1f1084927b3a5db59c9033bc5c4abefb7b93c",
		OutputStream: writer,
		Event:        evt,
	})
	c.Assert(err, check.ErrorMatches, `(?s).*---- ERROR during deploy: ----.*deploy error.*---- Last 1 log messages: ----.*\[web\]\[myapp-web-001\]: embedded log message.*`)
}

func (s *S) TestValidateOrigin(c *check.C) {
	c.Assert(ValidateOrigin("app-deploy"), check.Equals, true)
	c.Assert(ValidateOrigin("git"), check.Equals, true)
	c.Assert(ValidateOrigin("rollback"), check.Equals, true)
	c.Assert(ValidateOrigin("drag-and-drop"), check.Equals, true)
	c.Assert(ValidateOrigin("image"), check.Equals, true)
	c.Assert(ValidateOrigin("invalid"), check.Equals, false)
}

func (s *S) TestIncrementDeploy(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
	}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	incrementDeploy(context.TODO(), &a)
	c.Assert(a.Deploys, check.Equals, uint(1))
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": a.Name}).Decode(&a)
	c.Assert(err, check.IsNil)
	c.Assert(a.Deploys, check.Equals, uint(1))
}

func (s *S) TestDeployToProvisioner(c *check.C) {
	a := App{
		Name:      "some-app",
		Platform:  "django",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	opts := DeployOptions{App: &a, Image: "myimage"}
	_, err = deployToProvisioner(context.TODO(), &opts, evt)
	c.Assert(err, check.IsNil)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(evt.Log(), check.Matches, ".*Builder deploy called")
}

func (s *S) TestDeployToProvisionerArchive(c *check.C) {
	a := App{
		Name:      "some-app",
		Platform:  "django",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	opts := DeployOptions{App: &a, ArchiveURL: "https://s3.amazonaws.com/smt/archive.tar.gz"}
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = deployToProvisioner(context.TODO(), &opts, evt)
	c.Assert(err, check.IsNil)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(evt.Log(), check.Matches, ".*Builder deploy called")
}

func (s *S) TestDeployToProvisionerUpload(c *check.C) {
	a := App{
		Name:      "some-app",
		Platform:  "django",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	buf := strings.NewReader("my file")
	opts := DeployOptions{App: &a, File: io.NopCloser(buf), FileSize: int64(buf.Len())}
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = deployToProvisioner(context.TODO(), &opts, evt)
	c.Assert(err, check.IsNil)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(evt.Log(), check.Matches, ".*Builder deploy called")
}

func (s *S) TestDeployToProvisionerImage(c *check.C) {
	a := App{
		Name:      "some-app",
		Platform:  "django",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	opts := DeployOptions{App: &a, Image: "my-image-x"}
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = deployToProvisioner(context.TODO(), &opts, evt)
	c.Assert(err, check.IsNil)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(evt.Log(), check.Matches, ".*Builder deploy called")
}

func (s *S) TestRollbackWithNameImage(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	version := newSuccessfulAppVersion(c, &a)
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	testBaseImage, err := version.BaseImageName()
	c.Assert(err, check.IsNil)
	imgID, err := Deploy(context.TODO(), DeployOptions{
		App:          &a,
		OutputStream: writer,
		Image:        testBaseImage,
		Rollback:     true,
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	c.Assert(writer.String(), check.Matches, "(?s).*Builder deploy called.*")
	c.Assert(imgID, check.Equals, testBaseImage)
	var updatedApp App
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "otherapp"}).Decode(&updatedApp)
	c.Assert(err, check.IsNil)
	c.Assert(updatedApp.UpdatePlatform, check.Equals, true)
}

func (s *S) TestRollbackWithVersionImage(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	version := newSuccessfulAppVersion(c, &a)
	writer := &bytes.Buffer{}
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	imgID, err := Deploy(context.TODO(), DeployOptions{
		App:          &a,
		OutputStream: writer,
		Image:        fmt.Sprintf("v%d", version.Version()),
		Rollback:     true,
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	testBaseImage, err := version.BaseImageName()
	c.Assert(err, check.IsNil)
	c.Assert(writer.String(), check.Matches, "(?s).*Builder deploy called.*")
	c.Assert(imgID, check.Equals, testBaseImage)
	var updatedApp App
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "otherapp"}).Decode(&updatedApp)
	c.Assert(err, check.IsNil)
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
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	writer := &bytes.Buffer{}
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	imgID, err := Deploy(context.TODO(), DeployOptions{
		App:          &a,
		OutputStream: writer,
		Image:        "v20",
		Rollback:     true,
		Event:        evt,
	})
	c.Assert(err, check.NotNil)
	err = errors.Cause(err)
	e, ok := err.(appTypes.ErrInvalidVersion)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Version, check.Equals, "v20")
	c.Assert(imgID, check.Equals, "")
}

func (s *S) TestRollbackWithVersionMarkedToRemoved(c *check.C) {
	a := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newUnsuccessfulAppVersion(c, &a)
	writer := &bytes.Buffer{}
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = Deploy(context.TODO(), DeployOptions{
		App:          &a,
		OutputStream: writer,
		Image:        "v1",
		Rollback:     true,
		Event:        evt,
	})
	c.Assert(err, check.NotNil)
	err = errors.Cause(err)
	c.Assert(err, check.Equals, appTypes.ErrVersionMarkedToRemoval)
}

func (s *S) TestDeployKind(c *check.C) {
	var tests = []struct {
		input    DeployOptions
		expected provisionTypes.DeployKind
	}{
		{
			DeployOptions{},
			provisionTypes.DeployKind(""), // unknown kind
		},
		{
			DeployOptions{Rollback: true},
			provisionTypes.DeployRollback,
		},
		{
			DeployOptions{Image: "quay.io/tsuru/python"},
			provisionTypes.DeployImage,
		},
		{
			DeployOptions{File: io.NopCloser(bytes.NewBuffer(nil))},
			provisionTypes.DeployUpload,
		},
		{
			DeployOptions{File: io.NopCloser(bytes.NewBuffer(nil)), Build: true},
			provisionTypes.DeployUploadBuild,
		},
		{
			DeployOptions{Commit: "abcef48439"},
			provisionTypes.DeployGit,
		},
		{
			DeployOptions{ArchiveURL: "https://example.com/my-app/v123.tgz"},
			provisionTypes.DeployArchiveURL,
		},
	}
	for _, t := range tests {
		c.Check(t.input.GetKind(), check.Equals, t.expected)
		c.Check(t.input.Kind, check.Equals, t.expected)
	}
}

func (s *S) TestRebuild(c *check.C) {
	a := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	writer := &bytes.Buffer{}
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	imgID, err := Deploy(context.TODO(), DeployOptions{
		App:          &a,
		OutputStream: writer,
		Kind:         provisionTypes.DeployRebuild,
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	c.Assert(writer.String(), check.Matches, "(?s).*Builder deploy called.*")
	c.Assert(imgID, check.Equals, "registry.somewhere/tsuru/app-otherapp:v1")
}

func (s *S) TestRollbackUpdate(c *check.C) {
	app := App{
		Name:      "myapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	version := newSuccessfulAppVersion(c, &app)

	err = RollbackUpdate(context.TODO(), &app, fmt.Sprintf("v%d", version.Version()), "my reason", true)
	c.Assert(err, check.IsNil)

	versions, err := servicemanager.AppVersion.AppVersions(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	c.Assert(versions.Versions[version.Version()].Disabled, check.Equals, true)
	c.Assert(versions.Versions[version.Version()].DisabledReason, check.Equals, "my reason")

	err = RollbackUpdate(context.TODO(), &app, fmt.Sprintf("v%d", version.Version()), "", false)
	c.Assert(err, check.IsNil)

	versions, err = servicemanager.AppVersion.AppVersions(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	c.Assert(versions.Versions[version.Version()].Disabled, check.Equals, false)
	c.Assert(versions.Versions[version.Version()].DisabledReason, check.Equals, "")

	err = RollbackUpdate(context.TODO(), &app, "v20", "", false)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "Invalid version: v20")
}

func (s *S) TestRollbackUpdateInvalidApp(c *check.C) {
	invalidApp := App{
		Name:      "otherapp",
		Platform:  "zend",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err := RollbackUpdate(context.TODO(), &invalidApp, "v1", "", false)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "no versions available for app")
}
