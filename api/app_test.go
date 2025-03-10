// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cezarsa/form"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db/storagev2"
	tsuruEnvs "github.com/tsuru/tsuru/envs"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/servicemanager"
	apiTypes "github.com/tsuru/tsuru/types/api"
	appTypes "github.com/tsuru/tsuru/types/app"
	imgTypes "github.com/tsuru/tsuru/types/app/image"
	authTypes "github.com/tsuru/tsuru/types/auth"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	"github.com/tsuru/tsuru/types/cache"
	logTypes "github.com/tsuru/tsuru/types/log"
	permTypes "github.com/tsuru/tsuru/types/permission"
	provTypes "github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/types/quota"
	tagTypes "github.com/tsuru/tsuru/types/tag"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	check "gopkg.in/check.v1"
)

func (s *S) setupMockForCreateApp(c *check.C, platName string) {
	s.mockService.Platform.OnFindByName = func(name string) (*appTypes.Platform, error) {
		c.Assert(name, check.Equals, platName)
		return &appTypes.Platform{Name: platName}, nil
	}
}

func (s *S) TestAppListFilteringByPlatform(c *check.C) {
	ctx := context.Background()
	app1 := appTypes.App{Name: "app1", Platform: "zend", TeamOwner: s.team.Name, Tags: []string{"a"}}
	err := app.CreateApp(context.TODO(), &app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := appTypes.App{Name: "app2", Platform: "python", TeamOwner: s.team.Name, Tags: []string{"b", "c"}}
	err = app.CreateApp(context.TODO(), &app2, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/apps?platform=zend", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	apps := []appTypes.App{}
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	expected := []appTypes.App{app1}
	c.Assert(apps, check.HasLen, len(expected))
	for i, a := range apps {
		c.Assert(a.Name, check.DeepEquals, expected[i].Name)
		units, err := app.AppUnits(ctx, &a)
		c.Assert(err, check.IsNil)
		expectedUnits, err := app.AppUnits(ctx, &expected[i])
		c.Assert(err, check.IsNil)
		c.Assert(units, check.DeepEquals, expectedUnits)
		c.Assert(a.Tags, check.DeepEquals, expected[i].Tags)
	}
}

func (s *S) TestAppListFilteringByTeamOwner(c *check.C) {
	ctx := context.Background()
	team := authTypes.Team{Name: "angra"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{team, {Name: s.team.Name}}, nil
	}
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		return &authTypes.Team{Name: name}, nil
	}
	app1 := appTypes.App{Name: "app1", Platform: "zend", TeamOwner: s.team.Name, Tags: []string{"tag 1"}}
	err := app.CreateApp(context.TODO(), &app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := appTypes.App{Name: "app2", Platform: "zend", TeamOwner: team.Name, Tags: []string{"tag 2"}}
	err = app.CreateApp(context.TODO(), &app2, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", fmt.Sprintf("/apps?teamOwner=%s", s.team.Name), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	apps := []appTypes.App{}
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	expected := []appTypes.App{app1}
	c.Assert(apps, check.HasLen, len(expected))
	for i, a := range apps {
		c.Assert(a.Name, check.DeepEquals, expected[i].Name)
		units, err := app.AppUnits(ctx, &a)
		c.Assert(err, check.IsNil)
		expectedUnits, err := app.AppUnits(ctx, &expected[i])
		c.Assert(err, check.IsNil)
		c.Assert(units, check.DeepEquals, expectedUnits)
		c.Assert(a.Tags, check.DeepEquals, expected[i].Tags)
	}
}

func (s *S) TestAppListFilteringByOwner(c *check.C) {
	ctx := context.Background()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	u, _ := auth.ConvertNewUser(token.User(ctx))
	app1 := appTypes.App{Name: "app1", Platform: "zend", TeamOwner: s.team.Name, Tags: []string{"mytag"}}
	err := app.CreateApp(context.TODO(), &app1, u)
	c.Assert(err, check.IsNil)
	app2 := appTypes.App{Name: "app2", Platform: "python", TeamOwner: s.team.Name, Tags: []string{"mytag"}}
	err = app.CreateApp(context.TODO(), &app2, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", fmt.Sprintf("/apps?owner=%s", u.Email), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	apps := []appTypes.App{}
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	expected := []appTypes.App{app1}
	c.Assert(apps, check.HasLen, len(expected))
	for i, a := range apps {
		c.Assert(a.Name, check.DeepEquals, expected[i].Name)
		units, err := app.AppUnits(ctx, &a)
		c.Assert(err, check.IsNil)
		expectedUnits, err := app.AppUnits(ctx, &expected[i])
		c.Assert(err, check.IsNil)
		c.Assert(units, check.DeepEquals, expectedUnits)
		c.Assert(a.Tags, check.DeepEquals, expected[i].Tags)
	}
}

func (s *S) TestAppListFilteringByTags(c *check.C) {
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	u, _ := auth.ConvertNewUser(token.User(context.TODO()))
	app1 := appTypes.App{Name: "app1", TeamOwner: s.team.Name, Tags: []string{"tag1", "tag2"}}
	err := app.CreateApp(context.TODO(), &app1, u)
	c.Assert(err, check.IsNil)
	app2 := appTypes.App{Name: "app2", TeamOwner: s.team.Name, Tags: []string{"tag2", "tag3"}}
	err = app.CreateApp(context.TODO(), &app2, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/apps?tag=tag3", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	apps := []appTypes.App{}
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps[0].Name, check.Equals, app2.Name)
	c.Assert(apps[0].Tags, check.DeepEquals, app2.Tags)
	request, err = http.NewRequest("GET", "/apps?tag=tag2&tag=tag1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder = httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	apps = []appTypes.App{}
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps[0].Name, check.Equals, app1.Name)
	c.Assert(apps[0].Tags, check.DeepEquals, app1.Tags)
}

func (s *S) TestAppListFilteringByLockState(c *check.C) {
	ctx := context.Background()
	app1 := appTypes.App{Name: "app1", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := appTypes.App{
		Name:      "app2",
		Platform:  "python",
		TeamOwner: s.team.Name,
		Lock:      appTypes.AppLock{Locked: true},
		Tags:      []string{"mytag"},
	}
	err = app.CreateApp(context.TODO(), &app2, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/apps?locked=true", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	apps := []appTypes.App{}
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	expected := []appTypes.App{app2}
	c.Assert(apps, check.HasLen, len(expected))
	for i, a := range apps {
		c.Assert(a.Name, check.DeepEquals, expected[i].Name)
		units, err := app.AppUnits(ctx, &a)
		c.Assert(err, check.IsNil)
		expectedUnits, err := app.AppUnits(context.TODO(), &expected[i])
		c.Assert(err, check.IsNil)
		c.Assert(units, check.DeepEquals, expectedUnits)
		c.Assert(a.Tags, check.DeepEquals, expected[i].Tags)
	}
}

func (s *S) TestAppListFilteringByPool(c *check.C) {
	ctx := context.Background()
	opts := []pool.AddPoolOptions{
		{Name: "pool1", Default: false, Public: true},
		{Name: "pool2", Default: false, Public: true},
	}
	for _, opt := range opts {
		err := pool.AddPool(context.TODO(), opt)
		c.Assert(err, check.IsNil)
	}
	app1 := appTypes.App{Name: "app1", Platform: "zend", Pool: opts[0].Name, TeamOwner: s.team.Name, Tags: []string{"mytag"}}
	err := app.CreateApp(context.TODO(), &app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := appTypes.App{Name: "app2", Platform: "zend", Pool: opts[1].Name, TeamOwner: s.team.Name, Tags: []string{""}}
	err = app.CreateApp(context.TODO(), &app2, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", fmt.Sprintf("/apps?pool=%s", opts[1].Name), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	apps := []appTypes.App{}
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	expected := []appTypes.App{app2}
	c.Assert(apps, check.HasLen, len(expected))
	for i, a := range apps {
		c.Assert(a.Name, check.DeepEquals, expected[i].Name)
		units, err := app.AppUnits(ctx, &a)
		c.Assert(err, check.IsNil)
		expectedUnits, err := app.AppUnits(ctx, &expected[i])
		c.Assert(err, check.IsNil)
		c.Assert(units, check.DeepEquals, expectedUnits)
		c.Assert(a.Tags, check.DeepEquals, expected[i].Tags)
	}
}

func (s *S) TestAppListFilteringByStatus(c *check.C) {
	ctx := context.Background()
	recorder := httptest.NewRecorder()
	app1 := appTypes.App{Name: "app1", Platform: "zend", TeamOwner: s.team.Name, Tags: []string{}}
	err := app.CreateApp(context.TODO(), &app1, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app1)
	requestBody := strings.NewReader("units=2&process=web")
	request, err := http.NewRequest("PUT", "/apps/app1/units", requestBody)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	request, err = http.NewRequest("POST", fmt.Sprintf("/apps/%s/stop", app1.Name), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app2 := appTypes.App{Name: "app2", Platform: "zend", TeamOwner: s.team.Name, Tags: []string{}}
	err = app.CreateApp(context.TODO(), &app2, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app2)
	requestBody = strings.NewReader("units=1&process=web")
	request, err = http.NewRequest("PUT", "/apps/app2/units", requestBody)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app3 := appTypes.App{Name: "app3", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &app3, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app3)
	request, err = http.NewRequest("GET", "/apps?status=stopped&status=started", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	apps := []appTypes.App{}
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	expected := []appTypes.App{app1, app2}
	c.Assert(apps, check.HasLen, len(expected))
	for i, a := range apps {
		c.Assert(a.Name, check.DeepEquals, expected[i].Name)
		units, err := app.AppUnits(ctx, &a)
		c.Assert(err, check.IsNil)
		expectedUnits, err := app.AppUnits(ctx, &expected[i])
		c.Assert(err, check.IsNil)
		c.Assert(units, check.DeepEquals, expectedUnits)
		c.Assert(a.Tags, check.DeepEquals, expected[i].Tags)
	}
}

func (s *S) TestAppListFilteringByStatusIgnoresInvalidValues(c *check.C) {
	ctx := context.Background()
	recorder := httptest.NewRecorder()
	app1 := appTypes.App{Name: "app1", Platform: "zend", TeamOwner: s.team.Name, Tags: []string{}}
	err := app.CreateApp(context.TODO(), &app1, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app1)
	requestBody := strings.NewReader("units=2&process=web")
	request, err := http.NewRequest("PUT", "/apps/app1/units", requestBody)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	request, err = http.NewRequest("POST", fmt.Sprintf("/apps/%s/stop", app1.Name), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app2 := appTypes.App{Name: "app2", Platform: "zend", TeamOwner: s.team.Name, Tags: []string{"tag"}}
	err = app.CreateApp(context.TODO(), &app2, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app2)
	requestBody = strings.NewReader("units=1&process=web")
	request, err = http.NewRequest("PUT", "/apps/app2/units", requestBody)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	request, err = http.NewRequest("GET", "/apps?status=invalid&status=started", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	apps := []appTypes.App{}
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	expected := []appTypes.App{app2}
	c.Assert(apps, check.HasLen, len(expected))
	for i, a := range apps {
		c.Assert(a.Name, check.DeepEquals, expected[i].Name)
		units, err := app.AppUnits(ctx, &a)
		c.Assert(err, check.IsNil)
		expectedUnits, err := app.AppUnits(ctx, &expected[i])
		c.Assert(err, check.IsNil)
		c.Assert(units, check.DeepEquals, expectedUnits)
		c.Assert(a.Tags, check.DeepEquals, expected[i].Tags)
	}
}

func (s *S) TestSimplifiedAppList(c *check.C) {
	ctx := context.Background()
	p := pool.Pool{Name: "pool1"}
	opts := pool.AddPoolOptions{Name: p.Name, Public: true}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	app1 := appTypes.App{
		Name:      "app1",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		CName:     []string{"cname.app1"},
		Pool:      "pool1",
		Tags:      []string{},
	}
	err = app.CreateApp(context.TODO(), &app1, s.user)
	c.Assert(err, check.IsNil)
	acquireDate := time.Date(2015, time.February, 12, 12, 3, 0, 0, time.Local)
	app2 := appTypes.App{
		Name:      "app2",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		CName:     []string{"cname.app2"},
		Pool:      "pool1",
		Lock: appTypes.AppLock{
			Locked:      true,
			Reason:      "wanted",
			Owner:       s.user.Email,
			AcquireDate: acquireDate,
		},
		Tags: []string{"a"},
	}
	err = app.CreateApp(context.TODO(), &app2, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/apps?simplified=true", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var apps []appTypes.App
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 2)
	c.Assert(apps[0].Name, check.Equals, app1.Name)
	c.Assert(apps[0].Platform, check.Equals, "")
	app1u, _ := app.AppUnits(ctx, &apps[0])
	c.Assert(app1u, check.HasLen, 0)
	c.Assert(apps[1].Name, check.Equals, app2.Name)
	c.Assert(app1u, check.HasLen, 0)
}

func (s *S) TestExtendedAppList(c *check.C) {
	ctx := context.Background()
	p := pool.Pool{Name: "pool1"}
	opts := pool.AddPoolOptions{Name: p.Name, Public: true}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	app1 := appTypes.App{
		Name:        "app1",
		Platform:    "zend",
		Description: "app1",
		TeamOwner:   s.team.Name,
		CName:       []string{"cname.app1"},
		Pool:        "pool1",
		Tags:        []string{},
	}
	err = app.CreateApp(context.TODO(), &app1, s.user)
	c.Assert(err, check.IsNil)
	acquireDate := time.Date(2015, time.February, 12, 12, 3, 0, 0, time.Local)
	app2 := appTypes.App{
		Name:        "app2",
		Platform:    "zend",
		Description: "app2",
		TeamOwner:   s.team.Name,
		CName:       []string{"cname.app2"},
		Pool:        "pool1",
		Lock: appTypes.AppLock{
			Locked:      true,
			Reason:      "wanted",
			Owner:       s.user.Email,
			AcquireDate: acquireDate,
		},
		Tags: []string{"a"},
	}
	err = app.CreateApp(context.TODO(), &app2, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/apps?extended=true", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var apps []appTypes.App
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 2)
	c.Assert(apps[0].Name, check.Equals, app1.Name)
	c.Assert(apps[0].Description, check.Equals, app1.Description)
	c.Assert(apps[0].Platform, check.Equals, app1.Platform)
	app1u, _ := app.AppUnits(ctx, &apps[0])
	c.Assert(app1u, check.HasLen, 0)
	c.Assert(apps[1].Name, check.Equals, app2.Name)
	c.Assert(apps[1].Description, check.Equals, app2.Description)
}

func (s *S) TestAppList(c *check.C) {
	p := pool.Pool{Name: "pool1"}
	opts := pool.AddPoolOptions{Name: p.Name, Public: true}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	app1 := appTypes.App{
		Name:      "app1",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		CName:     []string{"cname.app1"},
		Pool:      "pool1",
		Tags:      []string{},
	}
	err = app.CreateApp(context.TODO(), &app1, s.user)
	c.Assert(err, check.IsNil)
	acquireDate := time.Date(2015, time.February, 12, 12, 3, 0, 0, time.Local)
	app2 := appTypes.App{
		Name:      "app2",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		CName:     []string{"cname.app2"},
		Pool:      "pool1",
		Lock: appTypes.AppLock{
			Locked:      true,
			Reason:      "wanted",
			Owner:       s.user.Email,
			AcquireDate: acquireDate,
		},
		Tags: []string{"a"},
	}
	err = app.CreateApp(context.TODO(), &app2, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var apps []appTypes.App
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 2)
	c.Assert(apps[0].Name, check.Equals, app1.Name)
	c.Assert(apps[0].CName, check.DeepEquals, app1.CName)
	c.Assert(apps[0].Routers, check.DeepEquals, []appTypes.AppRouter{{
		Name:    "fake",
		Address: "",
		Opts:    map[string]string{},
	}})
	c.Assert(apps[0].Pool, check.Equals, app1.Pool)
	c.Assert(apps[0].Tags, check.DeepEquals, app1.Tags)
	c.Assert(apps[1].Name, check.Equals, app2.Name)
	c.Assert(apps[1].CName, check.DeepEquals, app2.CName)
	c.Assert(apps[1].Routers, check.DeepEquals, []appTypes.AppRouter{{
		Name:    "fake",
		Address: "",
		Opts:    map[string]string{},
	}})
	c.Assert(apps[1].Pool, check.Equals, app2.Pool)
	c.Assert(apps[1].Tags, check.DeepEquals, app2.Tags)

}

func (s *S) TestAppListAfterAppInfoHasAddr(c *check.C) {
	s.mockService.Cache.OnList = func(keys ...string) ([]cache.CacheEntry, error) {
		return []cache.CacheEntry{{Key: "app-router-addr\x00app1\x00fake", Value: "app1.fakerouter.com"}}, nil
	}
	p := pool.Pool{Name: "pool1"}
	opts := pool.AddPoolOptions{Name: p.Name, Public: true}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	app1 := appTypes.App{
		Name:      "app1",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		CName:     []string{"cname.app1"},
		Pool:      "pool1",
		Tags:      []string{},
	}
	err = app.CreateApp(context.TODO(), &app1, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/apps/app1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	request, err = http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var apps []appTypes.App
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps[0].Name, check.Equals, app1.Name)
	c.Assert(apps[0].CName, check.DeepEquals, app1.CName)
	c.Assert(apps[0].Routers, check.DeepEquals, []appTypes.AppRouter{{
		Name:    "fake",
		Address: "app1.fakerouter.com",
		Opts:    map[string]string{},
	}})
	c.Assert(apps[0].Pool, check.Equals, app1.Pool)
	c.Assert(apps[0].Tags, check.DeepEquals, app1.Tags)
}

func (s *S) TestAppListAfterAppInfoHasAddrLegacyRouter(c *check.C) {
	s.mockService.Cache.OnList = func(keys ...string) ([]cache.CacheEntry, error) {
		return []cache.CacheEntry{{Key: "app-router-addr\x00app1\x00fake", Value: "app1.fakerouter.com"}}, nil
	}
	p := pool.Pool{Name: "pool1"}
	opts := pool.AddPoolOptions{Name: p.Name, Public: true}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	app1 := appTypes.App{
		Name:      "app1",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Pool:      "pool1",
		Teams:     []string{s.team.Name},
		Router:    "fake",
	}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)
	_, err = appsCollection.InsertOne(context.TODO(), app1)
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.EnsureBackend(context.TODO(), &app1, router.EnsureBackendOpts{})
	request, err := http.NewRequest("GET", "/apps/app1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	request, err = http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var apps []appTypes.App
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps[0].Name, check.Equals, app1.Name)
	c.Assert(apps[0].Routers, check.DeepEquals, []appTypes.AppRouter{{
		Name:    "fake",
		Address: "app1.fakerouter.com",
		Opts:    map[string]string{},
	}})
}

func (s *S) TestAppListUnitsError(c *check.C) {
	app1 := appTypes.App{
		Name:      "app1",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(context.TODO(), &app1, s.user)
	c.Assert(err, check.IsNil)
	provisiontest.ProvisionerInstance.PrepareFailure("Units", fmt.Errorf("some units error"))
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var apps []struct {
		Name  string
		Units []provTypes.Unit
		Error string
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps[0].Name, check.Equals, app1.Name)
	c.Assert(apps[0].Units, check.DeepEquals, []provTypes.Unit{})
	c.Assert(apps[0].Error, check.Equals, "unable to list app units: some units error")
}

func (s *S) TestAppListShouldListAllAppsOfAllTeamsThatTheUserHasPermission(c *check.C) {
	team := authTypes.Team{Name: "angra"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{team, {Name: s.team.Name}}, nil
	}
	s.mockService.Team.OnFindByName = func(_ string) (*authTypes.Team, error) {
		return &team, nil
	}
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permTypes.CtxTeam, team.Name),
	})
	u, _ := auth.ConvertNewUser(token.User(context.TODO()))
	app1 := appTypes.App{Name: "app1", Platform: "zend", TeamOwner: "angra"}
	err := app.CreateApp(context.TODO(), &app1, u)
	c.Assert(err, check.IsNil)
	app2 := appTypes.App{Name: "app2", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &app2, u)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var apps []appTypes.App
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps[0].Name, check.Equals, app1.Name)
}

func (s *S) TestAppListShouldListAllAppsOfAllTeamsThatTheUserHasPermissionAppInfo(c *check.C) {
	team := authTypes.Team{Name: "angra"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{team, {Name: s.team.Name}}, nil
	}
	s.mockService.Team.OnFindByName = func(_ string) (*authTypes.Team, error) {
		return &team, nil
	}
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppReadInfo,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	u, _ := auth.ConvertNewUser(token.User(context.TODO()))
	app1 := appTypes.App{Name: "app1", Platform: "zend", TeamOwner: "angra"}
	err := app.CreateApp(context.TODO(), &app1, u)
	c.Assert(err, check.IsNil)
	app2 := appTypes.App{Name: "app2", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &app2, u)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var apps []appTypes.App
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 2)
	c.Assert(apps[0].Name, check.Equals, app1.Name)
	c.Assert(apps[1].Name, check.Equals, app2.Name)
}

func (s *S) TestListShouldReturnStatusNoContentWhenAppListIsNil(c *check.C) {
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestDelete(c *check.C) {
	ctx := context.TODO()
	myApp := &appTypes.App{
		Name:      "myapptodelete",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(ctx, myApp, s.user)
	c.Assert(err, check.IsNil)
	myApp, err = app.GetByName(ctx, myApp.Name)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/apps/"+myApp.Name+"?:app="+myApp.Name, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	role, err := permission.NewRole(ctx, "deleter", "app", "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions(ctx, "app.delete")
	c.Assert(err, check.IsNil)
	err = s.user.AddRole(ctx, "deleter", myApp.Name)
	c.Assert(err, check.IsNil)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(myApp.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.delete",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": myApp.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestDeleteVersion(c *check.C) {
	ctx := context.TODO()
	myApp := &appTypes.App{
		Name:      "myversiontodelete",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(ctx, myApp, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, myApp)
	newSuccessfulAppVersion(c, myApp)
	myApp, err = app.GetByName(ctx, myApp.Name)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/apps/"+myApp.Name+"/versions/"+"2", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	role, err := permission.NewRole(ctx, "deleter", "app", "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions(ctx, "app.delete")
	c.Assert(err, check.IsNil)
	err = s.user.AddRole(ctx, "deleter", myApp.Name)
	c.Assert(err, check.IsNil)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(myApp.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": myApp.Name},
			{"name": ":version", "value": "2"},
			{"name": ":mux-path-template", "value": "/apps/{app}/versions/{version}"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestDeleteShouldReturnForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *check.C) {
	myApp := appTypes.App{Name: "app-to-delete", Platform: "zend"}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), myApp)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppDelete,
		Context: permission.Context(permTypes.CtxApp, "-other-app-"),
	})
	request, err := http.NewRequest("DELETE", "/apps/"+myApp.Name+"?:app="+myApp.Name, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestDeleteShouldReturnNotFoundIfTheAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("DELETE", "/apps/unknown?:app=unknown", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App unknown not found.\n")
}

func (s *S) TestDeleteAdminAuthorized(c *check.C) {
	myApp := &appTypes.App{
		Name:      "myapptodelete",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(context.TODO(), myApp, s.user)
	c.Assert(err, check.IsNil)
	myApp, err = app.GetByName(context.TODO(), myApp.Name)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/apps/"+myApp.Name+"?:app="+myApp.Name, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestAppInfo(c *check.C) {
	ctx := context.TODO()
	config.Set("host", "http://myhost.com")
	expectedApp := appTypes.App{Name: "new-app", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(ctx, &expectedApp, s.user)
	c.Assert(err, check.IsNil)
	var myApp map[string]interface{}
	request, err := http.NewRequest("GET", "/apps/"+expectedApp.Name+"?:app="+expectedApp.Name, nil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	c.Assert(err, check.IsNil)
	role, err := permission.NewRole(ctx, "reader", "app", "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions(ctx, "app.read")
	c.Assert(err, check.IsNil)
	s.user.AddRole(ctx, "reader", expectedApp.Name)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	err = json.Unmarshal(recorder.Body.Bytes(), &myApp)
	c.Assert(err, check.IsNil)
	c.Assert(myApp["name"], check.Equals, expectedApp.Name)
}

func (s *S) TestAppInfoReturnsForbiddenWhenTheUserDoesNotHaveAccessToTheApp(c *check.C) {
	expectedApp := appTypes.App{Name: "new-app", Platform: "zend"}

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), expectedApp)

	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/apps/"+expectedApp.Name+"?:app="+expectedApp.Name, nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permTypes.CtxApp, "-other-app-"),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestAppInfoReturnsNotFoundWhenAppDoesNotExist(c *check.C) {
	myApp := appTypes.App{Name: "SomeApp"}
	request, err := http.NewRequest("GET", "/apps/"+myApp.Name+"?:app="+myApp.Name, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App SomeApp not found.\n")
}

func (s *S) TestCreateAppRemoveRole(c *check.C) {
	ctx := context.TODO()
	s.setupMockForCreateApp(c, "zend")
	a := appTypes.App{Name: "someapp"}
	data := "name=someapp&platform=zend"
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	role, err := permission.NewRole(ctx, "test", "team", "")
	c.Assert(err, check.IsNil)
	user, err := auth.ConvertNewUser(token.User(context.TODO()))
	c.Assert(err, check.IsNil)
	err = user.AddRole(ctx, role.Name, "team")
	c.Assert(err, check.IsNil)

	rolesCollection, err := storagev2.RolesCollection()
	c.Assert(err, check.IsNil)
	_, err = rolesCollection.DeleteOne(ctx, mongoBSON.M{"_id": role.Name})
	c.Assert(err, check.IsNil)

	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var obtained map[string]string
	expected := map[string]string{
		"status": "success",
		"ip":     "someapp.fakerouter.com",
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained, check.DeepEquals, expected)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var gotApp appTypes.App

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	err = appsCollection.FindOne(ctx, mongoBSON.M{"name": "someapp"}).Decode(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  token.GetUserName(),
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": a.Name},
			{"name": "platform", "value": "zend"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestCreateApp(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	a := appTypes.App{Name: "someapp"}
	data := "name=someapp&platform=zend"
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	s.mockService.UserQuota.OnInc = func(item quota.QuotaItem, q int) error {
		c.Assert(item.GetName(), check.Equals, token.GetUserName())
		return nil
	}
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var obtained map[string]string
	expected := map[string]string{
		"status": "success",
		"ip":     "someapp.fakerouter.com",
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained, check.DeepEquals, expected)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var gotApp appTypes.App

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "someapp"}).Decode(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("someapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": a.Name},
			{"name": "platform", "value": "zend"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestCreateAppWithoutPlatform(c *check.C) {
	a := appTypes.App{Name: "someapp"}
	data := "name=someapp"
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	var obtained map[string]string
	expected := map[string]string{
		"status": "success",
		"ip":     "someapp.fakerouter.com",
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained, check.DeepEquals, expected)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var gotApp appTypes.App

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "someapp"}).Decode(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("someapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestCreateAppTeamOwner(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	t1 := authTypes.Team{Name: "team1"}
	t2 := authTypes.Team{Name: "team2"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{t1, t2}, nil
	}
	s.mockService.Team.OnFindByName = func(_ string) (*authTypes.Team, error) {
		return &t1, nil
	}
	permissions := []permTypes.Permission{
		{
			Scheme:  permission.PermAppCreate,
			Context: permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: t1.Name},
		},
		{
			Scheme:  permission.PermAppCreate,
			Context: permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: t2.Name},
		},
	}
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "anotheruser", permissions...)
	a := appTypes.App{Name: "someapp"}
	data := "name=someapp&platform=zend&teamOwner=" + t1.Name
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var gotApp appTypes.App

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": a.Name}).Decode(&gotApp)
	c.Assert(err, check.IsNil)
	var appIP string
	appIP, err = s.provisioner.Addr(&gotApp)
	c.Assert(err, check.IsNil)
	var obtained map[string]string
	expected := map[string]string{
		"status": "success",
		"ip":     appIP,
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(obtained, check.DeepEquals, expected)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Teams, check.DeepEquals, []string{t1.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("someapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": a.Name},
			{"name": "platform", "value": "zend"},
			{"name": "teamOwner", "value": t1.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestCreateAppAdminSingleTeam(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	a := appTypes.App{Name: "someapp"}
	data := "name=someapp&platform=zend"
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var gotApp appTypes.App

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": a.Name}).Decode(&gotApp)
	c.Assert(err, check.IsNil)

	var appIP string
	appIP, err = s.provisioner.Addr(&gotApp)
	c.Assert(err, check.IsNil)
	var obtained map[string]string
	expected := map[string]string{
		"status": "success",
		"ip":     appIP,
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(obtained, check.DeepEquals, expected)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("someapp"),
		Owner:  s.token.GetUserName(),
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": a.Name},
			{"name": "platform", "value": "zend"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestCreateAppCustomPlan(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	a := appTypes.App{Name: "someapp"}
	s.plan = appTypes.Plan{
		Name:   "myplan",
		Memory: 4194304,
	}
	data := "name=someapp&platform=zend&plan=myplan"
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var obtained map[string]string
	expected := map[string]string{
		"status": "success",
		"ip":     "someapp.fakerouter.com",
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained, check.DeepEquals, expected)
	var gotApp appTypes.App

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "someapp"}).Decode(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	c.Assert(gotApp.Plan, check.DeepEquals, s.plan)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("someapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": a.Name},
			{"name": "platform", "value": "zend"},
			{"name": "plan", "value": "myplan"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestCreateAppWithDescription(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	a := appTypes.App{Name: "someapp"}
	data, err := url.QueryUnescape("name=someapp&platform=zend&description=my app description")
	c.Assert(err, check.IsNil)
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var obtained map[string]string
	expected := map[string]string{
		"status": "success",
		"ip":     "someapp.fakerouter.com",
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained, check.DeepEquals, expected)
	var gotApp appTypes.App
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "someapp"}).Decode(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("someapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": a.Name},
			{"name": "platform", "value": "zend"},
			{"name": "description", "value": "my app description"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestCreateAppWithTags(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	data, err := url.QueryUnescape("name=someapp&platform=zend&tag=tag1&tag=tag2&tags.0=tag0")
	c.Assert(err, check.IsNil)
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var obtained map[string]string
	expected := map[string]string{
		"status": "success",
		"ip":     "someapp.fakerouter.com",
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained, check.DeepEquals, expected)
	var gotApp appTypes.App

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "someapp"}).Decode(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Tags, check.DeepEquals, []string{"tag0", "tag1", "tag2"})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("someapp"),
		Kind:   "app.create",
		Owner:  token.GetUserName(),
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "someapp"},
			{"name": "platform", "value": "zend"},
			{"name": "tag", "value": []string{"tag1", "tag2"}},
			{"name": "tags.0", "value": "tag0"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestCreateAppWithTagsAndTagValidator(c *check.C) {
	previousTagService := servicemanager.Tag
	defer func() {
		servicemanager.Tag = previousTagService
	}()
	servicemanager.Tag = &tagTypes.MockServiceTagServiceClient{
		OnValidate: func(in *tagTypes.TagValidationRequest) (*tagTypes.ValidationResponse, error) {
			c.Assert(in.Operation, check.Equals, tagTypes.OperationKind_OPERATION_KIND_CREATE)
			c.Assert(in.Tags, check.DeepEquals, []string{"tag0", "tag1", "tag2"})
			return &tagTypes.ValidationResponse{Valid: false, Error: "invalid tag"}, nil
		},
	}

	s.setupMockForCreateApp(c, "zend")
	data, err := url.QueryUnescape("name=someapp&platform=zend&tag=tag1&tag=tag2&tags.0=tag0")
	c.Assert(err, check.IsNil)
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "invalid tag\n")
}

func (s *S) TestCreateAppWithMetadata(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	data, err := url.QueryUnescape("name=someapp&platform=zend&metadata.annotations.0.name=a&metadata.annotations.0.value=b")
	c.Assert(err, check.IsNil)

	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())

	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)

	var obtained map[string]string
	expected := map[string]string{
		"status": "success",
		"ip":     "someapp.fakerouter.com",
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained, check.DeepEquals, expected)

	var gotApp appTypes.App

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "someapp"}).Decode(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Metadata.Annotations, check.DeepEquals, []appTypes.MetadataItem{
		{Name: "a", Value: "b", Delete: false},
	})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("someapp"),
		Kind:   "app.create",
		Owner:  token.GetUserName(),
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "someapp"},
			{"name": "platform", "value": "zend"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestCreateAppWithPool(c *check.C) {
	platName := "zend"
	s.setupMockForCreateApp(c, platName)
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "mypool1", Public: true})
	c.Assert(err, check.IsNil)
	appName := "someapp"
	data, err := url.QueryUnescape(fmt.Sprintf("name=%s&platform=%s&pool=mypool1", appName, platName))
	c.Assert(err, check.IsNil)
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var obtained map[string]string
	expected := map[string]string{
		"status": "success",
		"ip":     "someapp.fakerouter.com",
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained, check.DeepEquals, expected)
	var gotApp appTypes.App

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": appName}).Decode(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(gotApp.Pool, check.Equals, "mypool1")
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("someapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": appName},
			{"name": "platform", "value": platName},
			{"name": "pool", "value": "mypool1"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestCreateAppWithRouter(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	data, err := url.QueryUnescape("name=someapp&platform=zend&router=fake")
	c.Assert(err, check.IsNil)
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var obtained map[string]string
	expected := map[string]string{
		"status": "success",
		"ip":     "someapp.fakerouter.com",
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained, check.DeepEquals, expected)
	var gotApp appTypes.App

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "someapp"}).Decode(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Routers, check.DeepEquals, []appTypes.AppRouter{{
		Name: "fake",
		Opts: map[string]string{},
	}})
}

func (s *S) TestCreateAppWithRouterOpts(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	data, err := url.QueryUnescape("name=someapp&platform=zend&routeropts.opt1=val1&routeropts.opt2=val2")
	c.Assert(err, check.IsNil)
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var obtained map[string]string
	expected := map[string]string{
		"status": "success",
		"ip":     "someapp.fakerouter.com",
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained, check.DeepEquals, expected)
	var gotApp appTypes.App

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "someapp"}).Decode(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Routers, check.DeepEquals, []appTypes.AppRouter{{
		Name: "fake",
		Opts: map[string]string{"opt1": "val1", "opt2": "val2"},
	}})
}

func (s *S) TestCreateAppTwoTeams(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	team := authTypes.Team{Name: "tsurutwo"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{team, {Name: s.team.Name}}, nil
	}
	s.mockService.Team.OnFindByName = func(_ string) (*authTypes.Team, error) {
		return &team, nil
	}
	data := "name=someapp&platform=zend"
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permTypes.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "You must provide a team to execute this action.\n")
}

func (s *S) TestCreateAppUserQuotaExceeded(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	s.mockService.UserQuota.OnInc = func(item quota.QuotaItem, q int) error {
		c.Assert(item.GetName(), check.Equals, token.GetUserName())
		return &quota.QuotaExceededError{Available: 0, Requested: 1}
	}
	u, err := token.User(context.TODO())
	c.Assert(err, check.IsNil)

	usersCollection, err := storagev2.UsersCollection()
	c.Assert(err, check.IsNil)
	_, err = usersCollection.UpdateOne(context.TODO(), mongoBSON.M{"email": u.Email}, mongoBSON.M{"$set": mongoBSON.M{"quota": quota.Quota{Limit: 1, InUse: 1}}})
	c.Assert(err, check.IsNil)
	b := strings.NewReader("name=someapp&platform=zend")
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "Quota exceeded\n")
	c.Assert(eventtest.EventDesc{
		Target: appTarget("someapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "someapp"},
			{"name": "platform", "value": "zend"},
		},
		ErrorMatches: `Quota exceeded`,
	}, eventtest.HasEvent)
}

func (s *S) TestCreateAppInvalidName(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	b := strings.NewReader("name=123myapp&platform=zend")
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	msg := "Invalid app name, your app should have at most 40 " +
		"characters, containing only lower case letters, numbers " +
		"or dashes, starting with a letter."
	c.Assert(recorder.Body.String(), check.Equals, msg+"\n")
	c.Assert(eventtest.EventDesc{
		Target: appTarget("123myapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "123myapp"},
			{"name": "platform", "value": "zend"},
		},
		ErrorMatches: msg,
	}, eventtest.HasEvent)
}

func (s *S) TestCreateAppReturnsUnauthorizedIfNoPermissions(c *check.C) {
	s.setupMockForCreateApp(c, "django")
	token := userWithPermission(c)
	b := strings.NewReader("name=someapp&platform=django")
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "You don't have permission to do this action\n")
}

func (s *S) TestCreateAppReturnsConflictWithProperMessageWhenTheAppAlreadyExist(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	a := appTypes.App{Name: "plainsofdawn", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	b := strings.NewReader("name=plainsofdawn&platform=zend")
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
	c.Assert(recorder.Body.String(), check.Matches, "tsuru failed to create the app \"plainsofdawn\": there is already an app with this name\n")
}

func (s *S) TestCreateAppWithDisabledPlatformAndPlatformUpdater(c *check.C) {
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permTypes.Permission{
		Scheme:  permission.PermPlatformUpdate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	p := appTypes.Platform{Name: "platDis", Disabled: true}
	s.setupMockForCreateApp(c, p.Name)
	s.mockService.Platform.OnFindByName = func(name string) (*appTypes.Platform, error) {
		c.Assert(name, check.Equals, p.Name)
		return &p, nil
	}
	data := "name=someapp&platform=" + p.Name
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var obtained map[string]string
	expected := map[string]string{
		"status": "success",
		"ip":     "someapp.fakerouter.com",
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained, check.DeepEquals, expected)
	var gotApp appTypes.App

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "someapp"}).Decode(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	u, _ := token.User(context.TODO())
	c.Assert(eventtest.EventDesc{
		Target: appTarget("someapp"),
		Owner:  u.Email,
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "someapp"},
			{"name": "platform", "value": p.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestCreateAppWithDisabledPlatformAndNotAdminUser(c *check.C) {
	p := appTypes.Platform{Name: "platDis", Disabled: true}
	s.setupMockForCreateApp(c, p.Name)
	s.mockService.Platform.OnFindByName = func(name string) (*appTypes.Platform, error) {
		c.Assert(name, check.Equals, p.Name)
		return &p, nil
	}
	data := "name=someapp&platform=" + p.Name
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Invalid platform\n")
}

func (s *S) TestUpdateAppWithDescriptionOnly(c *check.C) {
	a := appTypes.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permTypes.CtxApp, a.Name),
	})
	b := strings.NewReader("description=my app description")
	request, err := http.NewRequest("PUT", "/apps/myapp", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	var gotApp appTypes.App

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "myapp"}).Decode(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Description, check.DeepEquals, "my app description")
	c.Assert(eventtest.EventDesc{
		Target: appTarget("myapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "description", "value": "my app description"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUpdateAppPlatformOnly(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	a := appTypes.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permTypes.CtxApp, a.Name),
	})
	s.setupMockForCreateApp(c, "heimerdinger")
	b := strings.NewReader("platform=heimerdinger")
	request, err := http.NewRequest("PUT", "/apps/myapp", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	var gotApp appTypes.App

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "myapp"}).Decode(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Platform, check.Equals, "heimerdinger")
	c.Assert(gotApp.UpdatePlatform, check.Equals, true)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("myapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "platform", "value": "heimerdinger"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUpdateAppPlatformWithVersion(c *check.C) {
	s.setupMockForCreateApp(c, "myplatform")
	a := appTypes.App{Name: "myapp", Platform: "myplatform", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permTypes.CtxApp, a.Name),
	})
	s.mockService.PlatformImage.OnFindImage = func(reg imgTypes.ImageRegistry, name, image string) (string, error) {
		c.Assert(reg, check.Equals, imgTypes.ImageRegistry(""))
		c.Assert(name, check.Equals, "myplatform")
		c.Assert(image, check.Equals, "v1")
		return "tsuru/myplatform:v1", nil
	}
	b := strings.NewReader("platform=myplatform:v1")
	request, err := http.NewRequest("PUT", "/apps/myapp", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	var gotApp appTypes.App

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "myapp"}).Decode(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.PlatformVersion, check.Equals, "v1")
	c.Assert(gotApp.UpdatePlatform, check.Equals, true)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("myapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "platform", "value": "myplatform:v1"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUpdateAppWithTagsOnly(c *check.C) {
	a := appTypes.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)

	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permTypes.CtxApp, a.Name),
	})
	b := strings.NewReader("description1=s&tag=tag1&tag=tag2&tag=tag3&tags.0=tag0")
	request, err := http.NewRequest("PUT", "/apps/myapp", b)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	var gotApp appTypes.App
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "myapp"}).Decode(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Tags, check.DeepEquals, []string{"tag0", "tag1", "tag2", "tag3"})
	c.Assert(eventtest.EventDesc{
		Target: appTarget("myapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "tag", "value": []string{"tag1", "tag2", "tag3"}},
			{"name": "tags.0", "value": "tag0"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUpdateAppWithTagsAndTagValidator(c *check.C) {
	previousTagService := servicemanager.Tag
	defer func() {
		servicemanager.Tag = previousTagService
	}()
	servicemanager.Tag = &tagTypes.MockServiceTagServiceClient{
		OnValidate: func(in *tagTypes.TagValidationRequest) (*tagTypes.ValidationResponse, error) {
			c.Assert(in.Operation, check.Equals, tagTypes.OperationKind_OPERATION_KIND_UPDATE)
			c.Assert(in.Tags, check.DeepEquals, []string{"tag0", "tag1", "tag2", "tag3"})
			return &tagTypes.ValidationResponse{Valid: false, Error: "invalid tag"}, nil
		},
	}
	a := appTypes.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)

	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permTypes.CtxApp, a.Name),
	})
	b := strings.NewReader("description1=s&tag=tag1&tag=tag2&tag=tag3&tags.0=tag0")
	request, err := http.NewRequest("PUT", "/apps/myapp", b)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "invalid tag\n")
}

func (s *S) TestUpdateAppWithTagsWithoutPermission(c *check.C) {
	a := appTypes.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdateDescription,
		Context: permission.Context(permTypes.CtxApp, a.Name),
	})
	b := strings.NewReader("description1=s&tag=tag1&tag=tag2&tag=tag3")
	request, err := http.NewRequest("PUT", "/apps/myapp", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestUpdateAppWithAnnotations(c *check.C) {
	a := appTypes.App{
		Name:      "myapp",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Metadata: appTypes.Metadata{
			Annotations: []appTypes.MetadataItem{{Name: "c", Value: "someData"}},
		},
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)

	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permTypes.CtxApp, a.Name),
	})
	b := strings.NewReader("description1=s&metadata.annotations.0.name=a&metadata.annotations.0.value=b&metadata.annotations.1.delete=true&metadata.annotations.1.name=c&noRestart=true")
	request, err := http.NewRequest("PUT", "/apps/myapp", b)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	var gotApp appTypes.App
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "myapp"}).Decode(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Metadata.Annotations, check.DeepEquals, []appTypes.MetadataItem{{Name: "a", Value: "b"}})
	c.Assert(eventtest.EventDesc{
		Target: appTarget("myapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUpdateAppWithLabels(c *check.C) {
	a := appTypes.App{
		Name:      "myapp",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Metadata: appTypes.Metadata{
			Labels: []appTypes.MetadataItem{{Name: "c", Value: "someData"}, {Name: "z", Value: "ground"}},
		},
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)

	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permTypes.CtxApp, a.Name),
	})
	b := strings.NewReader("description1=s&metadata.labels.0.name=a&metadata.labels.0.value=b&metadata.labels.1.delete=true&metadata.labels.1.name=c&noRestart=true")
	request, err := http.NewRequest("PUT", "/apps/myapp", b)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	var gotApp appTypes.App
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "myapp"}).Decode(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Metadata.Labels, check.DeepEquals, []appTypes.MetadataItem{{Name: "a", Value: "b"}, {Name: "z", Value: "ground"}})
	c.Assert(eventtest.EventDesc{
		Target: appTarget("myapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUpdateAppWithCustomPlanByProcesNotFound(c *check.C) {
	a := appTypes.App{
		Name:      "myapp",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Metadata: appTypes.Metadata{
			Labels: []appTypes.MetadataItem{{Name: "c", Value: "someData"}, {Name: "z", Value: "ground"}},
		},
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)

	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permTypes.CtxApp, a.Name),
	})
	b := strings.NewReader("processes.0.name=web&processes.0.plan=c1m1&noRestart=true")
	request, err := http.NewRequest("PUT", "/apps/myapp", b)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "could not find plan \"c1m1\": plan not found\n")
}

