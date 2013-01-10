// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/repository"
	"github.com/globocom/tsuru/service"
	"io"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	stdlog "log"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type testHandler struct {
	body    [][]byte
	method  []string
	url     []string
	content string
	header  []http.Header
}

func (h *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.method = append(h.method, r.Method)
	h.url = append(h.url, r.URL.String())
	b, _ := ioutil.ReadAll(r.Body)
	h.body = append(h.body, b)
	h.header = append(h.header, r.Header)
	w.Write([]byte(h.content))
}

type testBadHandler struct{}

func (h *testBadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "some error", http.StatusInternalServerError)
}

func (s *S) TestAppIsAvaliableHandlerShouldReturnErrorWhenAppStatusIsnotStarted(c *C) {
	a := app.App{
		Name:      "someapp",
		Framework: "python",
		Teams:     []string{s.team.Name},
		Units:     []app.Unit{{Name: "someapp/0", Type: "django", State: string(provision.StatusPending)}},
		State:     string(provision.StatusPending),
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/avaliable?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AppIsAvaliableHandler(recorder, request)
	c.Assert(err, NotNil)
}

func (s *S) TestAppIsAvaliableHandlerShouldReturn200WhenAppUnitStatusIsStarted(c *C) {
	a := app.App{
		Name:      "someapp",
		Framework: "python",
		Teams:     []string{s.team.Name},
		Units:     []app.Unit{{Name: "someapp/0", Type: "django", State: string(provision.StatusStarted)}},
		State:     string(provision.StatusStarted),
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/repository/clone?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AppIsAvaliableHandler(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, http.StatusOK)
}

func (s *S) TestCloneRepositoryHandlerShouldAddLogs(c *C) {
	output := `pre-restart:
  - pre.sh
pos-restart:
  - pos.sh
`
	s.provisioner.PrepareOutput(nil)            // clone
	s.provisioner.PrepareOutput(nil)            // install
	s.provisioner.PrepareOutput([]byte(output)) // loadHooks
	s.provisioner.PrepareOutput(nil)            // pre-restart
	s.provisioner.PrepareOutput(nil)            // restart
	s.provisioner.PrepareOutput(nil)            // pos-restart
	a := app.App{
		Name:      "someapp",
		Framework: "django",
		Teams:     []string{s.team.Name},
		State:     string(provision.StatusStarted),
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/repository/clone?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = CloneRepositoryHandler(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, http.StatusOK)
	messages := []string{
		" ---> Tsuru receiving push",
		" ---> Cloning your code in your machines",
		" ---> Installing dependencies",
		" ---> Deploy done!",
	}
	for _, msg := range messages {
		length, err := db.Session.Apps().Find(bson.M{"logs.message": msg}).Count()
		c.Check(err, IsNil)
		c.Check(length, Equals, 1)
	}
}

func (s *S) TestCloneRepositoryHandler(c *C) {
	output := `pre-restart:
  - pre.sh
pos-restart:
  - pos.sh
`
	s.provisioner.PrepareOutput(nil)            // clone
	s.provisioner.PrepareOutput(nil)            // install
	s.provisioner.PrepareOutput([]byte(output)) // loadHooks
	s.provisioner.PrepareOutput(nil)            // pre-restart
	s.provisioner.PrepareOutput(nil)            // restart
	s.provisioner.PrepareOutput(nil)            // pos-restart
	a := app.App{
		Name:      "someapp",
		Framework: "django",
		Teams:     []string{s.team.Name},
		State:     string(provision.StatusStarted),
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/repository/clone?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = CloneRepositoryHandler(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, http.StatusOK)
	regexp := `^# ---> Tsuru receiving push#.*
# ---> Cloning your code in your machines#.*
# ---> Installing dependencies#.*
# ---> Running pre-restart#.*
# ---> Restarting your app#.*
# ---> Running pos-restart#.*
# ---> Deploy done!##$
`
	c.Assert(strings.Replace(recorder.Body.String(), "\n", "#", -1), Matches, strings.Replace(regexp, "\n", "", -1))
	c.Assert(recorder.Header().Get("Content-Type"), Equals, "text")
}

func (s *S) TestCloneRepositoryRunsCloneOrPullThenPreRestartThenRestartThenPosRestartHooksInOrder(c *C) {
	w := new(bytes.Buffer)
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	output := `pre-restart:
  - pre.sh
pos-restart:
  - pos.sh
`
	s.provisioner.PrepareOutput(nil)            // clone
	s.provisioner.PrepareOutput(nil)            // install
	s.provisioner.PrepareOutput([]byte(output)) // loadHooks
	s.provisioner.PrepareOutput(nil)            // pre-restart
	s.provisioner.PrepareOutput(nil)            // restart
	s.provisioner.PrepareOutput(nil)            // pos-restart
	a := app.App{
		Name:      "someapp",
		Framework: "django",
		Teams:     []string{s.team.Name},
		State:     string(provision.StatusStarted),
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/repository/clone?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = CloneRepositoryHandler(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, http.StatusOK)
	str := strings.Replace(w.String(), "\n", "", -1)
	c.Assert(str, Matches, ".*\"git clone\" output.*Installing dependencies.*Restarting your app.*Executing pos-restart hook.*")
}

func (s *S) TestCloneRepositoryShouldReturnNotFoundWhenAppDoesNotExist(c *C) {
	request, err := http.NewRequest("GET", "/apps/abc/repository/clone?:name=abc", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = CloneRepositoryHandler(recorder, request)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^App abc not found.$")
}

func (s *S) TestAppList(c *C) {
	app1 := app.App{
		Name:  "app1",
		Teams: []string{s.team.Name},
		Units: []app.Unit{{Name: "app1/0", Ip: "10.10.10.10"}},
	}
	err := db.Session.Apps().Insert(app1)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app1.Name})
	app2 := app.App{
		Name:  "app2",
		Teams: []string{s.team.Name},
		Units: []app.Unit{{Name: "app2/0"}},
	}
	err = db.Session.Apps().Insert(app2)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app2.Name})
	expected := []app.App{app1, app2}
	request, err := http.NewRequest("GET", "/apps/", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = AppList(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	apps := []app.App{}
	err = json.Unmarshal(body, &apps)
	c.Assert(err, IsNil)
	c.Assert(len(apps), Equals, len(expected))
	for i, app := range apps {
		c.Assert(app.Name, DeepEquals, expected[i].Name)
		if app.Units[0].Ip != "" {
			c.Assert(app.Units[0].Ip, Equals, "10.10.10.10")
		}
	}
}

// Issue #52.
func (s *S) TestAppListShouldListAllAppsOfAllTeamsThatTheUserIsAMember(c *C) {
	u := auth.User{Email: "passing-by@angra.com"}
	team := auth.Team{Name: "angra", Users: []string{s.user.Email, u.Email}}
	err := db.Session.Teams().Insert(team)
	c.Assert(err, IsNil)
	defer db.Session.Teams().Remove(bson.M{"_id": team.Name})
	app1 := app.App{
		Name:  "app1",
		Teams: []string{s.team.Name, "angra"},
	}
	err = db.Session.Apps().Insert(app1)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app1.Name})
	request, err := http.NewRequest("GET", "/apps/", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = AppList(recorder, request, &u)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	var apps []app.App
	err = json.Unmarshal(body, &apps)
	c.Assert(err, IsNil)
	c.Assert(apps[0].Name, Equals, app1.Name)
}

func (s *S) TestListShouldReturnStatusNoContentWhenAppListIsNil(c *C) {
	request, err := http.NewRequest("GET", "/apps/", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = AppList(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, http.StatusNoContent)
}

func (s *S) TestDelete(c *C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	myApp := app.App{
		Name:      "myapptodelete",
		Framework: "django",
		Teams:     []string{s.team.Name},
		Units: []app.Unit{
			{Ip: "10.10.10.10", Machine: 1},
		},
	}
	err := app.CreateApp(&myApp, 1)
	c.Assert(err, IsNil)
	myApp.Get()
	defer myApp.Destroy()
	request, err := http.NewRequest("DELETE", "/apps/"+myApp.Name+"?:name="+myApp.Name, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AppDelete(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, http.StatusOK)
	c.Assert(h.url[1], Equals, "/repository/myapptodelete") // increment the index because of CreateApp action
	c.Assert(h.method[1], Equals, "DELETE")
	c.Assert(string(h.body[1]), Equals, "null")
}

func (s *S) TestDeleteShouldReturnForbiddenIfTheGivenUserDoesNotHaveAccesToTheApp(c *C) {
	myApp := app.App{
		Name:      "MyAppToDelete",
		Framework: "django",
	}
	err := db.Session.Apps().Insert(myApp)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": myApp.Name})
	request, err := http.NewRequest("DELETE", "/apps/"+myApp.Name+"?:name="+myApp.Name, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AppDelete(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^User does not have access to this app$")
}

func (s *S) TestDeleteShouldReturnNotFoundIfTheAppDoesNotExist(c *C) {
	request, err := http.NewRequest("DELETE", "/apps/unkown?:name=unknown", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AppDelete(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestDeleteShouldHandleWithGandalfError(c *C) {
	myApp := app.App{
		Name:      "myapptodelete",
		Framework: "django",
		Teams:     []string{s.team.Name},
		Units: []app.Unit{
			{Ip: "10.10.10.10", Machine: 1},
		},
	}
	err := db.Session.Apps().Insert(myApp)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": myApp.Name})
	request, err := http.NewRequest("DELETE", "/apps/"+myApp.Name+"?:name="+myApp.Name, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	h := testBadHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	err = AppDelete(recorder, request, s.user)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Could not remove app's repository at git server. Aborting...")
}

func (s *S) TestDeleteReturnsErrorIfAppDestroyFails(c *C) {
	s.provisioner.PrepareFailure("Destroy", &errors.Http{Code: 500, Message: "fatal"})
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	myApp := app.App{
		Name:      "MyAppToDelete",
		Framework: "django",
		Teams:     []string{s.team.Name},
		Units:     []app.Unit{{Name: "i-0800", Machine: 1}},
		State:     string(provision.StatusStarted),
	}
	err := db.Session.Apps().Insert(myApp)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": myApp.Name})
	request, err := http.NewRequest("DELETE", "/apps/"+myApp.Name+"?:name="+myApp.Name, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AppDelete(recorder, request, s.user)
	c.Assert(err, NotNil)
}

func (s *S) TestAppInfo(c *C) {
	expectedApp := app.App{
		Name:      "NewApp",
		Framework: "django",
		Teams:     []string{s.team.Name},
	}
	err := db.Session.Apps().Insert(expectedApp)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": expectedApp.Name})
	var myApp map[string]interface{}
	request, err := http.NewRequest("GET", "/apps/"+expectedApp.Name+"?:name="+expectedApp.Name, nil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	c.Assert(err, IsNil)
	err = AppInfo(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	err = json.Unmarshal(body, &myApp)
	c.Assert(err, IsNil)
	c.Assert(myApp["Name"], Equals, expectedApp.Name)
	c.Assert(myApp["Repository"], Equals, repository.GetUrl(expectedApp.Name))
}

func (s *S) TestAppInfoReturnsForbiddenWhenTheUserDoesNotHaveAccessToTheApp(c *C) {
	expectedApp := app.App{
		Name:      "NewApp",
		Framework: "django",
	}
	err := db.Session.Apps().Insert(expectedApp)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(expectedApp)
	request, err := http.NewRequest("GET", "/apps/"+expectedApp.Name+"?:name="+expectedApp.Name, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AppInfo(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^User does not have access to this app$")
}

func (s *S) TestAppInfoReturnsNotFoundWhenAppDoesNotExist(c *C) {
	myApp := app.App{Name: "SomeApp"}
	request, err := http.NewRequest("GET", "/apps/"+myApp.Name+"?:name="+myApp.Name, nil)
	c.Assert(err, IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = AppInfo(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^App SomeApp not found.$")
}

func (s *S) TestCreateAppHelperShouldNotCreateAnAppWhenAnErrorHappensOnCreateRepo(c *C) {
	h := testBadHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	a := app.App{Name: "someapp"}
	_, err := createAppHelper(&a, s.user, 1)
	c.Assert(err, NotNil)
	length, err := db.Session.Apps().Find(bson.M{"name": a.Name}).Count()
	c.Assert(err, IsNil)
	c.Assert(length, Equals, 0)
}

func (s *S) TestCreateAppHelperCreatesRepositoryInGandalf(c *C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	a := app.App{Name: "someapp"}
	_, err := createAppHelper(&a, s.user, 1)
	c.Assert(err, IsNil)
	a.Get()
	defer a.Destroy()
	c.Assert(h.url[0], Equals, "/repository")
	c.Assert(h.method[0], Equals, "POST")
	expected := fmt.Sprintf(`{"name":"someapp","users":["%s"],"ispublic":false}`, s.user.Email)
	c.Assert(string(h.body[0]), Equals, expected)
}

func (s *S) TestCreateAppHandler(c *C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	a := app.App{Name: "someapp"}
	defer func() {
		err := a.Get()
		c.Assert(err, IsNil)
		err = a.Destroy()
		c.Assert(err, IsNil)
		err = s.provisioner.Destroy(&a)
		c.Assert(err, IsNil)
	}()
	b := strings.NewReader(`{"name":"someapp","framework":"django","units":4}`)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateAppHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	repoUrl := repository.GetUrl(a.Name)
	var obtained map[string]string
	expected := map[string]string{
		"status":         "success",
		"repository_url": repoUrl,
	}
	err = json.Unmarshal(body, &obtained)
	c.Assert(obtained, DeepEquals, expected)
	c.Assert(recorder.Code, Equals, http.StatusOK)
	var gotApp app.App
	err = db.Session.Apps().Find(bson.M{"name": "someapp"}).One(&gotApp)
	c.Assert(err, IsNil)
	_, found := gotApp.Find(s.team)
	c.Assert(found, Equals, true)
	c.Assert(s.provisioner.GetUnits(&gotApp), HasLen, 4)
}

func (s *S) TestCreateAppReturnsPreconditionFailedIfTheAppNameIsInvalid(c *C) {
	b := strings.NewReader(`{"name":"123myapp","framework":"django"}`)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateAppHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusPreconditionFailed)
	msg := "Invalid app name, your app should have at most 63 " +
		"characters, containing only lower case letters or numbers, " +
		"starting with a letter."
	c.Assert(e.Error(), Equals, msg)
}

func (s *S) TestCreateAppReturns403IfTheUserIsNotMemberOfAnyTeam(c *C) {
	u := &auth.User{Email: "thetrees@rush.com", Password: "123"}
	b := strings.NewReader(`{"name":"someapp", "framework":"django"}`)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateAppHandler(recorder, request, u)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^In order to create an app, you should be member of at least one team$")
}

func (s *S) TestCreateAppReturnsConflictWithProperMessageWhenTheAppAlreadyExist(c *C) {
	a := app.App{
		Name:  "plainsofdawn",
		Units: []app.Unit{{Machine: 1}},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	b := strings.NewReader(`{"name":"plainsofdawn","framework":"django"}`)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateAppHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusConflict)
	c.Assert(e.Message, Equals, `There is already an app named "plainsofdawn".`)
}

func (s *S) TestAddUnits(c *C) {
	a := app.App{
		Name:      "armorandsword",
		Framework: "python",
		Teams:     []string{s.team.Name},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = s.provisioner.Provision(&a)
	c.Assert(err, IsNil)
	defer s.provisioner.Destroy(&a)
	body := strings.NewReader("3")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:name=armorandsword", body)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddUnitsHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	err = a.Get()
	c.Assert(err, IsNil)
	c.Assert(a.Units, HasLen, 3)
}

func (s *S) TestAddUnitsReturns404IfAppDoesNotExist(c *C) {
	body := strings.NewReader("1")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:name=armorandsword", body)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddUnitsHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e.Message, Equals, "App armorandsword not found.")
}

func (s *S) TestAddUnitsReturns403IfTheUserDoesNotHaveAccessToTheApp(c *C) {
	a := app.App{
		Name:      "armorandsword",
		Framework: "python",
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	body := strings.NewReader("1")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:name=armorandsword", body)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddUnitsHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e.Message, Equals, "User does not have access to this app")
}

func (s *S) TestAddUnitsReturns400IfNumberOfUnitsIsOmited(c *C) {
	bodies := []io.Reader{nil, strings.NewReader("")}
	for _, body := range bodies {
		request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:name=armorandsword", body)
		c.Assert(err, IsNil)
		recorder := httptest.NewRecorder()
		err = AddUnitsHandler(recorder, request, s.user)
		c.Assert(err, NotNil)
		e, ok := err.(*errors.Http)
		c.Assert(ok, Equals, true)
		c.Assert(e.Code, Equals, http.StatusBadRequest)
		c.Assert(e.Message, Equals, "You must provide the number of units.")
	}
}

func (s *S) TestAddUnitsReturns400IfNumberIsInvalid(c *C) {
	values := []string{"-1", "0", "far cry", "12345678909876543"}
	for _, value := range values {
		body := strings.NewReader(value)
		request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:name=armorandsword", body)
		c.Assert(err, IsNil)
		recorder := httptest.NewRecorder()
		err = AddUnitsHandler(recorder, request, s.user)
		c.Assert(err, NotNil)
		e, ok := err.(*errors.Http)
		c.Assert(ok, Equals, true)
		c.Assert(e.Code, Equals, http.StatusBadRequest)
		c.Assert(e.Message, Equals, "Invalid number of units: the number must be an integer greater than 0.")
	}
}

func (s *S) TestRemoveUnits(c *C) {
	a := app.App{
		Name:      "velha",
		Framework: "python",
		Teams:     []string{s.team.Name},
		Units: []app.Unit{
			{Name: "velha/0"}, {Name: "velha/1"}, {Name: "velha/2"},
		},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = s.provisioner.Provision(&a)
	c.Assert(err, IsNil)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 3)
	body := strings.NewReader("2")
	request, err := http.NewRequest("DELETE", "/apps/velha/units?:name=velha", body)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUnitsHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	err = a.Get()
	c.Assert(err, IsNil)
	c.Assert(a.Units, HasLen, 1)
	c.Assert(a.Units[0].Name, Equals, "velha/2")
	c.Assert(s.provisioner.GetUnits(&a), HasLen, 2)
}

func (s *S) TestRemoveUnitsReturns404IfAppDoesNotExist(c *C) {
	body := strings.NewReader("1")
	request, err := http.NewRequest("DELETE", "/apps/fetisha/units?:name=fetisha", body)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUnitsHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e.Message, Equals, "App fetisha not found.")
}

func (s *S) TestRemoveUnitsReturns403IfTheUserDoesNotHaveAccessToTheApp(c *C) {
	a := app.App{
		Name:      "fetisha",
		Framework: "python",
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	body := strings.NewReader("1")
	request, err := http.NewRequest("DELETE", "/apps/fetisha/units?:name=fetisha", body)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUnitsHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e.Message, Equals, "User does not have access to this app")
}

func (s *S) TestRemoveUnitsReturns400IfNumberOfUnitsIsOmited(c *C) {
	bodies := []io.Reader{nil, strings.NewReader("")}
	for _, body := range bodies {
		request, err := http.NewRequest("DELETE", "/apps/fetisha/units?:name=fetisha", body)
		c.Assert(err, IsNil)
		recorder := httptest.NewRecorder()
		err = RemoveUnitsHandler(recorder, request, s.user)
		c.Assert(err, NotNil)
		e, ok := err.(*errors.Http)
		c.Assert(ok, Equals, true)
		c.Assert(e.Code, Equals, http.StatusBadRequest)
		c.Assert(e.Message, Equals, "You must provide the number of units.")
	}
}

func (s *S) TestRemoveUnitsReturns400IfNumberIsInvalid(c *C) {
	values := []string{"-1", "0", "far cry", "12345678909876543"}
	for _, value := range values {
		body := strings.NewReader(value)
		request, err := http.NewRequest("DELETE", "/apps/fiend/units?:name=fiend", body)
		c.Assert(err, IsNil)
		recorder := httptest.NewRecorder()
		err = RemoveUnitsHandler(recorder, request, s.user)
		c.Assert(err, NotNil)
		e, ok := err.(*errors.Http)
		c.Assert(ok, Equals, true)
		c.Assert(e.Code, Equals, http.StatusBadRequest)
		c.Assert(e.Message, Equals, "Invalid number of units: the number must be an integer greater than 0.")
	}
}

func (s *S) TestAddTeamToTheApp(c *C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	t := auth.Team{Name: "itshardteam", Users: []string{s.user.Email}}
	err := db.Session.Teams().Insert(t)
	c.Assert(err, IsNil)
	defer db.Session.Teams().RemoveAll(bson.M{"_id": t.Name})
	a := app.App{
		Name:      "itshard",
		Framework: "django",
		Teams:     []string{t.Name},
	}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/%s?:app=%s&:team=%s", a.Name, s.team.Name, a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GrantAccessToTeamHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	err = a.Get()
	c.Assert(err, IsNil)
	_, found := a.Find(s.team)
	c.Assert(found, Equals, true)
}

func (s *S) TestGrantAccessToTeamReturn404IfTheAppDoesNotExist(c *C) {
	request, err := http.NewRequest("PUT", "/apps/a/b?:app=a&:team=b", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GrantAccessToTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^App a not found.$")
}

func (s *S) TestGrantAccessToTeamReturn403IfTheGivenUserDoesNotHasAccessToTheApp(c *C) {
	a := app.App{
		Name:      "itshard",
		Framework: "django",
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/%s?:app=%s&:team=%s", a.Name, s.team.Name, a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GrantAccessToTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^User does not have access to this app$")
}

func (s *S) TestGrantAccessToTeamReturn404IfTheTeamDoesNotExist(c *C) {
	a := app.App{
		Name:      "itshard",
		Framework: "django",
		Teams:     []string{s.team.Name},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/a?:app=%s&:team=a", a.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GrantAccessToTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^Team not found$")
}

func (s *S) TestGrantAccessToTeamReturn409IfTheTeamHasAlreadyAccessToTheApp(c *C) {
	a := app.App{
		Name:      "itshard",
		Framework: "django",
		Teams:     []string{s.team.Name},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/%s?:app=%s&:team=%s", a.Name, s.team.Name, a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GrantAccessToTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusConflict)
}

func (s *S) TestGrantAccessToTeamCallsGandalf(c *C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	t := &auth.Team{Name: "anything", Users: []string{s.user.Email}}
	err := db.Session.Teams().Insert(t)
	c.Assert(err, IsNil)
	defer db.Session.Teams().Remove(bson.M{"_id": t.Name})
	a := app.App{
		Name:      "tsuru",
		Framework: "golang",
		Teams:     []string{t.Name},
	}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = grantAccessToTeam(a.Name, s.team.Name, s.user)
	c.Assert(err, IsNil)
	c.Assert(h.url[0], Equals, "/repository/grant")
	c.Assert(h.method[0], Equals, "POST")
	expected := fmt.Sprintf(`{"repositories":["%s"],"users":["%s"]}`, a.Name, s.user.Email)
	c.Assert(string(h.body[0]), Equals, expected)
}

func (s *S) TestRevokeAccessFromTeam(c *C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	t := auth.Team{Name: "abcd"}
	err := db.Session.Teams().Insert(t)
	c.Assert(err, IsNil)
	defer db.Session.Teams().Remove(bson.M{"_id": t.Name})
	a := app.App{
		Name:      "itshard",
		Framework: "django",
		Teams:     []string{"abcd", s.team.Name},
	}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/%s?:app=%s&:team=%s", a.Name, s.team.Name, a.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RevokeAccessFromTeamHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	a.Get()
	_, found := a.Find(s.team)
	c.Assert(found, Equals, false)
}

func (s *S) TestRevokeAccessFromTeamReturn404IfTheAppDoesNotExist(c *C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	request, err := http.NewRequest("DELETE", "/apps/a/b?:app=a&:team=b", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RevokeAccessFromTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^App a not found.$")
}

func (s *S) TestRevokeAccessFromTeamReturn401IfTheGivenUserDoesNotHavePermissionInTheApp(c *C) {
	a := app.App{
		Name:      "itshard",
		Framework: "django",
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/%s?:app=%s&:team=%s", a.Name, s.team.Name, a.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RevokeAccessFromTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^User does not have access to this app$")
}

func (s *S) TestRevokeAccessFromTeamReturn404IfTheTeamDoesNotExist(c *C) {
	a := app.App{
		Name:      "itshard",
		Framework: "django",
		Teams:     []string{s.team.Name},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/x?:app=%s&:team=x", a.Name, a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RevokeAccessFromTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^Team not found$")
}

func (s *S) TestRevokeAccessFromTeamReturn404IfTheTeamDoesNotHaveAccessToTheApp(c *C) {
	t := auth.Team{Name: "blaaa"}
	err := db.Session.Teams().Insert(t)
	c.Assert(err, IsNil)
	t2 := auth.Team{Name: "team2"}
	err = db.Session.Teams().Insert(t2)
	c.Assert(err, IsNil)
	defer db.Session.Teams().Remove(bson.M{"_id": bson.M{"$in": []string{"blaaa", "team2"}}})
	a := app.App{
		Name:      "itshard",
		Framework: "django",
		Teams:     []string{s.team.Name, t2.Name},
	}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/%s?:app=%s&:team=%s", a.Name, t.Name, a.Name, t.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RevokeAccessFromTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
}

func (s *S) TestRevokeAccessFromTeamReturn403IfTheTeamIsTheLastWithAccessToTheApp(c *C) {
	a := app.App{
		Name:      "itshard",
		Framework: "django",
		Teams:     []string{s.team.Name},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/%s?:app=%s&:team=%s", a.Name, s.team.Name, a.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RevokeAccessFromTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^You can not revoke the access from this team, because it is the unique team with access to the app, and an app can not be orphaned$")
}

func (s *S) TestRevokeAccessFromTeamRemovesRepositoryFromGandalf(c *C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	u := auth.User{Email: "again@live.com"}
	err := db.Session.Users().Insert(u)
	c.Assert(err, IsNil)
	defer db.Session.Users().Remove(bson.M{"email": u.Email})
	t := auth.Team{Name: "anything", Users: []string{u.Email}}
	err = db.Session.Teams().Insert(t)
	c.Assert(err, IsNil)
	defer db.Session.Teams().Remove(bson.M{"_id": t.Name})
	a := app.App{
		Name:      "tsuru",
		Framework: "golang",
		Teams:     []string{t.Name},
	}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = grantAccessToTeam(a.Name, s.team.Name, &u)
	c.Assert(err, IsNil)
	err = revokeAccessFromTeam(a.Name, s.team.Name, &u)
	c.Assert(h.url[1], Equals, "/repository/revoke") //should inc the index (because of the grantAccess)
	c.Assert(h.method[1], Equals, "DELETE")
	expected := fmt.Sprintf(`{"repositories":["%s"],"users":["%s"]}`, a.Name, s.user.Email)
	c.Assert(string(h.body[1]), Equals, expected)
}

func (s *S) TestRevokeAccessFromTeamDontRemoveTheUserIfItHasAccesToTheAppThroughAnotherTeam(c *C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	u := auth.User{Email: "burning@angel.com"}
	err := db.Session.Users().Insert(u)
	c.Assert(err, IsNil)
	defer db.Session.Users().Remove(bson.M{"email": u.Email})
	t := auth.Team{Name: "anything", Users: []string{s.user.Email, u.Email}}
	err = db.Session.Teams().Insert(t)
	c.Assert(err, IsNil)
	defer db.Session.Teams().Remove(bson.M{"_id": t.Name})
	a := app.App{
		Name:      "tsuru",
		Framework: "golang",
		Teams:     []string{s.team.Name},
	}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = grantAccessToTeam(a.Name, t.Name, s.user)
	c.Assert(err, IsNil)
	err = revokeAccessFromTeam(a.Name, t.Name, s.user)
	c.Assert(err, IsNil)
	c.Assert(h.url[1], Equals, "/repository/revoke")
	c.Assert(h.method[1], Equals, "DELETE")
	expected := fmt.Sprintf(`{"repositories":[%q],"users":[%q]}`, a.Name, u.Email)
	c.Assert(string(h.body[1]), Equals, expected)
}

func (s *S) TestRevokeAccessFromTeamDontCallGandalfIfNoUserNeedToBeRevoked(c *C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	t := auth.Team{Name: "anything", Users: []string{s.user.Email}}
	err := db.Session.Teams().Insert(t)
	c.Assert(err, IsNil)
	defer db.Session.Teams().Remove(bson.M{"_id": t.Name})
	a := app.App{
		Name:      "tsuru",
		Framework: "golang",
		Teams:     []string{s.team.Name},
	}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = grantAccessToTeam(a.Name, t.Name, s.user)
	c.Assert(err, IsNil)
	err = revokeAccessFromTeam(a.Name, t.Name, s.user)
	c.Assert(err, IsNil)
	c.Assert(h.url, HasLen, 1)
	c.Assert(h.url[0], Equals, "/repository/grant")
}

func (s *S) TestRunHandlerShouldExecuteTheGivenCommandInTheGivenApp(c *C) {
	s.provisioner.PrepareOutput([]byte("lots of files"))
	a := app.App{
		Name:      "secrets",
		Framework: "arch enemy",
		Teams:     []string{s.team.Name},
		Units:     []app.Unit{{Name: "i-0800", Machine: 10}},
		State:     string(provision.StatusStarted),
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/run/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("ls"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RunCommand(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.String(), Equals, "lots of files")
	c.Assert(recorder.Header().Get("Content-Type"), Equals, "text")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls"
	cmds := s.provisioner.GetCmds(expected, &a)
	c.Assert(cmds, HasLen, 1)
}

func (s *S) TestRunHandlerReturnsTheOutputOfTheCommandEvenIfItFails(c *C) {
	s.provisioner.PrepareFailure("ExecuteCommand", &errors.Http{Code: 500, Message: "something went wrong"})
	s.provisioner.PrepareOutput([]byte("failure output"))
	a := app.App{
		Name:      "secrets",
		Framework: "arch enemy",
		Teams:     []string{s.team.Name},
		Units:     []app.Unit{{Name: "i-0800"}},
		State:     string(provision.StatusStarted),
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/run/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("ls"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RunCommand(recorder, request, s.user)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "something went wrong")
	c.Assert(recorder.Body.String(), Equals, "failure output")
}

func (s *S) TestRunHandlerReturnsBadRequestIfTheCommandIsMissing(c *C) {
	bodies := []io.Reader{nil, strings.NewReader("")}
	for _, body := range bodies {
		request, err := http.NewRequest("POST", "/apps/unknown/run/?:name=unkown", body)
		c.Assert(err, IsNil)
		recorder := httptest.NewRecorder()
		err = RunCommand(recorder, request, s.user)
		c.Assert(err, NotNil)
		e, ok := err.(*errors.Http)
		c.Assert(ok, Equals, true)
		c.Assert(e.Code, Equals, http.StatusBadRequest)
		c.Assert(e, ErrorMatches, "^You must provide the command to run$")
	}
}

func (s *S) TestRunHandlerReturnsInternalErrorIfReadAllFails(c *C) {
	b := s.getTestData("bodyToBeClosed.txt")
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, IsNil)
	request.Body.Close()
	recorder := httptest.NewRecorder()
	err = RunCommand(recorder, request, s.user)
	c.Assert(err, NotNil)
}

func (s *S) TestRunHandlerReturnsNotFoundIfTheAppDoesNotExist(c *C) {
	request, err := http.NewRequest("POST", "/apps/unknown/run/?:name=unknown", strings.NewReader("ls"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RunCommand(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestRunHandlerReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *C) {
	a := app.App{
		Name:      "secrets",
		Framework: "arch enemy",
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/run/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("ls"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RunCommand(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
}

func (s *S) TestGetEnvHandlerGetsEnvironmentVariableFromApp(c *C) {
	a := app.App{
		Name:      "everything-i-want",
		Framework: "gotthard",
		Teams:     []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/env/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, strings.NewReader("DATABASE_HOST"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.String(), Equals, "DATABASE_HOST=localhost\n")
}

func (s *S) TestGetEnvHandlerShouldAcceptMultipleVariables(c *C) {
	a := app.App{
		Name:  "four-sticks",
		Teams: []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/env/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, strings.NewReader("DATABASE_HOST DATABASE_USER"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.String(), Equals, "DATABASE_HOST=localhost\nDATABASE_USER=root\n")
}

func (s *S) TestGetEnvHandlerReturnsAllVariablesIfEnvironmentVariablesAreMissingWithMaskOnPrivateVars(c *C) {
	a := app.App{
		Name:      "time",
		Framework: "pink-floyd",
		Teams:     []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	expected := []string{"", "DATABASE_HOST=localhost", "DATABASE_PASSWORD=*** (private variable)", "DATABASE_USER=root"}
	bodies := []io.Reader{nil, strings.NewReader("")}
	for _, body := range bodies {
		request, err := http.NewRequest("GET", "/apps/time/env/?:name=time", body)
		c.Assert(err, IsNil)
		recorder := httptest.NewRecorder()
		err = GetEnv(recorder, request, s.user)
		c.Assert(err, IsNil)
		got := strings.Split(recorder.Body.String(), "\n")
		sort.Strings(got)
		c.Assert(got, DeepEquals, expected)
	}
}

func (s *S) TestGetEnvHandlerReturnsInternalErrorIfReadAllFails(c *C) {
	b := s.getTestData("bodyToBeClosed.txt")
	request, err := http.NewRequest("GET", "/apps/unkown/env/?:name=unknown", b)
	c.Assert(err, IsNil)
	request.Body.Close()
	recorder := httptest.NewRecorder()
	err = GetEnv(recorder, request, s.user)
	c.Assert(err, NotNil)
}

func (s *S) TestGetEnvHandlerReturnsNotFoundIfTheAppDoesNotExist(c *C) {
	request, err := http.NewRequest("GET", "/apps/unknown/env/?:name=unknown", strings.NewReader("DATABASE_HOST"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GetEnv(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestGetEnvHandlerReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *C) {
	a := app.App{
		Name:      "lost",
		Framework: "vougan",
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/env/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, strings.NewReader("DATABASE_HOST"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GetEnv(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
}

func (s *S) TestSetEnvHandlerShouldSetAPublicEnvironmentVariableInTheApp(c *C) {
	a := app.App{
		Name:  "black-dog",
		Teams: []string{s.team.Name},
		Units: []app.Unit{{Machine: 1}},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/env?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("DATABASE_HOST=localhost"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = SetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := &app.App{Name: "black-dog"}
	err = app.Get()
	c.Assert(err, IsNil)
	expected := bind.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: true}
	c.Assert(app.Env["DATABASE_HOST"], DeepEquals, expected)
}

func (s *S) TestSetEnvHandlerShouldSetMultipleEnvironmentVariablesInTheApp(c *C) {
	a := app.App{
		Name:  "vigil",
		Teams: []string{s.team.Name},
		Units: []app.Unit{{Machine: 1}},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/env?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("DATABASE_HOST=localhost DATABASE_USER=root"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = SetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := &app.App{Name: "vigil"}
	err = app.Get()
	c.Assert(err, IsNil)
	expectedHost := bind.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: true}
	expectedUser := bind.EnvVar{Name: "DATABASE_USER", Value: "root", Public: true}
	c.Assert(app.Env["DATABASE_HOST"], DeepEquals, expectedHost)
	c.Assert(app.Env["DATABASE_USER"], DeepEquals, expectedUser)
}

func (s *S) TestSetEnvHandlerShouldSupportSpacesInTheEnvironmentVariableValue(c *C) {
	a := app.App{
		Name:  "loser",
		Teams: []string{s.team.Name},
		Units: []app.Unit{{Machine: 1}},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/env?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("DATABASE_HOST=local host DATABASE_USER=root"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = SetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := &app.App{Name: "loser"}
	err = app.Get()
	c.Assert(err, IsNil)
	expectedHost := bind.EnvVar{Name: "DATABASE_HOST", Value: "local host", Public: true}
	expectedUser := bind.EnvVar{Name: "DATABASE_USER", Value: "root", Public: true}
	c.Assert(app.Env["DATABASE_HOST"], DeepEquals, expectedHost)
	c.Assert(app.Env["DATABASE_USER"], DeepEquals, expectedUser)
}

func (s *S) TestSetEnvHandlerShouldSupportValuesWithDot(c *C) {
	a := app.App{
		Name:  "losers",
		Teams: []string{s.team.Name},
		Units: []app.Unit{{Machine: 1}},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/env?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("DATABASE_HOST=http://foo.com:8080"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = SetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := &app.App{Name: "losers"}
	err = app.Get()
	c.Assert(err, IsNil)
	expected := bind.EnvVar{Name: "DATABASE_HOST", Value: "http://foo.com:8080", Public: true}
	c.Assert(app.Env["DATABASE_HOST"], DeepEquals, expected)
}

func (s *S) TestSetEnvHandlerShouldSupportNumbersOnVariableName(c *C) {
	a := app.App{
		Name:  "blinded",
		Teams: []string{s.team.Name},
		Units: []app.Unit{{Machine: 1}},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/env?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("EC2_HOST=http://foo.com:8080"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = SetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := &app.App{Name: "blinded"}
	err = app.Get()
	c.Assert(err, IsNil)
	expected := bind.EnvVar{Name: "EC2_HOST", Value: "http://foo.com:8080", Public: true}
	c.Assert(app.Env["EC2_HOST"], DeepEquals, expected)
}

func (s *S) TestSetEnvHandlerShouldSupportLowerCasedVariableName(c *C) {
	a := app.App{
		Name:  "fragments",
		Teams: []string{s.team.Name},
		Units: []app.Unit{{Machine: 1}},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/env?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("http_proxy=http://my_proxy.com:3128"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = SetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := &app.App{Name: "fragments"}
	err = app.Get()
	c.Assert(err, IsNil)
	expected := bind.EnvVar{Name: "http_proxy", Value: "http://my_proxy.com:3128", Public: true}
	c.Assert(app.Env["http_proxy"], DeepEquals, expected)
}

func (s *S) TestSetEnvHandlerShouldNotChangeValueOfPrivateVariables(c *C) {
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
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/env?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("DATABASE_HOST=http://foo.com:8080"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = SetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := &app.App{Name: "losers"}
	err = app.Get()
	c.Assert(err, IsNil)
	c.Assert(app.Env, DeepEquals, original)
}

func (s *S) TestSetEnvHandlerReturnsInternalErrorIfReadAllFails(c *C) {
	b := s.getTestData("bodyToBeClosed.txt")
	request, err := http.NewRequest("POST", "/apps/unkown/env/?:name=unknown", b)
	c.Assert(err, IsNil)
	request.Body.Close()
	recorder := httptest.NewRecorder()
	err = SetEnv(recorder, request, s.user)
	c.Assert(err, NotNil)
}

func (s *S) TestSetEnvHandlerReturnsBadRequestIfVariablesAreMissing(c *C) {
	bodies := []io.Reader{nil, strings.NewReader("")}
	for _, body := range bodies {
		request, err := http.NewRequest("POST", "/apps/unknown/env/?:name=unkown", body)
		c.Assert(err, IsNil)
		recorder := httptest.NewRecorder()
		err = SetEnv(recorder, request, s.user)
		c.Assert(err, NotNil)
		e, ok := err.(*errors.Http)
		c.Assert(ok, Equals, true)
		c.Assert(e.Code, Equals, http.StatusBadRequest)
		c.Assert(e, ErrorMatches, "^You must provide the environment variables$")
	}
}

func (s *S) TestSetEnvHandlerReturnsNotFoundIfTheAppDoesNotExist(c *C) {
	b := strings.NewReader("DATABASE_HOST=localhost")
	request, err := http.NewRequest("POST", "/apps/unknown/env/?:name=unknown", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = SetEnv(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestSetEnvHandlerReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *C) {
	a := app.App{Name: "rock-and-roll"}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/env/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("DATABASE_HOST=localhost"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = SetEnv(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
}

func (s *S) TestUnsetEnvHandlerRemovesTheEnvironmentVariablesFromTheApp(c *C) {
	a := app.App{
		Name:  "swift",
		Teams: []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	expected := a.Env
	delete(expected, "DATABASE_HOST")
	url := fmt.Sprintf("/apps/%s/env/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("DELETE", url, strings.NewReader("DATABASE_HOST"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = UnsetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := app.App{Name: "swift"}
	err = app.Get()
	c.Assert(err, IsNil)
	c.Assert(app.Env, DeepEquals, expected)
}

func (s *S) TestUnsetEnvHandlerRemovesAllGivenEnvironmentVariables(c *C) {
	a := app.App{
		Name:  "let-it-be",
		Teams: []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/env/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("DELETE", url, strings.NewReader("DATABASE_HOST DATABASE_USER"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = UnsetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := app.App{Name: "let-it-be"}
	err = app.Get()
	expected := map[string]bind.EnvVar{
		"DATABASE_PASSWORD": {
			Name:   "DATABASE_PASSWORD",
			Value:  "secret",
			Public: false,
		},
	}
	c.Assert(err, IsNil)
	c.Assert(app.Env, DeepEquals, expected)
}

func (s *S) TestUnsetHandlerDoesNotRemovePrivateVariables(c *C) {
	a := app.App{
		Name:  "letitbe",
		Teams: []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/env/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("DELETE", url, strings.NewReader("DATABASE_HOST DATABASE_USER DATABASE_PASSWORD"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = UnsetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := app.App{Name: "letitbe"}
	err = app.Get()
	expected := map[string]bind.EnvVar{
		"DATABASE_PASSWORD": {
			Name:   "DATABASE_PASSWORD",
			Value:  "secret",
			Public: false,
		},
	}
	c.Assert(err, IsNil)
	c.Assert(app.Env, DeepEquals, expected)
}

func (s *S) TestUnsetEnvHandlerReturnsInternalErrorIfReadAllFails(c *C) {
	b := s.getTestData("bodyToBeClosed.txt")
	request, err := http.NewRequest("POST", "/apps/unkown/env/?:name=unknown", b)
	c.Assert(err, IsNil)
	request.Body.Close()
	recorder := httptest.NewRecorder()
	err = UnsetEnv(recorder, request, s.user)
	c.Assert(err, NotNil)
}

func (s *S) TestUnsetEnvHandlerReturnsBadRequestIfVariablesAreMissing(c *C) {
	bodies := []io.Reader{nil, strings.NewReader("")}
	for _, body := range bodies {
		request, err := http.NewRequest("POST", "/apps/unknown/env/?:name=unkown", body)
		c.Assert(err, IsNil)
		recorder := httptest.NewRecorder()
		err = UnsetEnv(recorder, request, s.user)
		c.Assert(err, NotNil)
		e, ok := err.(*errors.Http)
		c.Assert(ok, Equals, true)
		c.Assert(e.Code, Equals, http.StatusBadRequest)
		c.Assert(e, ErrorMatches, "^You must provide the environment variables$")
	}
}

func (s *S) TestUnsetEnvHandlerReturnsNotFoundIfTheAppDoesNotExist(c *C) {
	b := strings.NewReader("DATABASE_HOST")
	request, err := http.NewRequest("POST", "/apps/unknown/env/?:name=unknown", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = UnsetEnv(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestUnsetEnvHandlerReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *C) {
	a := app.App{Name: "mountain-mama"}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/env/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("DATABASE_HOST=localhost"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = UnsetEnv(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
}

func (s *S) TestLogShouldReturnNotFoundWhenAppDoesNotExist(c *C) {
	request, err := http.NewRequest("GET", "/apps/unknown/log/?:name=unknown", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AppLog(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestLogReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *C) {
	a := app.App{
		Name:      "lost",
		Framework: "vougan",
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/log/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AppLog(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
}

func (s *S) TestAppLogShouldHaveContentType(c *C) {
	a := app.App{
		Name:      "lost",
		Framework: "vougan",
		Teams:     []string{s.team.Name},
		Logs: []app.Applog{
			{
				Date:    time.Now(),
				Message: "Something new",
			},
		},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/log/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = AppLog(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), Equals, "application/json")
}

func (s *S) TestAppLogSelectByLines(c *C) {
	a := app.App{
		Name:      "lost",
		Framework: "vougan",
		Teams:     []string{s.team.Name},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	for i := 0; i < 15; i++ {
		a.Log(strconv.Itoa(i), "source")
	}
	url := fmt.Sprintf("/apps/%s/log/?:name=%s&lines=10", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = AppLog(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	logs := []app.Applog{}
	err = json.Unmarshal(body, &logs)
	c.Assert(err, IsNil)
	c.Assert(logs, HasLen, 10)
}

func (s *S) TestAppLogSelectBySource(c *C) {
	a := app.App{
		Name:      "lost",
		Framework: "vougan",
		Teams:     []string{s.team.Name},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	a.Log("mars log", "mars")
	a.Log("earth log", "earth")
	url := fmt.Sprintf("/apps/%s/log/?:name=%s&source=mars", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = AppLog(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	logs := []app.Applog{}
	err = json.Unmarshal(body, &logs)
	c.Assert(err, IsNil)
	c.Assert(logs, HasLen, 1)
	c.Assert(logs[0].Message, Equals, "mars log")
	c.Assert(logs[0].Source, Equals, "mars")
}

func (s *S) TestAppLogSelectByLinesShouldReturnTheLastestEntries(c *C) {
	a := app.App{
		Name:      "lost",
		Framework: "vougan",
		Teams:     []string{s.team.Name},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	logs := make([]app.Applog, 15)
	now := time.Now()
	for i := 0; i < 15; i++ {
		logs[i] = app.Applog{
			Date:    now.Add(time.Duration(i) * time.Hour),
			Message: strconv.Itoa(i),
			Source:  "source",
		}
	}
	a.Logs = logs
	err = db.Session.Apps().Update(bson.M{"name": a.Name}, a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/log/?:name=%s&lines=3", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = AppLog(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	logs = []app.Applog{}
	err = json.Unmarshal(body, &logs)
	c.Assert(err, IsNil)
	c.Assert(logs, HasLen, 3)
	c.Assert(logs[0].Message, Equals, "12")
	c.Assert(logs[1].Message, Equals, "13")
	c.Assert(logs[2].Message, Equals, "14")
}

func (s *S) TestAppLogShouldReturnLogByApp(c *C) {
	app1 := app.App{
		Name:      "app1",
		Framework: "vougan",
		Teams:     []string{s.team.Name},
	}
	err := db.Session.Apps().Insert(app1)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app1.Name})
	app1.Log("app1 log", "source")
	app2 := app.App{
		Name:      "app2",
		Framework: "vougan",
		Teams:     []string{s.team.Name},
	}
	err = db.Session.Apps().Insert(app2)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app2.Name})
	app2.Log("app2 log", "source")
	app3 := app.App{
		Name:      "app3",
		Framework: "vougan",
		Teams:     []string{s.team.Name},
	}
	err = db.Session.Apps().Insert(app3)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app3.Name})
	app3.Log("app3 log", "tsuru")
	url := fmt.Sprintf("/apps/%s/log/?:name=%s", app3.Name, app3.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = AppLog(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	logs := []app.Applog{}
	err = json.Unmarshal(body, &logs)
	c.Assert(err, IsNil)
	var logged bool
	for _, log := range logs {
		// Should not show the app1 log
		c.Assert(log.Message, Not(Equals), "app1 log")
		// Should not show the app2 log
		c.Assert(log.Message, Not(Equals), "app2 log")
		if log.Message == "app3 log" {
			logged = true
		}
	}
	// Should show the app3 log
	c.Assert(logged, Equals, true)
}

func (s *S) TestGetTeamNamesReturnTheNameOfTeamsThatTheUserIsMember(c *C) {
	one := &auth.User{Email: "imone@thewho.com", Password: "123"}
	who := auth.Team{Name: "TheWho", Users: []string{one.Email}}
	err := db.Session.Teams().Insert(who)
	what := auth.Team{Name: "TheWhat", Users: []string{one.Email}}
	err = db.Session.Teams().Insert(what)
	c.Assert(err, IsNil)
	where := auth.Team{Name: "TheWhere", Users: []string{one.Email}}
	err = db.Session.Teams().Insert(where)
	c.Assert(err, IsNil)
	teams := []string{who.Name, what.Name, where.Name}
	defer db.Session.Teams().RemoveAll(bson.M{"_id": bson.M{"$in": teams}})
	names, err := getTeamNames(one)
	c.Assert(err, IsNil)
	c.Assert(names, DeepEquals, teams)
}

func (s *S) TestBindHandler(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a := app.App{
		Name:  "painkiller",
		Teams: []string{s.team.Name},
		Units: []app.Unit{{Ip: "127.0.0.1", Machine: 1}},
		Env:   map[string]bind.EnvVar{},
	}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, a.Name, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = BindHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	err = db.Session.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(err, IsNil)
	c.Assert(instance.Apps, DeepEquals, []string{a.Name})
	err = db.Session.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(err, IsNil)
	expectedUser := bind.EnvVar{Name: "DATABASE_USER", Value: "root", Public: false, InstanceName: instance.Name}
	expectedPassword := bind.EnvVar{Name: "DATABASE_PASSWORD", Value: "s3cr3t", Public: false, InstanceName: instance.Name}
	c.Assert(a.Env["DATABASE_USER"], DeepEquals, expectedUser)
	c.Assert(a.Env["DATABASE_PASSWORD"], DeepEquals, expectedPassword)
	var envs []string
	err = json.Unmarshal(recorder.Body.Bytes(), &envs)
	c.Assert(err, IsNil)
	sort.Strings(envs)
	c.Assert(envs, DeepEquals, []string{"DATABASE_PASSWORD", "DATABASE_USER"})
	c.Assert(recorder.Header().Get("Content-Type"), Equals, "application/json")
}

func (s *S) TestBindHandlerReturns404IfTheInstanceDoesNotExist(c *C) {
	a := app.App{
		Name:      "serviceApp",
		Framework: "django",
		Teams:     []string{s.team.Name},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/services/instances/unknown/%s?:instance=unknown&:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = BindHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^Instance not found$")
}

func (s *S) TestBindHandlerReturns403IfTheUserDoesNotHaveAccessToTheInstance(c *C) {
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql"}
	err := instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a := app.App{
		Name:      "serviceApp",
		Framework: "django",
		Teams:     []string{s.team.Name},
	}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, a.Name, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = BindHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^This user does not have access to this instance$")
}

func (s *S) TestBindHandlerReturns404IfTheAppDoesNotExist(c *C) {
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err := instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	url := fmt.Sprintf("/services/instances/%s/unknown?:instance=%s&:app=unknown", instance.Name, instance.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = BindHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestBindHandlerReturns403IfTheUserDoesNotHaveAccessToTheApp(c *C) {
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err := instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a := app.App{
		Name:      "serviceApp",
		Framework: "django",
	}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, a.Name, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = BindHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^This user does not have access to this app$")
}

func (s *S) TestUnbindHandler(c *C) {
	h := testHandler{}
	gts := s.t.StartGandalfTestServer(&h)
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
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
	}
	err = instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a := app.App{
		Name:  "painkiller",
		Teams: []string{s.team.Name},
		Units: []app.Unit{{Machine: 1}},
	}
	err = app.CreateApp(&a, 1)
	c.Assert(err, IsNil)
	a.Get()
	defer a.Destroy()
	a.Env["DATABASE_HOST"] = bind.EnvVar{
		Name:         "DATABASE_HOST",
		Value:        "arrea",
		Public:       false,
		InstanceName: instance.Name,
	}
	a.Env["MY_VAR"] = bind.EnvVar{Name: "MY_VAR", Value: "123"}
	a.Units = []app.Unit{{Ip: "127.0.0.1", Machine: 1}}
	err = db.Session.Apps().Update(bson.M{"name": a.Name}, &a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, a.Name, instance.Name, a.Name)
	req, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = UnbindHandler(recorder, req, s.user)
	c.Assert(err, IsNil)
	err = db.Session.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(err, IsNil)
	c.Assert(instance.Apps, DeepEquals, []string{})
	err = db.Session.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(err, IsNil)
	expected := bind.EnvVar{
		Name:  "MY_VAR",
		Value: "123",
	}
	c.Assert(a.Env["MY_VAR"], DeepEquals, expected)
	_, ok := a.Env["DATABASE_HOST"]
	c.Assert(ok, Equals, false)
	ch := make(chan bool)
	go func() {
		t := time.Tick(1)
		for _ = <-t; atomic.LoadInt32(&called) == 0; _ = <-t {
		}
		ch <- true
	}()
	select {
	case <-ch:
		c.SucceedNow()
	case <-time.After(1e9):
		c.Errorf("Failed to call API after 1 second.")
	}
}

func (s *S) TestUnbindHandlerReturns404IfTheInstanceDoesNotExist(c *C) {
	a := app.App{
		Name:      "serviceApp",
		Framework: "django",
		Teams:     []string{s.team.Name},
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/services/instances/unknown/%s?:instance=unknown&:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = UnbindHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^Instance not found$")
}

func (s *S) TestUnbindHandlerReturns403IfTheUserDoesNotHaveAccessToTheInstance(c *C) {
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql"}
	err := instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a := app.App{
		Name:      "serviceApp",
		Framework: "django",
		Teams:     []string{s.team.Name},
	}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, a.Name, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = UnbindHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^This user does not have access to this instance$")
}

func (s *S) TestUnbindHandlerReturns404IfTheAppDoesNotExist(c *C) {
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err := instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	url := fmt.Sprintf("/services/instances/%s/unknown?:instance=%s&:app=unknown", instance.Name, instance.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = UnbindHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestUnbindHandlerReturns403IfTheUserDoesNotHaveAccessToTheApp(c *C) {
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err := instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a := app.App{
		Name:      "serviceApp",
		Framework: "django",
	}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/services/instances/%s/%s?:instance=%s&:app=%s", instance.Name, a.Name, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = UnbindHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^This user does not have access to this app$")
}

func (s *S) TestRestartHandler(c *C) {
	s.provisioner.PrepareOutput(nil) // loadHooks
	s.provisioner.PrepareOutput([]byte("restarted"))
	a := app.App{
		Name:  "stress",
		Teams: []string{s.team.Name},
		State: string(provision.StatusStarted),
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/restart?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RestartHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	result := strings.Replace(recorder.Body.String(), "\n", "#", -1)
	c.Assert(result, Matches, ".*# ---> Restarting your app#.*")
	c.Assert(recorder.Header().Get("Content-Type"), Equals, "text")
}

func (s *S) TestRestartHandlerReturns404IfTheAppDoesNotExist(c *C) {
	request, err := http.NewRequest("GET", "/apps/unknown/restart?:name=unknown", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RestartHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
}

func (s *S) TestRestartHandlerReturns403IfTheUserDoesNotHaveAccessToTheApp(c *C) {
	a := app.App{Name: "nightmist"}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/restart?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RestartHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
}

func (s *S) TestAddLogHandler(c *C) {
	a := app.App{
		Name:      "myapp",
		Framework: "python",
	}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	b := strings.NewReader(`["message 1", "message 2", "message 3"]`)
	request, err := http.NewRequest("POST", "/apps/myapp/log/?:name=myapp", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddLogHandler(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, http.StatusOK)
	messages := []string{
		"message 1",
		"message 2",
		"message 3",
	}
	for _, msg := range messages {
		length, err := db.Session.Apps().Find(bson.M{"name": a.Name, "logs.message": msg}).Count()
		c.Check(err, IsNil)
		c.Check(length, Equals, 1)
	}
}

func (s *S) TestgetAppOrErrorWhenUserIsAdmin(c *C) {
	admin := auth.User{Email: "superuser@gmail.com", Password: "123"}
	err := db.Session.Users().Insert(&admin)
	c.Assert(err, IsNil)
	adminTeamName, err := config.GetString("admin-team")
	c.Assert(err, IsNil)
	adminTeam := auth.Team{Name: adminTeamName, Users: []string{admin.Email}}
	err = db.Session.Teams().Insert(&adminTeam)
	c.Assert(err, IsNil)
	a := app.App{Name: "testApp", Teams: []string{"notAdmin", "noSuperUser"}}
	err = db.Session.Apps().Insert(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	s.createAdminUserAndTeam(c)
	defer func(admin auth.User, adminTeam auth.Team) {
		err := db.Session.Teams().RemoveId(adminTeam.Name)
		c.Assert(err, IsNil)
		err = db.Session.Users().Remove(bson.M{"email": admin.Email})
		c.Assert(err, IsNil)
	}(admin, adminTeam)
	app, err := getAppOrError(a.Name, &admin)
	c.Assert(err, IsNil)
	err = a.Get()
	c.Assert(err, IsNil)
	c.Assert(app, DeepEquals, a)
}
