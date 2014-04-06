// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/testing"
	"github.com/tsuru/config"
	"io"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

func (s *S) TestAppIsAvailableHandlerShouldReturnErrorWhenAppStatusIsnotStarted(c *gocheck.C) {
	a := app.App{
		Name:     "someapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Units:    []app.Unit{{Name: "someapp/0", Type: "django", State: provision.StatusBuilding.String()}},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
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
		Units:    []app.Unit{{Name: "someapp/0", Type: "django", State: provision.StatusStarted.String()}},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/repository/clone?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = appIsAvailable(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
}

func (s *S) TestDeployHandler(c *gocheck.C) {
	a := app.App{
		Name:     "otherapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Units:    []app.Unit{{Name: "i-0800", State: "started"}},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	url := fmt.Sprintf("/apps/%s/repository/clone?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("version=a345f3e&user=fulano"))
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = deploy(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), gocheck.Equals, "text")
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert(string(b), gocheck.Equals, "Deploy called")
	c.Assert(s.provisioner.Version(&a), gocheck.Equals, "a345f3e")
}

func (s *S) TestDeployWithCommit(c *gocheck.C) {
	a := app.App{
		Name:     "otherapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Units:    []app.Unit{{Name: "i-0800", State: "started"}},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	url := fmt.Sprintf("/apps/%s/repository/clone?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("version=a345f3e&user=fulano&commit=123"))
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = deploy(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), gocheck.Equals, "text")
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert(string(b), gocheck.Equals, "Deploy called")
	deploys, err := s.conn.Deploys().Find(bson.M{"commit": "123"}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(deploys, gocheck.Equals, 1)
	c.Assert(s.provisioner.Version(&a), gocheck.Equals, "a345f3e")
}

func (s *S) TestCloneRepositoryShouldIncrementDeployNumberOnApp(c *gocheck.C) {
	a := app.App{
		Name:     "otherapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Units:    []app.Unit{{Name: "i-0800", State: "started"}},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	url := fmt.Sprintf("/apps/%s/repository/clone?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("version=a345f3e"))
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = deploy(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(a.Deploys, gocheck.Equals, uint(1))
	var result map[string]interface{}
	s.conn.Deploys().Find(bson.M{"app": a.Name}).One(&result)
	c.Assert(result["app"], gocheck.Equals, a.Name)
	now := time.Now()
	diff := now.Sub(result["timestamp"].(time.Time))
	c.Assert(diff < 60*time.Second, gocheck.Equals, true)
}

func (s *S) TestCloneRepositoryShouldReturnNotFoundWhenAppDoesNotExist(c *gocheck.C) {
	request, err := http.NewRequest("POST", "/apps/abc/repository/clone?:appname=abc", strings.NewReader("version=abcdef"))
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = deploy(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^App abc not found.$")
}

func (s *S) TestCloneRepositoryWithoutVersion(c *gocheck.C) {
	request, err := http.NewRequest("POST", "/apps/abc/repository/clone?:appname=abc", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = deploy(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e.Message, gocheck.Equals, "Missing parameter version")
}

func (s *S) TestAppList(c *gocheck.C) {
	app1 := app.App{
		Name:  "app1",
		Teams: []string{s.team.Name},
		Units: []app.Unit{{Name: "app1/0", Ip: "10.10.10.10"}},
	}
	err := s.conn.Apps().Insert(app1)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app1.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": app1.Name})
	app2 := app.App{
		Name:  "app2",
		Teams: []string{s.team.Name},
		Units: []app.Unit{{Name: "app2/0"}},
	}
	err = s.conn.Apps().Insert(app2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app2.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": app2.Name})
	expected := []app.App{app1, app2}
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
	c.Assert(len(apps), gocheck.Equals, len(expected))
	for i, app := range apps {
		c.Assert(app.Name, gocheck.DeepEquals, expected[i].Name)
		if app.Units[0].Ip != "" {
			c.Assert(app.Units[0].Ip, gocheck.Equals, "10.10.10.10")
		}
	}
	action := testing.Action{Action: "app-list", User: s.user.Email}
	c.Assert(action, testing.IsRecorded)
}

// Issue #52.
func (s *S) TestAppListShouldListAllAppsOfAllTeamsThatTheUserIsAMember(c *gocheck.C) {
	u := auth.User{Email: "passing-by@angra.com", Password: "123456"}
	u.HashPassword()
	err := s.conn.Users().Insert(u)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	token, err := u.CreateToken("123456")
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.Token})
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
	defer s.conn.Logs().Remove(bson.M{"appname": app1.Name})
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
		Units: []app.Unit{
			{Ip: "10.10.10.10", Machine: 1},
		},
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
	defer s.conn.Logs().Remove(bson.M{"appname": myApp.Name})
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
	defer s.conn.Logs().Remove(bson.M{"appname": expectedApp.Name})
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
	defer s.conn.Logs().Remove(bson.M{"appname": expectedApp.Name})
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
	config.Set("docker:allow-memory-set", true)
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
	data := `{"name":"someapp","platform":"zend","memory":"10"}`
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
	c.Assert(s.provisioner.GetUnits(&gotApp), gocheck.HasLen, 1)
	action := testing.Action{
		Action: "create-app",
		User:   s.user.Email,
		Extra:  []interface{}{"name=someapp", "platform=zend", "memory=10"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestCreateAppTeamOwner(c *gocheck.C) {
	config.Set("docker:allow-memory-set", true)
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
	data := `{"name":"someapp","platform":"zend","memory":"1","teamOwner":"tsuruteam"}`
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
	c.Assert(s.provisioner.GetUnits(&gotApp), gocheck.HasLen, 1)
	action := testing.Action{
		Action: "create-app",
		User:   s.user.Email,
		Extra:  []interface{}{"name=someapp", "platform=zend", "memory=1"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestCreateAppTwoTeamOwner(c *gocheck.C) {
	config.Set("docker:allow-memory-set", true)
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	team := auth.Team{Name: "tsurutwo", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(team)
	c.Check(err, gocheck.IsNil)
	defer s.conn.Teams().RemoveId(team.Name)
	data := `{"name":"someapp","platform":"zend","memory":"1"}`
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = createApp(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestCreateAppMemorySetNowAllowed(c *gocheck.C) {
	config.Set("docker:allow-memory-set", false)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	b := strings.NewReader(`{"name":"someapp","platform":"zend","memory":"10"}`)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = createApp(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e.Message, gocheck.Matches, "^.*Memory setting not allowed.$")
}

func (s *S) TestCreateAppInvalidMemorySize(c *gocheck.C) {
	config.Set("docker:allow-memory-set", true)
	config.Set("docker:max-allowed-memory", 1)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	b := strings.NewReader(`{"name":"someapp","platform":"zend","memory":"10"}`)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = createApp(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e.Message, gocheck.Matches, "^.*Invalid memory size. You cannot request more than.*")
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
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	token, err := u.CreateToken("123456")
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.Token})
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
		Name:  "plainsofdawn",
		Units: []app.Unit{{Machine: 1}},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
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
	c.Assert(app.Units, gocheck.HasLen, 3)
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
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
		Units: []app.Unit{
			{Name: "velha/0"}, {Name: "velha/1"}, {Name: "velha/2"},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 3)
	body := strings.NewReader("2")
	request, err := http.NewRequest("DELETE", "/apps/velha/units?:app=velha", body)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUnits(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	app, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Units, gocheck.HasLen, 1)
	c.Assert(app.Units[0].Name, gocheck.Equals, "velha/2")
	c.Assert(s.provisioner.GetUnits(app), gocheck.HasLen, 2)
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/%s?:app=%s&:team=%s", a.Name, s.team.Name, a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = grantAppAccess(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
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
	request, err := http.NewRequest("PUT", "/apps/a/b?:app=a&:team=b", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = grantAppAccess(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^App a not found.$")
}

func (s *S) TestGrantAccessToTeamReturn403IfTheGivenUserDoesNotHasAccessToTheApp(c *gocheck.C) {
	a := app.App{
		Name:     "itshard",
		Platform: "django",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/%s?:app=%s&:team=%s", a.Name, s.team.Name, a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = grantAppAccess(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e, gocheck.ErrorMatches, "^User does not have access to this app$")
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/a?:app=%s&:team=a", a.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = grantAppAccess(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^Team not found$")
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/%s?:app=%s&:team=%s", a.Name, s.team.Name, a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = grantAppAccess(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusConflict)
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/%s?:app=%s&:team=%s", a.Name, s.team.Name, a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = grantAppAccess(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/%s?:app=%s&:team=%s", a.Name, s.team.Name, a.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = revokeAppAccess(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
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
	request, err := http.NewRequest("DELETE", "/apps/a/b?:app=a&:team=b", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = revokeAppAccess(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^App a not found.$")
}

func (s *S) TestRevokeAccessFromTeamReturn401IfTheGivenUserDoesNotHavePermissionInTheApp(c *gocheck.C) {
	a := app.App{
		Name:     "itshard",
		Platform: "django",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/%s?:app=%s&:team=%s", a.Name, s.team.Name, a.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = revokeAppAccess(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e, gocheck.ErrorMatches, "^User does not have access to this app$")
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/x?:app=%s&:team=x", a.Name, a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = revokeAppAccess(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^Team not found$")
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/%s?:app=%s&:team=%s", a.Name, t.Name, a.Name, t.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = revokeAppAccess(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/%s?:app=%s&:team=%s", a.Name, s.team.Name, a.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = revokeAppAccess(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e, gocheck.ErrorMatches, "^You can not revoke the access from this team, because it is the unique team with access to the app, and an app can not be orphaned$")
}

func (s *S) TestRevokeAccessFromTeamRemovesRepositoryFromGandalf(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	u := auth.User{Email: "again@live.com", Password: "123456"}
	u.HashPassword()
	err := s.conn.Users().Insert(u)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	token, err := u.CreateToken("123456")
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.Token})
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/%s?:app=%s&:team=%s", a.Name, s.team.Name, a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = grantAppAccess(recorder, request, token)
	c.Assert(err, gocheck.IsNil)
	request, err = http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder = httptest.NewRecorder()
	err = revokeAppAccess(recorder, request, token)
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/%s?:app=%s&:team=%s", a.Name, t.Name, a.Name, t.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = grantAppAccess(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	request, err = http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder = httptest.NewRecorder()
	err = revokeAppAccess(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/%s?:app=%s&:team=%s", a.Name, t.Name, a.Name, t.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = grantAppAccess(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	request, err = http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder = httptest.NewRecorder()
	err = revokeAppAccess(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(h.url, gocheck.HasLen, 1)
	c.Assert(h.url[0], gocheck.Equals, "/repository/grant")
}

func (s *S) TestRunOnceHandler(c *gocheck.C) {
	s.provisioner.PrepareOutput([]byte("lots of files"))
	a := app.App{
		Name:     "secrets",
		Platform: "arch enemy",
		Teams:    []string{s.team.Name},
		Units: []app.Unit{
			{Name: "i-0800", State: "started", Machine: 10},
			{Name: "i-0801", State: "started", Machine: 11},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/run/?:app=%s&once=true", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("ls"))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = runCommand(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Body.String(), gocheck.Equals, "lots of files")
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
	s.provisioner.PrepareOutput([]byte("lots of files"))
	a := app.App{
		Name:     "secrets",
		Platform: "arch enemy",
		Teams:    []string{s.team.Name},
		Units:    []app.Unit{{Name: "i-0800", State: "started", Machine: 10}},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/run/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("ls"))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = runCommand(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Body.String(), gocheck.Equals, "lots of files")
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
		Units:    []app.Unit{{Name: "i-0800", State: "started"}},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/run/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("ls"))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = runCommand(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "something went wrong")
	c.Assert(recorder.Body.String(), gocheck.Equals, "failure output")
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
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

func (s *S) TestSetEnvHandlerShouldSetAPublicEnvironmentVariableInTheApp(c *gocheck.C) {
	a := app.App{
		Name:  "black-dog",
		Teams: []string{s.team.Name},
		Units: []app.Unit{{Machine: 1}},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/env?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader(`{"DATABASE_HOST":"localhost"}`))
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
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
	a := app.App{
		Name:  "vigil",
		Teams: []string{s.team.Name},
		Units: []app.Unit{{Machine: 1}},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/env?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"DATABASE_HOST": "localhost", "DATABASE_USER": "root"}`)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
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
		Units: []app.Unit{{Machine: 1}},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/env?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader(`{"DATABASE_HOST":"http://foo.com:8080"}`))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader(`{"DATABASE_HOST":"localhost"}`))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	expected := a.Env
	delete(expected, "DATABASE_HOST")
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("DELETE", url, strings.NewReader(`["DATABASE_HOST"]`))
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("DELETE", url, strings.NewReader(`["DATABASE_HOST", "DATABASE_USER"]`))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`["DATABASE_HOST", "DATABASE_USER", "DATABASE_PASSWORD"]`)
	request, err := http.NewRequest("DELETE", url, b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/env/?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader(`["DATABASE_HOST"]`))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = unsetEnv(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
}

func (s *S) TestSetCNameHandler(c *gocheck.C) {
	a := app.App{Name: "leper", Teams: []string{s.team.Name}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"cname":"leper.secretcompany.com"}`)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = setCName(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	app, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.CName, gocheck.Equals, "leper.secretcompany.com")
	action := testing.Action{
		Action: "set-cname",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + app.Name, "cname=leper.secretcompany.com"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestSetCNameHandlerAcceptsEmptyCName(c *gocheck.C) {
	a := app.App{Name: "leper", Teams: []string{s.team.Name}, CName: "leper.secretcompany.com"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"cname":""}`)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = setCName(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	app, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.CName, gocheck.Equals, "")
}

func (s *S) TestSetCNameHandlerReturnsInternalErrorIfItFailsToReadTheBody(c *gocheck.C) {
	b := s.getTestData("bodyToBeClosed.txt")
	request, err := http.NewRequest("POST", "/apps/unkown/cname?:app=unknown", b)
	c.Assert(err, gocheck.IsNil)
	request.Body.Close()
	recorder := httptest.NewRecorder()
	err = setCName(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestSetCNameHandlerReturnsBadRequestWhenCNameIsMissingFromTheBody(c *gocheck.C) {
	bodies := []io.Reader{nil, strings.NewReader(`{}`), strings.NewReader(`{"name":"something"}`)}
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

func (s *S) TestSetCNameHandlerInvalidJSON(c *gocheck.C) {
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

func (s *S) TestSetCNameHandlerUnknownApp(c *gocheck.C) {
	b := strings.NewReader(`{"cname": "leper.secretcompany.com"}`)
	request, err := http.NewRequest("POST", "/apps/unknown/cname?:app=unknown", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = setCName(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
}

func (s *S) TestSetCNameHandlerUserWithoutAccessToTheApp(c *gocheck.C) {
	a := app.App{
		Name:     "lost",
		Platform: "vougan",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"cname": "lost.secretcompany.com"}`)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = setCName(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
}

func (s *S) TestSetCNameHandlerInvalidCName(c *gocheck.C) {
	a := app.App{Name: "leper", Teams: []string{s.team.Name}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	b := strings.NewReader(`{"cname": ".leper.secretcompany.com"}`)
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

func (s *S) TestUnsetCNameHandler(c *gocheck.C) {
	a := app.App{Name: "leper", Teams: []string{s.team.Name}, CName: "foo.bar.com"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = unsetCName(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	app, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.CName, gocheck.Equals, "")
	action := testing.Action{
		Action: "unset-cname",
		User:   s.user.Email,
		Extra:  []interface{}{"app=" + app.Name},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestUnsetCNameHandlerUnknownApp(c *gocheck.C) {
	request, err := http.NewRequest("DELETE", "/apps/unknown/cname?:app=unknown", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = unsetCName(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
}

func (s *S) TestUnsetCNameHandlerUserWithoutAccessToTheApp(c *gocheck.C) {
	a := app.App{
		Name:     "lost",
		Platform: "vougan",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/apps/%s/cname?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
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

func (s *S) TestAppLogShouldHaveContentType(c *gocheck.C) {
	a := app.App{
		Name:     "lost",
		Platform: "vougan",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	for i := 0; i < 15; i++ {
		a.Log(strconv.Itoa(i), "source")
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	a.Log("mars log", "mars")
	a.Log("earth log", "earth")
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

func (s *S) TestAppLogSelectByLinesShouldReturnTheLastestEntries(c *gocheck.C) {
	a := app.App{
		Name:     "lost",
		Platform: "vougan",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	now := time.Now()
	coll := s.conn.Logs()
	defer coll.Remove(bson.M{"appname": a.Name})
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
	defer s.conn.Logs().Remove(bson.M{"appname": app1.Name})
	app1.Log("app1 log", "source")
	app2 := app.App{
		Name:     "app2",
		Platform: "vougan",
		Teams:    []string{s.team.Name},
	}
	err = s.conn.Apps().Insert(app2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app2.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": app2.Name})
	app2.Log("app2 log", "source")
	app3 := app.App{
		Name:     "app3",
		Platform: "vougan",
		Teams:    []string{s.team.Name},
	}
	err = s.conn.Apps().Insert(app3)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app3.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": app3.Name})
	app3.Log("app3 log", "tsuru")
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
		Units: []app.Unit{{Ip: "127.0.0.1", Machine: 1}},
		Env:   map[string]bind.EnvVar{},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
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
		Units: []app.Unit{{Ip: "127.0.0.1", Machine: 1}},
		Env:   map[string]bind.EnvVar{},
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, a.Name, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
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
	h := testHandler{}
	gts := testing.StartGandalfTestServer(&h)
	defer gts.Close()
	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/hostname/127.0.0.1" {
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
		Units:    []app.Unit{{Machine: 1}},
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, gocheck.IsNil)
	otherApp, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	defer app.Delete(&a)
	otherApp.Env["DATABASE_HOST"] = bind.EnvVar{
		Name:         "DATABASE_HOST",
		Value:        "arrea",
		Public:       false,
		InstanceName: instance.Name,
	}
	otherApp.Env["MY_VAR"] = bind.EnvVar{Name: "MY_VAR", Value: "123"}
	otherApp.Units = []app.Unit{{Ip: "127.0.0.1", Machine: 1}}
	err = s.conn.Apps().Update(bson.M{"name": otherApp.Name}, otherApp)
	c.Assert(err, gocheck.IsNil)
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, otherApp.Name,
		instance.Name, otherApp.Name)
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
		Extra:  []interface{}{"instance=" + instance.Name, "app=" + otherApp.Name},
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
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
		Units: []app.Unit{{Name: "i-0800", State: "started"}},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	url := fmt.Sprintf("/apps/%s/restart?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = restart(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	result := strings.Replace(recorder.Body.String(), "\n", "#", -1)
	c.Assert(result, gocheck.Matches, ".*# ---> Restarting your app#.*")
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
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
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
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
	logs, err := a.LastLogs(5, "")
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
	admin := auth.User{Email: "superuser@gmail.com", Password: "123"}
	err := s.conn.Users().Insert(&admin)
	c.Assert(err, gocheck.IsNil)
	adminTeamName, err := config.GetString("admin-team")
	c.Assert(err, gocheck.IsNil)
	adminTeam := auth.Team{Name: adminTeamName, Users: []string{admin.Email}}
	err = s.conn.Teams().Insert(&adminTeam)
	c.Assert(err, gocheck.IsNil)
	a := app.App{Name: "testApp", Teams: []string{"notAdmin", "noSuperUser"}}
	err = s.conn.Apps().Insert(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	defer func(admin auth.User, adminTeam auth.Team) {
		err := s.conn.Teams().RemoveId(adminTeam.Name)
		c.Assert(err, gocheck.IsNil)
		err = s.conn.Users().Remove(bson.M{"email": admin.Email})
		c.Assert(err, gocheck.IsNil)
	}(admin, adminTeam)
	expected, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	app, err := getApp(a.Name, &admin)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app, gocheck.DeepEquals, *expected)
}

func (s *S) TestSwap(c *gocheck.C) {
	app1 := app.App{Name: "app1", Teams: []string{s.team.Name}}
	err := s.conn.Apps().Insert(&app1)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().RemoveId(&app1.Name)
	app2 := app.App{Name: "app2", Teams: []string{s.team.Name}}
	err = s.conn.Apps().Insert(&app2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().RemoveId(&app2.Name)
	request, _ := http.NewRequest("PUT", "/swap?app1=app1&app2=app2", nil)
	recorder := httptest.NewRecorder()
	err = swap(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	action := testing.Action{Action: "swap", User: s.user.Email, Extra: []interface{}{"app1", "app2"}}
	c.Assert(action, testing.IsRecorded)
}

func (s *S) TestStartHandler(c *gocheck.C) {
	s.provisioner.PrepareOutput(nil) // loadHooks
	s.provisioner.PrepareOutput([]byte("started"))
	a := app.App{
		Name:  "stress",
		Teams: []string{s.team.Name},
		Units: []app.Unit{{Name: "i-0800", State: "started"}},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
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