func (s *S) TestUpdateAppWithCustomPlanByProcess(c *check.C) {
	oldPlanService := servicemanager.Plan
	servicemanager.Plan = &appTypes.MockPlanService{
		Plans: []appTypes.Plan{{Name: "c1m1"}},
	}
	defer func() {
		servicemanager.Plan = oldPlanService
	}()
	a := appTypes.App{
		Name:      "myapp",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Metadata: appTypes.Metadata{
			Labels: []appTypes.MetadataItem{{Name: "c", Value: "someData"}, {Name: "z", Value: "ground"}},
		},
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)

	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permTypes.CtxApp, a.Name),
	})
	b := strings.NewReader("processes.0.name=web&processes.0.plan=c1m1&noRestart=true")
	request, err := http.NewRequest("PUT", "/apps/myapp", b)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	var gotApp appTypes.App
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "myapp"}).Decode(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Processes, check.DeepEquals, []appTypes.Process{{Name: "web", Plan: "c1m1", Metadata: appTypes.Metadata{Labels: []appTypes.MetadataItem{}, Annotations: []appTypes.MetadataItem{}}}})
	c.Assert(eventtest.EventDesc{
		Target: appTarget("myapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "processes.0.name", "value": "web"},
			{"name": "processes.0.plan", "value": "c1m1"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUpdateAppWithResetPlanByProcess(c *check.C) {
	oldPlanService := servicemanager.Plan
	servicemanager.Plan = &appTypes.MockPlanService{
		Plans: []appTypes.Plan{{Name: "c1m1"}},
	}
	defer func() {
		servicemanager.Plan = oldPlanService
	}()

	a := appTypes.App{
		Name:      "myapp",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Metadata: appTypes.Metadata{
			Labels: []appTypes.MetadataItem{{Name: "c", Value: "someData"}, {Name: "z", Value: "ground"}},
		},
		Processes: []appTypes.Process{
			{
				Name: "web",
				Plan: "c1m1",
			},
		},
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)

	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permTypes.CtxApp, a.Name),
	})
	b := strings.NewReader("processes.0.name=web&processes.0.plan=$default&noRestart=true")
	request, err := http.NewRequest("PUT", "/apps/myapp", b)
	c.Assert(err, check.IsNil)

	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	var gotApp appTypes.App
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "myapp"}).Decode(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Processes, check.DeepEquals, []appTypes.Process{})
	c.Assert(eventtest.EventDesc{
		Target: appTarget("myapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "processes.0.name", "value": "web"},
			{"name": "processes.0.plan", "value": "$default"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUpdateAppImageReset(c *check.C) {
	a := appTypes.App{Name: "myappx", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	body := strings.NewReader("imageReset=true")
	request, err := http.NewRequest("PUT", "/apps/myappx", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var dbApp appTypes.App
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": a.Name}).Decode(&dbApp)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.UpdatePlatform, check.Equals, true)
}

func (s *S) TestUpdateAppWithPoolOnly(c *check.C) {
	a := appTypes.App{Name: "myappx", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	opts := pool.AddPoolOptions{Name: "test"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(context.TODO(), "test", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	body := strings.NewReader("pool=test")
	request, err := http.NewRequest("PUT", "/apps/myappx", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestUpdateAppPoolWithNoRestart(c *check.C) {
	a := appTypes.App{Name: "myappx", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	opts := pool.AddPoolOptions{Name: "test"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(context.TODO(), "test", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	body := strings.NewReader("pool=test&noRestart=true")
	request, err := http.NewRequest("PUT", "/apps/myappx", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Matches, "^You must restart the app when changing the pool.\n$")
}

func (s *S) TestUpdateAppPoolForbiddenIfTheUserDoesNotHaveAccess(c *check.C) {
	a := appTypes.App{Name: "myappx", Platform: "zend"}

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	opts := pool.AddPoolOptions{Name: "test"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdatePool,
		Context: permission.Context(permTypes.CtxApp, "-other-"),
	})
	body := strings.NewReader("pool=test")
	request, err := http.NewRequest("PUT", "/apps/myappx", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestUpdateAppPoolWhenAppDoesNotExist(c *check.C) {
	body := strings.NewReader("pool=test")
	request, err := http.NewRequest("PUT", "/apps/myappx", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Matches, "^App myappx not found.\n$")
}

func (s *S) TestUpdateAppPoolWithDifferentProvisioner(c *check.C) {
	p1 := provisiontest.NewFakeProvisioner()
	p1.Name = "fake1"
	provision.Register("fake1", func() (provision.Provisioner, error) {
		return p1, nil
	})
	a := appTypes.App{Name: "myappx", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	opts := pool.AddPoolOptions{Name: "fakepool", Provisioner: "fake1"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(context.TODO(), "fakepool", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	body := strings.NewReader("pool=fakepool")
	request, err := http.NewRequest("PUT", "/apps/myappx", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestUpdateAppPlanOnly(c *check.C) {
	config.Set("docker:router", "fake")
	defer config.Unset("docker:router")
	plans := []appTypes.Plan{
		{Name: "hiperplan", Memory: 536870912},
		{Name: "superplan", Memory: 268435456},
		s.defaultPlan,
	}
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		if name == plans[0].Name {
			return &plans[0], nil
		}
		if name == plans[1].Name {
			return &plans[1], nil
		}
		if name == s.defaultPlan.Name {
			return &s.defaultPlan, nil
		}
		c.Errorf("plan name not expected, got: %s", name)
		return nil, nil
	}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return plans, nil
	}
	a := appTypes.App{Name: "someapp", Platform: "zend", TeamOwner: s.team.Name, Plan: plans[1]}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	body := strings.NewReader("plan=hiperplan")
	request, err := http.NewRequest("PUT", "/apps/someapp", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app, err := app.GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.Plan, check.DeepEquals, plans[0])
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 1)
}

func (s *S) TestUpdateAppPlanOverrideOnly(c *check.C) {
	config.Set("docker:router", "fake")
	defer config.Unset("docker:router")
	originalPlan := appTypes.Plan{Name: "hiperplan", Memory: 536870912}
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		return &originalPlan, nil
	}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{originalPlan}, nil
	}
	a := appTypes.App{Name: "someapp", Platform: "zend", TeamOwner: s.team.Name, Plan: originalPlan}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)

	tests := []struct {
		body     string
		memory   int64
		cpuMilli int
	}{
		{
			body:     "planoverride.memory=314572800",
			memory:   314572800,
			cpuMilli: 0,
		},
		{
			body:     "planoverride.cpumilli=200",
			memory:   314572800,
			cpuMilli: 200,
		},
		{
			body:     "planoverride.memory=",
			memory:   536870912,
			cpuMilli: 200,
		},
		{
			body:     "planoverride.cpumilli=100",
			memory:   536870912,
			cpuMilli: 100,
		},
	}

	for i, tt := range tests {
		c.Logf("test %d", i)
		body := strings.NewReader(tt.body)
		request, err := http.NewRequest("PUT", "/apps/someapp", body)
		c.Assert(err, check.IsNil)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		recorder := httptest.NewRecorder()
		s.testServer.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %v", recorder.Body.String()))
		dbApp, err := app.GetByName(context.TODO(), a.Name)
		c.Assert(err, check.IsNil)
		c.Assert(dbApp.Plan.GetMemory(), check.Equals, tt.memory)
		c.Assert(dbApp.Plan.GetMilliCPU(), check.Equals, tt.cpuMilli)
		c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, i+1)
	}
}

func (s *S) TestUpdateAppPlanNotFound(c *check.C) {
	s.plan = appTypes.Plan{Name: "superplan", Memory: 268435456}
	a := appTypes.App{Name: "someapp", Platform: "zend", TeamOwner: s.team.Name, Plan: s.plan}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	body := strings.NewReader("plan=hiperplan")
	request, err := http.NewRequest("PUT", "/apps/someapp", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Check(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Check(recorder.Body.String(), check.Equals, appTypes.ErrPlanNotFound.Error()+"\n")
}

func (s *S) TestUpdateAppWithoutFlag(c *check.C) {
	a := appTypes.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permTypes.CtxApp, a.Name),
	})
	b := strings.NewReader("{}")
	request, err := http.NewRequest("PUT", "/apps/myapp", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	errorMessage := "Neither the description, tags, plan, pool, team owner or platform were set. You must define at least one.\n"
	c.Check(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Check(recorder.Body.String(), check.Equals, errorMessage)
}

func (s *S) TestUpdateAppReturnsUnauthorizedIfNoPermissions(c *check.C) {
	a := appTypes.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c)
	b := strings.NewReader("description=description of my app")
	request, err := http.NewRequest("PUT", "/apps/myapp", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, 403)
}

func (s *S) TestUpdateAppWithTeamOwnerOnly(c *check.C) {
	a := appTypes.App{Name: "myappx", Platform: "zend", TeamOwner: s.team.Name}
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdateTeamowner,
		Context: permission.Context(permTypes.CtxTeam, a.TeamOwner),
	})
	user, err := auth.ConvertNewUser(token.User(context.TODO()))
	c.Assert(err, check.IsNil)
	err = app.CreateApp(context.TODO(), &a, user)
	c.Assert(err, check.IsNil)
	team := authTypes.Team{Name: "newowner"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{team, {Name: s.team.Name}}, nil
	}
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, team.Name)
		return &team, nil
	}
	body := strings.NewReader("teamOwner=" + team.Name)
	req, err := http.NewRequest("PUT", "/apps/myappx", body)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
}

func (s *S) TestUpdateAppTeamOwnerToUserWhoCantBeOwner(c *check.C) {
	a := appTypes.App{Name: "myappx", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	user := &auth.User{Email: "teste@thewho.com", Password: "123456", Quota: quota.UnlimitedQuota}
	_, err = nativeScheme.Create(context.TODO(), user)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(context.TODO(), map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	body := strings.NewReader("teamOwner=newowner")
	req, err := http.NewRequest("PUT", "/apps/myappx", body)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusForbidden)

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": "myappx"}).Decode(&a)
	c.Assert(a.TeamOwner, check.Equals, s.team.Name)
}

func (s *S) TestUpdateAppTeamOwnerSetNewTeamToAppAddThatTeamToAppTeamList(c *check.C) {
	a := appTypes.App{Name: "myappx", Platform: "zend", TeamOwner: s.team.Name}
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdateTeamowner,
		Context: permission.Context(permTypes.CtxTeam, a.TeamOwner),
	})
	user, err := auth.ConvertNewUser(token.User(context.TODO()))
	c.Assert(err, check.IsNil)
	err = app.CreateApp(context.TODO(), &a, user)
	c.Assert(err, check.IsNil)
	team := authTypes.Team{Name: "newowner"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}, team}, nil
	}
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, team.Name)
		return &team, nil
	}
	body := strings.NewReader("teamOwner=" + team.Name)
	req, err := http.NewRequest("PUT", "/apps/myappx", body)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
}

func (s *S) TestAddUnits(c *check.C) {
	ctx := context.Background()
	a := appTypes.App{Name: "armorandsword", Platform: "zend", TeamOwner: s.team.Name, Quota: quota.Quota{Limit: 10, InUse: 0}}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	s.mockService.AppQuota.OnInc = func(item *appTypes.App, quantity int) error {
		c.Assert(item.Name, check.Equals, a.Name)
		c.Assert(quantity, check.Equals, 3)
		return nil
	}
	body := strings.NewReader("units=3&process=web")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app1, err := app.GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	units, err := app.AppUnits(ctx, app1)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 3)
	c.Assert(eventtest.EventDesc{
		Target:          appTarget("armorandsword"),
		Owner:           s.token.GetUserName(),
		Kind:            "app.update.unit.add",
		StartCustomData: []map[string]interface{}{{"name": "units", "value": "3"}, {"name": "process", "value": "web"}, {"name": ":app", "value": "armorandsword"}},
	}, eventtest.HasEvent)
	c.Assert(recorder.Body.String(), check.Matches, `{"Message":".*added 3 units","Timestamp":".*"}`+"\n")
}

func (s *S) TestAddUnitsUnlimited(c *check.C) {
	ctx := context.Background()
	a := appTypes.App{Name: "armorandsword", Platform: "zend", TeamOwner: s.team.Name, Quota: quota.UnlimitedQuota}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	s.mockService.AppQuota.OnInc = func(item *appTypes.App, quantity int) error {
		c.Assert(item.Name, check.Equals, a.Name)
		c.Assert(quantity, check.Equals, 3)
		return nil
	}
	body := strings.NewReader("units=3&process=web")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app1, err := app.GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	units, err := app.AppUnits(ctx, app1)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 3)
	c.Assert(eventtest.EventDesc{
		Target:          appTarget("armorandsword"),
		Owner:           s.token.GetUserName(),
		Kind:            "app.update.unit.add",
		StartCustomData: []map[string]interface{}{{"name": "units", "value": "3"}, {"name": "process", "value": "web"}, {"name": ":app", "value": "armorandsword"}},
	}, eventtest.HasEvent)
	c.Assert(recorder.Body.String(), check.Matches, `{"Message":".*added 3 units","Timestamp":".*"}`+"\n")
}

func (s *S) TestAddUnitsReturns404IfAppDoesNotExist(c *check.C) {
	body := strings.NewReader("units=1&process=web")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:app=armorandsword", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App armorandsword not found.\n")
}

func (s *S) TestAddUnitsReturns403IfTheUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := appTypes.App{Name: "armorandsword", Platform: "zend"}

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)

	c.Assert(err, check.IsNil)
	body := strings.NewReader("units=1&process=web")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:app=armorandsword", body)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdateUnitAdd,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestAddUnitsReturns400IfNumberOfUnitsIsOmitted(c *check.C) {
	bodies := []io.Reader{nil, strings.NewReader("")}
	for _, body := range bodies {
		request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:app=armorandsword", body)
		c.Assert(err, check.IsNil)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()
		err = addUnits(recorder, request, s.token)
		c.Assert(err, check.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, check.Equals, true)
		c.Assert(e.Code, check.Equals, http.StatusBadRequest)
		c.Assert(e.Message, check.Equals, "You must provide the number of units.")
	}
}

