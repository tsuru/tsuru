// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/rec/rectest"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/service"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestAppIsAvailableHandlerShouldReturnErrorWhenAppStatusIsnotStarted(c *check.C) {
	a := app.App{Name: "someapp", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	defer s.logConn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/available?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = appIsAvailable(recorder, request, s.token)
	c.Assert(err, check.NotNil)
}

func (s *S) TestAppIsAvailableHandlerShouldReturn200WhenAppUnitStatusIsStarted(c *check.C) {
	a := app.App{Name: "someapp", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	defer s.logConn.Logs(a.Name).DropCollection()
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	url := fmt.Sprintf("/apps/%s/repository/clone?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = appIsAvailable(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestAppListFilteringByPlatform(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&app1)
	platform := app.Platform{Name: "python"}
	s.conn.Platforms().Insert(platform)
	defer s.conn.Platforms().Remove(bson.M{"name": "python"})
	app2 := app.App{Name: "app2", Platform: "python", Teams: []string{s.team.Name}}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&app2)
	request, err := http.NewRequest("GET", "/apps?platform=zend", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = appList(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	apps := []app.App{}
	err = json.Unmarshal(body, &apps)
	c.Assert(err, check.IsNil)
	expected := []app.App{app1}
	c.Assert(len(apps), check.Equals, len(expected))
	for i, app := range apps {
		c.Assert(app.Name, check.DeepEquals, expected[i].Name)
		c.Assert(app.Units(), check.DeepEquals, expected[i].Units())
	}
	action := rectest.Action{Action: "app-list", User: s.user.Email, Extra: []interface{}{"platform=zend"}}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestAppListFilteringByTeamOwner(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&app1)
	app2 := app.App{Name: "app2", Platform: "zend", Teams: []string{s.adminteam.Name}}
	err = app.CreateApp(&app2, s.adminuser)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&app2)
	request, err := http.NewRequest("GET", fmt.Sprintf("/apps?teamowner=%s", s.team.Name), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = appList(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	apps := []app.App{}
	err = json.Unmarshal(body, &apps)
	c.Assert(err, check.IsNil)
	expected := []app.App{app1}
	c.Assert(len(apps), check.Equals, len(expected))
	for i, app := range apps {
		c.Assert(app.Name, check.DeepEquals, expected[i].Name)
		c.Assert(app.Units(), check.DeepEquals, expected[i].Units())
	}
	queryString := fmt.Sprintf("teamowner=%s", s.team.Name)
	action := rectest.Action{Action: "app-list", User: s.user.Email, Extra: []interface{}{queryString}}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestAppListFilteringByOwner(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&app1)
	platform := app.Platform{Name: "python"}
	s.conn.Platforms().Insert(platform)
	defer s.conn.Platforms().Remove(bson.M{"name": "python"})
	app2 := app.App{Name: "app2", Platform: "python", Teams: []string{s.team.Name}}
	err = app.CreateApp(&app2, s.adminuser)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&app2)
	request, err := http.NewRequest("GET", fmt.Sprintf("/apps?owner=%s", s.user.Email), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = appList(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	apps := []app.App{}
	err = json.Unmarshal(body, &apps)
	c.Assert(err, check.IsNil)
	expected := []app.App{app1}
	c.Assert(len(apps), check.Equals, len(expected))
	for i, app := range apps {
		c.Assert(app.Name, check.DeepEquals, expected[i].Name)
		c.Assert(app.Units(), check.DeepEquals, expected[i].Units())
	}
	queryString := fmt.Sprintf("owner=%s", s.user.Email)
	action := rectest.Action{Action: "app-list", User: s.user.Email, Extra: []interface{}{queryString}}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestAppList(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "zend", Teams: []string{s.team.Name}, CName: []string{"cname.app1"}}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&app1)
	app2 := app.App{Name: "app2", Platform: "zend", Teams: []string{s.team.Name}, CName: []string{"cname.app2"}}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&app2)
	request, err := http.NewRequest("GET", "/apps/", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = appList(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	var apps []miniApp
	err = json.Unmarshal(body, &apps)
	c.Assert(err, check.IsNil)
	expected := []miniApp{minifyApp(app1), minifyApp(app2)}
	c.Assert(len(apps), check.Equals, len(expected))
	c.Assert(apps, check.DeepEquals, expected)
	action := rectest.Action{Action: "app-list", User: s.user.Email}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestAppListShouldListAllAppsOfAllTeamsThatTheUserIsAMember(c *check.C) {
	u := auth.User{Email: "passing-by@angra.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	team := auth.Team{Name: "angra", Users: []string{s.user.Email, u.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": team.Name})
	app1 := app.App{Name: "app1", Platform: "zend", TeamOwner: "angra"}
	err = app.CreateApp(&app1, &u)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&app1)
	request, err := http.NewRequest("GET", "/apps/", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = appList(recorder, request, token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	var apps []miniApp
	err = json.Unmarshal(body, &apps)
	c.Assert(err, check.IsNil)
	c.Assert(apps[0].Name, check.Equals, app1.Name)
}

func (s *S) TestListShouldReturnStatusNoContentWhenAppListIsNil(c *check.C) {
	request, err := http.NewRequest("GET", "/apps/", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = appList(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestDelete(c *check.C) {
	myApp := &app.App{
		Name:     "myapptodelete",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := app.CreateApp(myApp, s.user)
	c.Assert(err, check.IsNil)
	myApp, err = app.GetByName(myApp.Name)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(myApp)
	request, err := http.NewRequest("DELETE", "/apps/"+myApp.Name+"?:app="+myApp.Name, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = appDelete(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	action := rectest.Action{
		Action: "app-delete",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + myApp.Name},
	}
	c.Assert(action, rectest.IsRecorded)
	_, err = repository.Manager().GetRepository(myApp.Name)
	c.Assert(err, check.NotNil)
}

func (s *S) TestDeleteShouldReturnForbiddenIfTheGivenUserDoesNotHaveAccesToTheApp(c *check.C) {
	myApp := app.App{Name: "app-to-delete", Platform: "zend"}
	err := s.conn.Apps().Insert(myApp)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": myApp.Name})
	request, err := http.NewRequest("DELETE", "/apps/"+myApp.Name+"?:app="+myApp.Name, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = appDelete(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e, check.ErrorMatches, "^user does not have access to this app$")
}

func (s *S) TestDeleteShouldReturnNotFoundIfTheAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("DELETE", "/apps/unknown?:app=unknown", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = appDelete(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestAppInfo(c *check.C) {
	config.Set("host", "http://myhost.com")
	expectedApp := app.App{Name: "new-app", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&expectedApp, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&expectedApp)
	var myApp map[string]interface{}
	request, err := http.NewRequest("GET", "/apps/"+expectedApp.Name+"?:app="+expectedApp.Name, nil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	c.Assert(err, check.IsNil)
	err = appInfo(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	err = json.Unmarshal(body, &myApp)
	c.Assert(err, check.IsNil)
	c.Assert(myApp["name"], check.Equals, expectedApp.Name)
	c.Assert(myApp["repository"], check.Equals, "git@"+repositorytest.ServerHost+":"+expectedApp.Name+".git")
	action := rectest.Action{
		Action: "app-info",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + expectedApp.Name},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestAppInfoReturnsForbiddenWhenTheUserDoesNotHaveAccessToTheApp(c *check.C) {
	expectedApp := app.App{Name: "new-app", Platform: "zend"}
	err := s.conn.Apps().Insert(expectedApp)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": expectedApp.Name})
	request, err := http.NewRequest("GET", "/apps/"+expectedApp.Name+"?:app="+expectedApp.Name, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = appInfo(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e, check.ErrorMatches, "^user does not have access to this app$")
}

func (s *S) TestAppInfoReturnsNotFoundWhenAppDoesNotExist(c *check.C) {
	myApp := app.App{Name: "SomeApp"}
	request, err := http.NewRequest("GET", "/apps/"+myApp.Name+"?:app="+myApp.Name, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = appInfo(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^App SomeApp not found.$")
}

func (s *S) TestCreateAppHandler(c *check.C) {
	a := app.App{Name: "someapp"}
	defer func() {
		a, err := app.GetByName("someapp")
		c.Assert(err, check.IsNil)
		s.deleteApp(a)
	}()
	data := `{"name":"someapp","platform":"zend"}`
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = createApp(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	repoURL := "git@" + repositorytest.ServerHost + ":" + a.Name + ".git"
	var obtained map[string]string
	expected := map[string]string{
		"status":         "success",
		"repository_url": repoURL,
		"ip":             "someapp.fake-lb.tsuru.io",
	}
	err = json.Unmarshal(body, &obtained)
	c.Assert(obtained, check.DeepEquals, expected)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "someapp"}).One(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	action := rectest.Action{
		Action: "create-app",
		User:   s.user.Email,
		Extra:  []interface{}{"app=someapp", "platform=zend", "plan="},
	}
	c.Assert(action, rectest.IsRecorded)
	_, err = repository.Manager().GetRepository(a.Name)
	c.Assert(err, check.IsNil)
}

func (s *S) TestCreateAppTeamOwner(c *check.C) {
	a := app.App{Name: "someapp"}
	defer func() {
		a, err := app.GetByName("someapp")
		c.Assert(err, check.IsNil)
		s.deleteApp(a)
	}()
	data := `{"name":"someapp","platform":"zend","teamOwner":"tsuruteam"}`
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = createApp(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": a.Name}).One(&gotApp)
	c.Assert(err, check.IsNil)
	repoURL := "git@" + repositorytest.ServerHost + ":" + a.Name + ".git"
	var appIP string
	appIP, err = s.provisioner.Addr(&gotApp)
	c.Assert(err, check.IsNil)
	var obtained map[string]string
	expected := map[string]string{
		"status":         "success",
		"repository_url": repoURL,
		"ip":             appIP,
	}
	err = json.Unmarshal(body, &obtained)
	c.Assert(obtained, check.DeepEquals, expected)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	action := rectest.Action{
		Action: "create-app",
		User:   s.user.Email,
		Extra:  []interface{}{"app=someapp", "platform=zend", "plan="},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestCreateAppCustomPlan(c *check.C) {
	a := app.App{Name: "someapp"}
	defer func() {
		a, err := app.GetByName("someapp")
		c.Assert(err, check.IsNil)
		s.deleteApp(a)
	}()
	expectedPlan := app.Plan{
		Name:     "myplan",
		Memory:   10,
		Swap:     5,
		CpuShare: 10,
	}
	err := expectedPlan.Save()
	c.Assert(err, check.IsNil)
	defer app.PlanRemove(expectedPlan.Name)
	data := `{"name":"someapp","platform":"zend","plan":{"name":"myplan"}}`
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = createApp(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	repoURL := "git@" + repositorytest.ServerHost + ":" + a.Name + ".git"
	var obtained map[string]string
	expected := map[string]string{
		"status":         "success",
		"repository_url": repoURL,
		"ip":             "someapp.fake-lb.tsuru.io",
	}
	err = json.Unmarshal(body, &obtained)
	c.Assert(obtained, check.DeepEquals, expected)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "someapp"}).One(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	c.Assert(gotApp.Plan, check.DeepEquals, expectedPlan)
	action := rectest.Action{
		Action: "create-app",
		User:   s.user.Email,
		Extra:  []interface{}{"app=someapp", "platform=zend", "plan=myplan"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestCreateAppTwoTeams(c *check.C) {
	team := auth.Team{Name: "tsurutwo", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(team)
	c.Check(err, check.IsNil)
	defer s.conn.Teams().RemoveId(team.Name)
	data := `{"name":"someapp","platform":"zend"}`
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = createApp(recorder, request, s.token)
	c.Assert(err, check.NotNil)
}

func (s *S) TestCreateAppQuotaExceeded(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	var limited quota.Quota
	conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota": limited}})
	defer conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota": quota.Unlimited}})
	b := strings.NewReader(`{"name":"someapp","platform":"zend"}`)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = createApp(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e.Message, check.Matches, "^.*Quota exceeded$")
}

func (s *S) TestCreateAppInvalidName(c *check.C) {
	b := strings.NewReader(`{"name":"123myapp","platform":"zend"}`)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = createApp(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	msg := "Invalid app name, your app should have at most 63 " +
		"characters, containing only lower case letters, numbers " +
		"or dashes, starting with a letter."
	c.Assert(e.Error(), check.Equals, msg)
}

func (s *S) TestCreateAppReturns400IfTheUserIsNotMemberOfAnyTeam(c *check.C) {
	u := &auth.User{Email: "thetrees@rush.com", Password: "123456"}
	_, err := nativeScheme.Create(u)
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	b := strings.NewReader(`{"name":"someapp", "platform":"django"}`)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = createApp(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e, check.ErrorMatches, "^In order to create an app, you should be member of at least one team$")
}

func (s *S) TestCreateAppReturnsConflictWithProperMessageWhenTheAppAlreadyExist(c *check.C) {
	a := app.App{Name: "plainsofdawn", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	b := strings.NewReader(`{"name":"plainsofdawn","platform":"zend"}`)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = createApp(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, ".*there is already an app with this name.*")
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusConflict)
}

func (s *S) TestAddUnits(c *check.C) {
	a := app.App{Name: "armorandsword", Platform: "zend", Teams: []string{s.team.Name}, Quota: quota.Unlimited}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	body := strings.NewReader("units=3&process=web")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:app=armorandsword", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = addUnits(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.Units(), check.HasLen, 3)
	action := rectest.Action{
		Action: "add-units",
		User:   s.user.Email,
		Extra:  []interface{}{"app=armorandsword", "units=3"},
	}
	c.Assert(action, rectest.IsRecorded)
	c.Assert(recorder.Body.String(), check.Equals, `{"Message":"added 3 units"}`+"\n")
}

func (s *S) TestAddUnitsReturns404IfAppDoesNotExist(c *check.C) {
	body := strings.NewReader("units=1&process=web")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:app=armorandsword", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = addUnits(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e.Message, check.Equals, "App armorandsword not found.")
}

func (s *S) TestAddUnitsReturns403IfTheUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := app.App{Name: "armorandsword", Platform: "zend"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	body := strings.NewReader("units=1&process=web")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:app=armorandsword", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = addUnits(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e.Message, check.Equals, "user does not have access to this app")
}

func (s *S) TestAddUnitsReturns400IfNumberOfUnitsIsOmited(c *check.C) {
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

func (s *S) TestAddUnitsWorksIfProcessIsOmited(c *check.C) {
	a := app.App{Name: "armorandsword", Platform: "zend", Teams: []string{s.team.Name}, Quota: quota.Unlimited}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	body := strings.NewReader("units=3&process=")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:app=armorandsword", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = addUnits(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.Units(), check.HasLen, 3)
	action := rectest.Action{
		Action: "add-units",
		User:   s.user.Email,
		Extra:  []interface{}{"app=armorandsword", "units=3"},
	}
	c.Assert(action, rectest.IsRecorded)
	c.Assert(recorder.Body.String(), check.Equals, `{"Message":"added 3 units"}`+"\n")
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
	a := app.App{Name: "armorandsword", Platform: "zend", Teams: []string{s.team.Name}, Quota: quota.Quota{Limit: 2}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	body := strings.NewReader("units=3&process=web")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:app=armorandsword", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = addUnits(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Body.String(), check.Equals, `{"Message":"","Error":"Quota exceeded. Available: 2. Requested: 3."}`+"\n")
}

func (s *S) TestRemoveUnits(c *check.C) {
	a := app.App{Name: "velha", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	request, err := http.NewRequest("DELETE", "/apps/velha/units?:app=velha&units=2&process=web", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUnits(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.Units(), check.HasLen, 1)
	c.Assert(s.provisioner.GetUnits(app), check.HasLen, 1)
	action := rectest.Action{
		Action: "remove-units",
		User:   s.user.Email,
		Extra:  []interface{}{"app=velha", "units=2"},
	}
	c.Assert(action, rectest.IsRecorded)
	c.Assert(recorder.Body.String(), check.Equals, `{"Message":"removing 2 units"}`+"\n")
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
	a := app.App{Name: "fetisha", Platform: "zend"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	request, err := http.NewRequest("DELETE", "/apps/fetisha/units?:app=fetisha&units=1&process=web", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUnits(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e.Message, check.Equals, "user does not have access to this app")
}

func (s *S) TestRemoveUnitsReturns400IfNumberOfUnitsIsOmited(c *check.C) {
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

func (s *S) TestRemoveUnitsWorksIfProcessIsOmited(c *check.C) {
	a := app.App{Name: "velha", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	s.provisioner.AddUnits(&a, 3, "", nil)
	request, err := http.NewRequest("DELETE", "/apps/velha/units?:app=velha&units=2&process=", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUnits(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.Units(), check.HasLen, 1)
	c.Assert(s.provisioner.GetUnits(app), check.HasLen, 1)
	action := rectest.Action{
		Action: "remove-units",
		User:   s.user.Email,
		Extra:  []interface{}{"app=velha", "units=2"},
	}
	c.Assert(action, rectest.IsRecorded)
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

func (s *S) TestSetUnitStatus(c *check.C) {
	a := app.App{Name: "telegram", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	body := strings.NewReader("status=error")
	unit := a.Units()[0]
	request, err := http.NewRequest("POST", "/apps/telegram/units/<unit-name>?:app=telegram&:unit="+unit.Name, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = setUnitStatus(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	unit = a.Units()[0]
	c.Assert(unit.Status, check.Equals, provision.StatusError)
}

func (s *S) TestSetUnitStatusNoUnit(c *check.C) {
	body := strings.NewReader("status=error")
	request, err := http.NewRequest("POST", "/apps/velha/units/af32db?:app=velha", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = setUnitStatus(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, "missing unit")
}

func (s *S) TestSetUnitStatusInvalidStatus(c *check.C) {
	bodies := []io.Reader{strings.NewReader("status=something"), strings.NewReader("")}
	for _, body := range bodies {
		request, err := http.NewRequest("POST", "/apps/velha/units/af32db?:app=velha&:unit=af32db", body)
		c.Assert(err, check.IsNil)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()
		err = setUnitStatus(recorder, request, s.token)
		c.Assert(err, check.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, check.Equals, true)
		c.Check(e.Code, check.Equals, http.StatusBadRequest)
		c.Check(e.Message, check.Equals, provision.ErrInvalidStatus.Error())
	}
}

func (s *S) TestSetUnitStatusAppNotFound(c *check.C) {
	body := strings.NewReader("status=error")
	request, err := http.NewRequest("POST", "/apps/velha/units/af32db?:app=velha&:unit=af32db", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = setUnitStatus(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Check(e.Code, check.Equals, http.StatusNotFound)
	c.Check(e.Message, check.Equals, "App not found.")
}

func (s *S) TestSetUnitStatusDoesntRequireLock(c *check.C) {
	a := app.App{Name: "telegram", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	locked, err := app.AcquireApplicationLock(a.Name, "test", "test")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	unit := a.Units()[0]
	body := strings.NewReader("status=error")
	request, err := http.NewRequest("POST", "/apps/telegram/units/"+unit.Name, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	unit = a.Units()[0]
	c.Assert(unit.Status, check.Equals, provision.StatusError)
}

func (s *S) TestSetUnitsStatus(c *check.C) {
	token, err := nativeScheme.AppLogin(app.InternalAppName)
	c.Assert(err, check.IsNil)
	a := app.App{Name: "telegram", Platform: "zend", Teams: []string{s.team.Name}}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	units := a.Units()
	var body bytes.Buffer
	status := []string{"started", "error", "stopped"}
	payload := make([]map[string]string, len(status)+2)
	for i, st := range status {
		payload[i] = map[string]string{"ID": units[i].Name, "Status": st}
	}
	payload[len(status)] = map[string]string{"ID": "not-found1", "Status": "error"}
	payload[len(status)+1] = map[string]string{"ID": "not-found2", "Status": "started"}
	err = json.NewEncoder(&body).Encode(payload)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/units/status", &body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	for i, unit := range a.Units() {
		c.Check(unit.Status, check.Equals, provision.Status(status[i]))
	}
	var got updateList
	expected := updateList([]updateUnitsResponse{
		{ID: units[0].Name, Found: true},
		{ID: units[1].Name, Found: true},
		{ID: units[2].Name, Found: true},
		{ID: "not-found1", Found: false},
		{ID: "not-found2", Found: false},
	})
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	sort.Sort(&got)
	sort.Sort(&expected)
	c.Assert(got, check.DeepEquals, expected)
}

type updateList []updateUnitsResponse

func (list *updateList) Len() int {
	return len(*list)
}

func (list *updateList) Less(i, j int) bool {
	l := *list
	return l[i].ID < l[j].ID
}

func (list *updateList) Swap(i, j int) {
	(*list)[i], (*list)[j] = (*list)[j], (*list)[i]
}

func (s *S) TestSetUnitsStatusInvalidBody(c *check.C) {
	token, err := nativeScheme.AppLogin(app.InternalAppName)
	c.Assert(err, check.IsNil)
	body := bytes.NewBufferString("{{{-")
	request, err := http.NewRequest("POST", "/units/status", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestSetUnitsStatusNonInternalToken(c *check.C) {
	body := bytes.NewBufferString("{{{-")
	request, err := http.NewRequest("POST", "/units/status", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestAddTeamToTheApp(c *check.C) {
	t := auth.Team{Name: "itshardteam", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(t)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().RemoveAll(bson.M{"_id": t.Name})
	a := app.App{Name: "itshard", Platform: "zend", Teams: []string{t.Name}, TeamOwner: t.Name}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.Teams, check.HasLen, 2)
	c.Assert(app.Teams[1], check.Equals, s.team.Name)
	action := rectest.Action{
		Action: "grant-app-access",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + app.Name, "team=" + s.team.Name},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestGrantAccessToTeamReturn404IfTheAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("PUT", "/apps/a/teams/b", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App a not found.\n")
}

func (s *S) TestGrantAccessToTeamReturn403IfTheGivenUserDoesNotHasAccessToTheApp(c *check.C) {
	a := app.App{Name: "itshard", Platform: "zend"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "user does not have access to this app\n")
}

func (s *S) TestGrantAccessToTeamReturn404IfTheTeamDoesNotExist(c *check.C) {
	a := app.App{Name: "itshard", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/teams/a", a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Team not found\n")
}

func (s *S) TestGrantAccessToTeamReturn409IfTheTeamHasAlreadyAccessToTheApp(c *check.C) {
	a := app.App{Name: "itshard", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
}

func (s *S) TestGrantAccessToTeamCallsRepositoryManager(c *check.C) {
	t := &auth.Team{Name: "anything", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(t)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": t.Name})
	a := app.App{
		Name:      "tsuru",
		Platform:  "zend",
		Teams:     []string{t.Name},
		TeamOwner: t.Name,
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	grants, err := repositorytest.Granted(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(grants, check.DeepEquals, []string{s.user.Email})
}

func (s *S) TestRevokeAccessFromTeam(c *check.C) {
	t := auth.Team{Name: "abcd", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(t)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": t.Name})
	a := app.App{Name: "itshard", Platform: "zend", Teams: []string{"abcd", s.team.Name}}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app, err := app.GetByName(a.Name)
	c.Assert(app.Teams, check.HasLen, 1)
	c.Assert(app.Teams[0], check.Equals, "abcd")
	action := rectest.Action{
		Action: "revoke-app-access",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + app.Name, "team=" + s.team.Name},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestRevokeAccessFromTeamReturn404IfTheAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("DELETE", "/apps/a/teams/b", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App a not found.\n")
}

func (s *S) TestRevokeAccessFromTeamReturn401IfTheGivenUserDoesNotHavePermissionInTheApp(c *check.C) {
	a := app.App{Name: "itshard", Platform: "zend"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "user does not have access to this app\n")
}

func (s *S) TestRevokeAccessFromTeamReturn404IfTheTeamDoesNotExist(c *check.C) {
	a := app.App{Name: "itshard", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/teams/x", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Team not found\n")
}

func (s *S) TestRevokeAccessFromTeamReturn404IfTheTeamDoesNotHaveAccessToTheApp(c *check.C) {
	t := auth.Team{Name: "blaaa"}
	err := s.conn.Teams().Insert(t)
	c.Assert(err, check.IsNil)
	t2 := auth.Team{Name: "team2"}
	err = s.conn.Teams().Insert(t2)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": bson.M{"$in": []string{"blaaa", "team2"}}})
	a := app.App{Name: "itshard", Platform: "zend", Teams: []string{s.team.Name, t2.Name}}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, t.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestRevokeAccessFromTeamReturn403IfTheTeamIsTheLastWithAccessToTheApp(c *check.C) {
	a := app.App{Name: "itshard", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "You can not revoke the access from this team, because it is the unique team with access to the app, and an app can not be orphaned\n")
}

func (s *S) TestRevokeAccessFromTeamRemovesRepositoryFromRepository(c *check.C) {
	u := auth.User{Email: "again@live.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	t := auth.Team{Name: "anything", Users: []string{u.Email}}
	err = s.conn.Teams().Insert(t)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": t.Name})
	a := app.App{Name: "tsuru", Platform: "zend", Teams: []string{t.Name}}
	err = app.CreateApp(&a, &u)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	request, err = http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder = httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler = RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	grants, err := repositorytest.Granted(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(grants, check.DeepEquals, []string{u.Email})
}

func (s *S) TestRevokeAccessFromTeamDontRemoveTheUserIfItHasAccesToTheAppThroughAnotherTeam(c *check.C) {
	u := auth.User{Email: "burning@angel.com", Quota: quota.Unlimited}
	err := s.conn.Users().Insert(u)
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	repository.Manager().CreateUser(u.Email)
	t := auth.Team{Name: "anything", Users: []string{s.user.Email, u.Email}}
	err = s.conn.Teams().Insert(t)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": t.Name})
	a := app.App{Name: "tsuru", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, t.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	request, err = http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder = httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler = RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	grants, err := repositorytest.Granted(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(grants, check.DeepEquals, []string{s.user.Email})
}

func (s *S) TestRunOnceHandler(c *check.C) {
	s.provisioner.PrepareOutput([]byte("lots of files"))
	a := app.App{Name: "secrets", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	url := fmt.Sprintf("/apps/%s/run/?:app=%s&once=true", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("ls"))
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = runCommand(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Body.String(), check.Equals, `{"Message":"lots of files"}`+"\n")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "text")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls"
	cmds := s.provisioner.GetCmds(expected, &a)
	c.Assert(cmds, check.HasLen, 1)
	action := rectest.Action{
		Action: "run-command",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, "command=ls"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestRunHandler(c *check.C) {
	s.provisioner.PrepareOutput([]byte("lots of\nfiles"))
	a := app.App{Name: "secrets", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	url := fmt.Sprintf("/apps/%s/run/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("ls"))
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = runCommand(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Body.String(), check.Equals, `{"Message":"lots of\nfiles"}`+"\n")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "text")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls"
	cmds := s.provisioner.GetCmds(expected, &a)
	c.Assert(cmds, check.HasLen, 1)
	action := rectest.Action{
		Action: "run-command",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, "command=ls"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestRunHandlerReturnsTheOutputOfTheCommandEvenIfItFails(c *check.C) {
	s.provisioner.PrepareFailure("ExecuteCommand", &errors.HTTP{Code: 500, Message: "something went wrong"})
	s.provisioner.PrepareOutput([]byte("failure output"))
	a := app.App{Name: "secrets", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	url := fmt.Sprintf("/apps/%s/run/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("ls"))
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = runCommand(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "something went wrong")
	expected := `{"Message":"failure output"}` + "\n" +
		`{"Message":"","Error":"something went wrong"}` + "\n"
	c.Assert(recorder.Body.String(), check.Equals, expected)
}

func (s *S) TestRunHandlerReturnsBadRequestIfTheCommandIsMissing(c *check.C) {
	bodies := []io.Reader{nil, strings.NewReader("")}
	for _, body := range bodies {
		request, err := http.NewRequest("POST", "/apps/unknown/run/?:app=unknown", body)
		c.Assert(err, check.IsNil)
		recorder := httptest.NewRecorder()
		err = runCommand(recorder, request, s.token)
		c.Assert(err, check.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, check.Equals, true)
		c.Assert(e.Code, check.Equals, http.StatusBadRequest)
		c.Assert(e, check.ErrorMatches, "^You must provide the command to run$")
	}
}

func (s *S) TestRunHandlerReturnsInternalErrorIfReadAllFails(c *check.C) {
	b := s.getTestData("bodyToBeClosed.txt")
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, check.IsNil)
	request.Body.Close()
	recorder := httptest.NewRecorder()
	err = runCommand(recorder, request, s.token)
	c.Assert(err, check.NotNil)
}

func (s *S) TestRunHandlerReturnsNotFoundIfTheAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("POST", "/apps/unknown/run/?:app=unknown", strings.NewReader("ls"))
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = runCommand(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestRunHandlerReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := app.App{Name: "secrets", Platform: "zend"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/run/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("ls"))
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = runCommand(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestGetEnvHandlerGetsEnvironmentVariableFromApp(c *check.C) {
	a := app.App{
		Name:     "everything-i-want",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, strings.NewReader(`["DATABASE_HOST"]`))
	request.Header.Set("Content-Type", "application/json")
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = getEnv(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	expected := []map[string]interface{}{{
		"name":   "DATABASE_HOST",
		"value":  "localhost",
		"public": true,
	}}
	result := []map[string]interface{}{}
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, expected)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	action := rectest.Action{
		Action: "get-env",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, "envs=[DATABASE_HOST]"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestGetEnvHandlerShouldAcceptMultipleVariables(c *check.C) {
	a := app.App{
		Name:     "four-sticks",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, strings.NewReader(`["DATABASE_HOST", "DATABASE_USER"]`))
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = getEnv(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-type"), check.Equals, "application/json")
	expected := []map[string]interface{}{
		{"name": "DATABASE_HOST", "value": "localhost", "public": true},
		{"name": "DATABASE_USER", "value": "root", "public": true},
	}
	var got []map[string]interface{}
	err = json.Unmarshal(recorder.Body.Bytes(), &got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, expected)
	action := rectest.Action{
		Action: "get-env",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, "envs=[DATABASE_HOST DATABASE_USER]"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestGetEnvHandlerReturnsInternalErrorIfReadAllFails(c *check.C) {
	b := s.getTestData("bodyToBeClosed.txt")
	request, err := http.NewRequest("GET", "/apps/unknown/env/?:app=unknown", b)
	c.Assert(err, check.IsNil)
	request.Body.Close()
	recorder := httptest.NewRecorder()
	err = getEnv(recorder, request, s.token)
	c.Assert(err, check.NotNil)
}

func (s *S) TestGetEnvHandlerReturnsNotFoundIfTheAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("GET", "/apps/unknown/env/?:app=unknown", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = getEnv(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestGetEnvHandlerReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := app.App{Name: "lost", Platform: "zend"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.logConn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, strings.NewReader(`["DATABASE_HOST"]`))
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = getEnv(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestGetEnvHandlerGetsEnvironmentVariableFromAppWithAppToken(c *check.C) {
	a := app.App{
		Name:     "everything-i-want",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, strings.NewReader(`["DATABASE_HOST"]`))
	request.Header.Set("Content-Type", "application/json")
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	token, err := nativeScheme.AppLogin("appToken")
	c.Assert(err, check.IsNil)
	err = getEnv(recorder, request, auth.Token(token))
	c.Assert(err, check.IsNil)
	expected := []map[string]interface{}{{
		"name":   "DATABASE_HOST",
		"value":  "localhost",
		"public": true,
	}}
	result := []map[string]interface{}{}
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, expected)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
}

func (s *S) TestSetEnvHandlerShouldSetAPublicEnvironmentVariableInTheApp(c *check.C) {
	a := app.App{Name: "black-dog", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/env?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader(`{"DATABASE_HOST":"localhost"}`))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = setEnv(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	app, err := app.GetByName("black-dog")
	c.Assert(err, check.IsNil)
	expected := bind.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: true}
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, expected)
	envs := map[string]string{
		"DATABASE_HOST": "localhost",
	}
	action := rectest.Action{
		Action: "set-env",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, envs, "private=false"},
	}
	c.Assert(action, rectest.IsRecorded)
	c.Assert(recorder.Body.String(), check.Equals,
		`{"Message":"---- Setting 1 new environment variables ----\n"}
`)
}

func (s *S) TestSetEnvHandlerShouldSetAPrivateEnvironmentVariableInTheApp(c *check.C) {
	a := app.App{Name: "black-dog", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/env?:app=%s&private=1", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader(`{"DATABASE_HOST":"localhost"}`))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = setEnv(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	app, err := app.GetByName("black-dog")
	c.Assert(err, check.IsNil)
	expected := bind.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: false}
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, expected)
	envs := map[string]string{
		"DATABASE_HOST": "localhost",
	}
	action := rectest.Action{
		Action: "set-env",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, envs, "private=true"},
	}
	c.Assert(action, rectest.IsRecorded)
	c.Assert(recorder.Body.String(), check.Equals,
		`{"Message":"---- Setting 1 new environment variables ----\n"}
`)
}

func (s *S) TestSetEnvHandlerShouldSetADoublePrivateEnvironmentVariableInTheApp(c *check.C) {
	a := app.App{Name: "black-dog", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/env?:app=%s&private=1", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader(`{"DATABASE_HOST":"localhost"}`))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = setEnv(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	request, err = http.NewRequest("POST", url, strings.NewReader(`{"DATABASE_HOST":"127.0.0.1"}`))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	err = setEnv(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	app, err := app.GetByName("black-dog")
	c.Assert(err, check.IsNil)
	expected := bind.EnvVar{Name: "DATABASE_HOST", Value: "127.0.0.1", Public: false}
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, expected)
	envs := map[string]string{
		"DATABASE_HOST": "127.0.0.1",
	}
	action := rectest.Action{
		Action: "set-env",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, envs, "private=true"},
	}
	c.Assert(action, rectest.IsRecorded)
	c.Assert(recorder.Body.String(), check.Equals,
		`{"Message":"---- Setting 1 new environment variables ----\n"}
`)
}

func (s *S) TestSetEnvHandlerShouldSetMultipleEnvironmentVariablesInTheApp(c *check.C) {
	a := app.App{Name: "vigil", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/env?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"DATABASE_HOST": "localhost", "DATABASE_USER": "root"}`)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = setEnv(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	app, err := app.GetByName("vigil")
	c.Assert(err, check.IsNil)
	expectedHost := bind.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: true}
	expectedUser := bind.EnvVar{Name: "DATABASE_USER", Value: "root", Public: true}
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, expectedHost)
	c.Assert(app.Env["DATABASE_USER"], check.DeepEquals, expectedUser)
	envs := map[string]string{
		"DATABASE_HOST": "localhost",
		"DATABASE_USER": "root",
	}
	action := rectest.Action{
		Action: "set-env",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, envs, "private=false"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestSetEnvHandlerShouldNotChangeValueOfSerivceVariables(c *check.C) {
	original := map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:         "DATABASE_HOST",
			Value:        "privatehost.com",
			Public:       false,
			InstanceName: "some service",
		},
	}
	a := app.App{Name: "losers", Platform: "zend", Teams: []string{s.team.Name}, Env: original}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/env?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader(`{"DATABASE_HOST":"http://foo.com:8080"}`))
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = setEnv(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	app, err := app.GetByName("losers")
	c.Assert(err, check.IsNil)
	c.Assert(app.Env, check.DeepEquals, original)
}

func (s *S) TestSetEnvHandlerReturnsInternalErrorIfReadAllFails(c *check.C) {
	b := s.getTestData("bodyToBeClosed.txt")
	request, err := http.NewRequest("POST", "/apps/unknown/env/?:app=unknown", b)
	c.Assert(err, check.IsNil)
	request.Body.Close()
	recorder := httptest.NewRecorder()
	err = setEnv(recorder, request, s.token)
	c.Assert(err, check.NotNil)
}

func (s *S) TestSetEnvHandlerReturnsBadRequestIfVariablesAreMissing(c *check.C) {
	bodies := []io.Reader{nil, strings.NewReader("")}
	for _, body := range bodies {
		request, err := http.NewRequest("POST", "/apps/unknown/env/?:app=unknown", body)
		c.Assert(err, check.IsNil)
		recorder := httptest.NewRecorder()
		err = setEnv(recorder, request, s.token)
		c.Assert(err, check.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, check.Equals, true)
		c.Assert(e.Code, check.Equals, http.StatusBadRequest)
		c.Assert(e, check.ErrorMatches, "^You must provide the environment variables in a JSON object$")
	}
}

func (s *S) TestSetEnvHandlerReturnsNotFoundIfTheAppDoesNotExist(c *check.C) {
	b := strings.NewReader(`{"DATABASE_HOST":"localhost"}`)
	request, err := http.NewRequest("POST", "/apps/unknown/env/?:app=unknown", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = setEnv(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestSetEnvHandlerReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := app.App{Name: "rock-and-roll", Platform: "zend"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader(`{"DATABASE_HOST":"localhost"}`))
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = setEnv(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestUnsetEnvHandlerRemovesTheEnvironmentVariablesFromTheApp(c *check.C) {
	a := app.App{
		Name:     "swift",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	expected := a.Env
	delete(expected, "DATABASE_HOST")
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("DELETE", url, strings.NewReader(`["DATABASE_HOST"]`))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = unsetEnv(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	app, err := app.GetByName("swift")
	c.Assert(err, check.IsNil)
	c.Assert(app.Env, check.DeepEquals, expected)
	action := rectest.Action{
		Action: "unset-env",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, "envs=[DATABASE_HOST]"},
	}
	c.Assert(action, rectest.IsRecorded)
	c.Assert(recorder.Body.String(), check.Equals,
		`{"Message":"---- Unsetting 1 environment variables ----\n"}
`)
}

func (s *S) TestUnsetEnvHandlerRemovesAllGivenEnvironmentVariables(c *check.C) {
	a := app.App{
		Name:     "let-it-be",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("DELETE", url, strings.NewReader(`["DATABASE_HOST", "DATABASE_USER"]`))
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unsetEnv(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	app, err := app.GetByName("let-it-be")
	c.Assert(err, check.IsNil)
	expected := map[string]bind.EnvVar{
		"DATABASE_PASSWORD": {
			Name:   "DATABASE_PASSWORD",
			Value:  "secret",
			Public: false,
		},
	}
	c.Assert(app.Env, check.DeepEquals, expected)
	action := rectest.Action{
		Action: "unset-env",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, "envs=[DATABASE_HOST DATABASE_USER]"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestUnsetHandlerDoesNotRemovePrivateVariables(c *check.C) {
	a := app.App{
		Name:     "letitbe",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`["DATABASE_HOST", "DATABASE_USER", "DATABASE_PASSWORD"]`)
	request, err := http.NewRequest("DELETE", url, b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unsetEnv(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	app, err := app.GetByName("letitbe")
	c.Assert(err, check.IsNil)
	expected := map[string]bind.EnvVar{
		"DATABASE_PASSWORD": {
			Name:   "DATABASE_PASSWORD",
			Value:  "secret",
			Public: false,
		},
	}
	c.Assert(app.Env, check.DeepEquals, expected)
}

func (s *S) TestUnsetEnvHandlerReturnsInternalErrorIfReadAllFails(c *check.C) {
	b := s.getTestData("bodyToBeClosed.txt")
	request, err := http.NewRequest("POST", "/apps/unknown/env/?:app=unknown", b)
	c.Assert(err, check.IsNil)
	request.Body.Close()
	recorder := httptest.NewRecorder()
	err = unsetEnv(recorder, request, s.token)
	c.Assert(err, check.NotNil)
}

func (s *S) TestUnsetEnvHandlerReturnsBadRequestIfVariablesAreMissing(c *check.C) {
	bodies := []io.Reader{nil, strings.NewReader(""), strings.NewReader("[]")}
	for _, body := range bodies {
		request, err := http.NewRequest("POST", "/apps/unknown/env/?:app=unknown", body)
		c.Assert(err, check.IsNil)
		recorder := httptest.NewRecorder()
		err = unsetEnv(recorder, request, s.token)
		c.Assert(err, check.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, check.Equals, true)
		c.Assert(e.Code, check.Equals, http.StatusBadRequest)
		c.Assert(e, check.ErrorMatches, "^You must provide the list of environment variables, in JSON format$")
	}
}

func (s *S) TestUnsetEnvHandlerReturnsNotFoundIfTheAppDoesNotExist(c *check.C) {
	b := strings.NewReader(`["DATABASE_HOST"]`)
	request, err := http.NewRequest("POST", "/apps/unknown/env/?:app=unknown", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unsetEnv(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestUnsetEnvHandlerReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := app.App{Name: "mountain-mama"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.logConn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader(`["DATABASE_HOST"]`))
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = s.provisioner.Provision(&a)
	c.Assert(err, check.IsNil)
	err = unsetEnv(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestAddCNameHandler(c *check.C) {
	a := app.App{Name: "leper", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"cname":["leper.secretcompany.com"]}`)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = setCName(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.CName, check.DeepEquals, []string{"leper.secretcompany.com"})
	action := rectest.Action{
		Action: "add-cname",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + app.Name, "cname=leper.secretcompany.com"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestAddCNameHandlerAcceptsWildCard(c *check.C) {
	a := app.App{Name: "leper", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"cname":["*.leper.secretcompany.com"]}`)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = setCName(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.CName, check.DeepEquals, []string{"*.leper.secretcompany.com"})
	action := rectest.Action{
		Action: "add-cname",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + app.Name, "cname=*.leper.secretcompany.com"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestAddCNameHandlerErrsOnInvalidCName(c *check.C) {
	a := app.App{Name: "leper", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"cname":["_leper.secretcompany.com"]}`)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = setCName(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, "Invalid cname")
}

func (s *S) TestAddCNameHandlerReturnsInternalErrorIfItFailsToReadTheBody(c *check.C) {
	b := s.getTestData("bodyToBeClosed.txt")
	request, err := http.NewRequest("POST", "/apps/unknown/cname?:app=unknown", b)
	c.Assert(err, check.IsNil)
	request.Body.Close()
	recorder := httptest.NewRecorder()
	err = setCName(recorder, request, s.token)
	c.Assert(err, check.NotNil)
}

func (s *S) TestAddCNameHandlerReturnsBadRequestWhenCNameIsMissingFromTheBody(c *check.C) {
	bodies := []io.Reader{nil, strings.NewReader(`{}`)}
	for _, b := range bodies {
		request, err := http.NewRequest("POST", "/apps/unknown/cname?:app=unknown", b)
		c.Assert(err, check.IsNil)
		recorder := httptest.NewRecorder()
		err = setCName(recorder, request, s.token)
		c.Check(err, check.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Check(ok, check.Equals, true)
		c.Check(e.Code, check.Equals, http.StatusBadRequest)
		c.Check(e.Message, check.Equals, "You must provide the cname.")
	}
}

func (s *S) TestAddCNameHandlerInvalidJSON(c *check.C) {
	b := strings.NewReader(`}"I'm invalid json"`)
	request, err := http.NewRequest("POST", "/apps/unknown/cname?:app=unknown", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = setCName(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, "Invalid JSON in request body.")
}

func (s *S) TestAddCNameHandlerUnknownApp(c *check.C) {
	b := strings.NewReader(`{"cname": ["leper.secretcompany.com"]}`)
	request, err := http.NewRequest("POST", "/apps/unknown/cname?:app=unknown", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = setCName(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestAddCNameHandlerUserWithoutAccessToTheApp(c *check.C) {
	a := app.App{Name: "lost", Platform: "vougan"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.logConn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"cname": ["lost.secretcompany.com"]}`)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = setCName(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestRemoveCNameHandler(c *check.C) {
	a := app.App{
		Name:     "leper",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		CName:    []string{"foo.bar.com"},
	}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"cname": ["foo.bar.com"]}`)
	request, err := http.NewRequest("DELETE", url, b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unsetCName(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.CName, check.DeepEquals, []string{})
	action := rectest.Action{
		Action: "remove-cname",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + app.Name, "cnames=foo.bar.com"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestUnsetTwoCnames(c *check.C) {
	a := app.App{
		Name:     "leper",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		CName:    []string{"foo.bar.com", "bar.com"},
	}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"cname": ["foo.bar.com", "bar.com"]}`)
	request, err := http.NewRequest("DELETE", url, b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unsetCName(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.CName, check.DeepEquals, []string{})
	action := rectest.Action{
		Action: "remove-cname",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + app.Name, "cnames=foo.bar.com, bar.com"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestRemoveCNameHandlerUnknownApp(c *check.C) {
	b := strings.NewReader(`{"cname": ["foo.bar.com"]}`)
	request, err := http.NewRequest("DELETE", "/apps/unknown/cname?:app=unknown", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unsetCName(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestRemoveCNameHandlerUserWithoutAccessToTheApp(c *check.C) {
	a := app.App{
		Name:     "lost",
		Platform: "vougan",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.logConn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"cname": ["foo.bar.com"]}`)
	request, err := http.NewRequest("DELETE", url, b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unsetCName(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
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
	a := app.App{Name: "lost", Platform: "vougan"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.logConn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&lines=10", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = appLog(recorder, request, s.token)
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

func (s *S) TestAppLogFollowWithPubSub(c *check.C) {
	a := app.App{Name: "lost1", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := "/apps/something/log/?:app=" + a.Name + "&lines=10&follow=1"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		recorder := httptest.NewRecorder()
		err := appLog(recorder, request, s.token)
		c.Assert(err, check.IsNil)
		body, err := ioutil.ReadAll(recorder.Body)
		c.Assert(err, check.IsNil)
		splitted := strings.Split(strings.TrimSpace(string(body)), "\n")
		c.Assert(splitted, check.HasLen, 2)
		c.Assert(splitted[0], check.Equals, "[]")
		logs := []app.Applog{}
		err = json.Unmarshal([]byte(splitted[1]), &logs)
		c.Assert(err, check.IsNil)
		c.Assert(logs, check.HasLen, 1)
		c.Assert(logs[0].Message, check.Equals, "x")
	}()
	var listener *app.LogListener
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
	factory, err := queue.Factory()
	c.Assert(err, check.IsNil)
	q, err := factory.PubSub("pubsub:" + a.Name)
	c.Assert(err, check.IsNil)
	err = q.Pub([]byte(`{"message": "x"}`))
	c.Assert(err, check.IsNil)
	time.Sleep(500 * time.Millisecond)
	listener.Close()
	wg.Wait()
}

func (s *S) TestAppLogFollowWithFilter(c *check.C) {
	a := app.App{Name: "lost2", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := "/apps/something/log/?:app=" + a.Name + "&lines=10&follow=1&source=web"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		recorder := httptest.NewRecorder()
		err := appLog(recorder, request, s.token)
		c.Assert(err, check.IsNil)
		body, err := ioutil.ReadAll(recorder.Body)
		c.Assert(err, check.IsNil)
		splitted := strings.Split(strings.TrimSpace(string(body)), "\n")
		c.Assert(splitted, check.HasLen, 2)
		c.Assert(splitted[0], check.Equals, "[]")
		logs := []app.Applog{}
		err = json.Unmarshal([]byte(splitted[1]), &logs)
		c.Assert(err, check.IsNil)
		c.Assert(logs, check.HasLen, 1)
		c.Assert(logs[0].Message, check.Equals, "y")
	}()
	var listener *app.LogListener
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
	factory, err := queue.Factory()
	c.Assert(err, check.IsNil)
	q, err := factory.PubSub("pubsub:" + a.Name)
	c.Assert(err, check.IsNil)
	err = q.Pub([]byte(`{"message": "x", "source": "app"}`))
	c.Assert(err, check.IsNil)
	err = q.Pub([]byte(`{"message": "y", "source": "web"}`))
	c.Assert(err, check.IsNil)
	time.Sleep(500 * time.Millisecond)
	listener.Close()
	wg.Wait()
}

func (s *S) TestAppLogShouldHaveContentType(c *check.C) {
	a := app.App{Name: "lost", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&lines=10", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = appLog(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
}

func (s *S) TestAppLogSelectByLines(c *check.C) {
	a := app.App{Name: "lost", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	for i := 0; i < 15; i++ {
		a.Log(strconv.Itoa(i), "source", "")
	}
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&lines=10", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = appLog(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	logs := []app.Applog{}
	err = json.Unmarshal(body, &logs)
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 10)
	action := rectest.Action{
		Action: "app-log",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, "lines=10"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestAppLogSelectBySource(c *check.C) {
	a := app.App{Name: "lost", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	a.Log("mars log", "mars", "")
	a.Log("earth log", "earth", "")
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&source=mars&lines=10", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = appLog(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	logs := []app.Applog{}
	err = json.Unmarshal(body, &logs)
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 1)
	c.Assert(logs[0].Message, check.Equals, "mars log")
	c.Assert(logs[0].Source, check.Equals, "mars")
	action := rectest.Action{
		Action: "app-log",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, "lines=10", "source=mars"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestAppLogSelectByUnit(c *check.C) {
	a := app.App{Name: "lost", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	a.Log("mars log", "mars", "prospero")
	a.Log("earth log", "earth", "caliban")
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&unit=caliban&lines=10", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = appLog(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	logs := []app.Applog{}
	err = json.Unmarshal(body, &logs)
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 1)
	c.Assert(logs[0].Message, check.Equals, "earth log")
	c.Assert(logs[0].Source, check.Equals, "earth")
	c.Assert(logs[0].Unit, check.Equals, "caliban")
	action := rectest.Action{
		Action: "app-log",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, "lines=10", "unit=caliban"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestAppLogSelectByLinesShouldReturnTheLastestEntries(c *check.C) {
	a := app.App{Name: "lost", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	now := time.Now()
	coll := s.logConn.Logs(a.Name)
	defer coll.DropCollection()
	for i := 0; i < 15; i++ {
		l := app.Applog{
			Date:    now.Add(time.Duration(i) * time.Hour),
			Message: strconv.Itoa(i),
			Source:  "source",
			AppName: a.Name,
		}
		coll.Insert(l)
	}
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&lines=3", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = appLog(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	var logs []app.Applog
	err = json.Unmarshal(body, &logs)
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 3)
	c.Assert(logs[0].Message, check.Equals, "12")
	c.Assert(logs[1].Message, check.Equals, "13")
	c.Assert(logs[2].Message, check.Equals, "14")
}

func (s *S) TestAppLogShouldReturnLogByApp(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&app1)
	app1.Log("app1 log", "source", "")
	app2 := app.App{Name: "app2", Platform: "zend", Teams: []string{s.team.Name}}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&app2)
	app2.Log("app2 log", "sourc ", "")
	app3 := app.App{Name: "app3", Platform: "zend", Teams: []string{s.team.Name}}
	err = app.CreateApp(&app3, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&app3)
	app3.Log("app3 log", "tsuru", "")
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&lines=10", app3.Name, app3.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = appLog(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	logs := []app.Applog{}
	err = json.Unmarshal(body, &logs)
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
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": "http://localhost:1234"}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a := app.App{
		Name:     "painkiller",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env:      map[string]bind.EnvVar{},
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, a.Name, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = bindServiceInstance(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	c.Assert(recorder.Body.String(), check.Equals, `{"Message":"","Error":"my-mysql api is down."}`+"\n")
}

func (s *S) TestBindHandler(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a := app.App{
		Name:     "painkiller",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env:      map[string]bind.EnvVar{},
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, a.Name, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	s.provisioner.PrepareOutput([]byte("exported"))
	err = bindServiceInstance(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	err = s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Apps, check.DeepEquals, []string{a.Name})
	err = s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(err, check.IsNil)
	expectedUser := bind.EnvVar{Name: "DATABASE_USER", Value: "root", Public: false, InstanceName: instance.Name}
	expectedPassword := bind.EnvVar{Name: "DATABASE_PASSWORD", Value: "s3cr3t", Public: false, InstanceName: instance.Name}
	c.Assert(a.Env["DATABASE_USER"], check.DeepEquals, expectedUser)
	c.Assert(a.Env["DATABASE_PASSWORD"], check.DeepEquals, expectedPassword)
	parts := strings.Split(recorder.Body.String(), "\n")
	c.Assert(parts, check.HasLen, 8)
	c.Assert(parts[0], check.Equals, `{"Message":"---- Setting 3 new environment variables ----\n"}`)
	c.Assert(parts[1], check.Equals, `{"Message":"restarting app"}`)
	c.Assert(parts[2], check.Equals, `{"Message":"\nInstance \"my-mysql\" is now bound to the app \"painkiller\".\n"}`)
	c.Assert(parts[3], check.Equals, `{"Message":"The following environment variables are available for use in your app:\n\n"}`)
	c.Assert(parts[4], check.Matches, `{"Message":"- DATABASE_(USER|PASSWORD)\\n"}`)
	c.Assert(parts[5], check.Matches, `{"Message":"- DATABASE_(USER|PASSWORD)\\n"}`)
	c.Assert(parts[6], check.Matches, `{"Message":"- TSURU_SERVICES\\n"}`)
	c.Assert(parts[7], check.Equals, "")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	action := rectest.Action{
		Action: "bind-app",
		User:   s.user.Email,
		Extra:  []interface{}{"instance=" + instance.Name, "app=" + a.Name},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestBindHandlerReturns404IfTheInstanceDoesNotExist(c *check.C) {
	a := app.App{Name: "serviceapp", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/services/instances/unknown/%s?:instance=unknown&:app=%s", a.Name, a.Name)
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

func (s *S) TestBindHandlerReturns403IfTheUserDoesNotHaveAccessToTheInstance(c *check.C) {
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql"}
	err := instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a := app.App{Name: "serviceapp", Platform: "zend", Teams: []string{s.team.Name}}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, a.Name, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = bindServiceInstance(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e.Message, check.Equals, service.ErrAccessNotAllowed.Error())
}

func (s *S) TestBindHandlerReturns404IfTheAppDoesNotExist(c *check.C) {
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err := instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	url := fmt.Sprintf("/services/instances/%s/unknown?:instance=%s&:app=unknown", instance.Name, instance.Name)
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
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err := instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a := app.App{Name: "serviceapp", Platform: "zend"}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.logConn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, a.Name, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = bindServiceInstance(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e, check.ErrorMatches, "^This user does not have access to this app$")
}

func (s *S) TestUnbindHandler(c *check.C) {
	s.provisioner.PrepareOutput([]byte("exported"))
	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/bind" {
			atomic.StoreInt32(&called, 1)
		}
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	a := app.App{
		Name:     "painkiller",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	units, _ := s.provisioner.AddUnits(&a, 1, "web", nil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
		Units:       []string{units[0].Name},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	otherApp, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	otherApp.Env["DATABASE_HOST"] = bind.EnvVar{
		Name:         "DATABASE_HOST",
		Value:        "arrea",
		Public:       false,
		InstanceName: instance.Name,
	}
	otherApp.Env["MY_VAR"] = bind.EnvVar{Name: "MY_VAR", Value: "123"}
	err = s.conn.Apps().Update(bson.M{"name": otherApp.Name}, otherApp)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, a.Name,
		instance.Name, a.Name)
	req, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, req, s.token)
	c.Assert(err, check.IsNil)
	err = s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Apps, check.DeepEquals, []string{})
	otherApp, err = app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	expected := bind.EnvVar{
		Name:  "MY_VAR",
		Value: "123",
	}
	c.Assert(otherApp.Env["MY_VAR"], check.DeepEquals, expected)
	_, ok := otherApp.Env["DATABASE_HOST"]
	c.Assert(ok, check.Equals, false)
	ch := make(chan bool)
	go func() {
		t := time.Tick(1)
		for _ = <-t; atomic.LoadInt32(&called) == 0; _ = <-t {
		}
		ch <- true
	}()
	select {
	case <-ch:
		c.Succeed()
	case <-time.After(1e9):
		c.Errorf("Failed to call API after 1 second.")
	}
	parts := strings.Split(recorder.Body.String(), "\n")
	c.Assert(parts, check.HasLen, 4)
	c.Assert(parts[0], check.Equals, `{"Message":"---- Unsetting 1 environment variables ----\n"}`)
	c.Assert(parts[1], check.Equals, `{"Message":"restarting app"}`)
	c.Assert(parts[2], check.Equals, `{"Message":"\nInstance \"my-mysql\" is not bound to the app \"painkiller\" anymore.\n"}`)
	c.Assert(parts[3], check.Equals, "")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	action := rectest.Action{
		Action: "unbind-app",
		User:   s.user.Email,
		Extra:  []interface{}{"instance=" + instance.Name, "app=" + a.Name},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestUnbindHandlerReturns404IfTheInstanceDoesNotExist(c *check.C) {
	a := app.App{Name: "serviceapp", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/services/instances/unknown/%s?:instance=unknown&:app=%s", a.Name, a.Name)
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
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql"}
	err := instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a := app.App{Name: "serviceapp", Platform: "zend", Teams: []string{s.team.Name}}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, a.Name, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e.Message, check.Equals, service.ErrAccessNotAllowed.Error())
}

func (s *S) TestUnbindHandlerReturns404IfTheAppDoesNotExist(c *check.C) {
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err := instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	url := fmt.Sprintf("/services/instances/%s/unknown?:instance=%s&:app=unknown", instance.Name, instance.Name)
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
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err := instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a := app.App{Name: "serviceapp", Platform: "zend"}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.logConn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, a.Name, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e, check.ErrorMatches, "^This user does not have access to this app$")
}

func (s *S) TestRestartHandler(c *check.C) {
	a := app.App{Name: "stress", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/restart?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = restart(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "text")
	action := rectest.Action{
		Action: "restart",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name},
	}
	c.Assert(action, rectest.IsRecorded)
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
	a := app.App{Name: "nightmist"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.logConn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/restart?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = restart(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

type LogList []app.Applog

func (l LogList) Len() int           { return len(l) }
func (l LogList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l LogList) Less(i, j int) bool { return l[i].Message < l[j].Message }

func (s *S) TestAddLogHandler(c *check.C) {
	a := app.App{Name: "myapp", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	body := strings.NewReader(`["message 1", "message 2", "message 3"]`)
	body2 := strings.NewReader(`["message 4", "message 5"]`)
	request, err := http.NewRequest("POST", "/apps/myapp/log/?:app=myapp", body)
	c.Assert(err, check.IsNil)
	withSourceRequest, err := http.NewRequest("POST", "/apps/myapp/log/?:app=myapp&source=mysource", body2)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = addLog(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	recorder = httptest.NewRecorder()
	err = addLog(recorder, withSourceRequest, s.token)
	c.Assert(err, check.IsNil)
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
	logs, err := a.LastLogs(5, app.Applog{})
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

func (s *S) TestPlatformList(c *check.C) {
	platforms := []app.Platform{
		{Name: "python"},
		{Name: "java"},
		{Name: "ruby20"},
		{Name: "static"},
	}
	for _, p := range platforms {
		s.conn.Platforms().Insert(p)
		defer s.conn.Platforms().Remove(p)
	}
	want := make([]app.Platform, 1, len(platforms)+1)
	want[0] = app.Platform{Name: "zend"}
	want = append(want, platforms...)
	request, _ := http.NewRequest("GET", "/platforms", nil)
	recorder := httptest.NewRecorder()
	err := platformList(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var got []app.Platform
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, want)
	action := rectest.Action{Action: "platform-list", User: s.user.Email}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestGetApp(c *check.C) {
	a := app.App{Name: "testapp", Platform: "zend", Teams: []string{"notAdmin", "noSuperUser"}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	expected, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	app, err := getApp(a.Name, s.adminuser, nil)
	c.Assert(err, check.IsNil)
	c.Assert(app, check.DeepEquals, *expected)
}

func (s *S) TestSwap(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&app1)
	app2 := app.App{Name: "app2", Platform: "zend", Teams: []string{s.team.Name}}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&app2)
	request, _ := http.NewRequest("PUT", "/swap?app1=app1&app2=app2", nil)
	recorder := httptest.NewRecorder()
	err = swap(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	action := rectest.Action{Action: "swap", User: s.user.Email, Extra: []interface{}{"app1=app1", "app2=app2"}}
	c.Assert(action, rectest.IsRecorded)
	var dbApp app.App
	err = s.conn.Apps().Find(bson.M{"name": app1.Name}).One(&dbApp)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Lock, check.Equals, app.AppLock{})
	err = s.conn.Apps().Find(bson.M{"name": app2.Name}).One(&dbApp)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Lock, check.Equals, app.AppLock{})
}

func (s *S) TestSwapApp1Locked(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "zend", Teams: []string{s.team.Name}, Lock: app.AppLock{
		Locked: true, Reason: "/test", Owner: "x",
	}}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&app1)
	app2 := app.App{Name: "app2", Platform: "zend", Teams: []string{s.team.Name}}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&app2)
	request, _ := http.NewRequest("PUT", "/swap?app1=app1&app2=app2", nil)
	recorder := httptest.NewRecorder()
	err = swap(recorder, request, s.token)
	c.Assert(err, check.ErrorMatches, "app1: App locked by x, running /test. Acquired in .*")
}

func (s *S) TestSwapApp2Locked(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&app1)
	app2 := app.App{Name: "app2", Platform: "zend", Teams: []string{s.team.Name}, Lock: app.AppLock{
		Locked: true, Reason: "/test", Owner: "x",
	}}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&app2)
	request, _ := http.NewRequest("PUT", "/swap?app1=app1&app2=app2", nil)
	recorder := httptest.NewRecorder()
	err = swap(recorder, request, s.token)
	c.Assert(err, check.ErrorMatches, "app2: App locked by x, running /test. Acquired in .*")
}

func (s *S) TestSwapIncompatiblePlatforms(c *check.C) {
	app1 := app.App{Name: "app1", Teams: []string{s.team.Name}, Platform: "x"}
	err := s.conn.Apps().Insert(&app1)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app1.Name})
	err = s.provisioner.Provision(&app1)
	c.Assert(err, check.IsNil)
	app2 := app.App{Name: "app2", Teams: []string{s.team.Name}, Platform: "y"}
	err = s.conn.Apps().Insert(&app2)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app2.Name})
	err = s.provisioner.Provision(&app2)
	c.Assert(err, check.IsNil)
	request, _ := http.NewRequest("PUT", "/swap?app1=app1&app2=app2", nil)
	recorder := httptest.NewRecorder()
	err = swap(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusPreconditionFailed)
	c.Assert(e.Message, check.Equals, "platforms don't match")
}

func (s *S) TestSwapIncompatibleUnits(c *check.C) {
	app1 := app.App{Name: "app1", Teams: []string{s.team.Name}, Platform: "x"}
	err := s.conn.Apps().Insert(&app1)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app1.Name})
	err = s.provisioner.Provision(&app1)
	c.Assert(err, check.IsNil)
	app2 := app.App{Name: "app2", Teams: []string{s.team.Name}, Platform: "x"}
	err = s.conn.Apps().Insert(&app2)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app2.Name})
	err = s.provisioner.Provision(&app2)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnit(&app2, provision.Unit{})
	request, _ := http.NewRequest("PUT", "/swap?app1=app1&app2=app2", nil)
	recorder := httptest.NewRecorder()
	err = swap(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusPreconditionFailed)
	c.Assert(e.Message, check.Equals, "number of units doesn't match")
}

func (s *S) TestSwapIncompatibleAppsForceSwap(c *check.C) {
	app1 := app.App{Name: "app1", Teams: []string{s.team.Name}, Platform: "x"}
	err := s.conn.Apps().Insert(&app1)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app1.Name})
	err = s.provisioner.Provision(&app1)
	c.Assert(err, check.IsNil)
	app2 := app.App{Name: "app2", Teams: []string{s.team.Name}, Platform: "y"}
	err = s.conn.Apps().Insert(&app2)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app2.Name})
	err = s.provisioner.Provision(&app2)
	c.Assert(err, check.IsNil)
	request, _ := http.NewRequest("PUT", "/swap?app1=app1&app2=app2&force=true", nil)
	recorder := httptest.NewRecorder()
	err = swap(recorder, request, s.token)
	c.Assert(err, check.IsNil)
}

func (s *S) TestStartHandler(c *check.C) {
	a := app.App{Name: "stress", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/start?:app=%s&process=web", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = start(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	starts := s.provisioner.Starts(&a, "web")
	c.Assert(starts, check.Equals, 1)
	starts = s.provisioner.Starts(&a, "worker")
	c.Assert(starts, check.Equals, 0)
	action := rectest.Action{
		Action: "start",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestStopHandler(c *check.C) {
	a := app.App{Name: "stress", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	url := fmt.Sprintf("/apps/%s/stop?:app=%s&process=web", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = stop(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	stops := s.provisioner.Stops(&a, "web")
	c.Assert(stops, check.Equals, 1)
	stops = s.provisioner.Stops(&a, "worker")
	c.Assert(stops, check.Equals, 0)
	action := rectest.Action{
		Action: "stop",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *S) TestForceDeleteLock(c *check.C) {
	a := app.App{Name: "locked", Lock: app.AppLock{Locked: true}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/apps/locked/lock", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
	c.Assert(recorder.Body.String(), check.Equals, "")
	var dbApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "locked"}).One(&dbApp)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Lock.Locked, check.Equals, false)
}

func (s *S) TestForceDeleteLockOnlyAdmins(c *check.C) {
	a := app.App{Name: "locked", Lock: app.AppLock{Locked: true}, Teams: []string{s.team.Name}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/apps/locked/lock", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "You must be an admin\n")
	var dbApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "locked"}).One(&dbApp)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Lock.Locked, check.Equals, true)
}

func (s *S) TestRegisterUnit(c *check.C) {
	a := app.App{
		Name:     "myappx",
		Platform: "python",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"MY_VAR_1": {Name: "MY_VAR_1", Value: "value1", Public: true},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.logConn.Logs(a.Name).DropCollection()
	err = s.provisioner.Provision(&a)
	c.Assert(err, check.IsNil)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	units := a.Units()
	oldIp := units[0].Ip
	body := strings.NewReader("hostname=" + units[0].Name)
	request, err := http.NewRequest("POST", "/apps/myappx/units/register", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	expected := []map[string]interface{}{{
		"name":   "MY_VAR_1",
		"value":  "value1",
		"public": true,
	}}
	result := []map[string]interface{}{}
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, expected)
	units = a.Units()
	c.Assert(units[0].Ip, check.Equals, oldIp+"-updated")
}

func (s *S) TestRegisterUnitInvalidUnit(c *check.C) {
	a := app.App{
		Name:     "myappx",
		Platform: "python",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"MY_VAR_1": {Name: "MY_VAR_1", Value: "value1", Public: true},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.logConn.Logs(a.Name).DropCollection()
	err = s.provisioner.Provision(&a)
	c.Assert(err, check.IsNil)
	defer s.provisioner.Destroy(&a)
	body := strings.NewReader("hostname=invalid-unit-host")
	request, err := http.NewRequest("POST", "/apps/myappx/units/register", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "unit not found\n")
}

func (s *S) TestRegisterUnitWithCustomData(c *check.C) {
	a := app.App{
		Name:     "myappx",
		Platform: "python",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"MY_VAR_1": {Name: "MY_VAR_1", Value: "value1", Public: true},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.logConn.Logs(a.Name).DropCollection()
	err = s.provisioner.Provision(&a)
	c.Assert(err, check.IsNil)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	units := a.Units()
	oldIp := units[0].Ip
	v := url.Values{}
	v.Set("hostname", units[0].Name)
	v.Set("customdata", `{"mydata": "something"}`)
	body := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", "/apps/myappx/units/register", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	expected := []map[string]interface{}{{
		"name":   "MY_VAR_1",
		"value":  "value1",
		"public": true,
	}}
	result := []map[string]interface{}{}
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, expected)
	units = a.Units()
	c.Assert(units[0].Ip, check.Equals, oldIp+"-updated")
	c.Assert(s.provisioner.CustomData(&a), check.DeepEquals, map[string]interface{}{
		"mydata": "something",
	})
}

func (s *S) TestSetTeamOwnerWithoutTeam(c *check.C) {
	a := app.App{Name: "myappx", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	request, err := http.NewRequest("POST", "/apps/myappx/team-owner", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "You must provide a team name.\n")
}

func (s *S) TestSetTeamOwner(c *check.C) {
	a := app.App{Name: "myappx", Platform: "zend", Teams: []string{s.team.Name}, TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	team := &auth.Team{Name: "newowner", Users: []string{s.user.Email}}
	err = s.conn.Teams().Insert(team)
	defer s.conn.Teams().Remove(bson.M{"_id": team.Name})
	c.Assert(err, check.IsNil)
	body := strings.NewReader(team.Name)
	req, err := http.NewRequest("POST", "/apps/myappx/team-owner", body)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	s.conn.Apps().Find(bson.M{"name": "myappx"}).One(&a)
	c.Assert(a.TeamOwner, check.Equals, team.Name)
}

func (s *S) TestSetTeamOwnerToUserWhoCantBeOwner(c *check.C) {
	a := app.App{Name: "myappx", Platform: "zend", Teams: []string{s.team.Name}, TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	user := &auth.User{Email: "teste@thewho.com", Password: "123456", Quota: quota.Unlimited}
	_, err = nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	team := &auth.Team{Name: "newowner", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	defer s.conn.Teams().Remove(bson.M{"_id": team.Name})
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	body := strings.NewReader(team.Name)
	req, err := http.NewRequest("POST", "/apps/myappx/team-owner", body)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	rec := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusForbidden)
	s.conn.Apps().Find(bson.M{"name": "myappx"}).One(&a)
	c.Assert(a.TeamOwner, check.Equals, s.team.Name)
}

func (s *S) TestSetTeamOwnerAsAdmin(c *check.C) {
	a := app.App{Name: "myappx", Platform: "zend", Teams: []string{s.team.Name}, TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	user := &auth.User{Email: "teste@thewho2.com", Password: "123456", Quota: quota.Unlimited}
	_, err = nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	team := &auth.Team{Name: "newowner", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	defer s.conn.Teams().Remove(bson.M{"_id": team.Name})
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	body := strings.NewReader(team.Name)
	req, err := http.NewRequest("POST", "/apps/myappx/team-owner", body)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	rec := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusForbidden)
	s.conn.Apps().Find(bson.M{"name": "myappx"}).One(&a)
	c.Assert(a.TeamOwner, check.Equals, s.team.Name)
	req, err = http.NewRequest("POST", "/apps/myappx/team-owner", body)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	rec = httptest.NewRecorder()
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	s.conn.Apps().Find(bson.M{"name": "myappx"}).One(&a)
	c.Assert(a.TeamOwner, check.Equals, team.Name)
}

func (s *S) TestSetTeamOwnerSetNewTeamToAppAddThatTeamToAppTeamList(c *check.C) {
	a := app.App{Name: "myappx", Platform: "zend", Teams: []string{s.team.Name}, TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	user := &auth.User{Email: "teste@thewho3.com", Password: "123456", Quota: quota.Unlimited}
	_, err = nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	team := &auth.Team{Name: "newowner", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	defer s.conn.Teams().Remove(bson.M{"_id": team.Name})
	c.Assert(err, check.IsNil)
	body := strings.NewReader(team.Name)
	req, err := http.NewRequest("POST", "/apps/myappx/team-owner", body)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	rec := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	s.conn.Apps().Find(bson.M{"name": "myappx"}).One(&a)
	c.Assert([]string{team.Name, s.team.Name}, check.DeepEquals, a.Teams)
}

func (s *S) deleteApp(a *app.App) {
	app.Delete(a)
	count, err := s.conn.Apps().Find(bson.M{"name": a.Name}).Count()
	for count > 0 && err == nil {
		count, err = s.conn.Apps().Find(bson.M{"name": a.Name}).Count()
		time.Sleep(1e6)
	}
}

func (s *S) TestAppChangePool(c *check.C) {
	a := app.App{Name: "myappx", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a)
	err = provision.AddPool("test", false)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool("test", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	defer provision.RemovePool("test")
	body := strings.NewReader("test")
	request, err := http.NewRequest("POST", "/apps/myappx/pool", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}
