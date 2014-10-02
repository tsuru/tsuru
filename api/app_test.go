// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/testing"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

func (s *S) TestAppIsAvailableHandlerShouldReturnErrorWhenAppStatusIsnotStarted(c *gocheck.C) {
	a := app.App{
		Name:     "someapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/available?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = appIsAvailable(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestAppIsAvailableHandlerShouldReturn200WhenAppUnitStatusIsStarted(c *gocheck.C) {
	a := app.App{
		Name:     "someapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 1, nil)
	url := fmt.Sprintf("/apps/%s/repository/clone?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = appIsAvailable(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
}

func (s *S) TestAppList(c *gocheck.C) {
	app1 := app.App{
		Name:  "app1",
		Teams: []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(app1)
	c.Assert(err, gocheck.IsNil)
	s.provisioner.Provision(&app1)
	defer s.provisioner.Destroy(&app1)
	defer s.conn.Apps().Remove(bson.M{"name": app1.Name})
	defer s.conn.Logs(app1.Name).DropCollection()
	app2 := app.App{
		Name:  "app2",
		Teams: []string{s.team.Name},
	}
	err = s.conn.Apps().Insert(app2)
	c.Assert(err, gocheck.IsNil)
	s.provisioner.Provision(&app2)
	defer s.provisioner.Destroy(&app2)
	defer s.conn.Apps().Remove(bson.M{"name": app2.Name})
	defer s.conn.Logs(app2.Name).DropCollection()
	request, err := http.NewRequest("GET", "/apps/", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = appList(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	apps := []app.App{}
	err = json.Unmarshal(body, &apps)
	c.Assert(err, gocheck.IsNil)
	expected := []app.App{app1, app2}
	c.Assert(len(apps), gocheck.Equals, len(expected))
	for i, app := range apps {
		c.Assert(app.Name, gocheck.DeepEquals, expected[i].Name)
		c.Assert(app.Units(), gocheck.DeepEquals, expected[i].Units())
	}
	action := testing.Action{Action: "app-list", User: s.user.Email}
	c.Assert(action, testing.IsRecorded)
}

// Issue #52.
func (s *S) TestAppListShouldListAllAppsOfAllTeamsThatTheUserIsAMember(c *gocheck.C) {
	u := auth.User{Email: "passing-by@angra.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	team := auth.Team{Name: "angra", Users: []string{s.user.Email, u.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": team.Name})
	app1 := app.App{
		Name:  "app1",
		Teams: []string{s.team.Name, "angra"},
	}
	err = s.conn.Apps().Insert(app1)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app1.Name})
	defer s.conn.Logs(app1.Name).DropCollection()
	request, err := http.NewRequest("GET", "/apps/", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = appList(recorder, request, token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	var apps []app.App
	err = json.Unmarshal(body, &apps)
	c.Assert(err, gocheck.IsNil)
	c.Assert(apps[0].Name, gocheck.Equals, app1.Name)
}

func (s *S) TestListShouldReturnStatusNoContentWhenAppListIsNil(c *gocheck.C) {
	request, err := http.NewRequest("GET", "/apps/", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = appList(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusNoContent)
}

func (s *S) TestDelete(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	myApp := &app.App{
		Name:     "myapptodelete",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := app.CreateApp(myApp, s.user)
	c.Assert(err, gocheck.IsNil)
	myApp, err = app.GetByName(myApp.Name)
	c.Assert(err, gocheck.IsNil)
	defer app.Delete(myApp)
	request, err := http.NewRequest("DELETE", "/apps/"+myApp.Name+"?:app="+myApp.Name, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = appDelete(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	c.Assert(h.url[1], gocheck.Equals, "/repository/myapptodelete") // increment the index because of CreateApp action
	c.Assert(h.method[1], gocheck.Equals, "DELETE")
	c.Assert(string(h.body[1]), gocheck.Equals, "null")
	action := testing.Action{
		Action: "app-delete",
		User:   s.user.Email,
		Extra:  []interface{}{myApp.Name},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestDeleteShouldReturnForbiddenIfTheGivenUserDoesNotHaveAccesToTheApp(c *gocheck.C) {
	myApp := app.App{
		Name:     "MyAppToDelete",
		Platform: "zend",
	}
	err := s.conn.Apps().Insert(myApp)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": myApp.Name})
	defer s.conn.Logs(myApp.Name).DropCollection()
	request, err := http.NewRequest("DELETE", "/apps/"+myApp.Name+"?:app="+myApp.Name, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = appDelete(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e, gocheck.ErrorMatches, "^User does not have access to this app$")
}

func (s *S) TestDeleteShouldReturnNotFoundIfTheAppDoesNotExist(c *gocheck.C) {
	request, err := http.NewRequest("DELETE", "/apps/unkown?:app=unknown", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = appDelete(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestAppInfo(c *gocheck.C) {
	config.Set("host", "http://myhost.com")
	expectedApp := app.App{
		Name:     "NewApp",
		Platform: "django",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(expectedApp)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": expectedApp.Name})
	defer s.conn.Logs(expectedApp.Name).DropCollection()
	var myApp map[string]interface{}
	request, err := http.NewRequest("GET", "/apps/"+expectedApp.Name+"?:app="+expectedApp.Name, nil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	c.Assert(err, gocheck.IsNil)
	err = appInfo(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), gocheck.Equals, "application/json; profile=http://myhost.com/schema/app")
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	err = json.Unmarshal(body, &myApp)
	c.Assert(err, gocheck.IsNil)
	c.Assert(myApp["name"], gocheck.Equals, expectedApp.Name)
	c.Assert(myApp["repository"], gocheck.Equals, repository.ReadWriteURL(expectedApp.Name))
	action := testing.Action{
		Action: "app-info",
		User:   s.user.Email,
		Extra:  []interface{}{expectedApp.Name},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestAppInfoReturnsForbiddenWhenTheUserDoesNotHaveAccessToTheApp(c *gocheck.C) {
	expectedApp := app.App{
		Name:     "NewApp",
		Platform: "django",
	}
	err := s.conn.Apps().Insert(expectedApp)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": expectedApp.Name})
	defer s.conn.Logs(expectedApp.Name).DropCollection()
	request, err := http.NewRequest("GET", "/apps/"+expectedApp.Name+"?:app="+expectedApp.Name, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = appInfo(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e, gocheck.ErrorMatches, "^User does not have access to this app$")
}

func (s *S) TestAppInfoReturnsNotFoundWhenAppDoesNotExist(c *gocheck.C) {
	myApp := app.App{Name: "SomeApp"}
	request, err := http.NewRequest("GET", "/apps/"+myApp.Name+"?:app="+myApp.Name, nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = appInfo(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^App SomeApp not found.$")
}

func (s *S) TestCreateAppHandler(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	a := app.App{Name: "someapp"}
	defer func() {
		a, err := app.GetByName("someapp")
		c.Assert(err, gocheck.IsNil)
		err = app.Delete(a)
		c.Assert(err, gocheck.IsNil)
	}()
	data := `{"name":"someapp","platform":"zend"}`
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = createApp(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	repoURL := repository.ReadWriteURL(a.Name)
	var obtained map[string]string
	expected := map[string]string{
		"status":         "success",
		"repository_url": repoURL,
		"ip":             "someapp.fake-lb.tsuru.io",
	}
	err = json.Unmarshal(body, &obtained)
	c.Assert(obtained, gocheck.DeepEquals, expected)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "someapp"}).One(&gotApp)
	c.Assert(err, gocheck.IsNil)
	c.Assert(gotApp.Teams, gocheck.DeepEquals, []string{s.team.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), gocheck.HasLen, 0)
	action := testing.Action{
		Action: "create-app",
		User:   s.user.Email,
		Extra:  []interface{}{"name=someapp", "platform=zend", "plan="},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestCreateAppTeamOwner(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	a := app.App{Name: "someapp"}
	defer func() {
		a, err := app.GetByName("someapp")
		c.Assert(err, gocheck.IsNil)
		err = app.Delete(a)
		c.Assert(err, gocheck.IsNil)
	}()
	data := `{"name":"someapp","platform":"zend","teamOwner":"tsuruteam"}`
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = createApp(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": a.Name}).One(&gotApp)
	c.Assert(err, gocheck.IsNil)
	repoURL := repository.ReadWriteURL(a.Name)
	var appIP string
	appIP, err = s.provisioner.Addr(&gotApp)
	c.Assert(err, gocheck.IsNil)
	var obtained map[string]string
	expected := map[string]string{
		"status":         "success",
		"repository_url": repoURL,
		"ip":             appIP,
	}
	err = json.Unmarshal(body, &obtained)
	c.Assert(obtained, gocheck.DeepEquals, expected)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	c.Assert(err, gocheck.IsNil)
	c.Assert(gotApp.Teams, gocheck.DeepEquals, []string{s.team.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), gocheck.HasLen, 0)
	action := testing.Action{
		Action: "create-app",
		User:   s.user.Email,
		Extra:  []interface{}{"name=someapp", "platform=zend", "plan="},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestCreateAppCustomPlan(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	a := app.App{Name: "someapp"}
	defer func() {
		a, err := app.GetByName("someapp")
		c.Assert(err, gocheck.IsNil)
		err = app.Delete(a)
		c.Assert(err, gocheck.IsNil)
	}()
	expectedPlan := app.Plan{
		Name:     "myplan",
		Memory:   10,
		Swap:     5,
		CpuShare: 10,
	}
	err := expectedPlan.Save()
	c.Assert(err, gocheck.IsNil)
	defer app.PlanRemove(expectedPlan.Name)
	data := `{"name":"someapp","platform":"zend","plan":{"name":"myplan"}}`
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = createApp(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	repoURL := repository.ReadWriteURL(a.Name)
	var obtained map[string]string
	expected := map[string]string{
		"status":         "success",
		"repository_url": repoURL,
		"ip":             "someapp.fake-lb.tsuru.io",
	}
	err = json.Unmarshal(body, &obtained)
	c.Assert(obtained, gocheck.DeepEquals, expected)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "someapp"}).One(&gotApp)
	c.Assert(err, gocheck.IsNil)
	c.Assert(gotApp.Teams, gocheck.DeepEquals, []string{s.team.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), gocheck.HasLen, 0)
	c.Assert(gotApp.Plan, gocheck.DeepEquals, expectedPlan)
	action := testing.Action{
		Action: "create-app",
		User:   s.user.Email,
		Extra:  []interface{}{"name=someapp", "platform=zend", "plan=myplan"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestCreateAppTwoTeamOwner(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	team := auth.Team{Name: "tsurutwo", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(team)
	c.Check(err, gocheck.IsNil)
	defer s.conn.Teams().RemoveId(team.Name)
	data := `{"name":"someapp","platform":"zend"}`
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = createApp(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestCreateAppQuotaExceeded(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	var limited quota.Quota
	conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota": limited}})
	defer conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota": quota.Unlimited}})
	b := strings.NewReader(`{"name":"someapp","platform":"zend"}`)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = createApp(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e.Message, gocheck.Matches, "^.*Quota exceeded$")
}

func (s *S) TestCreateAppInvalidName(c *gocheck.C) {
	b := strings.NewReader(`{"name":"123myapp","platform":"zend"}`)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = createApp(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	msg := "Invalid app name, your app should have at most 63 " +
		"characters, containing only lower case letters, numbers " +
		"or dashes, starting with a letter."
	c.Assert(e.Error(), gocheck.Equals, msg)
}

func (s *S) TestCreateAppReturns400IfTheUserIsNotMemberOfAnyTeam(c *gocheck.C) {
	u := &auth.User{Email: "thetrees@rush.com", Password: "123456"}
	_, err := nativeScheme.Create(u)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	b := strings.NewReader(`{"name":"someapp", "platform":"django"}`)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = createApp(recorder, request, token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e, gocheck.ErrorMatches, "^In order to create an app, you should be member of at least one team$")
}

func (s *S) TestCreateAppReturnsConflictWithProperMessageWhenTheAppAlreadyExist(c *gocheck.C) {
	a := app.App{
		Name: "plainsofdawn",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	b := strings.NewReader(`{"name":"plainsofdawn","platform":"zend"}`)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = createApp(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, ".*there is already an app with this name.*")
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusConflict)
}

func (s *S) TestAddUnits(c *gocheck.C) {
	a := app.App{
		Name:     "armorandsword",
		Platform: "python",
		Teams:    []string{s.team.Name},
		Quota:    quota.Unlimited,
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.provisioner.Destroy(&a)
	body := strings.NewReader("3")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:app=armorandsword", body)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = addUnits(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	app, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Units(), gocheck.HasLen, 3)
	action := testing.Action{
		Action: "add-units",
		User:   s.user.Email,
		Extra:  []interface{}{"app=armorandsword", "units=3"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestAddUnitsReturns404IfAppDoesNotExist(c *gocheck.C) {
	body := strings.NewReader("1")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:app=armorandsword", body)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = addUnits(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e.Message, gocheck.Equals, "App armorandsword not found.")
}

func (s *S) TestAddUnitsReturns403IfTheUserDoesNotHaveAccessToTheApp(c *gocheck.C) {
	a := app.App{
		Name:     "armorandsword",
		Platform: "python",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	body := strings.NewReader("1")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:app=armorandsword", body)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = addUnits(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e.Message, gocheck.Equals, "User does not have access to this app")
}

func (s *S) TestAddUnitsReturns400IfNumberOfUnitsIsOmited(c *gocheck.C) {
	bodies := []io.Reader{nil, strings.NewReader("")}
	for _, body := range bodies {
		request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:app=armorandsword", body)
		c.Assert(err, gocheck.IsNil)
		recorder := httptest.NewRecorder()
		err = addUnits(recorder, request, s.token)
		c.Assert(err, gocheck.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, gocheck.Equals, true)
		c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
		c.Assert(e.Message, gocheck.Equals, "You must provide the number of units.")
	}
}

func (s *S) TestAddUnitsReturns400IfNumberIsInvalid(c *gocheck.C) {
	values := []string{"-1", "0", "far cry", "12345678909876543"}
	for _, value := range values {
		body := strings.NewReader(value)
		request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:app=armorandsword", body)
		c.Assert(err, gocheck.IsNil)
		recorder := httptest.NewRecorder()
		err = addUnits(recorder, request, s.token)
		c.Assert(err, gocheck.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, gocheck.Equals, true)
		c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
		c.Assert(e.Message, gocheck.Equals, "Invalid number of units: the number must be an integer greater than 0.")
	}
}

func (s *S) TestAddUnitsQuotaExceeded(c *gocheck.C) {
	a := app.App{
		Name:     "armorandsword",
		Platform: "python",
		Teams:    []string{s.team.Name},
		Quota:    quota.Quota{Limit: 2},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.provisioner.Destroy(&a)
	body := strings.NewReader("3")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:app=armorandsword", body)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = addUnits(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e.Message, gocheck.Equals, "Quota exceeded. Available: 2. Requested: 3.")
}

func (s *S) TestRemoveUnits(c *gocheck.C) {
	a := app.App{
		Name:     "velha",
		Platform: "python",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 3, nil)
	body := strings.NewReader("2")
	request, err := http.NewRequest("DELETE", "/apps/velha/units?:app=velha", body)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUnits(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	app, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(context.IsPreventUnlock(request), gocheck.Equals, true)
	c.Assert(app.Units(), gocheck.HasLen, 1)
	c.Assert(s.provisioner.GetUnits(app), gocheck.HasLen, 1)
	action := testing.Action{
		Action: "remove-units",
		User:   s.user.Email,
		Extra:  []interface{}{"app=velha", "units=2"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestRemoveUnitsReturns404IfAppDoesNotExist(c *gocheck.C) {
	body := strings.NewReader("1")
	request, err := http.NewRequest("DELETE", "/apps/fetisha/units?:app=fetisha", body)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUnits(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e.Message, gocheck.Equals, "App fetisha not found.")
}

func (s *S) TestRemoveUnitsReturns403IfTheUserDoesNotHaveAccessToTheApp(c *gocheck.C) {
	a := app.App{
		Name:     "fetisha",
		Platform: "python",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	body := strings.NewReader("1")
	request, err := http.NewRequest("DELETE", "/apps/fetisha/units?:app=fetisha", body)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUnits(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e.Message, gocheck.Equals, "User does not have access to this app")
}

func (s *S) TestRemoveUnitsReturns400IfNumberOfUnitsIsOmited(c *gocheck.C) {
	bodies := []io.Reader{nil, strings.NewReader("")}
	for _, body := range bodies {
		request, err := http.NewRequest("DELETE", "/apps/fetisha/units?:app=fetisha", body)
		c.Assert(err, gocheck.IsNil)
		recorder := httptest.NewRecorder()
		err = removeUnits(recorder, request, s.token)
		c.Assert(err, gocheck.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, gocheck.Equals, true)
		c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
		c.Assert(e.Message, gocheck.Equals, "You must provide the number of units.")
	}
}

func (s *S) TestRemoveUnitsReturns400IfNumberIsInvalid(c *gocheck.C) {
	values := []string{"-1", "0", "far cry", "12345678909876543"}
	for _, value := range values {
		body := strings.NewReader(value)
		request, err := http.NewRequest("DELETE", "/apps/fiend/units?:app=fiend", body)
		c.Assert(err, gocheck.IsNil)
		recorder := httptest.NewRecorder()
		err = removeUnits(recorder, request, s.token)
		c.Assert(err, gocheck.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, gocheck.Equals, true)
		c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
		c.Assert(e.Message, gocheck.Equals, "Invalid number of units: the number must be an integer greater than 0.")
	}
}

func (s *S) TestSetUnitStatus(c *gocheck.C) {
	a := app.App{
		Name:     "telegram",
		Platform: "python",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 3, nil)
	body := strings.NewReader("status=error")
	unit := a.Units()[0]
	request, err := http.NewRequest("POST", "/apps/telegram/units/<unit-name>?:app=telegram&:unit="+unit.Name, body)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = setUnitStatus(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	unit = a.Units()[0]
	c.Assert(unit.Status, gocheck.Equals, provision.StatusError)
}

func (s *S) TestSetUnitStatusNoUnit(c *gocheck.C) {
	body := strings.NewReader("status=error")
	request, err := http.NewRequest("POST", "/apps/velha/units/af32db?:app=velha", body)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = setUnitStatus(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e.Message, gocheck.Equals, "missing unit")
}

func (s *S) TestSetUnitStatusInvalidStatus(c *gocheck.C) {
	bodies := []io.Reader{strings.NewReader("status=something"), strings.NewReader("")}
	for _, body := range bodies {
		request, err := http.NewRequest("POST", "/apps/velha/units/af32db?:app=velha&:unit=af32db", body)
		c.Assert(err, gocheck.IsNil)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()
		err = setUnitStatus(recorder, request, s.token)
		c.Assert(err, gocheck.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, gocheck.Equals, true)
		c.Check(e.Code, gocheck.Equals, http.StatusBadRequest)
		c.Check(e.Message, gocheck.Equals, provision.ErrInvalidStatus.Error())
	}
}

func (s *S) TestSetUnitStatusAppNotFound(c *gocheck.C) {
	body := strings.NewReader("status=error")
	request, err := http.NewRequest("POST", "/apps/velha/units/af32db?:app=velha&:unit=af32db", body)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = setUnitStatus(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Check(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Check(e.Message, gocheck.Equals, "app not found")
}

func (s *S) TestAddTeamToTheApp(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	t := auth.Team{Name: "itshardteam", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(t)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Teams().RemoveAll(bson.M{"_id": t.Name})
	a := app.App{
		Name:     "itshard",
		Platform: "django",
		Teams:    []string{t.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	app, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Teams, gocheck.HasLen, 2)
	c.Assert(app.Teams[1], gocheck.Equals, s.team.Name)
	action := testing.Action{
		Action: "grant-app-access",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + app.Name, "team=" + s.team.Name},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestGrantAccessToTeamReturn404IfTheAppDoesNotExist(c *gocheck.C) {
	request, err := http.NewRequest("PUT", "/apps/a/teams/b", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), gocheck.Equals, "App a not found.\n")
}

func (s *S) TestGrantAccessToTeamReturn403IfTheGivenUserDoesNotHasAccessToTheApp(c *gocheck.C) {
	a := app.App{
		Name:     "itshard",
		Platform: "django",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), gocheck.Equals, "User does not have access to this app\n")
}

func (s *S) TestGrantAccessToTeamReturn404IfTheTeamDoesNotExist(c *gocheck.C) {
	a := app.App{
		Name:     "itshard",
		Platform: "django",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/teams/a", a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), gocheck.Equals, "Team not found\n")
}

func (s *S) TestGrantAccessToTeamReturn409IfTheTeamHasAlreadyAccessToTheApp(c *gocheck.C) {
	a := app.App{
		Name:     "itshard",
		Platform: "django",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusConflict)
}

func (s *S) TestGrantAccessToTeamCallsGandalf(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	t := &auth.Team{Name: "anything", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(t)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": t.Name})
	a := app.App{
		Name:     "tsuru",
		Platform: "golang",
		Teams:    []string{t.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	c.Assert(h.url[0], gocheck.Equals, "/repository/grant")
	c.Assert(h.method[0], gocheck.Equals, "POST")
	expected := fmt.Sprintf(`{"repositories":["%s"],"users":["%s"]}`, a.Name, s.user.Email)
	c.Assert(string(h.body[0]), gocheck.Equals, expected)
}

func (s *S) TestRevokeAccessFromTeam(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	t := auth.Team{Name: "abcd"}
	err := s.conn.Teams().Insert(t)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": t.Name})
	a := app.App{
		Name:     "itshard",
		Platform: "django",
		Teams:    []string{"abcd", s.team.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	app, err := app.GetByName(a.Name)
	c.Assert(app.Teams, gocheck.HasLen, 1)
	c.Assert(app.Teams[0], gocheck.Equals, "abcd")
	action := testing.Action{
		Action: "revoke-app-access",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + app.Name, "team=" + s.team.Name},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestRevokeAccessFromTeamReturn404IfTheAppDoesNotExist(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	request, err := http.NewRequest("DELETE", "/apps/a/teams/b", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), gocheck.Equals, "App a not found.\n")
}

func (s *S) TestRevokeAccessFromTeamReturn401IfTheGivenUserDoesNotHavePermissionInTheApp(c *gocheck.C) {
	a := app.App{
		Name:     "itshard",
		Platform: "django",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), gocheck.Equals, "User does not have access to this app\n")
}

func (s *S) TestRevokeAccessFromTeamReturn404IfTheTeamDoesNotExist(c *gocheck.C) {
	a := app.App{
		Name:     "itshard",
		Platform: "django",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/teams/x", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), gocheck.Equals, "Team not found\n")
}

func (s *S) TestRevokeAccessFromTeamReturn404IfTheTeamDoesNotHaveAccessToTheApp(c *gocheck.C) {
	t := auth.Team{Name: "blaaa"}
	err := s.conn.Teams().Insert(t)
	c.Assert(err, gocheck.IsNil)
	t2 := auth.Team{Name: "team2"}
	err = s.conn.Teams().Insert(t2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": bson.M{"$in": []string{"blaaa", "team2"}}})
	a := app.App{
		Name:     "itshard",
		Platform: "django",
		Teams:    []string{s.team.Name, t2.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, t.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusNotFound)
}

func (s *S) TestRevokeAccessFromTeamReturn403IfTheTeamIsTheLastWithAccessToTheApp(c *gocheck.C) {
	a := app.App{
		Name:     "itshard",
		Platform: "django",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), gocheck.Equals, "You can not revoke the access from this team, because it is the unique team with access to the app, and an app can not be orphaned\n")
}

func (s *S) TestRevokeAccessFromTeamRemovesRepositoryFromGandalf(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	u := auth.User{Email: "again@live.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	t := auth.Team{Name: "anything", Users: []string{u.Email}}
	err = s.conn.Teams().Insert(t)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": t.Name})
	a := app.App{
		Name:     "tsuru",
		Platform: "golang",
		Teams:    []string{t.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	request, err = http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder = httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler = RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	c.Assert(h.url[1], gocheck.Equals, "/repository/revoke") //should inc the index (because of the grantAccess)
	c.Assert(h.method[1], gocheck.Equals, "DELETE")
	expected := fmt.Sprintf(`{"repositories":["%s"],"users":["%s"]}`, a.Name, s.user.Email)
	c.Assert(string(h.body[1]), gocheck.Equals, expected)
}

func (s *S) TestRevokeAccessFromTeamDontRemoveTheUserIfItHasAccesToTheAppThroughAnotherTeam(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	u := auth.User{Email: "burning@angel.com"}
	err := s.conn.Users().Insert(u)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	t := auth.Team{Name: "anything", Users: []string{s.user.Email, u.Email}}
	err = s.conn.Teams().Insert(t)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": t.Name})
	a := app.App{
		Name:     "tsuru",
		Platform: "golang",
		Teams:    []string{s.team.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, t.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	request, err = http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder = httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler = RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	c.Assert(h.url[1], gocheck.Equals, "/repository/revoke")
	c.Assert(h.method[1], gocheck.Equals, "DELETE")
	expected := fmt.Sprintf(`{"repositories":[%q],"users":[%q]}`, a.Name, u.Email)
	c.Assert(string(h.body[1]), gocheck.Equals, expected)
}

func (s *S) TestRevokeAccessFromTeamDontCallGandalfIfNoUserNeedToBeRevoked(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	t := auth.Team{Name: "anything", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(t)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": t.Name})
	a := app.App{
		Name:     "tsuru",
		Platform: "golang",
		Teams:    []string{s.team.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, t.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	c.Assert(err, gocheck.IsNil)
	request, err = http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder = httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler = RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	c.Assert(h.url, gocheck.HasLen, 1)
	c.Assert(h.url[0], gocheck.Equals, "/repository/grant")
}

func (s *S) TestRunOnceHandler(c *gocheck.C) {
	s.provisioner.PrepareOutput([]byte("lots of files"))
	a := app.App{
		Name:     "secrets",
		Platform: "arch enemy",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 1, nil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/run/?:app=%s&once=true", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("ls"))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = runCommand(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Body.String(), gocheck.Equals, `{"Message":"lots of files"}`+"\n")
	c.Assert(recorder.Header().Get("Content-Type"), gocheck.Equals, "text")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls"
	cmds := s.provisioner.GetCmds(expected, &a)
	c.Assert(cmds, gocheck.HasLen, 1)
	action := testing.Action{
		Action: "run-command",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, "command=ls"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestRunHandler(c *gocheck.C) {
	s.provisioner.PrepareOutput([]byte("lots of\nfiles"))
	a := app.App{
		Name:     "secrets",
		Platform: "arch enemy",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 1, nil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/run/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("ls"))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = runCommand(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Body.String(), gocheck.Equals, `{"Message":"lots of\nfiles"}`+"\n")
	c.Assert(recorder.Header().Get("Content-Type"), gocheck.Equals, "text")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls"
	cmds := s.provisioner.GetCmds(expected, &a)
	c.Assert(cmds, gocheck.HasLen, 1)
	action := testing.Action{
		Action: "run-command",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, "command=ls"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestRunHandlerReturnsTheOutputOfTheCommandEvenIfItFails(c *gocheck.C) {
	s.provisioner.PrepareFailure("ExecuteCommand", &errors.HTTP{Code: 500, Message: "something went wrong"})
	s.provisioner.PrepareOutput([]byte("failure output"))
	a := app.App{
		Name:     "secrets",
		Platform: "arch enemy",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 1, nil)
	url := fmt.Sprintf("/apps/%s/run/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("ls"))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = runCommand(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "something went wrong")
	expected := `{"Message":"failure output"}` + "\n" +
		`{"Message":"","Error":"something went wrong"}` + "\n"
	c.Assert(recorder.Body.String(), gocheck.Equals, expected)
}

func (s *S) TestRunHandlerReturnsBadRequestIfTheCommandIsMissing(c *gocheck.C) {
	bodies := []io.Reader{nil, strings.NewReader("")}
	for _, body := range bodies {
		request, err := http.NewRequest("POST", "/apps/unknown/run/?:app=unkown", body)
		c.Assert(err, gocheck.IsNil)
		recorder := httptest.NewRecorder()
		err = runCommand(recorder, request, s.token)
		c.Assert(err, gocheck.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, gocheck.Equals, true)
		c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
		c.Assert(e, gocheck.ErrorMatches, "^You must provide the command to run$")
	}
}

func (s *S) TestRunHandlerReturnsInternalErrorIfReadAllFails(c *gocheck.C) {
	b := s.getTestData("bodyToBeClosed.txt")
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, gocheck.IsNil)
	request.Body.Close()
	recorder := httptest.NewRecorder()
	err = runCommand(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestRunHandlerReturnsNotFoundIfTheAppDoesNotExist(c *gocheck.C) {
	request, err := http.NewRequest("POST", "/apps/unknown/run/?:app=unknown", strings.NewReader("ls"))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = runCommand(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestRunHandlerReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *gocheck.C) {
	a := app.App{
		Name:     "secrets",
		Platform: "arch enemy",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/run/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("ls"))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = runCommand(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
}

func (s *S) TestGetEnvHandlerGetsEnvironmentVariableFromApp(c *gocheck.C) {
	a := app.App{
		Name:     "everything-i-want",
		Platform: "gotthard",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, strings.NewReader(`["DATABASE_HOST"]`))
	request.Header.Set("Content-Type", "application/json")
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = getEnv(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	expected := []map[string]interface{}{{
		"name":   "DATABASE_HOST",
		"value":  "localhost",
		"public": true,
	}}
	result := []map[string]interface{}{}
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.DeepEquals, expected)
	c.Assert(recorder.Header().Get("Content-Type"), gocheck.Equals, "application/json")
	action := testing.Action{
		Action: "get-env",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, "envs=[DATABASE_HOST]"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestGetEnvHandlerShouldAcceptMultipleVariables(c *gocheck.C) {
	a := app.App{
		Name:  "four-sticks",
		Teams: []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, strings.NewReader(`["DATABASE_HOST", "DATABASE_USER"]`))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = getEnv(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Header().Get("Content-type"), gocheck.Equals, "application/json")
	expected := []map[string]interface{}{
		{"name": "DATABASE_HOST", "value": "localhost", "public": true},
		{"name": "DATABASE_USER", "value": "root", "public": true},
	}
	var got []map[string]interface{}
	err = json.Unmarshal(recorder.Body.Bytes(), &got)
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.DeepEquals, expected)
	action := testing.Action{
		Action: "get-env",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, "envs=[DATABASE_HOST DATABASE_USER]"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestGetEnvHandlerReturnsInternalErrorIfReadAllFails(c *gocheck.C) {
	b := s.getTestData("bodyToBeClosed.txt")
	request, err := http.NewRequest("GET", "/apps/unkown/env/?:app=unknown", b)
	c.Assert(err, gocheck.IsNil)
	request.Body.Close()
	recorder := httptest.NewRecorder()
	err = getEnv(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestGetEnvHandlerReturnsNotFoundIfTheAppDoesNotExist(c *gocheck.C) {
	request, err := http.NewRequest("GET", "/apps/unknown/env/?:app=unknown", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = getEnv(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestGetEnvHandlerReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *gocheck.C) {
	a := app.App{
		Name:     "lost",
		Platform: "vougan",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, strings.NewReader(`["DATABASE_HOST"]`))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = getEnv(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
}

func (s *S) TestGetEnvHandlerGetsEnvironmentVariableFromAppWithAppToken(c *gocheck.C) {
	a := app.App{
		Name:     "everything-i-want",
		Platform: "gotthard",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, strings.NewReader(`["DATABASE_HOST"]`))
	request.Header.Set("Content-Type", "application/json")
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	token, err := nativeScheme.AppLogin("appToken")
	c.Assert(err, gocheck.IsNil)
	err = getEnv(recorder, request, auth.Token(token))
	c.Assert(err, gocheck.IsNil)
	expected := []map[string]interface{}{{
		"name":   "DATABASE_HOST",
		"value":  "localhost",
		"public": true,
	}}
	result := []map[string]interface{}{}
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.DeepEquals, expected)
	c.Assert(recorder.Header().Get("Content-Type"), gocheck.Equals, "application/json")
}

func (s *S) TestSetEnvHandlerShouldSetAPublicEnvironmentVariableInTheApp(c *gocheck.C) {
	s.provisioner.PrepareOutput([]byte("exported"))
	a := app.App{
		Name:  "black-dog",
		Teams: []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/env?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader(`{"DATABASE_HOST":"localhost"}`))
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	err = setEnv(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	app, err := app.GetByName("black-dog")
	c.Assert(err, gocheck.IsNil)
	expected := bind.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: true}
	c.Assert(app.Env["DATABASE_HOST"], gocheck.DeepEquals, expected)
	envs := map[string]string{
		"DATABASE_HOST": "localhost",
	}
	action := testing.Action{
		Action: "set-env",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, envs},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestSetEnvHandlerShouldSetMultipleEnvironmentVariablesInTheApp(c *gocheck.C) {
	s.provisioner.PrepareOutput([]byte("exported"))
	a := app.App{
		Name:  "vigil",
		Teams: []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/env?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"DATABASE_HOST": "localhost", "DATABASE_USER": "root"}`)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	err = setEnv(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	app, err := app.GetByName("vigil")
	c.Assert(err, gocheck.IsNil)
	expectedHost := bind.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: true}
	expectedUser := bind.EnvVar{Name: "DATABASE_USER", Value: "root", Public: true}
	c.Assert(app.Env["DATABASE_HOST"], gocheck.DeepEquals, expectedHost)
	c.Assert(app.Env["DATABASE_USER"], gocheck.DeepEquals, expectedUser)
	envs := map[string]string{
		"DATABASE_HOST": "localhost",
		"DATABASE_USER": "root",
	}
	action := testing.Action{
		Action: "set-env",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, envs},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestSetEnvHandlerShouldNotChangeValueOfPrivateVariables(c *gocheck.C) {
	s.provisioner.PrepareOutput([]byte("exported"))
	original := map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:   "DATABASE_HOST",
			Value:  "privatehost.com",
			Public: false,
		},
	}
	a := app.App{
		Name:  "losers",
		Teams: []string{s.team.Name},
		Env:   original,
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/env?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader(`{"DATABASE_HOST":"http://foo.com:8080"}`))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	err = setEnv(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	app, err := app.GetByName("losers")
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Env, gocheck.DeepEquals, original)
}

func (s *S) TestSetEnvHandlerReturnsInternalErrorIfReadAllFails(c *gocheck.C) {
	b := s.getTestData("bodyToBeClosed.txt")
	request, err := http.NewRequest("POST", "/apps/unkown/env/?:app=unknown", b)
	c.Assert(err, gocheck.IsNil)
	request.Body.Close()
	recorder := httptest.NewRecorder()
	err = setEnv(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestSetEnvHandlerReturnsBadRequestIfVariablesAreMissing(c *gocheck.C) {
	bodies := []io.Reader{nil, strings.NewReader("")}
	for _, body := range bodies {
		request, err := http.NewRequest("POST", "/apps/unknown/env/?:app=unkown", body)
		c.Assert(err, gocheck.IsNil)
		recorder := httptest.NewRecorder()
		err = setEnv(recorder, request, s.token)
		c.Assert(err, gocheck.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, gocheck.Equals, true)
		c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
		c.Assert(e, gocheck.ErrorMatches, "^You must provide the environment variables in a JSON object$")
	}
}

func (s *S) TestSetEnvHandlerReturnsNotFoundIfTheAppDoesNotExist(c *gocheck.C) {
	b := strings.NewReader(`{"DATABASE_HOST":"localhost"}`)
	request, err := http.NewRequest("POST", "/apps/unknown/env/?:app=unknown", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = setEnv(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestSetEnvHandlerReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *gocheck.C) {
	a := app.App{Name: "rock-and-roll"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader(`{"DATABASE_HOST":"localhost"}`))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	err = setEnv(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
}

func (s *S) TestUnsetEnvHandlerRemovesTheEnvironmentVariablesFromTheApp(c *gocheck.C) {
	a := app.App{
		Name:  "swift",
		Teams: []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	expected := a.Env
	delete(expected, "DATABASE_HOST")
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("DELETE", url, strings.NewReader(`["DATABASE_HOST"]`))
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	err = unsetEnv(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	app, err := app.GetByName("swift")
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Env, gocheck.DeepEquals, expected)
	action := testing.Action{
		Action: "unset-env",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, "envs=[DATABASE_HOST]"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestUnsetEnvHandlerRemovesAllGivenEnvironmentVariables(c *gocheck.C) {
	a := app.App{
		Name:  "let-it-be",
		Teams: []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("DELETE", url, strings.NewReader(`["DATABASE_HOST", "DATABASE_USER"]`))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	err = unsetEnv(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	app, err := app.GetByName("let-it-be")
	c.Assert(err, gocheck.IsNil)
	expected := map[string]bind.EnvVar{
		"DATABASE_PASSWORD": {
			Name:   "DATABASE_PASSWORD",
			Value:  "secret",
			Public: false,
		},
	}
	c.Assert(app.Env, gocheck.DeepEquals, expected)
	action := testing.Action{
		Action: "unset-env",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, "envs=[DATABASE_HOST DATABASE_USER]"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestUnsetHandlerDoesNotRemovePrivateVariables(c *gocheck.C) {
	a := app.App{
		Name:  "letitbe",
		Teams: []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`["DATABASE_HOST", "DATABASE_USER", "DATABASE_PASSWORD"]`)
	request, err := http.NewRequest("DELETE", url, b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	err = unsetEnv(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	app, err := app.GetByName("letitbe")
	c.Assert(err, gocheck.IsNil)
	expected := map[string]bind.EnvVar{
		"DATABASE_PASSWORD": {
			Name:   "DATABASE_PASSWORD",
			Value:  "secret",
			Public: false,
		},
	}
	c.Assert(app.Env, gocheck.DeepEquals, expected)
}

func (s *S) TestUnsetEnvHandlerReturnsInternalErrorIfReadAllFails(c *gocheck.C) {
	b := s.getTestData("bodyToBeClosed.txt")
	request, err := http.NewRequest("POST", "/apps/unkown/env/?:app=unknown", b)
	c.Assert(err, gocheck.IsNil)
	request.Body.Close()
	recorder := httptest.NewRecorder()
	err = unsetEnv(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestUnsetEnvHandlerReturnsBadRequestIfVariablesAreMissing(c *gocheck.C) {
	bodies := []io.Reader{nil, strings.NewReader(""), strings.NewReader("[]")}
	for _, body := range bodies {
		request, err := http.NewRequest("POST", "/apps/unknown/env/?:app=unkown", body)
		c.Assert(err, gocheck.IsNil)
		recorder := httptest.NewRecorder()
		err = unsetEnv(recorder, request, s.token)
		c.Assert(err, gocheck.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, gocheck.Equals, true)
		c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
		c.Assert(e, gocheck.ErrorMatches, "^You must provide the list of environment variables, in JSON format$")
	}
}

func (s *S) TestUnsetEnvHandlerReturnsNotFoundIfTheAppDoesNotExist(c *gocheck.C) {
	b := strings.NewReader(`["DATABASE_HOST"]`)
	request, err := http.NewRequest("POST", "/apps/unknown/env/?:app=unknown", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = unsetEnv(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestUnsetEnvHandlerReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *gocheck.C) {
	a := app.App{Name: "mountain-mama"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader(`["DATABASE_HOST"]`))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	err = unsetEnv(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
}

func (s *S) TestAddCNameHandler(c *gocheck.C) {
	a := app.App{Name: "leper", Teams: []string{s.team.Name}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"cname":["leper.secretcompany.com"]}`)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = setCName(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	app, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.CName, gocheck.DeepEquals, []string{"leper.secretcompany.com"})
	action := testing.Action{
		Action: "add-cname",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + app.Name, "cname=leper.secretcompany.com"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestAddCNameHandlerAcceptsWildCard(c *gocheck.C) {
	a := app.App{Name: "leper", Teams: []string{s.team.Name}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"cname":["*.leper.secretcompany.com"]}`)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = setCName(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	app, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.CName, gocheck.DeepEquals, []string{"*.leper.secretcompany.com"})
	action := testing.Action{
		Action: "add-cname",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + app.Name, "cname=*.leper.secretcompany.com"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestAddCNameHandlerAcceptsEmptyCName(c *gocheck.C) {
	a := app.App{Name: "leper", Teams: []string{s.team.Name}, CName: []string{"leper.secretcompany.com"}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"cname":[]}`)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = setCName(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	app, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.CName, gocheck.DeepEquals, []string{"leper.secretcompany.com"})
}

func (s *S) TestAddCNameHandlerErrsOnInvalidCName(c *gocheck.C) {
	a := app.App{Name: "leper", Teams: []string{s.team.Name}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"cname":["_leper.secretcompany.com"]}`)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = setCName(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e.Message, gocheck.Equals, "Invalid cname")
}

func (s *S) TestAddCNameHandlerReturnsInternalErrorIfItFailsToReadTheBody(c *gocheck.C) {
	b := s.getTestData("bodyToBeClosed.txt")
	request, err := http.NewRequest("POST", "/apps/unkown/cname?:app=unknown", b)
	c.Assert(err, gocheck.IsNil)
	request.Body.Close()
	recorder := httptest.NewRecorder()
	err = setCName(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestAddCNameHandlerReturnsBadRequestWhenCNameIsMissingFromTheBody(c *gocheck.C) {
	bodies := []io.Reader{nil, strings.NewReader(`{}`)}
	for _, b := range bodies {
		request, err := http.NewRequest("POST", "/apps/unknown/cname?:app=unknown", b)
		c.Assert(err, gocheck.IsNil)
		recorder := httptest.NewRecorder()
		err = setCName(recorder, request, s.token)
		c.Check(err, gocheck.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Check(ok, gocheck.Equals, true)
		c.Check(e.Code, gocheck.Equals, http.StatusBadRequest)
		c.Check(e.Message, gocheck.Equals, "You must provide the cname.")
	}
}

func (s *S) TestAddCNameHandlerInvalidJSON(c *gocheck.C) {
	b := strings.NewReader(`}"I'm invalid json"`)
	request, err := http.NewRequest("POST", "/apps/unknown/cname?:app=unknown", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = setCName(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e.Message, gocheck.Equals, "Invalid JSON in request body.")
}

func (s *S) TestAddCNameHandlerUnknownApp(c *gocheck.C) {
	b := strings.NewReader(`{"cname": ["leper.secretcompany.com"]}`)
	request, err := http.NewRequest("POST", "/apps/unknown/cname?:app=unknown", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = setCName(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
}

func (s *S) TestAddCNameHandlerUserWithoutAccessToTheApp(c *gocheck.C) {
	a := app.App{
		Name:     "lost",
		Platform: "vougan",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"cname": ["lost.secretcompany.com"]}`)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = setCName(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
}

func (s *S) TestAddCNameHandlerInvalidCName(c *gocheck.C) {
	a := app.App{Name: "leper", Teams: []string{s.team.Name}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"cname": [".leper.secretcompany.com"]}`)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = setCName(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e.Message, gocheck.Equals, "Invalid cname")
}

func (s *S) TestRemoveCNameHandler(c *gocheck.C) {
	a := app.App{Name: "leper", Teams: []string{s.team.Name}, CName: []string{"foo.bar.com"}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"cname": ["foo.bar.com"]}`)
	request, err := http.NewRequest("DELETE", url, b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = unsetCName(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	app, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.CName, gocheck.DeepEquals, []string{})
	action := testing.Action{
		Action: "remove-cname",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + app.Name, "cnames=foo.bar.com"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestUnsetTwoCnames(c *gocheck.C) {
	a := app.App{Name: "leper", Teams: []string{s.team.Name}, CName: []string{"foo.bar.com", "bar.com"}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"cname": ["foo.bar.com", "bar.com"]}`)
	request, err := http.NewRequest("DELETE", url, b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = unsetCName(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	app, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.CName, gocheck.DeepEquals, []string{})
	action := testing.Action{
		Action: "remove-cname",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + app.Name, "cnames=foo.bar.com, bar.com"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestRemoveCNameHandlerUnknownApp(c *gocheck.C) {
	b := strings.NewReader(`{"cname": ["foo.bar.com"]}`)
	request, err := http.NewRequest("DELETE", "/apps/unknown/cname?:app=unknown", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = unsetCName(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
}

func (s *S) TestRemoveCNameHandlerUserWithoutAccessToTheApp(c *gocheck.C) {
	a := app.App{
		Name:     "lost",
		Platform: "vougan",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"cname": ["foo.bar.com"]}`)
	request, err := http.NewRequest("DELETE", url, b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = unsetCName(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
}

func (s *S) TestAppLogShouldReturnNotFoundWhenAppDoesNotExist(c *gocheck.C) {
	request, err := http.NewRequest("GET", "/apps/unknown/log/?:app=unknown&lines=10", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = appLog(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestAppLogReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *gocheck.C) {
	a := app.App{
		Name:     "lost",
		Platform: "vougan",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&lines=10", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = appLog(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
}

func (s *S) TestAppLogReturnsBadRequestIfNumberOfLinesIsMissing(c *gocheck.C) {
	url := "/apps/something/log/?:app=doesntmatter"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = appLog(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e.Message, gocheck.Equals, `Parameter "lines" is mandatory.`)
}

func (s *S) TestAppLogReturnsBadRequestIfNumberOfLinesIsNotAnInteger(c *gocheck.C) {
	url := "/apps/something/log/?:app=doesntmatter&lines=2.34"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = appLog(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e.Message, gocheck.Equals, `Parameter "lines" must be an integer.`)
}

func (s *S) TestAppLogFollowWithPubSub(c *gocheck.C) {
	a := app.App{
		Name:     "lost",
		Platform: "vougan",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	url := "/apps/something/log/?:app=lost&lines=10&follow=1"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		recorder := httptest.NewRecorder()
		err := appLog(recorder, request, s.token)
		c.Assert(err, gocheck.IsNil)
		body, err := ioutil.ReadAll(recorder.Body)
		c.Assert(err, gocheck.IsNil)
		splitted := strings.Split(strings.TrimSpace(string(body)), "\n")
		c.Assert(splitted, gocheck.HasLen, 2)
		c.Assert(splitted[0], gocheck.Equals, "[]")
		logs := []app.Applog{}
		err = json.Unmarshal([]byte(splitted[1]), &logs)
		c.Assert(err, gocheck.IsNil)
		c.Assert(logs, gocheck.HasLen, 1)
		c.Assert(logs[0].Message, gocheck.Equals, "x")
	}()
	time.Sleep(1e8)
	factory, err := queue.Factory()
	c.Assert(err, gocheck.IsNil)
	q, err := factory.Get("pubsub:" + a.Name)
	c.Assert(err, gocheck.IsNil)
	pubSubQ, ok := q.(queue.PubSubQ)
	c.Assert(ok, gocheck.Equals, true)
	err = pubSubQ.Pub([]byte(`{"message": "x"}`))
	c.Assert(err, gocheck.IsNil)
	time.Sleep(1e8)
	pubSubQ.UnSub()
	wg.Wait()
}

func (s *S) TestAppLogFollowWithFilter(c *gocheck.C) {
	a := app.App{
		Name:     "lost",
		Platform: "vougan",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	url := "/apps/something/log/?:app=lost&lines=10&follow=1&source=web"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		recorder := httptest.NewRecorder()
		err := appLog(recorder, request, s.token)
		c.Assert(err, gocheck.IsNil)
		body, err := ioutil.ReadAll(recorder.Body)
		c.Assert(err, gocheck.IsNil)
		splitted := strings.Split(strings.TrimSpace(string(body)), "\n")
		c.Assert(splitted, gocheck.HasLen, 2)
		c.Assert(splitted[0], gocheck.Equals, "[]")
		logs := []app.Applog{}
		err = json.Unmarshal([]byte(splitted[1]), &logs)
		c.Assert(err, gocheck.IsNil)
		c.Assert(logs, gocheck.HasLen, 1)
		c.Assert(logs[0].Message, gocheck.Equals, "y")
	}()
	time.Sleep(1e8)
	factory, err := queue.Factory()
	c.Assert(err, gocheck.IsNil)
	q, err := factory.Get("pubsub:" + a.Name)
	c.Assert(err, gocheck.IsNil)
	pubSubQ, ok := q.(queue.PubSubQ)
	c.Assert(ok, gocheck.Equals, true)
	err = pubSubQ.Pub([]byte(`{"message": "x", "source": "app"}`))
	c.Assert(err, gocheck.IsNil)
	err = pubSubQ.Pub([]byte(`{"message": "y", "source": "web"}`))
	c.Assert(err, gocheck.IsNil)
	time.Sleep(1e8)
	pubSubQ.UnSub()
	wg.Wait()
}

func (s *S) TestAppLogShouldHaveContentType(c *gocheck.C) {
	a := app.App{
		Name:     "lost",
		Platform: "vougan",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&lines=10", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = appLog(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), gocheck.Equals, "application/json")
}

func (s *S) TestAppLogSelectByLines(c *gocheck.C) {
	a := app.App{
		Name:     "lost",
		Platform: "vougan",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	for i := 0; i < 15; i++ {
		a.Log(strconv.Itoa(i), "source", "")
	}
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&lines=10", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = appLog(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	logs := []app.Applog{}
	err = json.Unmarshal(body, &logs)
	c.Assert(err, gocheck.IsNil)
	c.Assert(logs, gocheck.HasLen, 10)
	action := testing.Action{
		Action: "app-log",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, "lines=10"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestAppLogSelectBySource(c *gocheck.C) {
	a := app.App{
		Name:     "lost",
		Platform: "vougan",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	a.Log("mars log", "mars", "")
	a.Log("earth log", "earth", "")
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&source=mars&lines=10", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = appLog(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	logs := []app.Applog{}
	err = json.Unmarshal(body, &logs)
	c.Assert(err, gocheck.IsNil)
	c.Assert(logs, gocheck.HasLen, 1)
	c.Assert(logs[0].Message, gocheck.Equals, "mars log")
	c.Assert(logs[0].Source, gocheck.Equals, "mars")
	action := testing.Action{
		Action: "app-log",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, "lines=10", "source=mars"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestAppLogSelectByUnit(c *gocheck.C) {
	a := app.App{
		Name:     "lost",
		Platform: "vougan",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	a.Log("mars log", "mars", "prospero")
	a.Log("earth log", "earth", "caliban")
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&unit=caliban&lines=10", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = appLog(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	logs := []app.Applog{}
	err = json.Unmarshal(body, &logs)
	c.Assert(err, gocheck.IsNil)
	c.Assert(logs, gocheck.HasLen, 1)
	c.Assert(logs[0].Message, gocheck.Equals, "earth log")
	c.Assert(logs[0].Source, gocheck.Equals, "earth")
	c.Assert(logs[0].Unit, gocheck.Equals, "caliban")
	action := testing.Action{
		Action: "app-log",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + a.Name, "lines=10", "unit=caliban"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestAppLogSelectByLinesShouldReturnTheLastestEntries(c *gocheck.C) {
	a := app.App{
		Name:     "lost",
		Platform: "vougan",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	now := time.Now()
	coll := s.conn.Logs(a.Name)
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
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = appLog(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	var logs []app.Applog
	err = json.Unmarshal(body, &logs)
	c.Assert(err, gocheck.IsNil)
	c.Assert(logs, gocheck.HasLen, 3)
	c.Assert(logs[0].Message, gocheck.Equals, "12")
	c.Assert(logs[1].Message, gocheck.Equals, "13")
	c.Assert(logs[2].Message, gocheck.Equals, "14")
}

func (s *S) TestAppLogShouldReturnLogByApp(c *gocheck.C) {
	app1 := app.App{
		Name:     "app1",
		Platform: "vougan",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(app1)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app1.Name})
	defer s.conn.Logs(app1.Name).DropCollection()
	app1.Log("app1 log", "source", "")
	app2 := app.App{
		Name:     "app2",
		Platform: "vougan",
		Teams:    []string{s.team.Name},
	}
	err = s.conn.Apps().Insert(app2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app2.Name})
	defer s.conn.Logs(app2.Name).DropCollection()
	app2.Log("app2 log", "source", "")
	app3 := app.App{
		Name:     "app3",
		Platform: "vougan",
		Teams:    []string{s.team.Name},
	}
	err = s.conn.Apps().Insert(app3)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app3.Name})
	defer s.conn.Logs(app3.Name).DropCollection()
	app3.Log("app3 log", "tsuru", "")
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&lines=10", app3.Name, app3.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = appLog(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	logs := []app.Applog{}
	err = json.Unmarshal(body, &logs)
	c.Assert(err, gocheck.IsNil)
	var logged bool
	for _, log := range logs {
		// Should not show the app1 log
		c.Assert(log.Message, gocheck.Not(gocheck.Equals), "app1 log")
		// Should not show the app2 log
		c.Assert(log.Message, gocheck.Not(gocheck.Equals), "app2 log")
		if log.Message == "app3 log" {
			logged = true
		}
	}
	// Should show the app3 log
	c.Assert(logged, gocheck.Equals, true)
}

func (s *S) TestBindHandlerEndpointIsDown(c *gocheck.C) {
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": "http://localhost:1234"}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = instance.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a := app.App{
		Name:  "painkiller",
		Teams: []string{s.team.Name},
		Env:   map[string]bind.EnvVar{},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 1, nil)
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, a.Name, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = bindServiceInstance(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestBindHandler(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = instance.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a := app.App{
		Name:  "painkiller",
		Teams: []string{s.team.Name},
		Env:   map[string]bind.EnvVar{},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 1, nil)
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, a.Name, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	s.provisioner.PrepareOutput([]byte("exported"))
	err = bindServiceInstance(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	err = s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(err, gocheck.IsNil)
	c.Assert(instance.Apps, gocheck.DeepEquals, []string{a.Name})
	err = s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(err, gocheck.IsNil)
	expectedUser := bind.EnvVar{Name: "DATABASE_USER", Value: "root", Public: false, InstanceName: instance.Name}
	expectedPassword := bind.EnvVar{Name: "DATABASE_PASSWORD", Value: "s3cr3t", Public: false, InstanceName: instance.Name}
	c.Assert(a.Env["DATABASE_USER"], gocheck.DeepEquals, expectedUser)
	c.Assert(a.Env["DATABASE_PASSWORD"], gocheck.DeepEquals, expectedPassword)
	var envs []string
	err = json.Unmarshal(recorder.Body.Bytes(), &envs)
	c.Assert(err, gocheck.IsNil)
	sort.Strings(envs)
	c.Assert(envs, gocheck.DeepEquals, []string{"DATABASE_PASSWORD", "DATABASE_USER"})
	c.Assert(recorder.Header().Get("Content-Type"), gocheck.Equals, "application/json")
	action := testing.Action{
		Action: "bind-app",
		User:   s.user.Email,
		Extra:  []interface{}{"instance=" + instance.Name, "app=" + a.Name},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestBindHandlerReturns404IfTheInstanceDoesNotExist(c *gocheck.C) {
	a := app.App{
		Name:     "serviceApp",
		Platform: "django",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/services/instances/unknown/%s?:instance=unknown&:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = bindServiceInstance(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e.Message, gocheck.Equals, service.ErrServiceInstanceNotFound.Error())
}

func (s *S) TestBindHandlerReturns403IfTheUserDoesNotHaveAccessToTheInstance(c *gocheck.C) {
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql"}
	err := instance.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a := app.App{
		Name:     "serviceApp",
		Platform: "django",
		Teams:    []string{s.team.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, a.Name, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = bindServiceInstance(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e.Message, gocheck.Equals, service.ErrAccessNotAllowed.Error())
}

func (s *S) TestBindHandlerReturns404IfTheAppDoesNotExist(c *gocheck.C) {
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err := instance.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	url := fmt.Sprintf("/services/instances/%s/unknown?:instance=%s&:app=unknown", instance.Name, instance.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = bindServiceInstance(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestBindHandlerReturns403IfTheUserDoesNotHaveAccessToTheApp(c *gocheck.C) {
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err := instance.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a := app.App{
		Name:     "serviceApp",
		Platform: "django",
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, a.Name, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = bindServiceInstance(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e, gocheck.ErrorMatches, "^This user does not have access to this app$")
}

func (s *S) TestUnbindHandler(c *gocheck.C) {
	s.provisioner.PrepareOutput([]byte("exported"))
	h := testHandler{}
	gts := testing.StartGandalfTestServer(&h)
	defer gts.Close()
	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(r.URL.Path)
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/hostname/10.10.10.1" {
			atomic.StoreInt32(&called, 1)
		}
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
	}
	err = instance.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a := app.App{
		Name:     "painkiller",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, gocheck.IsNil)
	defer app.Delete(&a)
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 1, nil)
	otherApp, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	otherApp.Env["DATABASE_HOST"] = bind.EnvVar{
		Name:         "DATABASE_HOST",
		Value:        "arrea",
		Public:       false,
		InstanceName: instance.Name,
	}
	otherApp.Env["MY_VAR"] = bind.EnvVar{Name: "MY_VAR", Value: "123"}
	err = s.conn.Apps().Update(bson.M{"name": otherApp.Name}, otherApp)
	c.Assert(err, gocheck.IsNil)
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, a.Name,
		instance.Name, a.Name)
	req, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, req, s.token)
	c.Assert(err, gocheck.IsNil)
	err = s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(err, gocheck.IsNil)
	c.Assert(instance.Apps, gocheck.DeepEquals, []string{})
	otherApp, err = app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	expected := bind.EnvVar{
		Name:  "MY_VAR",
		Value: "123",
	}
	c.Assert(otherApp.Env["MY_VAR"], gocheck.DeepEquals, expected)
	_, ok := otherApp.Env["DATABASE_HOST"]
	c.Assert(ok, gocheck.Equals, false)
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
	action := testing.Action{
		Action: "unbind-app",
		User:   s.user.Email,
		Extra:  []interface{}{"instance=" + instance.Name, "app=" + a.Name},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestUnbindHandlerReturns404IfTheInstanceDoesNotExist(c *gocheck.C) {
	a := app.App{
		Name:     "serviceApp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/services/instances/unknown/%s?:instance=unknown&:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e.Message, gocheck.Equals, service.ErrServiceInstanceNotFound.Error())
}

func (s *S) TestUnbindHandlerReturns403IfTheUserDoesNotHaveAccessToTheInstance(c *gocheck.C) {
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql"}
	err := instance.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a := app.App{
		Name:     "serviceApp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, a.Name, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e.Message, gocheck.Equals, service.ErrAccessNotAllowed.Error())
}

func (s *S) TestUnbindHandlerReturns404IfTheAppDoesNotExist(c *gocheck.C) {
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err := instance.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	url := fmt.Sprintf("/services/instances/%s/unknown?:instance=%s&:app=unknown", instance.Name, instance.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestUnbindHandlerReturns403IfTheUserDoesNotHaveAccessToTheApp(c *gocheck.C) {
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err := instance.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a := app.App{
		Name:     "serviceApp",
		Platform: "zend",
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, a.Name, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e, gocheck.ErrorMatches, "^This user does not have access to this app$")
}

func (s *S) TestRestartHandler(c *gocheck.C) {
	s.provisioner.PrepareOutput(nil) // loadHooks
	s.provisioner.PrepareOutput([]byte("restarted"))
	a := app.App{
		Name:  "stress",
		Teams: []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	url := fmt.Sprintf("/apps/%s/restart?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = restart(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Body.String(), gocheck.Matches, "(?s).*---- Restarting your app ----.*")
	c.Assert(recorder.Header().Get("Content-Type"), gocheck.Equals, "text")
	action := testing.Action{
		Action: "restart",
		User:   s.user.Email,
		Extra:  []interface{}{a.Name},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestRestartHandlerReturns404IfTheAppDoesNotExist(c *gocheck.C) {
	request, err := http.NewRequest("GET", "/apps/unknown/restart?:app=unknown", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = restart(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
}

func (s *S) TestRestartHandlerReturns403IfTheUserDoesNotHaveAccessToTheApp(c *gocheck.C) {
	a := app.App{Name: "nightmist"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/apps/%s/restart?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = restart(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
}

type LogList []app.Applog

func (l LogList) Len() int           { return len(l) }
func (l LogList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l LogList) Less(i, j int) bool { return l[i].Message < l[j].Message }
func (s *S) TestAddLogHandler(c *gocheck.C) {
	a := app.App{
		Name:     "myapp",
		Platform: "zend",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	body := strings.NewReader(`["message 1", "message 2", "message 3"]`)
	body2 := strings.NewReader(`["message 4", "message 5"]`)
	request, err := http.NewRequest("POST", "/apps/myapp/log/?:app=myapp", body)
	c.Assert(err, gocheck.IsNil)
	withSourceRequest, err := http.NewRequest("POST", "/apps/myapp/log/?:app=myapp&source=mysource", body2)
	c.Assert(err, gocheck.IsNil)

	recorder := httptest.NewRecorder()
	err = addLog(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)

	recorder = httptest.NewRecorder()
	err = addLog(recorder, withSourceRequest, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)

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
	c.Assert(err, gocheck.IsNil)
	got := make([]string, len(logs))
	gotSource := make([]string, len(logs))

	sort.Sort(LogList(logs))
	for i, l := range logs {
		got[i] = l.Message
		gotSource[i] = l.Source
	}
	c.Assert(got, gocheck.DeepEquals, want)
	c.Assert(gotSource, gocheck.DeepEquals, wantSource)
}

func (s *S) TestPlatformList(c *gocheck.C) {
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
	c.Assert(err, gocheck.IsNil)
	var got []app.Platform
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.DeepEquals, want)
	action := testing.Action{Action: "platform-list", User: s.user.Email}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestgetAppOrErrorWhenUserIsAdmin(c *gocheck.C) {
	a := app.App{Name: "testApp", Teams: []string{"notAdmin", "noSuperUser"}}
	err := s.conn.Apps().Insert(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	expected, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	app, err := getApp(a.Name, s.adminuser)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app, gocheck.DeepEquals, *expected)
}

func (s *S) TestSwap(c *gocheck.C) {
	app1 := app.App{Name: "app1", Teams: []string{s.team.Name}}
	err := s.conn.Apps().Insert(&app1)
	c.Assert(err, gocheck.IsNil)
	err = s.provisioner.Provision(&app1)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app1.Name})
	app2 := app.App{Name: "app2", Teams: []string{s.team.Name}}
	err = s.conn.Apps().Insert(&app2)
	c.Assert(err, gocheck.IsNil)
	err = s.provisioner.Provision(&app2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app2.Name})
	request, _ := http.NewRequest("PUT", "/swap?app1=app1&app2=app2", nil)
	recorder := httptest.NewRecorder()
	err = swap(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	action := testing.Action{Action: "swap", User: s.user.Email, Extra: []interface{}{"app1", "app2"}}
	c.Assert(action, testing.IsRecorded)
	var dbApp app.App
	err = s.conn.Apps().Find(bson.M{"name": app1.Name}).One(&dbApp)
	c.Assert(err, gocheck.IsNil)
	c.Assert(dbApp.Lock, gocheck.Equals, app.AppLock{})
	err = s.conn.Apps().Find(bson.M{"name": app2.Name}).One(&dbApp)
	c.Assert(err, gocheck.IsNil)
	c.Assert(dbApp.Lock, gocheck.Equals, app.AppLock{})
}

func (s *S) TestSwapApp1Locked(c *gocheck.C) {
	app1 := app.App{Name: "app1", Teams: []string{s.team.Name}, Lock: app.AppLock{
		Locked: true, Reason: "/test", Owner: "x",
	}}
	err := s.conn.Apps().Insert(&app1)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app1.Name})
	app2 := app.App{Name: "app2", Teams: []string{s.team.Name}}
	err = s.conn.Apps().Insert(&app2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app2.Name})
	request, _ := http.NewRequest("PUT", "/swap?app1=app1&app2=app2", nil)
	recorder := httptest.NewRecorder()
	err = swap(recorder, request, s.token)
	c.Assert(err, gocheck.ErrorMatches, "app1: App locked by x, running /test. Acquired in .*")
}

func (s *S) TestSwapApp2Locked(c *gocheck.C) {
	app1 := app.App{Name: "app1", Teams: []string{s.team.Name}}
	err := s.conn.Apps().Insert(&app1)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app1.Name})
	app2 := app.App{Name: "app2", Teams: []string{s.team.Name}, Lock: app.AppLock{
		Locked: true, Reason: "/test", Owner: "x",
	}}
	err = s.conn.Apps().Insert(&app2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app2.Name})
	request, _ := http.NewRequest("PUT", "/swap?app1=app1&app2=app2", nil)
	recorder := httptest.NewRecorder()
	err = swap(recorder, request, s.token)
	c.Assert(err, gocheck.ErrorMatches, "app2: App locked by x, running /test. Acquired in .*")
}

func (s *S) TestSwapIncompatibleAppsShouldNotSwap(c *gocheck.C) {
	app1 := app.App{Name: "app1", Teams: []string{s.team.Name}, Platform: "x"}
	err := s.conn.Apps().Insert(&app1)
	c.Assert(err, gocheck.IsNil)
	err = s.provisioner.Provision(&app1)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app1.Name})
	app2 := app.App{Name: "app2", Teams: []string{s.team.Name}, Platform: "y"}
	err = s.conn.Apps().Insert(&app2)
	c.Assert(err, gocheck.IsNil)
	err = s.provisioner.Provision(&app2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app2.Name})
	request, _ := http.NewRequest("PUT", "/swap?app1=app1&app2=app2", nil)
	recorder := httptest.NewRecorder()
	err = swap(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.DeepEquals, app.ErrAppNotEqual)
}

func (s *S) TestSwapIncompatibleAppsForceSwap(c *gocheck.C) {
	app1 := app.App{Name: "app1", Teams: []string{s.team.Name}, Platform: "x"}
	err := s.conn.Apps().Insert(&app1)
	c.Assert(err, gocheck.IsNil)
	err = s.provisioner.Provision(&app1)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app1.Name})
	app2 := app.App{Name: "app2", Teams: []string{s.team.Name}, Platform: "y"}
	err = s.conn.Apps().Insert(&app2)
	c.Assert(err, gocheck.IsNil)
	err = s.provisioner.Provision(&app2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app2.Name})
	request, _ := http.NewRequest("PUT", "/swap?app1=app1&app2=app2&force=true", nil)
	recorder := httptest.NewRecorder()
	err = swap(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestStartHandler(c *gocheck.C) {
	s.provisioner.PrepareOutput(nil) // loadHooks
	s.provisioner.PrepareOutput([]byte("started"))
	a := app.App{
		Name:  "stress",
		Teams: []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	url := fmt.Sprintf("/apps/%s/start?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = start(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	starts := s.provisioner.Starts(&a)
	c.Assert(starts, gocheck.Equals, 1)
	action := testing.Action{
		Action: "start",
		User:   s.user.Email,
		Extra:  []interface{}{a.Name},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestStopHandler(c *gocheck.C) {
	a := app.App{
		Name:  "stress",
		Teams: []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	url := fmt.Sprintf("/apps/%s/stop?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = stop(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	stops := s.provisioner.Stops(&a)
	c.Assert(stops, gocheck.Equals, 1)
	action := testing.Action{
		Action: "stop",
		User:   s.user.Email,
		Extra:  []interface{}{a.Name},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestForceDeleteLock(c *gocheck.C) {
	a := app.App{
		Name: "locked",
		Lock: app.AppLock{Locked: true},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/apps/locked/lock", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusNoContent)
	c.Assert(recorder.Body.String(), gocheck.Equals, "")
	var dbApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "locked"}).One(&dbApp)
	c.Assert(err, gocheck.IsNil)
	c.Assert(dbApp.Lock.Locked, gocheck.Equals, false)
}

func (s *S) TestForceDeleteLockOnlyAdmins(c *gocheck.C) {
	a := app.App{
		Name:  "locked",
		Lock:  app.AppLock{Locked: true},
		Teams: []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/apps/locked/lock", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), gocheck.Equals, "You must be an admin\n")
	var dbApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "locked"}).One(&dbApp)
	c.Assert(err, gocheck.IsNil)
	c.Assert(dbApp.Lock.Locked, gocheck.Equals, true)
}

func (s *S) TestRegisterUnit(c *gocheck.C) {
	a := app.App{
		Name:     "myappx",
		Platform: "python",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"MY_VAR_1": {Name: "MY_VAR_1", Value: "value1", Public: true},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 1, nil)
	units := a.Units()
	oldIp := units[0].Ip
	body := strings.NewReader("hostname=" + units[0].Name)
	request, err := http.NewRequest("POST", "/apps/myappx/units/register", body)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	expected := []map[string]interface{}{{
		"name":   "MY_VAR_1",
		"value":  "value1",
		"public": true,
	}}
	result := []map[string]interface{}{}
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.DeepEquals, expected)
	units = a.Units()
	c.Assert(units[0].Ip, gocheck.Equals, oldIp+"-updated")
}

func (s *S) TestRegisterUnitInvalidUnit(c *gocheck.C) {
	a := app.App{
		Name:     "myappx",
		Platform: "python",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"MY_VAR_1": {Name: "MY_VAR_1", Value: "value1", Public: true},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.provisioner.Destroy(&a)
	body := strings.NewReader("hostname=invalid-unit-host")
	request, err := http.NewRequest("POST", "/apps/myappx/units/register", body)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), gocheck.Equals, "unit not found\n")
}

func (s *S) TestSetTeamOwnerWithoutTeam(c *gocheck.C) {
	a := app.App{
		Name:     "myappx",
		Platform: "python",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.provisioner.Destroy(&a)
	request, err := http.NewRequest("POST", "/apps/myappx/team-owner", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), gocheck.Equals, "You must provide a team name.\n")
}

func (s *S) TestSetTeamOwner(c *gocheck.C) {
	a := app.App{
		Name:      "myappx",
		Platform:  "python",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.provisioner.Destroy(&a)
	team := &auth.Team{Name: "newowner", Users: []string{s.user.Email}}
	err = s.conn.Teams().Insert(team)
	defer s.conn.Teams().Remove(bson.M{"_id": team.Name})
	c.Assert(err, gocheck.IsNil)
	body := strings.NewReader(team.Name)
	req, err := http.NewRequest("POST", "/apps/myappx/team-owner", body)
	c.Assert(err, gocheck.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, gocheck.Equals, http.StatusOK)
	s.conn.Apps().Find(bson.M{"name": "myappx"}).One(&a)
	c.Assert(a.TeamOwner, gocheck.Equals, team.Name)
}

func (s *S) TestSetTeamOwnerToUserWhoCantBeOwner(c *gocheck.C) {
	a := app.App{
		Name:      "myappx",
		Platform:  "python",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.provisioner.Destroy(&a)
	user := &auth.User{Email: "teste@thewho.com", Password: "123456", Quota: quota.Unlimited}
	_, err = nativeScheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	team := &auth.Team{Name: "newowner", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	defer s.conn.Teams().Remove(bson.M{"_id": team.Name})
	c.Assert(err, gocheck.IsNil)
	token, err := nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	body := strings.NewReader(team.Name)
	req, err := http.NewRequest("POST", "/apps/myappx/team-owner", body)
	c.Assert(err, gocheck.IsNil)
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	rec := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, gocheck.Equals, http.StatusForbidden)
	s.conn.Apps().Find(bson.M{"name": "myappx"}).One(&a)
	c.Assert(a.TeamOwner, gocheck.Equals, s.team.Name)
}

func (s *S) TestSetTeamOwnerAsAdmin(c *gocheck.C) {
	a := app.App{
		Name:      "myappx",
		Platform:  "python",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.provisioner.Destroy(&a)
	user := &auth.User{Email: "teste@thewho2.com", Password: "123456", Quota: quota.Unlimited}
	_, err = nativeScheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	team := &auth.Team{Name: "newowner", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	defer s.conn.Teams().Remove(bson.M{"_id": team.Name})
	c.Assert(err, gocheck.IsNil)
	token, err := nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	body := strings.NewReader(team.Name)
	req, err := http.NewRequest("POST", "/apps/myappx/team-owner", body)
	c.Assert(err, gocheck.IsNil)
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	rec := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, gocheck.Equals, http.StatusForbidden)
	s.conn.Apps().Find(bson.M{"name": "myappx"}).One(&a)
	c.Assert(a.TeamOwner, gocheck.Equals, s.team.Name)
	req, err = http.NewRequest("POST", "/apps/myappx/team-owner", body)
	c.Assert(err, gocheck.IsNil)
	req.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	rec = httptest.NewRecorder()
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, gocheck.Equals, http.StatusOK)
	s.conn.Apps().Find(bson.M{"name": "myappx"}).One(&a)
	c.Assert(a.TeamOwner, gocheck.Equals, team.Name)
}

func (s *S) TestSetTeamOwnerSetNewTeamToAppAddThatTeamToAppTeamList(c *gocheck.C) {
	a := app.App{
		Name:      "myappx",
		Platform:  "python",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.provisioner.Destroy(&a)
	user := &auth.User{Email: "teste@thewho3.com", Password: "123456", Quota: quota.Unlimited}
	_, err = nativeScheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	team := &auth.Team{Name: "newowner", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	defer s.conn.Teams().Remove(bson.M{"_id": team.Name})
	c.Assert(err, gocheck.IsNil)
	body := strings.NewReader(team.Name)
	req, err := http.NewRequest("POST", "/apps/myappx/team-owner", body)
	c.Assert(err, gocheck.IsNil)
	req.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	rec := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, gocheck.Equals, http.StatusOK)
	s.conn.Apps().Find(bson.M{"name": "myappx"}).One(&a)
	c.Assert([]string{team.Name, s.team.Name}, gocheck.DeepEquals, a.Teams)
}

func (s *S) TestSaveAppCustomData(c *gocheck.C) {
	a := app.App{
		Name:  "mycustomdataapp",
		Teams: []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	body := strings.NewReader(`{"a": "b", "c": {"d": [1, 2]}, "f": [{"a": "b"}]}`)
	req, err := http.NewRequest("POST", "/apps/mycustomdataapp/customdata", body)
	c.Assert(err, gocheck.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, gocheck.Equals, http.StatusOK)
	dbApp, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	expected := map[string]interface{}{
		"a": "b",
		"c": map[string]interface{}{"d": []interface{}{float64(1), float64(2)}},
		"f": []interface{}{map[string]interface{}{"a": "b"}},
	}
	c.Assert(dbApp.CustomData, gocheck.DeepEquals, expected)
}