func (s *S) TestAddUnitsWorksIfProcessIsOmitted(c *check.C) {
	ctx := context.Background()
	a := appTypes.App{Name: "armorandsword", Platform: "zend", TeamOwner: s.team.Name, Quota: quota.UnlimitedQuota}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	s.mockService.AppQuota.OnInc = func(item *appTypes.App, quantity int) error {
		c.Assert(item.Name, check.Equals, a.Name)
		c.Assert(quantity, check.Equals, 3)
		return nil
	}
	body := strings.NewReader("units=3&process=")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:app=armorandsword", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = addUnits(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app1, err := app.GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	units, err := app.AppUnits(ctx, app1)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 3)
	c.Assert(recorder.Body.String(), check.Matches, `{"Message":".*added 3 units","Timestamp":".*"}`+"\n")
	c.Assert(eventtest.EventDesc{
		Target:          appTarget("armorandsword"),
		Owner:           s.token.GetUserName(),
		Kind:            "app.update.unit.add",
		StartCustomData: []map[string]interface{}{{"name": "units", "value": "3"}, {"name": "process", "value": ""}, {"name": ":app", "value": "armorandsword"}},
	}, eventtest.HasEvent)
}

func (s *S) TestAddUnitsReturns400IfNumberIsInvalid(c *check.C) {
	values := []string{"-1", "0", "far cry", "12345678909876543"}
	for _, value := range values {
		body := strings.NewReader("units=" + value + "&process=web")
		request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:app=armorandsword", body)
		c.Assert(err, check.IsNil)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()
		err = addUnits(recorder, request, s.token)
		c.Assert(err, check.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, check.Equals, true)
		c.Assert(e.Code, check.Equals, http.StatusBadRequest)
		c.Assert(e.Message, check.Equals, "Invalid number of units: the number must be an integer greater than 0.")
	}
}

func (s *S) TestAddUnitsQuotaExceeded(c *check.C) {
	a := appTypes.App{Name: "armorandsword", Platform: "zend", TeamOwner: s.team.Name, Quota: quota.Quota{Limit: 2, InUse: 0}}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	s.mockService.AppQuota.OnInc = func(item *appTypes.App, quantity int) error {
		c.Assert(item.Name, check.Equals, a.Name)
		c.Assert(quantity, check.Equals, 3)
		return &quota.QuotaExceededError{Available: 2, Requested: 3}
	}
	body := strings.NewReader("units=3&process=web")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Matches, `(?s).*Quota exceeded. Available: 2, Requested: 3.*`)
	c.Assert(eventtest.EventDesc{
		Target:          appTarget("armorandsword"),
		Owner:           s.token.GetUserName(),
		Kind:            "app.update.unit.add",
		StartCustomData: []map[string]interface{}{{"name": "units", "value": "3"}, {"name": "process", "value": "web"}, {"name": ":app", "value": "armorandsword"}},
		ErrorMatches:    `Quota exceeded. Available: 2, Requested: 3.`,
	}, eventtest.HasEvent)
}

func (s *S) TestRemoveUnits(c *check.C) {
	ctx := context.Background()
	a := appTypes.App{Name: "velha", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	s.mockService.AppQuota.OnInc = func(item *appTypes.App, quantity int) error {
		c.Assert(item.Name, check.Equals, a.Name)
		c.Assert(quantity, check.Equals, 2)
		return nil
	}
	s.provisioner.AddUnits(context.TODO(), &a, 3, "web", nil, nil)
	request, err := http.NewRequest("DELETE", "/apps/velha/units?units=2&process=web", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-type"), check.Equals, "application/x-json-stream")
	app1, err := app.GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	units, err := app.AppUnits(ctx, app1)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	c.Assert(s.provisioner.GetUnits(app1), check.HasLen, 1)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("velha"),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.unit.remove",
		StartCustomData: []map[string]interface{}{
			{"name": "units", "value": "2"},
			{"name": "process", "value": "web"},
			{"name": ":app", "value": "velha"},
		},
	}, eventtest.HasEvent)
	c.Assert(recorder.Body.String(), check.Matches, `{"Message":".*removing 2 units","Timestamp":".*"}`+"\n")
}

func (s *S) TestRemoveUnitsReturns404IfAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("DELETE", "/apps/fetisha/units?:app=fetisha&units=1&process=web", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUnits(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e.Message, check.Equals, "App fetisha not found.")
}

func (s *S) TestRemoveUnitsReturns403IfTheUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := appTypes.App{Name: "fetisha", Platform: "zend"}

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)

	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdateUnitRemove,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	request, err := http.NewRequest("DELETE", "/apps/fetisha/units?:app=fetisha&units=1&process=web", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUnits(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestRemoveUnitsReturns400IfNumberOfUnitsIsOmitted(c *check.C) {
	request, err := http.NewRequest("DELETE", "/apps/fetisha/units?:app=fetisha", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUnits(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, "You must provide the number of units.")
}

func (s *S) TestRemoveUnitsWorksIfProcessIsOmitted(c *check.C) {
	ctx := context.Background()
	a := appTypes.App{Name: "velha", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	s.mockService.AppQuota.OnInc = func(item *appTypes.App, quantity int) error {
		c.Assert(item.Name, check.Equals, a.Name)
		c.Assert(quantity, check.Equals, 2)
		return nil
	}
	s.provisioner.AddUnits(context.TODO(), &a, 3, "", nil, nil)
	request, err := http.NewRequest("DELETE", "/apps/velha/units?:app=velha&units=2&process=", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUnits(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	app1, err := app.GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	units, err := app.AppUnits(ctx, app1)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	c.Assert(s.provisioner.GetUnits(app1), check.HasLen, 1)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("velha"),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.unit.remove",
		StartCustomData: []map[string]interface{}{
			{"name": "units", "value": "2"},
			{"name": "process", "value": ""},
			{"name": ":app", "value": "velha"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRemoveUnitsReturns400IfNumberIsInvalid(c *check.C) {
	values := []string{"-1", "0", "far cry", "12345678909876543"}
	for _, value := range values {
		v := url.Values{
			":app":    []string{"fiend"},
			"units":   []string{value},
			"process": []string{"web"},
		}
		request, err := http.NewRequest("DELETE", "/apps/fiend/units?"+v.Encode(), nil)
		c.Assert(err, check.IsNil)
		recorder := httptest.NewRecorder()
		err = removeUnits(recorder, request, s.token)
		c.Assert(err, check.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, check.Equals, true)
		c.Assert(e.Code, check.Equals, http.StatusBadRequest)
		c.Assert(e.Message, check.Equals, "Invalid number of units: the number must be an integer greater than 0.")
	}
}

func (s *S) TestAddTeamToTheApp(c *check.C) {
	t := authTypes.Team{Name: "itshardteam"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}, t}, nil
	}
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		return &authTypes.Team{Name: name}, nil
	}
	a := appTypes.App{Name: "itshard", Platform: "zend", TeamOwner: t.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app, err := app.GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.Teams, check.HasLen, 2)
	c.Assert(app.Teams[1], check.Equals, s.team.Name)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.grant",
		StartCustomData: []map[string]interface{}{
			{"name": ":team", "value": s.team.Name},
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestGrantAccessToTeamReturn404IfTheAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("PUT", "/apps/a/teams/b", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App a not found.\n")
}

func (s *S) TestGrantAccessToTeamReturn403IfTheGivenUserDoesNotHasAccessToTheApp(c *check.C) {
	a := appTypes.App{Name: "itshard", Platform: "zend"}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdateGrant,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestGrantAccessToTeamReturn404IfTheTeamDoesNotExist(c *check.C) {
	a := appTypes.App{Name: "itshard", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newTeamName := "newteam"
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, newTeamName)
		return nil, authTypes.ErrTeamNotFound
	}
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, newTeamName)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Team not found\n")
}

func (s *S) TestGrantAccessToTeamReturn409IfTheTeamHasAlreadyAccessToTheApp(c *check.C) {
	a := appTypes.App{Name: "itshard", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
	c.Assert(eventtest.EventDesc{
		Target:       appTarget(a.Name),
		Owner:        s.token.GetUserName(),
		Kind:         "app.update.grant",
		ErrorMatches: "team already have access to this app",
		StartCustomData: []map[string]interface{}{
			{"name": ":team", "value": s.team.Name},
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRevokeAccessFromTeam(c *check.C) {
	a := appTypes.App{Name: "itshard", Platform: "zend", Teams: []string{"abcd", s.team.Name}}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app, err := app.GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.Teams, check.HasLen, 1)
	c.Assert(app.Teams[0], check.Equals, "abcd")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.revoke",
		StartCustomData: []map[string]interface{}{
			{"name": ":team", "value": s.team.Name},
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRevokeAccessFromTeamReturn404IfTheAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("DELETE", "/apps/a/teams/b", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App a not found.\n")
}

func (s *S) TestRevokeAccessFromTeamReturn401IfTheGivenUserDoesNotHavePermissionInTheApp(c *check.C) {
	a := appTypes.App{Name: "itshard", Platform: "zend"}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdateRevoke,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestRevokeAccessFromTeamReturn404IfTheTeamDoesNotExist(c *check.C) {
	a := appTypes.App{Name: "itshard", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	teamName := "notfound"
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, teamName)
		return nil, authTypes.ErrTeamNotFound
	}
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, teamName)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Team not found\n")
}

func (s *S) TestRevokeAccessFromTeamReturn404IfTheTeamDoesNotHaveAccessToTheApp(c *check.C) {
	t := authTypes.Team{Name: "blaaa"}
	t2 := authTypes.Team{Name: "team2"}
	a := appTypes.App{Name: "itshard", Platform: "zend", Teams: []string{s.team.Name, t2.Name}}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, t.Name)
		return &t, nil
	}
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, t.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestRevokeAccessFromTeamReturn403IfTheTeamIsTheLastWithAccessToTheApp(c *check.C) {
	a := appTypes.App{Name: "itshard", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "You can not revoke the access from this team, because it is the unique team with access to the app, and an app can not be orphaned\n")
	c.Assert(eventtest.EventDesc{
		Target:       appTarget(a.Name),
		Owner:        s.token.GetUserName(),
		Kind:         "app.update.revoke",
		ErrorMatches: "You can not revoke the access from this team, because it is the unique team with access to the app, and an app can not be orphaned",
		StartCustomData: []map[string]interface{}{
			{"name": ":team", "value": s.team.Name},
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRunOnce(c *check.C) {
	ctx := context.Background()
	s.provisioner.PrepareOutput([]byte("lots of files"))
	a := appTypes.App{Name: "secrets", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 3, "web", nil, nil)
	url := fmt.Sprintf("/apps/%s/run", a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("command=ls&once=true"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(recorder.Body.String(), check.Matches, `{"Message":"lots of files","Timestamp":".*"}`+"\n")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls"
	units, err := app.AppUnits(ctx, &a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 3)
	allExecs := s.provisioner.AllExecs()
	c.Assert(allExecs, check.HasLen, 1)
	c.Assert(allExecs[units[0].GetID()], check.HasLen, 1)
	c.Assert(allExecs[units[0].GetID()][0].Cmds, check.DeepEquals, []string{"/bin/sh", "-c", expected})
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.run",
		StartCustomData: []map[string]interface{}{
			{"name": "command", "value": "ls"},
			{"name": "once", "value": "true"},
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRun(c *check.C) {
	ctx := context.Background()
	s.provisioner.PrepareOutput([]byte("lots of\nfiles"))
	a := appTypes.App{Name: "secrets", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 1, "web", nil, nil)
	url := fmt.Sprintf("/apps/%s/run", a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("command=ls"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Matches, `{"Message":"lots of\\nfiles","Timestamp":".*"}`+"\n")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls"
	units, err := app.AppUnits(ctx, &a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	allExecs := s.provisioner.AllExecs()
	c.Assert(allExecs, check.HasLen, 1)
	c.Assert(allExecs[units[0].GetID()], check.HasLen, 1)
	c.Assert(allExecs[units[0].GetID()][0].Cmds, check.DeepEquals, []string{"/bin/sh", "-c", expected})
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.run",
		StartCustomData: []map[string]interface{}{
			{"name": "command", "value": "ls"},
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRunIsolated(c *check.C) {
	s.provisioner.PrepareOutput([]byte("lots of files"))
	a := appTypes.App{Name: "secrets", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 3, "web", nil, nil)
	url := fmt.Sprintf("/apps/%s/run", a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("command=ls&isolated=true"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(recorder.Body.String(), check.Matches, `{"Message":"lots of files","Timestamp":".*"}`+"\n")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls"
	allExecs := s.provisioner.AllExecs()
	c.Assert(allExecs, check.HasLen, 1)
	c.Assert(allExecs["isolated"], check.HasLen, 1)
	c.Assert(allExecs["isolated"][0].Cmds, check.DeepEquals, []string{"/bin/sh", "-c", expected})
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.run",
		StartCustomData: []map[string]interface{}{
			{"name": "command", "value": "ls"},
			{"name": "isolated", "value": "true"},
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRunReturnsTheOutputOfTheCommandEvenIfItFails(c *check.C) {
	s.provisioner.PrepareFailure("ExecuteCommand", &errors.HTTP{Code: 500, Message: "something went wrong"})
	s.provisioner.PrepareOutput([]byte("failure output"))
	a := appTypes.App{Name: "secrets", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 1, "web", nil, nil)
	url := fmt.Sprintf("/apps/%s/run", a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("command=ls"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	expected := `{"Message":"failure output","Timestamp":".*"}` + "\n" +
		`{"Message":"","Timestamp":".*","Error":"something went wrong"}` + "\n"
	c.Assert(recorder.Body.String(), check.Matches, expected)
	c.Assert(eventtest.EventDesc{
		Target:       appTarget(a.Name),
		Owner:        s.token.GetUserName(),
		Kind:         "app.run",
		ErrorMatches: "something went wrong",
		StartCustomData: []map[string]interface{}{
			{"name": "command", "value": "ls"},
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRunReturnsBadRequestIfTheCommandIsMissing(c *check.C) {
	a := appTypes.App{Name: "secrets", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 1, "web", nil, nil)
	bodies := []io.Reader{nil, strings.NewReader("")}
	for _, body := range bodies {
		request, err := http.NewRequest("POST", "/apps/secrets/run", body)
		c.Assert(err, check.IsNil)
		request.Header.Set("content-type", "application/x-www-form-urlencoded")
		request.Header.Set("authorization", "b "+s.token.GetValue())
		recorder := httptest.NewRecorder()
		s.testServer.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
		c.Assert(recorder.Body.String(), check.Equals, "You must provide the command to run\n")
	}
}

func (s *S) TestRunAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("POST", "/apps/unknown/run", strings.NewReader("command=ls"))
	c.Assert(err, check.IsNil)
	request.Header.Set("content-type", "application/x-www-form-urlencoded")
	request.Header.Set("authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App unknown not found.\n")
}

func (s *S) TestRunUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := appTypes.App{Name: "secrets", Platform: "zend"}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppRun,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	url := fmt.Sprintf("/apps/%s/run", a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("command=ls"))
	c.Assert(err, check.IsNil)
	request.Header.Set("content-type", "application/x-www-form-urlencoded")
	request.Header.Set("authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestGetEnvAllEnvs(c *check.C) {
	a := appTypes.App{
		Name:      "everything-i-want",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env: map[string]bindTypes.EnvVar{
			"DATABASE_HOST": {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER": {Name: "DATABASE_USER", Value: "root", Public: true},
		},
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env?envs=", a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	expected := []bindTypes.EnvVar{
		{Name: "DATABASE_HOST", Value: "localhost", Public: true},
		{Name: "DATABASE_USER", Value: "root", Public: true},
		{Name: "TSURU_APPNAME", Value: "everything-i-want", Public: false},
		{Name: "TSURU_APPDIR", Value: "/home/application/current", Public: false},
		{Name: "TSURU_SERVICES", Value: "{}", Public: false},
	}
	result := []bindTypes.EnvVar{}
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(len(result), check.Equals, len(expected))
	for _, r := range result {
		for _, e := range expected {
			if e.Name == r.Name {
				c.Check(e.Public, check.Equals, r.Public)
				c.Check(e.Value, check.Equals, r.Value)
			}
		}
	}
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
}

func (s *S) TestGetEnv(c *check.C) {
	a := appTypes.App{
		Name:      "everything-i-want",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env: map[string]bindTypes.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env?env=DATABASE_HOST", a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	expected := []map[string]interface{}{{
		"name":   "DATABASE_HOST",
		"value":  "localhost",
		"public": true,
		"alias":  "",
	}}
	result := []map[string]interface{}{}
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, expected)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
}

func (s *S) TestGetEnvMultipleVariables(c *check.C) {
	a := appTypes.App{
		Name:      "four-sticks",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env: map[string]bindTypes.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env?env=DATABASE_HOST&env=DATABASE_USER", a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-type"), check.Equals, "application/json")
	expected := []map[string]interface{}{
		{"name": "DATABASE_HOST", "value": "localhost", "public": true, "alias": ""},
		{"name": "DATABASE_USER", "value": "root", "public": true, "alias": ""},
	}
	var got []map[string]interface{}
	err = json.Unmarshal(recorder.Body.Bytes(), &got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, expected)
}

func (s *S) TestGetEnvAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("GET", "/apps/unknown/env", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App unknown not found.\n")
}

func (s *S) TestGetEnvUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := appTypes.App{Name: "lost", Platform: "zend"}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppReadEnv,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	url := fmt.Sprintf("/apps/%s/env?envs=DATABASE_HOST", a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestSetEnvTsuruInternalEnvorimentVariableInApp(c *check.C) {
	a := appTypes.App{Name: "black-erro", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "TSURU_APPNAME", Value: "everything-i-want", Alias: ""},
			{Name: "TSURU_APPDIR", Value: "everything-i-want", Alias: ""},
			{Name: "TSURU_SERVICES", Value: "everything-i-want", Alias: ""},
		},
		NoRestart: false,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestSetEnvPublicEnvironmentVariableInTheApp(c *check.C) {
	a := appTypes.App{Name: "black-dog", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "localhost", Alias: ""},
		},
		NoRestart: false,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(context.TODO(), "black-dog")
	c.Assert(err, check.IsNil)
	expected := bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: true}
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, expected)
	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Setting 1 new environment variables ----\\n","Timestamp":".*"}
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "localhost"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvPublicAndPrivate(c *check.C) {
	a := appTypes.App{Name: "black-dog", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)

	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "localhost", Private: func(b bool) *bool { return &b }(true)},
			{Name: "MY_DB_HOST", Value: "otherhost", Private: func(b bool) *bool { return &b }(false)},
		},
		ManagedBy: "terraform",
	}

	v, err := json.Marshal(d)
	c.Assert(err, check.IsNil)
	b := bytes.NewReader(v)

	url := fmt.Sprintf("/apps/%s/env", a.Name)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())

	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")

	app, err := app.GetByName(context.TODO(), "black-dog")
	c.Assert(err, check.IsNil)

	expected1 := bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: false, ManagedBy: "terraform"}
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, expected1)

	expected2 := bindTypes.EnvVar{Name: "MY_DB_HOST", Value: "otherhost", Public: true, ManagedBy: "terraform"}
	c.Assert(app.Env["MY_DB_HOST"], check.DeepEquals, expected2)
	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Setting 2 new environment variables ----\\n","Timestamp":".*"}
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "*****"},
			{"name": "Envs.1.Name", "value": "MY_DB_HOST"},
			{"name": "Envs.1.Value", "value": "otherhost"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvCanPruneOldVariables(c *check.C) {
	a := appTypes.App{
		Name:      "black-dog",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env: map[string]bindTypes.EnvVar{
			"CMDLINE": {Name: "CMDLINE", Value: "1", Public: true, ManagedBy: "tsuru-client"},
			"OLDENV":  {Name: "OLDENV", Value: "1", Public: true, ManagedBy: "terraform"},
		},
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)

	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "localhost", Private: func(b bool) *bool { return &b }(true)},
			{Name: "MY_DB_HOST", Value: "otherhost", Private: func(b bool) *bool { return &b }(false)},
		},
		PruneUnused: true,
		ManagedBy:   "terraform",
	}

	v, err := json.Marshal(d)
	c.Assert(err, check.IsNil)
	b := bytes.NewReader(v)

	url := fmt.Sprintf("/apps/%s/env", a.Name)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())

	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")

	app, err := app.GetByName(context.TODO(), "black-dog")
	c.Assert(err, check.IsNil)

	_, hasOldVar := app.Env["OLDENV"]
	c.Assert(hasOldVar, check.Equals, false)

	expected0 := bindTypes.EnvVar{Name: "CMDLINE", Value: "1", Public: true, ManagedBy: "tsuru-client"}
	c.Assert(app.Env["CMDLINE"], check.DeepEquals, expected0)

	expected1 := bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: false, ManagedBy: "terraform"}
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, expected1)

	expected2 := bindTypes.EnvVar{Name: "MY_DB_HOST", Value: "otherhost", Public: true, ManagedBy: "terraform"}
	c.Assert(app.Env["MY_DB_HOST"], check.DeepEquals, expected2)
	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Setting 2 new environment variables ----\\n","Timestamp":".*"}
{"Message":".*---- Pruning OLDENV from environment variables ----\\n","Timestamp":".*"}
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "*****"},
			{"name": "Envs.1.Name", "value": "MY_DB_HOST"},
			{"name": "Envs.1.Value", "value": "otherhost"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvCanPruneAllVariables(c *check.C) {
	a := appTypes.App{
		Name:      "black-dog",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env: map[string]bindTypes.EnvVar{
			"CMDLINE": {Name: "CMDLINE", Value: "1", Public: true, ManagedBy: "tsuru-client"},
			"OLDENV":  {Name: "OLDENV", Value: "1", Public: true, ManagedBy: "terraform"},
		},
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)

	d := apiTypes.Envs{
		Envs:        []apiTypes.Env{},
		PruneUnused: true,
		ManagedBy:   "terraform",
	}

	v, err := json.Marshal(d)
	c.Assert(err, check.IsNil)
	b := bytes.NewReader(v)

	url := fmt.Sprintf("/apps/%s/env", a.Name)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())

	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")

	app, err := app.GetByName(context.TODO(), "black-dog")
	c.Assert(err, check.IsNil)

	_, hasOldVar := app.Env["OLDENV"]
	c.Assert(hasOldVar, check.Equals, false)

	expected0 := bindTypes.EnvVar{Name: "CMDLINE", Value: "1", Public: true, ManagedBy: "tsuru-client"}
	c.Assert(app.Env["CMDLINE"], check.DeepEquals, expected0)

	c.Assert(recorder.Body.String(), check.Matches,
		`.*---- Pruning OLDENV from environment variables ----.*
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvDontPruneWhenMissingManagedBy(c *check.C) {
	a := appTypes.App{
		Name:      "black-dog",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env: map[string]bindTypes.EnvVar{
			"CMDLINE": {Name: "CMDLINE", Value: "1", Public: true, ManagedBy: "tsuru-client"},
			"OLDENV":  {Name: "OLDENV", Value: "1", Public: true, ManagedBy: "terraform"},
		},
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)

	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "localhost", Private: func(b bool) *bool { return &b }(true)},
			{Name: "MY_DB_HOST", Value: "otherhost", Private: func(b bool) *bool { return &b }(false)},
		},
		PruneUnused: true,
	}

	v, err := json.Marshal(d)
	c.Assert(err, check.IsNil)
	b := bytes.NewReader(v)

	url := fmt.Sprintf("/apps/%s/env", a.Name)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())

	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "text/plain; charset=utf-8")
	c.Assert(recorder.Body.String(), check.Matches,
		"Prune unused requires a managed-by value\n")
}

func (s *S) TestSetEnvPublicEnvironmentVariableAlias(c *check.C) {
	a := appTypes.App{Name: "black-dog", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "", Alias: "MY_DB_HOST"},
			{Name: "MY_DB_HOST", Value: "localhost", Alias: ""},
		},
		NoRestart: false,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(context.TODO(), "black-dog")
	c.Assert(err, check.IsNil)
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, bindTypes.EnvVar{
		Name:   "DATABASE_HOST",
		Alias:  "MY_DB_HOST",
		Public: true,
	})
	c.Assert(app.Env["MY_DB_HOST"], check.DeepEquals, bindTypes.EnvVar{
		Name:   "MY_DB_HOST",
		Value:  "localhost",
		Public: true,
	})
	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Setting 2 new environment variables ----\\n","Timestamp":".*"}
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Alias", "value": "MY_DB_HOST"},
			{"name": "Envs.1.Name", "value": "MY_DB_HOST"},
			{"name": "Envs.1.Value", "value": "localhost"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvHandlerShouldSetAPrivateEnvironmentVariableInTheApp(c *check.C) {
	a := appTypes.App{Name: "black-dog", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "localhost", Alias: ""},
		},
		NoRestart: false,
		Private:   true,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(context.TODO(), "black-dog")
	c.Assert(err, check.IsNil)
	expected := bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: false}
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, expected)
	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Setting 1 new environment variables ----\\n","Timestamp":".*"}
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "*****"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvHandlerShouldSetADoublePrivateEnvironmentVariableInTheApp(c *check.C) {
	a := appTypes.App{Name: "black-dog", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "localhost", Alias: ""},
		},
		NoRestart: false,
		Private:   true,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "*****"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": "true"},
		},
	}, eventtest.HasEvent)
	d = apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "127.0.0.1", Alias: ""},
			{Name: "DATABASE_PORT", Value: "6379", Alias: ""},
		},
		NoRestart: false,
		Private:   true,
	}
	v, err = form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b = strings.NewReader(v.Encode())
	request, err = http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(context.TODO(), "black-dog")
	c.Assert(err, check.IsNil)
	expected := bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "127.0.0.1", Public: false}
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, expected)
	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Setting 2 new environment variables ----\\n","Timestamp":".*"}
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "*****"},
			{"name": "Envs.1.Name", "value": "DATABASE_PORT"},
			{"name": "Envs.1.Value", "value": "*****"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvHandlerShouldSetMultipleEnvironmentVariablesInTheApp(c *check.C) {
	a := appTypes.App{Name: "vigil", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "localhost", Alias: ""},
			{Name: "DATABASE_USER", Value: "root", Alias: ""},
		},
		NoRestart: false,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(context.TODO(), "vigil")
	c.Assert(err, check.IsNil)
	expectedHost := bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: true}
	expectedUser := bindTypes.EnvVar{Name: "DATABASE_USER", Value: "root", Public: true}
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, expectedHost)
	c.Assert(app.Env["DATABASE_USER"], check.DeepEquals, expectedUser)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "localhost"},
			{"name": "Envs.1.Name", "value": "DATABASE_USER"},
			{"name": "Envs.1.Value", "value": "root"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvHandlerShouldNotChangeValueOfServiceVariables(c *check.C) {
	a := &appTypes.App{Name: "losers", Platform: "zend", Teams: []string{s.team.Name}, ServiceEnvs: []bindTypes.ServiceEnvVar{
		{
			EnvVar: bindTypes.EnvVar{
				Name:  "DATABASE_HOST",
				Value: "privatehost.com",
			},
			ServiceName:  "srv1",
			InstanceName: "some-service",
		},
	}}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "http://foo.com:8080", Alias: ""},
		},
		NoRestart: false,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	a, err = app.GetByName(context.TODO(), "losers")
	c.Assert(err, check.IsNil)
	envs := provision.EnvsForApp(a)
	delete(envs, tsuruEnvs.TsuruServicesEnvVar)
	delete(envs, "TSURU_APPNAME")
	delete(envs, "TSURU_APPDIR")

	expected := map[string]bindTypes.EnvVar{
		"DATABASE_HOST": {
			Name:      "DATABASE_HOST",
			Value:     "privatehost.com",
			ManagedBy: "srv1/some-service",
		},
	}
	c.Assert(envs, check.DeepEquals, expected)
	c.Assert(eventtest.EventDesc{
		Target:       appTarget(a.Name),
		Owner:        s.token.GetUserName(),
		Kind:         "app.update.env.set",
		ErrorMatches: "Environment variable \"DATABASE_HOST\" is already in use by service bind \"srv1/some-service\"",
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvHandlerNoRestart(c *check.C) {
	a := appTypes.App{Name: "black-dog", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "localhost", Alias: ""},
		},
		NoRestart: true,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(context.TODO(), "black-dog")
	c.Assert(err, check.IsNil)
	expected := bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: true}
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, expected)
	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Setting 1 new environment variables ----\\n","Timestamp":".*"}
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "localhost"},
			{"name": "NoRestart", "value": "true"},
			{"name": "Private", "value": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvMissingFormBody(c *check.C) {
	a := appTypes.App{Name: "rock", Platform: "zend"}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/apps/rock/env", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	msg := ".*missing form body\n"
	c.Assert(recorder.Body.String(), check.Matches, msg)
}

func (s *S) TestSetEnvHandlerReturnsBadRequestIfVariablesAreMissing(c *check.C) {
	a := appTypes.App{Name: "rock", Platform: "zend"}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/apps/rock/env", strings.NewReader(""))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	msg := "You must provide the list of environment variables\n"
	c.Assert(recorder.Body.String(), check.Equals, msg)
}

func (s *S) TestSetEnvHandlerReturnsNotFoundIfTheAppDoesNotExist(c *check.C) {
	b := strings.NewReader("noRestart=false&private=&false&envs.0.name=DATABASE_HOST&envs.0.value=localhost")
	request, err := http.NewRequest("POST", "/apps/unknown/env", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App unknown not found.\n")
}

func (s *S) TestSetEnvHandlerReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := appTypes.App{Name: "rock-and-roll", Platform: "zend"}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdateEnvSet,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "localhost", Alias: ""},
		},
		NoRestart: false,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestSetEnvInvalidEnvName(c *check.C) {
	a := appTypes.App{Name: "black-dog", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "INVALID ENV", Value: "value"},
		},
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest(http.MethodPost, url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestIsEnvVarValid(c *check.C) {
	tests := []struct {
		envs          []apiTypes.Env
		expectedError string
	}{
		{},
		{
			envs:          []apiTypes.Env{{Name: "TSURU_SERVICES"}},
			expectedError: "cannot change an internal environment variable (TSURU_SERVICES): write-protected environment variable",
		},
		{
			envs:          []apiTypes.Env{{Name: "TSURU_APPNAME"}},
			expectedError: "cannot change an internal environment variable (TSURU_APPNAME): write-protected environment variable",
		},
		{
			envs:          []apiTypes.Env{{Name: "TSURU_APPDIR"}},
			expectedError: "cannot change an internal environment variable (TSURU_APPDIR): write-protected environment variable",
		},
		{
			envs:          []apiTypes.Env{{Name: "MY-ENV-NAME"}},
			expectedError: "\"MY-ENV-NAME\" is not valid environment variable name: a valid environment variable name must consist of alphabetic characters, digits, '_' and must not start with a digit: invalid environment variable name",
		},
		{
			envs:          []apiTypes.Env{{Name: " foo_bar"}},
			expectedError: "\" foo_bar\" is not valid environment variable name: a valid environment variable name must consist of alphabetic characters, digits, '_' and must not start with a digit: invalid environment variable name",
		},
	}

	for i, tt := range tests {
		got := validateApiEnvVars(tt.envs)

		if tt.expectedError == "" {
			c.Assert(got, check.IsNil)
			continue
		}

		c.Assert(got.Error(), check.Equals, tt.expectedError, check.Commentf("test case: %d", i+1))
	}
}

func (s *S) TestUnsetEnv(c *check.C) {
	a := appTypes.App{
		Name:     "swift",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env: map[string]bindTypes.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	expected := a.Env
	delete(expected, "DATABASE_HOST")
	url := fmt.Sprintf("/apps/%s/env?noRestart=false&env=DATABASE_HOST", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(context.TODO(), "swift")
	c.Assert(err, check.IsNil)
	c.Assert(app.Env, check.DeepEquals, expected)
	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Unsetting 1 environment variables ----\\n","Timestamp":".*"}
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.unset",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "env", "value": "DATABASE_HOST"},
			{"name": "noRestart", "value": "false"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnsetEnvNoRestart(c *check.C) {
	a := appTypes.App{
		Name:     "swift",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env: map[string]bindTypes.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	expected := a.Env
	delete(expected, "DATABASE_HOST")
	url := fmt.Sprintf("/apps/%s/env?noRestart=true&env=DATABASE_HOST", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(context.TODO(), "swift")
	c.Assert(err, check.IsNil)
	c.Assert(app.Env, check.DeepEquals, expected)
	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Unsetting 1 environment variables ----\\n","Timestamp":".*"}
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.unset",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "env", "value": "DATABASE_HOST"},
			{"name": "noRestart", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnsetEnvHandlerRemovesAllGivenEnvironmentVariables(c *check.C) {
	a := appTypes.App{
		Name:     "let-it-be",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env: map[string]bindTypes.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env?noRestart=false&env=DATABASE_HOST&env=DATABASE_USER", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(context.TODO(), "let-it-be")
	c.Assert(err, check.IsNil)
	expected := map[string]bindTypes.EnvVar{
		"DATABASE_PASSWORD": {
			Name:   "DATABASE_PASSWORD",
			Value:  "secret",
			Public: false,
		},
	}
	c.Assert(app.Env, check.DeepEquals, expected)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.unset",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "env", "value": []string{"DATABASE_HOST", "DATABASE_USER"}},
			{"name": "noRestart", "value": "false"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnsetHandlerRemovesPrivateVariables(c *check.C) {
	a := appTypes.App{
		Name:     "letitbe",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env: map[string]bindTypes.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env?noRestart=false&env=DATABASE_HOST&env=DATABASE_USER&env=DATABASE_PASSWORD", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(context.TODO(), "letitbe")
	c.Assert(err, check.IsNil)
	expected := map[string]bindTypes.EnvVar{}
	c.Assert(app.Env, check.DeepEquals, expected)
}

func (s *S) TestUnsetEnvVariablesMissing(c *check.C) {
	a := appTypes.App{
		Name:     "swift",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env: map[string]bindTypes.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/apps/swift/env?noRestart=false&env=", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "You must provide the list of environment variables.\n")
}

func (s *S) TestUnsetEnvAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("DELETE", "/apps/unknown/env?noRestart=false&env=ble", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App unknown not found.\n")
}

func (s *S) TestUnsetEnvUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := appTypes.App{Name: "mountain-mama"}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdateEnvUnset,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	url := fmt.Sprintf("/apps/%s/env?noRestart=false&env=DATABASE_HOST", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	err = s.provisioner.Provision(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestAddCName(c *check.C) {
	a := appTypes.App{Name: "leper", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/cname", a.Name)
	b := strings.NewReader("cname=leper.secretcompany.com&cname=blog.tsuru.com")
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app, err := app.GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.CName, check.DeepEquals, []string{"leper.secretcompany.com", "blog.tsuru.com"})
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.cname.add",
		StartCustomData: []map[string]interface{}{
			{"name": "cname", "value": []interface{}{"leper.secretcompany.com", "blog.tsuru.com"}},
			{"name": ":app", "value": "leper"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAddCNameAcceptsWildCard(c *check.C) {
	a := appTypes.App{Name: "leper", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/cname", a.Name)
	b := strings.NewReader("cname=*.leper.secretcompany.com")
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app, err := app.GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.CName, check.DeepEquals, []string{"*.leper.secretcompany.com"})
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.cname.add",
		StartCustomData: []map[string]interface{}{
			{"name": "cname", "value": "*.leper.secretcompany.com"},
			{"name": ":app", "value": "leper"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAddCNameErrsOnInvalidCName(c *check.C) {
	a := appTypes.App{Name: "leper", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/cname", a.Name)
	b := strings.NewReader("cname=_leper.secretcompany.com")
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Invalid cname\n")
	c.Assert(eventtest.EventDesc{
		Target:       appTarget(a.Name),
		Owner:        s.token.GetUserName(),
		Kind:         "app.update.cname.add",
		ErrorMatches: "Invalid cname",
		StartCustomData: []map[string]interface{}{
			{"name": "cname", "value": "_leper.secretcompany.com"},
			{"name": ":app", "value": "leper"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAddCNameHandlerReturnsBadRequestWhenCNameIsEmpty(c *check.C) {
	a := appTypes.App{Name: "leper", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/apps/leper/cname", strings.NewReader("cname="))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Invalid cname\n")
	c.Assert(eventtest.EventDesc{
		Target:       appTarget(a.Name),
		Owner:        s.token.GetUserName(),
		Kind:         "app.update.cname.add",
		ErrorMatches: "Invalid cname",
		StartCustomData: []map[string]interface{}{
			{"name": "cname", "value": ""},
			{"name": ":app", "value": "leper"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAddCNameHandlerReturnsBadRequestWhenCNameIsMissing(c *check.C) {
	a := appTypes.App{Name: "leper", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	bodies := []io.Reader{nil, strings.NewReader("")}
	for _, b := range bodies {
		request, err := http.NewRequest("POST", "/apps/leper/cname", b)
		c.Assert(err, check.IsNil)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Authorization", "b "+s.token.GetValue())
		recorder := httptest.NewRecorder()
		s.testServer.ServeHTTP(recorder, request)
		c.Check(recorder.Code, check.Equals, http.StatusBadRequest)
		c.Check(recorder.Body.String(), check.Equals, "You must provide the cname.\n")
	}
}

func (s *S) TestAddCNameHandlerUnknownApp(c *check.C) {
	b := strings.NewReader("cname=leper.secretcompany.com")
	request, err := http.NewRequest("POST", "/apps/unknown/cname", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestAddCNameHandlerUserWithoutAccessToTheApp(c *check.C) {
	a := appTypes.App{Name: "lost", Platform: "vougan", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/cname", a.Name)
	b := strings.NewReader("cname=lost.secretcompany.com")
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdateCname,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestRemoveCNameHandler(c *check.C) {
	ctx := context.Background()
	a := appTypes.App{
		Name:      "leper",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddCName(ctx, &a, "foo.bar.com")
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/cname?cname=foo.bar.com", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app, err := app.GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.CName, check.DeepEquals, []string{})
	c.Assert(eventtest.EventDesc{
		Target: appTarget(app.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.cname.remove",
		StartCustomData: []map[string]interface{}{
			{"name": "cname", "value": "foo.bar.com"},
			{"name": ":app", "value": "leper"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRemoveCNameTwoCnames(c *check.C) {
	ctx := context.Background()
	a := appTypes.App{
		Name:      "leper",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddCName(ctx, &a, "foo.bar.com")
	c.Assert(err, check.IsNil)
	err = app.AddCName(ctx, &a, "bar.com")
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/cname?cname=foo.bar.com&cname=bar.com", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app, err := app.GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.CName, check.DeepEquals, []string{})
	c.Assert(eventtest.EventDesc{
		Target: appTarget(app.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.cname.remove",
		StartCustomData: []map[string]interface{}{
			{"name": "cname", "value": []interface{}{"foo.bar.com", "bar.com"}},
			{"name": ":app", "value": "leper"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRemoveCNameUnknownApp(c *check.C) {
	request, err := http.NewRequest("DELETE", "/apps/unknown/cname?cname=foo.bar.com", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestRemoveCNameHandlerUserWithoutAccessToTheApp(c *check.C) {
	a := appTypes.App{
		Name:     "lost",
		Platform: "vougan",
	}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdateCnameRemove,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	url := fmt.Sprintf("/apps/%s/cname?cname=foo.bar.com", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestAppLogShouldReturnNotFoundWhenAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("GET", "/apps/unknown/log/?:app=unknown&lines=10", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = appLog(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestAppLogReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := appTypes.App{Name: "lost", Platform: "vougan"}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppReadLog,
		Context: permission.Context(permTypes.CtxTeam, "no-access"),
	})
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&lines=10", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = appLog(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestAppLogReturnsBadRequestIfNumberOfLinesIsMissing(c *check.C) {
	url := "/apps/something/log/?:app=doesntmatter"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = appLog(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, `Parameter "lines" is mandatory.`)
}

func (s *S) TestAppLogReturnsBadRequestIfNumberOfLinesIsNotAnInteger(c *check.C) {
	url := "/apps/something/log/?:app=doesntmatter&lines=2.34"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = appLog(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, `Parameter "lines" must be an integer.`)
}

func (s *S) TestAppLogFollow(c *check.C) {
	a := appTypes.App{Name: "lost1", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	path := "/apps/something/log/?:app=" + a.Name + "&lines=10&follow=1"
	request, err := http.NewRequest("GET", path, nil)
	c.Assert(err, check.IsNil)
	ctx, cancel := context.WithCancel(context.Background())
	request = request.WithContext(ctx)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppReadLog,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	recorder := httptest.NewRecorder()
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		logErr := appLog(recorder, request, token)
		c.Assert(logErr, check.IsNil)
		splitted := strings.Split(strings.TrimSpace(recorder.Body.String()), "\n")
		c.Assert(splitted, check.HasLen, 2)
		c.Assert(splitted[0], check.Equals, "[]")
		logs := []appTypes.Applog{}
		logErr = json.Unmarshal([]byte(splitted[1]), &logs)
		c.Assert(logErr, check.IsNil)
		c.Assert(logs, check.HasLen, 1)
		c.Assert(logs[0].Message, check.Equals, "x")
	}()
	var listener appTypes.LogWatcher
	timeout := time.After(5 * time.Second)
	for listener == nil {
		select {
		case <-timeout:
			c.Fatal("timeout after 5 seconds")
		case <-time.After(50 * time.Millisecond):
		}
		logTracker.Lock()
		for listener = range logTracker.conn {
		}
		logTracker.Unlock()
	}
	err = servicemanager.LogService.Add(a.Name, "x", "", "")
	c.Assert(err, check.IsNil)
	time.Sleep(500 * time.Millisecond)
	cancel()
	wg.Wait()
}

func (s *S) TestAppLogFollowWithFilter(c *check.C) {
	a := appTypes.App{Name: "lost2", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	path := "/apps/something/log/?:app=" + a.Name + "&lines=10&follow=1&source=web"
	request, err := http.NewRequest("GET", path, nil)
	c.Assert(err, check.IsNil)
	ctx, cancel := context.WithCancel(context.Background())
	request = request.WithContext(ctx)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppReadLog,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	recorder := httptest.NewRecorder()
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		logErr := appLog(recorder, request, token)
		c.Assert(logErr, check.IsNil)
		splitted := strings.Split(strings.TrimSpace(recorder.Body.String()), "\n")
		c.Assert(splitted, check.HasLen, 2)
		c.Assert(splitted[0], check.Equals, "[]")
		logs := []appTypes.Applog{}
		logErr = json.Unmarshal([]byte(splitted[1]), &logs)
		c.Assert(logErr, check.IsNil)
		c.Assert(logs, check.HasLen, 1)
		c.Assert(logs[0].Message, check.Equals, "y")
	}()
	var listener appTypes.LogWatcher
	timeout := time.After(5 * time.Second)
	for listener == nil {
		select {
		case <-timeout:
			c.Fatal("timeout after 5 seconds")
		case <-time.After(50 * time.Millisecond):
		}
		logTracker.Lock()
		for listener = range logTracker.conn {
		}
		logTracker.Unlock()
	}
	err = servicemanager.LogService.Add(a.Name, "x", "app", "")
	c.Assert(err, check.IsNil)
	err = servicemanager.LogService.Add(a.Name, "y", "web", "")
	c.Assert(err, check.IsNil)
	time.Sleep(500 * time.Millisecond)
	cancel()
	wg.Wait()
}

func (s *S) TestAppLogShouldHaveContentType(c *check.C) {
	a := appTypes.App{Name: "lost", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppReadLog,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&lines=10", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = appLog(recorder, request, token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
}

func (s *S) TestAppLogSelectByLines(c *check.C) {
	a := appTypes.App{Name: "lost", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	for i := 0; i < 15; i++ {
		servicemanager.LogService.Add(a.Name, strconv.Itoa(i), "source", "")
	}
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppReadLog,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&lines=10", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = appLog(recorder, request, token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	logs := []appTypes.Applog{}
	err = json.Unmarshal(recorder.Body.Bytes(), &logs)
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 10)
}

func (s *S) TestAppLogAllowNegativeLines(c *check.C) {
	a := appTypes.App{Name: "lost", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	for i := 0; i < 15; i++ {
		servicemanager.LogService.Add(a.Name, strconv.Itoa(i), "source", "")
	}
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppReadLog,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&lines=-1", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = appLog(recorder, request, token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	logs := []appTypes.Applog{}
	err = json.Unmarshal(recorder.Body.Bytes(), &logs)
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 0)
}

func (s *S) TestAppLogExplicitZeroLines(c *check.C) {
	a := appTypes.App{Name: "lost", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	for i := 0; i < 15; i++ {
		servicemanager.LogService.Add(a.Name, strconv.Itoa(i), "source", "")
	}
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppReadLog,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&lines=0", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = appLog(recorder, request, token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	logs := []appTypes.Applog{}
	err = json.Unmarshal(recorder.Body.Bytes(), &logs)
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 15)
}

func (s *S) TestAppLogSelectBySource(c *check.C) {
	a := appTypes.App{Name: "lost", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	servicemanager.LogService.Add(a.Name, "mars log", "mars", "")
	servicemanager.LogService.Add(a.Name, "earth log", "earth", "")
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppReadLog,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&source=mars&lines=10", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = appLog(recorder, request, token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	logs := []appTypes.Applog{}
	err = json.Unmarshal(recorder.Body.Bytes(), &logs)
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 1)
	c.Assert(logs[0].Message, check.Equals, "mars log")
	c.Assert(logs[0].Source, check.Equals, "mars")
}

func (s *S) TestAppLogSelectBySourceInvert(c *check.C) {
	a := appTypes.App{Name: "lost", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	servicemanager.LogService.Add(a.Name, "mars log", "mars", "")
	servicemanager.LogService.Add(a.Name, "earth log", "earth", "")
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppReadLog,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&source=mars&lines=10&invert-source=true", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = appLog(recorder, request, token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	logs := []appTypes.Applog{}
	err = json.Unmarshal(recorder.Body.Bytes(), &logs)
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 1)
	c.Assert(logs[0].Message, check.Equals, "earth log")
	c.Assert(logs[0].Source, check.Equals, "earth")
}

func (s *S) TestAppLogSelectByUnit(c *check.C) {
	a := appTypes.App{Name: "lost", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	servicemanager.LogService.Add(a.Name, "mars log", "mars", "prospero")
	servicemanager.LogService.Add(a.Name, "mars log", "mars", "mahnmut")
	servicemanager.LogService.Add(a.Name, "earth log", "earth", "caliban")
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppReadLog,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&unit=caliban&unit=mahnmut&lines=10", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = appLog(recorder, request, token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	logs := []appTypes.Applog{}
	err = json.Unmarshal(recorder.Body.Bytes(), &logs)
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 2)
	c.Assert(logs[0].Message, check.Equals, "mars log")
	c.Assert(logs[0].Source, check.Equals, "mars")
	c.Assert(logs[0].Unit, check.Equals, "mahnmut")
	c.Assert(logs[1].Message, check.Equals, "earth log")
	c.Assert(logs[1].Source, check.Equals, "earth")
	c.Assert(logs[1].Unit, check.Equals, "caliban")
}

func (s *S) TestAppLogSelectByLinesShouldReturnTheLatestEntries(c *check.C) {
	a := appTypes.App{Name: "lost", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	for i := 0; i < 15; i++ {
		err = servicemanager.LogService.Add(a.Name, strconv.Itoa(i), "source", "unit")
		c.Assert(err, check.IsNil)
	}
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppReadLog,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&lines=3", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = appLog(recorder, request, token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var logs []appTypes.Applog
	err = json.Unmarshal(recorder.Body.Bytes(), &logs)
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 3)
	c.Assert(logs[0].Message, check.Equals, "12")
	c.Assert(logs[1].Message, check.Equals, "13")
	c.Assert(logs[2].Message, check.Equals, "14")
}

func (s *S) TestAppLogShouldReturnLogByApp(c *check.C) {
	app1 := appTypes.App{Name: "app1", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &app1, s.user)
	c.Assert(err, check.IsNil)
	servicemanager.LogService.Add(app1.Name, "app1 log", "source", "")
	app2 := appTypes.App{Name: "app2", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &app2, s.user)
	c.Assert(err, check.IsNil)
	servicemanager.LogService.Add(app2.Name, "app2 log", "sourc ", "")
	app3 := appTypes.App{Name: "app3", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &app3, s.user)
	c.Assert(err, check.IsNil)
	servicemanager.LogService.Add(app3.Name, "app3 log", "tsuru", "")
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppReadLog,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&lines=10", app3.Name, app3.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = appLog(recorder, request, token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	logs := []appTypes.Applog{}
	err = json.Unmarshal(recorder.Body.Bytes(), &logs)
	c.Assert(err, check.IsNil)
	var logged bool
	for _, log := range logs {
		// Should not show the app1 log
		c.Assert(log.Message, check.Not(check.Equals), "app1 log")
		// Should not show the app2 log
		c.Assert(log.Message, check.Not(check.Equals), "app2 log")
		if log.Message == "app3 log" {
			logged = true
		}
	}
	// Should show the app3 log
	c.Assert(logged, check.Equals, true)
}

func (s *S) TestBindHandlerEndpointIsDown(c *check.C) {
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": "http://localhost:1234"}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := service.Create(context.TODO(), srvc)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)
	a := appTypes.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env:       map[string]bindTypes.EnvVar{},
	}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 1, "web", nil, nil)
	u := fmt.Sprintf("/services/%s/instances/%s/%s", instance.ServiceName, instance.Name, a.Name)
	v := url.Values{}
	v.Set("noRestart", "false")
	request, err := http.NewRequest("PUT", u, strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	errRegex := `Failed to bind app "painkiller" to service instance "mysql/my-mysql":.*`
	c.Assert(recorder.Body.String(), check.Matches, errRegex+"\n")
	c.Assert(eventtest.EventDesc{
		Target:       appTarget(a.Name),
		Owner:        s.token.GetUserName(),
		Kind:         "app.update.bind",
		ErrorMatches: errRegex,
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": ":instance", "value": instance.Name},
			{"name": ":service", "value": instance.ServiceName},
			{"name": "noRestart", "value": "false"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestBindHandler(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := service.Create(context.TODO(), srvc)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)
	a := appTypes.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env:       map[string]bindTypes.EnvVar{},
	}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	s.provisioner.AddUnits(context.TODO(), &a, 1, "web", nil, nil)
	s.provisioner.PrepareOutput([]byte("exported"))
	u := fmt.Sprintf("/services/%s/instances/%s/%s", instance.ServiceName, instance.Name, a.Name)
	b := strings.NewReader("noRestart=false")
	request, err := http.NewRequest("PUT", u, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Check(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": instance.Name}).Decode(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Apps, check.DeepEquals, []string{a.Name})

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": a.Name}).Decode(&a)
	c.Assert(err, check.IsNil)
	allEnvs := provision.EnvsForApp(&a)
	c.Assert(allEnvs["DATABASE_USER"], check.DeepEquals, bindTypes.EnvVar{Name: "DATABASE_USER", Value: "root", Public: false, ManagedBy: "mysql/my-mysql"})
	c.Assert(allEnvs["DATABASE_PASSWORD"], check.DeepEquals, bindTypes.EnvVar{Name: "DATABASE_PASSWORD", Value: "s3cr3t", Public: false, ManagedBy: "mysql/my-mysql"})
	parts := strings.Split(recorder.Body.String(), "\n")
	c.Assert(parts, check.HasLen, 8)
	c.Assert(parts[0], check.Matches, `{"Message":".*---- Setting 3 new environment variables ----\\n","Timestamp":".*"}`)
	c.Assert(parts[1], check.Matches, `{"Message":".*restarting app","Timestamp":".*"}`)
	c.Assert(parts[2], check.Matches, `{"Message":"\\nInstance \\"my-mysql\\" is now bound to the app \\"painkiller\\".\\n","Timestamp":".*"}`)
	c.Assert(parts[3], check.Matches, `{"Message":"The following environment variables are available for use in your app:\\n\\n","Timestamp":".*"}`)
	c.Assert(parts[4], check.Matches, `{"Message":"- DATABASE_(USER|PASSWORD)\\n","Timestamp":".*"}`)
	c.Assert(parts[5], check.Matches, `{"Message":"- DATABASE_(USER|PASSWORD)\\n","Timestamp":".*"}`)
	c.Assert(parts[6], check.Matches, `{"Message":"- TSURU_SERVICES\\n","Timestamp":".*"}`)
	c.Assert(parts[7], check.Equals, "")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.bind",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": ":instance", "value": instance.Name},
			{"name": ":service", "value": instance.ServiceName},
			{"name": "noRestart", "value": "false"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestBindHandlerWithoutEnvsDontRestartTheApp(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := service.Create(context.TODO(), srvc)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)
	a := appTypes.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env:       map[string]bindTypes.EnvVar{},
	}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 1, "web", nil, nil)
	u := fmt.Sprintf("/services/%s/instances/%s/%s", instance.ServiceName, instance.Name, a.Name)
	v := url.Values{}
	v.Set("noRestart", "false")
	request, err := http.NewRequest("PUT", u, strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.provisioner.PrepareOutput([]byte("exported"))
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": instance.Name}).Decode(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Apps, check.DeepEquals, []string{a.Name})

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": a.Name}).Decode(&a)
	c.Assert(err, check.IsNil)
	parts := strings.Split(recorder.Body.String(), "\n")
	c.Assert(parts, check.HasLen, 2)
	c.Assert(parts[0], check.Matches, `{"Message":"\\nInstance \\"my-mysql\\" is now bound to the app \\"painkiller\\".\\n","Timestamp":".*"}`)
	c.Assert(parts[1], check.Equals, "")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.bind",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": ":instance", "value": instance.Name},
			{"name": ":service", "value": instance.ServiceName},
			{"name": "noRestart", "value": "false"},
		},
	}, eventtest.HasEvent)
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 0)
}

func (s *S) TestBindHandlerErrorShowsStatusMessage(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := service.Create(context.TODO(), srvc)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)
	a := appTypes.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env:       map[string]bindTypes.EnvVar{},
	}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 1, "web", nil, nil)
	u := fmt.Sprintf("/services/%s/instances/%s/%s", instance.ServiceName, instance.Name, a.Name)
	v := url.Values{}
	v.Set("noRestart", "false")
	request, err := http.NewRequest("PUT", u, strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), check.Equals, "Failed to bind the instance \"mysql/my-mysql\" to the app \"painkiller\": invalid response:  (code: 500) (\"my-mysql\" is down)\n")
}

func (s *S) TestBindHandlerReturns404IfTheInstanceDoesNotExist(c *check.C) {
	a := appTypes.App{Name: "serviceapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/unknown/instances/unknown/%s?:instance=unknown&:app=%s&:service=unknown&noRestart=false", a.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = bindServiceInstance(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e.Message, check.Equals, service.ErrServiceInstanceNotFound.Error())
}

func (s *S) TestBindHandlerReturns403IfUserIsNotTeamOwner(c *check.C) {
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermServiceInstanceUpdateBind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permTypes.Permission{
		Scheme:  permission.PermAppUpdateBind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permTypes.Permission{
		Scheme:  permission.PermServiceInstanceUpdateBind,
		Context: permission.Context(permTypes.CtxTeam, "anotherteam"),
	})

	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql"}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)

	a := appTypes.App{Name: "serviceapp", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/%s?:service=%s&:instance=%s&:app=%s&noRestart=false", instance.ServiceName, instance.Name,
		a.Name, instance.ServiceName, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = bindServiceInstance(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestBindHandlerReturns403IfTheUserDoesNotHaveAccessToTheInstance(c *check.C) {
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermServiceInstanceUpdateBind,
		Context: permission.Context(permTypes.CtxTeam, "other-team"),
	}, permTypes.Permission{
		Scheme:  permission.PermAppUpdateBind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql"}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)
	a := appTypes.App{Name: "serviceapp", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/%s?:instance=%s&:app=%s&:service=%s&noRestart=false", instance.ServiceName,
		instance.Name, a.Name, instance.Name, a.Name, instance.ServiceName)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = bindServiceInstance(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestBindHandlerReturns404IfTheAppDoesNotExist(c *check.C) {
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/unknown?:instance=%s&:app=unknown&:service=%s&noRestart=false", instance.ServiceName,
		instance.Name, instance.Name, instance.ServiceName)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = bindServiceInstance(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestBindHandlerReturns403IfTheUserDoesNotHaveAccessToTheApp(c *check.C) {
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermServiceInstanceUpdateBind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permTypes.Permission{
		Scheme:  permission.PermAppUpdateBind,
		Context: permission.Context(permTypes.CtxTeam, "other-team"),
	})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)
	a := appTypes.App{Name: "serviceapp", Platform: "zend"}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/%s?:instance=%s&:app=%s&:service=%s&noRestart=false", instance.ServiceName,
		instance.Name, a.Name, instance.Name, a.Name, instance.ServiceName)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = bindServiceInstance(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestBindWithManyInstanceNameWithSameNameAndNoRestartFlag(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvcs := []service.Service{
		{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}},
		{Name: "mysql2", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}},
	}
	for _, srvc := range srvcs {
		err := service.Create(context.TODO(), srvc)
		c.Assert(err, check.IsNil)
	}
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)
	instance2 := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql2",
		Teams:       []string{s.team.Name},
	}
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance2)
	c.Assert(err, check.IsNil)
	a := appTypes.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env:       map[string]bindTypes.EnvVar{},
	}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 1, "web", nil, nil)
	u := fmt.Sprintf("/services/%s/instances/%s/%s", instance2.ServiceName, instance2.Name, a.Name)
	v := url.Values{}
	v.Set("noRestart", "true")
	request, err := http.NewRequest("PUT", u, strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.provisioner.PrepareOutput([]byte("exported"))
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	var result service.ServiceInstance
	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": instance2.Name, "service_name": instance2.ServiceName}).Decode(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Apps, check.DeepEquals, []string{a.Name})

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": a.Name}).Decode(&a)
	c.Assert(err, check.IsNil)

	allEnvs := provision.EnvsForApp(&a)
	c.Assert(allEnvs["DATABASE_USER"], check.DeepEquals, bindTypes.EnvVar{Name: "DATABASE_USER", Value: "root", Public: false, ManagedBy: "mysql2/my-mysql"})
	c.Assert(allEnvs["DATABASE_PASSWORD"], check.DeepEquals, bindTypes.EnvVar{Name: "DATABASE_PASSWORD", Value: "s3cr3t", Public: false, ManagedBy: "mysql2/my-mysql"})
	parts := strings.Split(recorder.Body.String(), "\n")
	c.Assert(parts, check.HasLen, 7)
	c.Assert(parts[0], check.Matches, `{"Message":".*---- Setting 3 new environment variables ----\\n","Timestamp":".*"}`)
	c.Assert(parts[1], check.Matches, `{"Message":"\\nInstance \\"my-mysql\\" is now bound to the app \\"painkiller\\".\\n","Timestamp":".*"}`)
	c.Assert(parts[2], check.Matches, `{"Message":"The following environment variables are available for use in your app:\\n\\n","Timestamp":".*"}`)
	c.Assert(parts[3], check.Matches, `{"Message":"- DATABASE_(USER|PASSWORD)\\n","Timestamp":".*"}`)
	c.Assert(parts[4], check.Matches, `{"Message":"- DATABASE_(USER|PASSWORD)\\n","Timestamp":".*"}`)
	c.Assert(parts[5], check.Matches, `{"Message":"- TSURU_SERVICES\\n","Timestamp":".*"}`)
	c.Assert(parts[6], check.Equals, "")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": instance.Name, "service_name": instance.ServiceName}).Decode(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Apps, check.DeepEquals, []string{})
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.bind",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": ":instance", "value": instance2.Name},
			{"name": ":service", "value": instance2.ServiceName},
			{"name": "noRestart", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnbindHandler(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	s.provisioner.PrepareOutput([]byte("exported"))
	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.StoreInt32(&called, 1)

	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err = service.Create(context.TODO(), srvc)
	c.Assert(err, check.IsNil)
	a := appTypes.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	err = s.provisioner.AddUnits(context.TODO(), &a, 1, "web", nil, nil)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)
	otherApp, err := app.GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	otherApp.ServiceEnvs = append(otherApp.ServiceEnvs, bindTypes.ServiceEnvVar{
		EnvVar: bindTypes.EnvVar{
			Name:  "DATABASE_HOST",
			Value: "arrea",
		},
		InstanceName: instance.Name,
		ServiceName:  instance.ServiceName,
	})
	otherApp.Env["MY_VAR"] = bindTypes.EnvVar{Name: "MY_VAR", Value: "123"}
	_, err = appsCollection.ReplaceOne(context.TODO(), mongoBSON.M{"name": otherApp.Name}, otherApp)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/%s?:service=%s&:instance=%s&:app=%s&noRestart=false", instance.ServiceName, instance.Name, a.Name,
		instance.ServiceName, instance.Name, a.Name)
	req, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, req, s.token)
	c.Assert(err, check.IsNil)
	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": instance.Name}).Decode(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Apps, check.DeepEquals, []string{})
	otherApp, err = app.GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	expected := bindTypes.EnvVar{
		Name:  "MY_VAR",
		Value: "123",
	}
	allEnvs := provision.EnvsForApp(otherApp)
	c.Assert(allEnvs["MY_VAR"], check.DeepEquals, expected)
	_, ok := allEnvs["DATABASE_HOST"]
	c.Assert(ok, check.Equals, false)
	ch := make(chan bool)
	go func() {
		t := time.Tick(1)
		for <-t; atomic.LoadInt32(&called) == 0; <-t {
		}
		ch <- true
	}()
	select {
	case <-ch:
		c.Succeed()
	case <-time.After(1e9):
		c.Error("Failed to call API after 1 second.")
	}
	parts := strings.Split(recorder.Body.String(), "\n")
	c.Assert(parts, check.HasLen, 4)
	c.Assert(parts[0], check.Matches, `{"Message":".*---- Unsetting 1 environment variables ----\\n","Timestamp":".*"}`)
	c.Assert(parts[1], check.Matches, `{"Message":".*restarting app","Timestamp":".*"}`)
	c.Assert(parts[2], check.Matches, `{"Message":".*\\n.*Instance \\"my-mysql\\" is not bound to the app \\"painkiller\\" anymore.\\n","Timestamp":".*"}`)
	c.Assert(parts[3], check.Equals, "")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.unbind",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": ":instance", "value": instance.Name},
			{"name": ":service", "value": instance.ServiceName},
			{"name": "noRestart", "value": "false"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnbindNoRestartFlag(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	s.provisioner.PrepareOutput([]byte("exported"))
	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.StoreInt32(&called, 1)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err = service.Create(context.TODO(), srvc)
	c.Assert(err, check.IsNil)
	a := appTypes.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(context.TODO(), &a, 1, "web", nil, nil)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)
	otherApp, err := app.GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	otherApp.ServiceEnvs = append(otherApp.ServiceEnvs, bindTypes.ServiceEnvVar{
		EnvVar: bindTypes.EnvVar{
			Name:  "DATABASE_HOST",
			Value: "arrea",
		},
		InstanceName: instance.Name,
		ServiceName:  instance.ServiceName,
	})
	otherApp.Env["MY_VAR"] = bindTypes.EnvVar{Name: "MY_VAR", Value: "123"}
	_, err = appsCollection.ReplaceOne(context.TODO(), mongoBSON.M{"name": otherApp.Name}, otherApp)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/%s?:service=%s&:instance=%s&:app=%s&noRestart=true", instance.ServiceName, instance.Name, a.Name,
		instance.ServiceName, instance.Name, a.Name)
	req, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, req, s.token)
	c.Assert(err, check.IsNil)
	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": instance.Name}).Decode(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Apps, check.DeepEquals, []string{})
	otherApp, err = app.GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	expected := bindTypes.EnvVar{
		Name:  "MY_VAR",
		Value: "123",
	}
	allEnvs := provision.EnvsForApp(otherApp)
	c.Assert(allEnvs["MY_VAR"], check.DeepEquals, expected)
	_, ok := allEnvs["DATABASE_HOST"]
	c.Assert(ok, check.Equals, false)
	ch := make(chan bool)
	go func() {
		t := time.Tick(1)
		for <-t; atomic.LoadInt32(&called) == 0; <-t {
		}
		ch <- true
	}()
	select {
	case <-ch:
		c.Succeed()
	case <-time.After(1e9):
		c.Error("Failed to call API after 1 second.")
	}
	parts := strings.Split(recorder.Body.String(), "\n")
	c.Assert(parts, check.HasLen, 3)
	c.Assert(parts[0], check.Matches, `{"Message":".*---- Unsetting 1 environment variables ----\\n","Timestamp":".*"}`)
	c.Assert(parts[1], check.Matches, `{"Message":".*\\n.*Instance \\"my-mysql\\" is not bound to the app \\"painkiller\\" anymore.\\n","Timestamp":".*"}`)
	c.Assert(parts[2], check.Equals, "")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.unbind",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": ":instance", "value": instance.Name},
			{"name": ":service", "value": instance.ServiceName},
			{"name": "noRestart", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnbindForceFlag(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	s.provisioner.PrepareOutput([]byte("exported"))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/bind-app" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("my unbind err"))
		}
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err = service.Create(context.TODO(), srvc)
	c.Assert(err, check.IsNil)
	a := appTypes.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	err = s.provisioner.AddUnits(context.TODO(), &a, 1, "web", nil, nil)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)
	otherApp, err := app.GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	otherApp.ServiceEnvs = append(otherApp.ServiceEnvs, bindTypes.ServiceEnvVar{
		EnvVar: bindTypes.EnvVar{
			Name:  "DATABASE_HOST",
			Value: "arrea",
		},
		InstanceName: instance.Name,
		ServiceName:  instance.ServiceName,
	})
	otherApp.Env["MY_VAR"] = bindTypes.EnvVar{Name: "MY_VAR", Value: "123"}
	_, err = appsCollection.ReplaceOne(context.TODO(), mongoBSON.M{"name": otherApp.Name}, otherApp)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/%s?:service=%s&:instance=%s&:app=%s&force=true", instance.ServiceName, instance.Name, a.Name,
		instance.ServiceName, instance.Name, a.Name)
	req, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermServiceInstanceUpdateUnbind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permTypes.Permission{
		Scheme:  permission.PermAppUpdateUnbind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permTypes.Permission{
		Scheme:  permission.PermServiceUpdate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	err = unbindServiceInstance(recorder, req, token)
	c.Assert(err, check.IsNil)
	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": instance.Name}).Decode(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Apps, check.DeepEquals, []string{})
	otherApp, err = app.GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	expected := bindTypes.EnvVar{
		Name:  "MY_VAR",
		Value: "123",
	}
	allEnvs := provision.EnvsForApp(otherApp)
	c.Assert(allEnvs["MY_VAR"], check.DeepEquals, expected)
	_, ok := allEnvs["DATABASE_HOST"]
	c.Assert(ok, check.Equals, false)
	parts := strings.Split(recorder.Body.String(), "\n")
	c.Assert(parts, check.HasLen, 5)
	c.Assert(parts[0], check.Matches, `{"Message":".*\[unbind-app-endpoint\] ignored error due to force: Failed to unbind \(\\"/resources/my-mysql/bind-app\\"\): invalid response: my unbind err \(code: 500\)\\n","Timestamp":".*"}`)
	c.Assert(parts[1], check.Matches, `{"Message":".*---- Unsetting 1 environment variables ----\\n","Timestamp":".*"}`)
	c.Assert(parts[2], check.Matches, `{"Message":".*restarting app","Timestamp":".*"}`)
	c.Assert(parts[3], check.Matches, `{"Message":".*\\n.*Instance \\"my-mysql\\" is not bound to the app \\"painkiller\\" anymore.\\n","Timestamp":".*"}`)
	c.Assert(parts[4], check.Equals, "")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  token.GetUserName(),
		Kind:   "app.update.unbind",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": ":instance", "value": instance.Name},
			{"name": ":service", "value": instance.ServiceName},
			{"name": "force", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnbindForceFlagNotFailWhenNotAdmin(c *check.C) {
	s.provisioner.PrepareOutput([]byte("exported"))
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": "myendpoint"}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := service.Create(context.TODO(), srvc)
	c.Assert(err, check.IsNil)
	a := appTypes.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(context.TODO(), &a, 1, "web", nil, nil)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/%s?:service=%s&:instance=%s&:app=%s&force=true", instance.ServiceName, instance.Name, a.Name,
		instance.ServiceName, instance.Name, a.Name)
	req, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermServiceInstanceUpdateUnbind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permTypes.Permission{
		Scheme:  permission.PermAppUpdateUnbind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	err = unbindServiceInstance(recorder, req, token)
	c.Assert(err, check.Equals, nil)
}

func (s *S) TestUnbindWithSameInstanceName(c *check.C) {
	s.provisioner.PrepareOutput([]byte("exported"))
	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/bind" {
			atomic.StoreInt32(&called, 1)
		}
	}))
	defer ts.Close()
	srvcs := []service.Service{
		{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}},
		{Name: "mysql2", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}},
	}
	for _, srvc := range srvcs {
		err := service.Create(context.TODO(), srvc)
		c.Assert(err, check.IsNil)
	}
	a := appTypes.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(context.TODO(), &a, 1, "web", nil, nil)
	c.Assert(err, check.IsNil)
	instances := []service.ServiceInstance{
		{
			Name:        "my-mysql",
			ServiceName: "mysql",
			Teams:       []string{s.team.Name},
			Apps:        []string{"painkiller"},
		},
		{
			Name:        "my-mysql",
			ServiceName: "mysql2",
			Teams:       []string{s.team.Name},
			Apps:        []string{"painkiller"},
		},
	}

	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)

	for _, instance := range instances {
		_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
		c.Assert(err, check.IsNil)
	}
	url := fmt.Sprintf("/services/%s/instances/%s/%s?:instance=%s&:app=%s&:service=%s&noRestart=true", instances[1].ServiceName, instances[1].Name, a.Name,
		instances[1].Name, a.Name, instances[1].ServiceName)
	req, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, req, s.token)
	c.Assert(err, check.IsNil)
	var result service.ServiceInstance
	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": instances[1].Name, "service_name": instances[1].ServiceName}).Decode(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Apps, check.DeepEquals, []string{})
	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": instances[0].Name, "service_name": instances[0].ServiceName}).Decode(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Apps, check.DeepEquals, []string{a.Name})
}

func (s *S) TestUnbindHandlerReturns404IfTheInstanceDoesNotExist(c *check.C) {
	a := appTypes.App{Name: "serviceapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/instances/unknown/%s?:instance=unknown&:app=%s&noRestart=false", a.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e.Message, check.Equals, service.ErrServiceInstanceNotFound.Error())
}

func (s *S) TestUnbindHandlerReturns403IfTheUserDoesNotHaveAccessToTheInstance(c *check.C) {
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermServiceInstanceUpdateUnbind,
		Context: permission.Context(permTypes.CtxTeam, "other-team"),
	}, permTypes.Permission{
		Scheme:  permission.PermAppUpdateUnbind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql"}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)
	a := appTypes.App{Name: "serviceapp", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/%s?:service=%s&:instance=%s&:app=%s&noRestart=false", instance.ServiceName, instance.Name,
		a.Name, instance.ServiceName, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestUnbindHandlerReturns403IfUserIsNotTeamOwner(c *check.C) {
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermServiceInstanceUpdateUnbind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permTypes.Permission{
		Scheme:  permission.PermAppUpdateUnbind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permTypes.Permission{
		Scheme:  permission.PermServiceInstanceUpdateUnbind,
		Context: permission.Context(permTypes.CtxTeam, "anotherteam"),
	})

	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql"}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)

	a := appTypes.App{Name: "serviceapp", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/%s?:service=%s&:instance=%s&:app=%s&noRestart=false", instance.ServiceName, instance.Name,
		a.Name, instance.ServiceName, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestUnbindHandlerReturns404IfTheAppDoesNotExist(c *check.C) {
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/unknown?:service=%s&:instance=%s&:app=unknown&noRestart=false", instance.ServiceName,
		instance.Name, instance.ServiceName, instance.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestUnbindHandlerReturns403IfTheUserDoesNotHaveAccessToTheApp(c *check.C) {
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermServiceInstanceUpdateUnbind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permTypes.Permission{
		Scheme:  permission.PermAppUpdateUnbind,
		Context: permission.Context(permTypes.CtxTeam, "other-team"),
	})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)
	a := appTypes.App{Name: "serviceapp", Platform: "zend"}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/%s?:service=%s&:instance=%s&:app=%s&noRestart=false", instance.ServiceName, instance.Name,
		a.Name, instance.ServiceName, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestRestartHandler(c *check.C) {
	config.Set("docker:router", "fake")
	defer config.Unset("docker:router")
	a := appTypes.App{
		Name:      "stress",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	url := fmt.Sprintf("/apps/%s/restart", a.Name)
	request, err := http.NewRequest("POST", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Restarting the app \\"stress\\" ----\\n","Timestamp":".*"}`+"\n"+
			`{"Message":".*restarting app","Timestamp":".*"}`+"\n",
	)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.restart",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRestartHandlerSingleProcess(c *check.C) {
	config.Set("docker:router", "fake")
	defer config.Unset("docker:router")
	a := appTypes.App{
		Name:      "stress",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	url := fmt.Sprintf("/apps/%s/restart", a.Name)
	body := strings.NewReader("process=web")
	request, err := http.NewRequest("POST", url, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Restarting process \\"web\\" ----\\n","Timestamp":".*"}`+"\n"+
			`{"Message":".*restarting app","Timestamp":".*"}`+"\n",
	)
	restarts := s.provisioner.Restarts(&a, "web")
	c.Assert(restarts, check.Equals, 1)
	restarts = s.provisioner.Restarts(&a, "worker")
	c.Assert(restarts, check.Equals, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.restart",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "process", "value": "web"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRestartHandlerReturns404IfTheAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("GET", "/apps/unknown/restart?:app=unknown", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = restart(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestRestartHandlerReturns403IfTheUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := appTypes.App{Name: "nightmist"}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdateRestart,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	url := fmt.Sprintf("/apps/%s/restart?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = restart(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

type LogList []appTypes.Applog

func (l LogList) Len() int           { return len(l) }
func (l LogList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l LogList) Less(i, j int) bool { return l[i].Message < l[j].Message }

func (s *S) TestAddLog(c *check.C) {
	a := appTypes.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Add("message", "message 1")
	v.Add("message", "message 2")
	v.Add("message", "message 3")
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdateLog,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request, err := http.NewRequest("POST", "/apps/myapp/log", strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	v = url.Values{}
	v.Add("message", "message 4")
	v.Add("message", "message 5")
	v.Set("source", "mysource")
	withSourceRequest, err := http.NewRequest("POST", "/apps/myapp/log", strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	withSourceRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	withSourceRequest.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	recorder = httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, withSourceRequest)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	want := []string{
		"message 1",
		"message 2",
		"message 3",
		"message 4",
		"message 5",
	}
	wantSource := []string{
		"app",
		"app",
		"app",
		"mysource",
		"mysource",
	}
	logs, err := app.LastLogs(context.TODO(), &a, servicemanager.LogService, appTypes.ListLogArgs{
		Limit: 5,
	})
	c.Assert(err, check.IsNil)
	got := make([]string, len(logs))
	gotSource := make([]string, len(logs))
	sort.Sort(LogList(logs))
	for i, l := range logs {
		got[i] = l.Message
		gotSource[i] = l.Source
	}
	c.Assert(got, check.DeepEquals, want)
	c.Assert(gotSource, check.DeepEquals, wantSource)
}

func (s *S) TestGetApp(c *check.C) {
	ctx := context.Background()
	a := appTypes.App{Name: "testapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(ctx, &a, s.user)
	c.Assert(err, check.IsNil)
	expected, err := app.GetByName(ctx, a.Name)
	c.Assert(err, check.IsNil)
	r, err := http.NewRequest(http.MethodGet, "", nil)
	c.Assert(err, check.IsNil)
	app, err := getAppFromContext(a.Name, r)
	c.Assert(err, check.IsNil)
	c.Assert(app, check.DeepEquals, expected)
}

func (s *S) TestSwapDeprecated(c *check.C) {
	app1 := appTypes.App{Name: "app1", Platform: "x", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := appTypes.App{Name: "app2", Platform: "y", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &app2, s.user)
	c.Assert(err, check.IsNil)
	b := strings.NewReader("app1=app1&app2=app2&force=true&cnameOnly=false")
	request, err := http.NewRequest("POST", "/swap", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusPreconditionFailed)
	c.Assert(recorder.Body.String(), check.Equals, "swapping is deprecated\n")
}

func (s *S) TestStartHandler(c *check.C) {
	config.Set("docker:router", "fake")
	defer config.Unset("docker:router")
	a := appTypes.App{
		Name:      "stress",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	url := fmt.Sprintf("/apps/%s/start", a.Name)
	body := strings.NewReader("process=web")
	request, err := http.NewRequest("POST", url, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*\\n.* ---\\u003e Starting the process \\"web\\"\\n","Timestamp":".*"}`+"\n",
	)
	starts := s.provisioner.Starts(&a, "web")
	c.Assert(starts, check.Equals, 1)
	starts = s.provisioner.Starts(&a, "worker")
	c.Assert(starts, check.Equals, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.start",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "process", "value": "web"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestStopHandler(c *check.C) {
	a := appTypes.App{Name: "stress", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	url := fmt.Sprintf("/apps/%s/stop", a.Name)
	body := strings.NewReader("process=web")
	request, err := http.NewRequest("POST", url, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*\\n.* ---\\u003e Stopping the process \\"web\\"\\n","Timestamp":".*"}`+"\n",
	)
	stops := s.provisioner.Stops(&a, "web")
	c.Assert(stops, check.Equals, 1)
	stops = s.provisioner.Stops(&a, "worker")
	c.Assert(stops, check.Equals, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.stop",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "process", "value": "web"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestForceDeleteLock(c *check.C) {
	a := appTypes.App{Name: "locked"}
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/apps/locked/lock", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusGone)
	c.Assert(recorder.Body.String(), check.Equals, "app unlock is deprecated, this call does nothing\n")
}

func (s *S) TestRebuildRoutes(c *check.C) {
	a := appTypes.App{Name: "myappx", Platform: "zend", TeamOwner: s.team.Name, Router: "fake"}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.Provision(context.TODO(), &a)
	err = routertest.FakeRouter.EnsureBackend(context.TODO(), &a, router.EnsureBackendOpts{})
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("dry", "true")
	body := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", "/apps/myappx/routes", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)

	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.admin.routes",
		StartCustomData: []map[string]interface{}{
			{"name": "dry", "value": "true"},
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetCertificate(c *check.C) {
	a := appTypes.App{Name: "myapp", TeamOwner: s.team.Name, CName: []string{"app.io"}, Router: "fake-tls"}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("cname", "app.io")
	v.Set("certificate", testCert)
	v.Set("key", testKey)
	body := strings.NewReader(v.Encode())
	request, err := http.NewRequest("PUT", "/apps/myapp/certificate", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.certificate.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "cname", "value": "app.io"},
			{"name": "certificate", "value": testCert},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetCertificateInvalidCname(c *check.C) {
	a := appTypes.App{Name: "myapp", TeamOwner: s.team.Name, CName: []string{"app.io"}, Router: "fake-tls"}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("cname", "app2.io")
	v.Set("certificate", testCert)
	v.Set("key", testKey)
	body := strings.NewReader(v.Encode())
	request, err := http.NewRequest("PUT", "/apps/myapp/certificate", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Equals, "invalid name\n")
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestSetCertificateInvalidCertificate(c *check.C) {
	a := appTypes.App{Name: "myapp", TeamOwner: s.team.Name, CName: []string{"myapp.io"}, Router: "fake-tls"}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("cname", "myapp.io")
	v.Set("certificate", testCert)
	v.Set("key", testKey)
	body := strings.NewReader(v.Encode())
	request, err := http.NewRequest("PUT", "/apps/myapp/certificate", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Equals, "x509: certificate is valid for app.io, not myapp.io\n")
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestSetCertificateNonSupportedRouter(c *check.C) {
	a := appTypes.App{Name: "myapp", TeamOwner: s.team.Name, CName: []string{"app.io"}}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("cname", "app.io")
	v.Set("certificate", testCert)
	v.Set("key", testKey)
	body := strings.NewReader(v.Encode())
	request, err := http.NewRequest("PUT", "/apps/myapp/certificate", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Equals, "no router with tls support\n")
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestUnsetCertificate(c *check.C) {
	ctx := context.Background()
	a := appTypes.App{Name: "myapp", TeamOwner: s.team.Name, CName: []string{"app.io"}, Router: "fake-tls"}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = app.SetCertificate(ctx, &a, "app.io", testCert, testKey)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/apps/myapp/certificate?cname=app.io", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.certificate.unset",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "cname", "value": "app.io"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnsetCertificateWithoutCName(c *check.C) {
	ctx := context.Background()
	a := appTypes.App{Name: "myapp", TeamOwner: s.team.Name, CName: []string{"app.io"}, Router: "fake-tls"}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = app.SetCertificate(ctx, &a, "app.io", testCert, testKey)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/apps/myapp/certificate", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "You must provide a cname.\n")
}

func (s *S) TestUnsetCertificateInvalidCName(c *check.C) {
	ctx := context.Background()
	a := appTypes.App{Name: "myapp", TeamOwner: s.team.Name, CName: []string{"app.io"}, Router: "fake-tls"}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = app.SetCertificate(ctx, &a, "app.io", testCert, testKey)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/apps/myapp/certificate?cname=myapp.io", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "invalid name\n")
}

func (s *S) TestListCertificates(c *check.C) {
	ctx := context.Background()
	a := appTypes.App{
		Name:        "myapp",
		TeamOwner:   s.team.Name,
		Router:      "fake-tls",
		CName:       []string{"app.io"},
		CertIssuers: map[string]string{"app.io": "letsencrypt"},
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = app.SetCertificate(ctx, &a, "app.io", testCert, testKey)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/apps/myapp/certificate", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	certs := map[string]map[string]string{}
	err = json.Unmarshal(recorder.Body.Bytes(), &certs)
	c.Assert(err, check.IsNil)
	c.Assert(certs, check.DeepEquals, map[string]map[string]string{
		"fake-tls": {
			"app.io":                  string(testCert),
			"myapp.faketlsrouter.com": "<mock cert>",
		},
	})
}

func (s *S) TestListCertificatesLegacy(c *check.C) {
	ctx := context.Background()
	a := appTypes.App{
		Name:        "myapp",
		TeamOwner:   s.team.Name,
		Router:      "fake-tls",
		CName:       []string{"app.io"},
		CertIssuers: map[string]string{"app.io": "letsencrypt"},
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = app.SetCertificate(ctx, &a, "app.io", testCert, testKey)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/1.2/apps/myapp/certificate", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	certs := map[string]map[string]string{}
	err = json.Unmarshal(recorder.Body.Bytes(), &certs)
	c.Assert(err, check.IsNil)
	c.Assert(certs, check.DeepEquals, map[string]map[string]string{
		"fake-tls": {
			"app.io":                  string(testCert),
			"myapp.faketlsrouter.com": "<mock cert>",
		},
	})

}

func (s *S) TestListCertificatesNew(c *check.C) {
	ctx := context.Background()
	a := appTypes.App{
		Name:        "myapp",
		TeamOwner:   s.team.Name,
		Router:      "fake-tls",
		CName:       []string{"app.io"},
		CertIssuers: map[string]string{"app.io": "letsencrypt"},
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = app.SetCertificate(ctx, &a, "app.io", testCert, testKey)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/1.24/apps/myapp/certificate", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var certs appTypes.CertificateSetInfo
	err = json.Unmarshal(recorder.Body.Bytes(), &certs)
	c.Assert(err, check.IsNil)
	c.Assert(certs, check.DeepEquals, appTypes.CertificateSetInfo{
		Routers: map[string]appTypes.RouterCertificateInfo{
			"fake-tls": {
				CNames: map[string]appTypes.CertificateInfo{
					"app.io": {
						Certificate: string(testCert),
						Issuer:      "letsencrypt",
					},
					"myapp.faketlsrouter.com": {
						Certificate: "<mock cert>",
					},
				},
			},
		},
	})
}

func (s *S) TestSetCertIssuer(c *check.C) {
	a := appTypes.App{Name: "myapp", TeamOwner: s.team.Name, CName: []string{"app.io"}, Router: "fake-tls"}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("cname", "app.io")
	v.Set("issuer", "letsencrypt")
	body := strings.NewReader(v.Encode())
	request, err := http.NewRequest("PUT", "/apps/myapp/certissuer", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "certissuer.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "cname", "value": "app.io"},
			{"name": "issuer", "value": "letsencrypt"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetCertIssuerInvalidCname(c *check.C) {
	a := appTypes.App{Name: "myapp", TeamOwner: s.team.Name, CName: []string{"app.io"}, Router: "fake-tls"}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("cname", "app2.io")
	v.Set("issuer", "letsencrypt")
	body := strings.NewReader(v.Encode())
	request, err := http.NewRequest("PUT", "/apps/myapp/certissuer", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Equals, "cname does not exist in app (app2.io)\n")
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestUnsetCertIssuer(c *check.C) {
	a := appTypes.App{Name: "myapp", TeamOwner: s.team.Name, CName: []string{"app.io"}, Router: "fake-tls"}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/apps/myapp/certissuer?cname=app.io", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "certissuer.unset",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "cname", "value": "app.io"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnsetCertIssuerWithoutCName(c *check.C) {
	a := appTypes.App{Name: "myapp", TeamOwner: s.team.Name, CName: []string{"app.io"}, Router: "fake-tls"}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/apps/myapp/certissuer", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "You must provide a cname.\n")
}

func (s *S) TestUnsetCertIssuerInvalidCName(c *check.C) {
	a := appTypes.App{Name: "myapp", TeamOwner: s.team.Name, CName: []string{"app.io"}, Router: "fake-tls"}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/apps/myapp/certissuer?cname=myapp.io", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "cname does not exist in app (myapp.io)\n")
}

type fakeEncoder struct {
	done chan struct{}
	msg  interface{}
}

func (e *fakeEncoder) Encode(msg interface{}) error {
	e.msg = msg
	close(e.done)
	return nil
}

func (s *S) TestFollowLogs(c *check.C) {
	a := appTypes.App{Name: "lost1", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	ctx, cancel := context.WithCancel(context.Background())
	enc := &fakeEncoder{
		done: make(chan struct{}),
	}
	l, err := servicemanager.LogService.Watch(context.TODO(), appTypes.ListLogArgs{
		Name: a.Name,
		Type: logTypes.LogTypeApp,
	})
	c.Assert(err, check.IsNil)
	go func() {
		err = servicemanager.LogService.Add(a.Name, "xyz", "", "")
		c.Assert(err, check.IsNil)
		<-enc.done
		cancel()
	}()
	err = followLogs(ctx, a.Name, l, enc)
	c.Assert(err, check.IsNil)
	msgSlice, ok := enc.msg.([]appTypes.Applog)
	c.Assert(ok, check.Equals, true)
	c.Assert(msgSlice, check.HasLen, 1)
	c.Assert(msgSlice[0].Message, check.Equals, "xyz")
}
