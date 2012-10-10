// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/globocom/commandmocker"
	"github.com/globocom/tsuru/api/auth"
	"github.com/globocom/tsuru/api/bind"
	"github.com/globocom/tsuru/api/service"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/repository"
	"io"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	stdlog "log"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"time"
)

var output = `2012-06-05 17:03:36,887 WARNING ssl-hostname-verification is disabled for this environment
2012-06-05 17:03:36,887 WARNING EC2 API calls not using secure transport
2012-06-05 17:03:36,887 WARNING S3 API calls not using secure transport
2012-06-05 17:03:36,887 WARNING Ubuntu Cloud Image lookups encrypted but not authenticated
2012-06-05 17:03:36,896 INFO Connecting to environment...
2012-06-05 17:03:37,599 INFO Connected to environment.
2012-06-05 17:03:37,727 INFO Connecting to machine 0 at 10.170.0.191
export DATABASE_HOST=localhost
export DATABASE_USER=root
export DATABASE_PASSWORD=secret`

func (s *S) TestAppIsAvaliableHandlerShouldReturnsErrorWhenAppUnitStatusIsnotStarted(c *C) {
	u := Unit{
		Name:              "someapp/0",
		Type:              "django",
		AgentState:        "creating",
		MachineAgentState: "running",
		InstanceState:     "running",
	}
	a := App{
		Name:      "someapp",
		Framework: "python",
		Teams:     []string{s.team.Name},
	}
	err := createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	a.Units = []Unit{u}
	err = db.Session.Apps().Update(bson.M{"name": a.Name}, &a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/avaliable?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AppIsAvaliableHandler(recorder, request)
	c.Assert(err, NotNil)
}

func (s *S) TestAppIsAvaliableHandlerShouldReturns200WhenAppUnitStatusIsStarted(c *C) {
	u := Unit{
		Name:              "someapp/0",
		Type:              "django",
		AgentState:        "started",
		MachineAgentState: "running",
		InstanceState:     "running",
	}
	a := App{
		Name:      "someapp",
		Framework: "python",
		Teams:     []string{s.team.Name},
	}
	err := createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	a.Units = []Unit{u}
	err = db.Session.Apps().Update(bson.M{"name": a.Name}, &a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/repository/clone?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AppIsAvaliableHandler(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, http.StatusOK)
}

func (s *S) TestCloudRepositoryHandlerShouldFilterOutputFromJuju(c *C) {
	output := `
========
2012-06-05 17:03:36,887 WARNING ssl-hostname-verification is disabled for this environment
pre-restart:
    pre.sh
pos-restart:
    pos.sh
`
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	u := Unit{
		Name:              "someapp/0",
		Type:              "django",
		Machine:           10,
		AgentState:        "started",
		MachineAgentState: "running",
		InstanceState:     "running",
	}
	a := App{
		Name:      "unspeakable",
		Framework: "vougan",
		Teams:     []string{s.team.Name},
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	a.Units = []Unit{u}
	err = db.Session.Apps().Update(bson.M{"name": a.Name}, &a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/repository/clone?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = CloneRepositoryHandler(recorder, request)
	notExpected := ".*2012-06-05 17:03:36,887 WARNING.*"
	result := strings.Replace(recorder.Body.String(), "\n", "", -1)
	c.Assert(result, Not(Matches), notExpected)
}

func (s *S) TestCloneRepositoryHandler(c *C) {
	output := `
========
pre-restart:
    pre.sh
pos-restart:
    pos.sh
`
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	u := Unit{
		Name:              "someapp/0",
		Type:              "django",
		AgentState:        "started",
		MachineAgentState: "running",
		InstanceState:     "running",
	}
	a := App{
		Name:      "someapp",
		Framework: "django",
		Teams:     []string{s.team.Name},
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	a.Units = []Unit{u}
	err = db.Session.Apps().Update(bson.M{"name": a.Name}, &a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/repository/clone?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = CloneRepositoryHandler(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 200)
	c.Assert(strings.Replace(recorder.Body.String(), "\n", "#", -1), Matches, "^# ---> Tsuru receiving push#.*")
	c.Assert(strings.Replace(recorder.Body.String(), "\n", "#", -1), Matches, ".*# ---> Clonning your code in your machines#.*")
	c.Assert(strings.Replace(recorder.Body.String(), "\n", "#", -1), Matches, ".*# ---> Parsing app.conf#.*")
	c.Assert(strings.Replace(recorder.Body.String(), "\n", "#", -1), Matches, ".*# ---> Running pre-restart#.*")
	c.Assert(strings.Replace(recorder.Body.String(), "\n", "#", -1), Matches, ".*# ---> Installing dependencies#.*")
	c.Assert(strings.Replace(recorder.Body.String(), "\n", "#", -1), Matches, ".*# ---> Restarting your app#.*")
	c.Assert(strings.Replace(recorder.Body.String(), "\n", "#", -1), Matches, ".*# ---> Running pos-restart#.*")
	c.Assert(strings.Replace(recorder.Body.String(), "\n", "#", -1), Matches, ".*# ---> Deploy done!##$.*")
	c.Assert(recorder.Header().Get("Content-Type"), Equals, "text")
}

func (s *S) TestCloneRepositoryRunsCloneOrPullThenPreRestartThenRestartThenPosRestartHooksInOrder(c *C) {
	w := new(bytes.Buffer)
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	output := `
========
pre-restart:
    pre.sh
pos-restart:
    pos.sh
`
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	u := Unit{
		Name:              "someapp/0",
		Type:              "django",
		AgentState:        "started",
		MachineAgentState: "running",
		InstanceState:     "running",
	}
	a := App{
		Name:      "someapp",
		Framework: "django",
		Teams:     []string{s.team.Name},
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	a.Units = []Unit{u}
	err = db.Session.Apps().Update(bson.M{"name": a.Name}, &a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/repository/clone?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = CloneRepositoryHandler(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 200)
	str := strings.Replace(w.String(), "\n", "", -1)
	c.Assert(str, Matches, ".*executing git clone.*executting hook dependencies.*Executing pre-restart hook.*executting hook to restarting.*Executing pos-restart hook.*")
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
	c.Assert(e, ErrorMatches, "^App not found$")
}

func (s *S) TestAppList(c *C) {
	u := Unit{Name: "app1/0", Ip: "10.10.10.10"}
	app1 := App{Name: "app1", Teams: []string{s.team.Name}, Units: []Unit{u}}
	err := createApp(&app1)
	c.Assert(err, IsNil)
	app1.Units = []Unit{u}
	err = db.Session.Apps().Update(bson.M{"name": app1.Name}, &app1)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app1.Name})
	u2 := Unit{Name: "app2/0"}
	app2 := App{
		Name:  "app2",
		Teams: []string{s.team.Name},
	}
	err = createApp(&app2)
	c.Assert(err, IsNil)
	app2.Units = []Unit{u2}
	err = db.Session.Apps().Update(bson.M{"name": app2.Name}, &app2)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app2.Name})
	expected := []App{app1, app2}

	request, err := http.NewRequest("GET", "/apps/", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = AppList(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 200)

	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)

	apps := []App{}
	err = json.Unmarshal(body, &apps)
	c.Assert(err, IsNil)
	c.Assert(len(apps), Equals, len(expected))
	for i, app := range apps {
		c.Assert(app.Name, DeepEquals, expected[i].Name)
		if app.Units[0].Ip != "" {
			c.Assert(app.Units[0].Ip, Equals, u.Ip)
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
	app1 := App{
		Name:  "app1",
		Teams: []string{s.team.Name, "angra"},
	}
	err = createApp(&app1)
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
	var apps []App
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
	myApp := App{
		Name:      "MyAppToDelete",
		Framework: "django",
		Teams:     []string{s.team.Name},
		Units: []Unit{
			{Ip: "10.10.10.10", Machine: 1},
		},
	}
	err := createApp(&myApp)
	c.Assert(err, IsNil)
	request, err := http.NewRequest("DELETE", "/apps/"+myApp.Name+"?:name="+myApp.Name, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AppDelete(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 200)
}

func (s *S) TestDeleteShouldReturnForbiddenIfTheGivenUserDoesNotHaveAccesToTheApp(c *C) {
	myApp := App{
		Name:      "MyAppToDelete",
		Framework: "django",
	}
	err := createApp(&myApp)
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
	c.Assert(e, ErrorMatches, "^App not found$")
}

func (s *S) TestDeleteAppRemovesProjectFromAllTeamsInGitosis(c *C) {
	s.addGroup()
	myApp := &App{
		Name:      "MyAppToDelete",
		Framework: "django",
		Units: []Unit{
			{Ip: "10.10.10.10", Machine: 1},
		},
	}
	_, err := createAppHelper(myApp, s.user)
	c.Assert(err, IsNil)
	request, err := http.NewRequest("DELETE", "/apps/"+myApp.Name+"?:name="+myApp.Name, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AppDelete(recorder, request, s.user)
	c.Assert(err, IsNil)
	time.Sleep(1e9)
	c.Assert("writable = "+myApp.Name, NotInGitosis)
}

func (s *S) TestDeleteReturnsErrorIfAppDestroyFails(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	myApp := App{
		Name:      "MyAppToDelete",
		Framework: "django",
		Teams:     []string{s.team.Name},
	}
	err = createApp(&myApp)
	commandmocker.Remove(dir)
	c.Assert(err, IsNil)
	request, err := http.NewRequest("DELETE", "/apps/"+myApp.Name+"?:name="+myApp.Name, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	dir, err = commandmocker.Error("juju", "$*", 1)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	err = AppDelete(recorder, request, s.user)
	c.Assert(err, NotNil)
}

func (s *S) TestAppInfo(c *C) {
	expectedApp := App{
		Name:      "NewApp",
		Framework: "django",
		Teams:     []string{s.team.Name},
	}
	err := createApp(&expectedApp)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": expectedApp.Name})

	var myApp map[string]interface{}
	request, err := http.NewRequest("GET", "/apps/"+expectedApp.Name+"?:name="+expectedApp.Name, nil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	c.Assert(err, IsNil)

	err = AppInfo(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 200)

	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)

	err = json.Unmarshal(body, &myApp)
	c.Assert(err, IsNil)
	c.Assert(myApp["Name"], Equals, expectedApp.Name)
	c.Assert(myApp["Repository"], Equals, repository.GetUrl(expectedApp.Name))
}

func (s *S) TestAppInfoReturnsForbiddenWhenTheUserDoesNotHaveAccessToTheApp(c *C) {
	expectedApp := App{
		Name:      "NewApp",
		Framework: "django",
	}
	err := createApp(&expectedApp)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": expectedApp.Name})
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
	myApp := App{Name: "SomeApp"}
	request, err := http.NewRequest("GET", "/apps/"+myApp.Name+"?:name="+myApp.Name, nil)
	c.Assert(err, IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = AppInfo(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^App not found$")
}

func (s *S) TestCreateAppHandler(c *C) {
	a := App{Name: "someApp"}
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	b := strings.NewReader(`{"name":"someApp", "framework":"django"}`)
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
	c.Assert(recorder.Code, Equals, 200)
	var gotApp App
	err = db.Session.Apps().Find(bson.M{"name": "someApp"}).One(&gotApp)
	c.Assert(err, IsNil)
	_, found := gotApp.find(&s.team)
	c.Assert(found, Equals, true)
}

func (s *S) TestCreateAppReturns403IfTheUserIsNotMemberOfAnyTeam(c *C) {
	u := &auth.User{Email: "thetrees@rush.com", Password: "123"}
	b := strings.NewReader(`{"name":"someApp", "framework":"django"}`)
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

func (s *S) TestCreateAppAddsProjectToGroupsInGitosis(c *C) {
	s.addGroup()
	app := &App{Name: "devincachu", Framework: "django"}
	_, err := createAppHelper(app, s.user)
	c.Assert(err, IsNil)
	time.Sleep(1e9)
	c.Assert("writable = "+app.Name, IsInGitosis)
}

func (s *S) TestCreateAppReturnsConflictWithProperMessageWhenTheAppAlreadyExist(c *C) {
	a := App{Name: "plainsofdawn"}
	err := createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	b := strings.NewReader(`{"name":"plainsofdawn", "framework":"django"}`)
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

func (s *S) TestAddTeamToTheApp(c *C) {
	t := auth.Team{Name: "itshardteam", Users: []string{s.user.Email}}
	err := db.Session.Teams().Insert(t)
	c.Assert(err, IsNil)
	defer db.Session.Teams().RemoveAll(bson.M{"_id": t.Name})
	a := App{
		Name:      "itshard",
		Framework: "django",
		Teams:     []string{t.Name},
	}
	err = createApp(&a)
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
	_, found := a.find(&s.team)
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
	c.Assert(e, ErrorMatches, "^App not found$")
}

func (s *S) TestGrantAccessToTeamReturn401IfTheGivenUserDoesNotHasAccessToTheApp(c *C) {
	a := App{
		Name:      "itshard",
		Framework: "django",
	}
	err := createApp(&a)
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
	c.Assert(e.Code, Equals, http.StatusUnauthorized)
	c.Assert(e, ErrorMatches, "^User unauthorized$")
}

func (s *S) TestGrantAccessToTeamReturn404IfTheTeamDoesNotExist(c *C) {
	a := App{
		Name:      "itshard",
		Framework: "django",
		Teams:     []string{s.team.Name},
	}
	err := createApp(&a)
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
	a := App{
		Name:      "itshard",
		Framework: "django",
		Teams:     []string{s.team.Name},
	}
	err := createApp(&a)
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

func (s *S) TestGrantAccessToAppAddsTheProjectInGitosis(c *C) {
	t := &auth.Team{Name: "anything", Users: []string{s.user.Email}}
	err := db.Session.Teams().Insert(t)
	c.Assert(err, IsNil)
	defer db.Session.Teams().Remove(bson.M{"_id": t.Name})
	s.addGroup()
	a := App{
		Name:      "tsuru",
		Framework: "golang",
		Teams:     []string{t.Name},
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	err = grantAccessToTeam(a.Name, s.team.Name, s.user)
	c.Assert(err, IsNil)
	time.Sleep(1e9)
	c.Assert("writable = "+a.Name, IsInGitosis)
}

func (s *S) TestRevokeAccessFromTeam(c *C) {
	t := auth.Team{Name: "abcd"}
	err := db.Session.Teams().Insert(t)
	c.Assert(err, IsNil)
	defer db.Session.Teams().Remove(bson.M{"_id": t.Name})
	a := App{
		Name:      "itshard",
		Framework: "django",
		Teams:     []string{"abcd", s.team.Name},
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	url := fmt.Sprintf("/apps/%s/%s?:app=%s&:team=%s", a.Name, s.team.Name, a.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RevokeAccessFromTeamHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	a.Get()
	_, found := a.find(&s.team)
	c.Assert(found, Equals, false)
}

func (s *S) TestRevokeAccessFromTeamReturn404IfTheAppDoesNotExist(c *C) {
	request, err := http.NewRequest("DELETE", "/apps/a/b?:app=a&:team=b", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RevokeAccessFromTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^App not found$")
}

func (s *S) TestRevokeAccessFromTeamReturn401IfTheGivenUserDoesNotHavePermissionInTheApp(c *C) {
	a := App{
		Name:      "itshard",
		Framework: "django",
	}
	err := createApp(&a)
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
	c.Assert(e.Code, Equals, http.StatusUnauthorized)
	c.Assert(e, ErrorMatches, "^User unauthorized$")
}

func (s *S) TestRevokeAccessFromTeamReturn404IfTheTeamDoesNotExist(c *C) {
	a := App{
		Name:      "itshard",
		Framework: "django",
		Teams:     []string{s.team.Name},
	}
	err := createApp(&a)
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
	db.Session.Teams().Insert(t)
	t2 := auth.Team{Name: "team2"}
	db.Session.Teams().Insert(t2)
	defer db.Session.Teams().Remove(bson.M{"_id": bson.M{"$in": []string{"blaaa", "team2"}}})
	a := App{
		Name:      "itshard",
		Framework: "django",
		Teams:     []string{s.team.Name, t2.Name},
	}
	err := createApp(&a)
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
	a := App{
		Name:      "itshard",
		Framework: "django",
		Teams:     []string{s.team.Name},
	}
	err := createApp(&a)
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

func (s *S) TestRevokeAccessFromTeamRemovesTheProjectFromGitosisConf(c *C) {
	t := auth.Team{Name: "anything", Users: []string{s.user.Email}}
	err := db.Session.Teams().Insert(t)
	c.Assert(err, IsNil)
	defer db.Session.Teams().Remove(bson.M{"_id": t.Name})
	s.addGroup()
	a := App{
		Name:      "tsuru",
		Framework: "golang",
		Teams:     []string{t.Name},
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	err = grantAccessToTeam(a.Name, s.team.Name, s.user)
	c.Assert(err, IsNil)
	time.Sleep(1e9)
	err = revokeAccessFromTeam(a.Name, s.team.Name, s.user)
	time.Sleep(1e9)
	c.Assert("writable = "+a.Name, NotInGitosis)
}

func (s *S) TestRunHandlerShouldExecuteTheGivenCommandInTheGivenApp(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	u := Unit{
		Name:              "someapp/0",
		Type:              "django",
		Machine:           10,
		AgentState:        "started",
		MachineAgentState: "running",
		InstanceState:     "running",
	}
	a := App{
		Name:      "secrets",
		Framework: "arch enemy",
		Teams:     []string{s.team.Name},
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	a.Units = []Unit{u}
	err = db.Session.Apps().Update(bson.M{"name": a.Name}, &a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/run/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("ls"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RunCommand(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.String(), Equals, "ssh -o StrictHostKeyChecking no -q 10 [ -f /home/application/apprc ] && source /home/application/apprc; [ -d /home/application/current ] && cd /home/application/current; ls")
}

func (s *S) TestRunHandlerReturnsTheOutputOfTheCommandEvenIfItFails(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	u := Unit{
		Name:              "someapp/0",
		Type:              "django",
		Machine:           10,
		AgentState:        "started",
		MachineAgentState: "running",
		InstanceState:     "running",
	}
	a := App{
		Name:      "secrets",
		Framework: "arch enemy",
		Teams:     []string{s.team.Name},
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	a.Units = []Unit{u}
	err = db.Session.Apps().Update(bson.M{"name": a.Name}, &a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/run/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("ls"))
	c.Assert(err, IsNil)
	errdir, err := commandmocker.Error("juju", "juju failed", 1)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(errdir)
	recorder := httptest.NewRecorder()
	err = RunCommand(recorder, request, s.user)
	c.Assert(err, NotNil)
	c.Assert(recorder.Body.String(), Equals, "juju failed")
}

func (s *S) TestRunHandlerShouldFilterOutputFromJuju(c *C) {
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	u := Unit{
		Name:              "someapp/0",
		Type:              "django",
		Machine:           10,
		AgentState:        "started",
		MachineAgentState: "running",
		InstanceState:     "running",
	}
	a := App{
		Name:      "unspeakable",
		Framework: "vougan",
		Teams:     []string{s.team.Name},
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	a.Units = []Unit{u}
	err = db.Session.Apps().Update(bson.M{"name": a.Name}, &a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/run/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("ls"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RunCommand(recorder, request, s.user)
	expected := `export DATABASE_HOST=localhost
export DATABASE_USER=root
export DATABASE_PASSWORD=secret`
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.String(), Equals, expected)
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
	c.Assert(e, ErrorMatches, "^App not found$")
}

func (s *S) TestRunHandlerReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *C) {
	a := App{
		Name:      "secrets",
		Framework: "arch enemy",
	}
	err := createApp(&a)
	c.Assert(err, IsNil)
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
	a := App{
		Name:      "everything-i-want",
		Framework: "gotthard",
		Teams:     []string{s.team.Name},
	}
	err := createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	a.Env = map[string]bind.EnvVar{
		"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
		"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
		"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
	}
	err = db.Session.Apps().Update(bson.M{"name": a.Name}, a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/env/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, strings.NewReader("DATABASE_HOST"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.String(), Equals, "DATABASE_HOST=localhost\n")
}

func (s *S) TestGetEnvHandlerShouldAcceptMultipleVariables(c *C) {
	a := App{
		Name:  "four-sticks",
		Teams: []string{s.team.Name},
	}
	err := createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	a.Env = map[string]bind.EnvVar{
		"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
		"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
		"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
	}
	err = db.Session.Apps().Update(bson.M{"name": a.Name}, a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/env/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, strings.NewReader("DATABASE_HOST DATABASE_USER"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.String(), Equals, "DATABASE_HOST=localhost\nDATABASE_USER=root\n")
}

func (s *S) TestGetEnvHandlerReturnsAllVariablesIfEnvironmentVariablesAreMissingWithMaskOnPrivateVars(c *C) {
	a := App{
		Name:      "time",
		Framework: "pink-floyd",
		Teams:     []string{s.team.Name},
	}
	err := createApp(&a)
	c.Assert(err, IsNil)
	a.Env = map[string]bind.EnvVar{
		"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
		"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
		"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
	}
	err = db.Session.Apps().Update(bson.M{"name": "time"}, a)
	c.Assert(err, IsNil)
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
	c.Assert(e, ErrorMatches, "^App not found$")
}

func (s *S) TestGetEnvHandlerReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *C) {
	a := App{
		Name:      "lost",
		Framework: "vougan",
	}
	err := createApp(&a)
	c.Assert(err, IsNil)
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

func (s *S) TestSetEnvRespectsThePublicOnlyFlagKeepPrivateVariablesWhenItsTrue(c *C) {
	a := App{Name: "myapp"}
	err := createApp(&a)
	c.Assert(err, IsNil)
	a.Env = map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:   "DATABASE_HOST",
			Value:  "localhost",
			Public: false,
		},
	}
	err = db.Session.Apps().Update(bson.M{"name": "myapp"}, a)
	c.Assert(err, IsNil)
	envs := []bind.EnvVar{
		{
			Name:   "DATABASE_HOST",
			Value:  "remotehost",
			Public: false,
		},
		{
			Name:   "DATABASE_PASSWORD",
			Value:  "123",
			Public: true,
		},
	}
	err = setEnvsToApp(&a, envs, true)
	c.Assert(err, IsNil)
	newApp := App{Name: a.Name}
	err = newApp.Get()
	c.Assert(err, IsNil)
	expected := map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:   "DATABASE_HOST",
			Value:  "localhost",
			Public: false,
		},
		"DATABASE_PASSWORD": {
			Name:   "DATABASE_PASSWORD",
			Value:  "123",
			Public: true,
		},
	}
	c.Assert(newApp.Env, DeepEquals, expected)
}

func (s *S) TestSetEnvRespectsThePublicOnlyFlagOverwrittenAllVariablesWhenItsFalse(c *C) {
	a := App{Name: "myapp"}
	err := createApp(&a)
	c.Assert(err, IsNil)
	a.Env = map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:   "DATABASE_HOST",
			Value:  "localhost",
			Public: false,
		},
	}
	err = db.Session.Apps().Update(bson.M{"name": "myapp"}, a)
	c.Assert(err, IsNil)
	envs := []bind.EnvVar{
		{
			Name:   "DATABASE_HOST",
			Value:  "remotehost",
			Public: true,
		},
		{
			Name:   "DATABASE_PASSWORD",
			Value:  "123",
			Public: true,
		},
	}
	err = setEnvsToApp(&a, envs, false)
	c.Assert(err, IsNil)
	newApp := App{Name: a.Name}
	err = newApp.Get()
	c.Assert(err, IsNil)
	expected := map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:   "DATABASE_HOST",
			Value:  "remotehost",
			Public: true,
		},
		"DATABASE_PASSWORD": {
			Name:   "DATABASE_PASSWORD",
			Value:  "123",
			Public: true,
		},
	}
	c.Assert(newApp.Env, DeepEquals, expected)
}

func (s *S) TestSetEnvHandlerShouldSetAPublicEnvironmentVariableInTheApp(c *C) {
	a := App{
		Name:  "black-dog",
		Teams: []string{s.team.Name},
	}
	err := createApp(&a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/env?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("DATABASE_HOST=localhost"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = SetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := &App{Name: "black-dog"}
	err = app.Get()
	c.Assert(err, IsNil)
	expected := map[string]bind.EnvVar{
		"DATABASE_HOST": {Name: "DATABASE_HOST", Value: "localhost", Public: true},
	}
	c.Assert(app.Env, DeepEquals, expected)
}

func (s *S) TestSetEnvHandlerShouldSetMultipleEnvironmentVariablesInTheApp(c *C) {
	a := App{
		Name:  "vigil",
		Teams: []string{s.team.Name},
	}
	err := createApp(&a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/env?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("DATABASE_HOST=localhost DATABASE_USER=root"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = SetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := &App{Name: "vigil"}
	err = app.Get()
	c.Assert(err, IsNil)
	expected := map[string]bind.EnvVar{
		"DATABASE_HOST": {Name: "DATABASE_HOST", Value: "localhost", Public: true},
		"DATABASE_USER": {Name: "DATABASE_USER", Value: "root", Public: true},
	}
	c.Assert(app.Env, DeepEquals, expected)
}

func (s *S) TestSetEnvHandlerShouldSupportSpacesInTheEnvironmentVariableValue(c *C) {
	a := App{
		Name:  "loser",
		Teams: []string{s.team.Name},
	}
	err := createApp(&a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/env?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("DATABASE_HOST=local host DATABASE_USER=root"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = SetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := &App{Name: "loser"}
	err = app.Get()
	c.Assert(err, IsNil)
	expected := map[string]bind.EnvVar{
		"DATABASE_HOST": {Name: "DATABASE_HOST", Value: "local host", Public: true},
		"DATABASE_USER": {Name: "DATABASE_USER", Value: "root", Public: true},
	}
	c.Assert(app.Env, DeepEquals, expected)
}

func (s *S) TestSetEnvHandlerShouldSupportValuesWithDot(c *C) {
	a := App{
		Name:  "losers",
		Teams: []string{s.team.Name},
	}
	err := createApp(&a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/env?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("DATABASE_HOST=http://foo.com:8080"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = SetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := &App{Name: "losers"}
	err = app.Get()
	c.Assert(err, IsNil)
	expected := map[string]bind.EnvVar{
		"DATABASE_HOST": {Name: "DATABASE_HOST", Value: "http://foo.com:8080", Public: true},
	}
	c.Assert(app.Env, DeepEquals, expected)
}

func (s *S) TestSetEnvHandlerShouldSupportNumbersOnVariableName(c *C) {
	a := App{
		Name:  "blinded",
		Teams: []string{s.team.Name},
	}
	err := createApp(&a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/env?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("EC2_HOST=http://foo.com:8080"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = SetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := &App{Name: "blinded"}
	err = app.Get()
	c.Assert(err, IsNil)
	expected := map[string]bind.EnvVar{
		"EC2_HOST": {Name: "EC2_HOST", Value: "http://foo.com:8080", Public: true},
	}
	c.Assert(app.Env, DeepEquals, expected)
}

func (s *S) TestSetEnvHandlerShouldSupportLowerCasedVariableName(c *C) {
	a := App{
		Name:  "fragments",
		Teams: []string{s.team.Name},
	}
	err := createApp(&a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/env?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("http_proxy=http://my_proxy.com:3128"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = SetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := &App{Name: "fragments"}
	err = app.Get()
	c.Assert(err, IsNil)
	expected := map[string]bind.EnvVar{
		"http_proxy": {Name: "http_proxy", Value: "http://my_proxy.com:3128", Public: true},
	}
	c.Assert(app.Env, DeepEquals, expected)
}

func (s *S) TestSetEnvHandlerShouldNotChangeValueOfPrivateVariables(c *C) {
	original := map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:   "DATABASE_HOST",
			Value:  "privatehost.com",
			Public: false,
		},
	}
	a := App{
		Name:  "losers",
		Teams: []string{s.team.Name},
	}
	err := createApp(&a)
	c.Assert(err, IsNil)
	a.Env = original
	err = db.Session.Apps().Update(bson.M{"name": "losers"}, a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/env?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("DATABASE_HOST=http://foo.com:8080"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = SetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := &App{Name: "losers"}
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
	c.Assert(e, ErrorMatches, "^App not found$")
}

func (s *S) TestSetEnvHandlerReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *C) {
	a := App{Name: "rock-and-roll"}
	err := createApp(&a)
	c.Assert(err, IsNil)
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
	a := App{
		Name:  "swift",
		Teams: []string{s.team.Name},
	}
	err := createApp(&a)
	c.Assert(err, IsNil)
	a.Env = map[string]bind.EnvVar{
		"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
		"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
		"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
	}
	err = db.Session.Apps().Update(bson.M{"name": "swift"}, a)
	c.Assert(err, IsNil)
	expected := a.Env
	delete(expected, "DATABASE_HOST")
	url := fmt.Sprintf("/apps/%s/env/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("DELETE", url, strings.NewReader("DATABASE_HOST"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = UnsetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := App{Name: "swift"}
	err = app.Get()
	c.Assert(err, IsNil)
	c.Assert(app.Env, DeepEquals, expected)
}

func (s *S) TestUnsetEnvHandlerRemovesAllGivenEnvironmentVariables(c *C) {
	a := App{
		Name:  "let-it-be",
		Teams: []string{s.team.Name},
	}
	err := createApp(&a)
	c.Assert(err, IsNil)
	a.Env = map[string]bind.EnvVar{
		"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
		"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
		"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
	}
	err = db.Session.Apps().Update(bson.M{"name": "let-it-be"}, a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/env/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("DELETE", url, strings.NewReader("DATABASE_HOST DATABASE_USER"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = UnsetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := App{Name: "let-it-be"}
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
	a := App{
		Name:  "let-it-be",
		Teams: []string{s.team.Name},
	}
	err := createApp(&a)
	c.Assert(err, IsNil)
	a.Env = map[string]bind.EnvVar{
		"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
		"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
		"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
	}
	err = db.Session.Apps().Update(bson.M{"name": "let-it-be"}, a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/env/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("DELETE", url, strings.NewReader("DATABASE_HOST DATABASE_USER DATABASE_PASSWORD"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = UnsetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := App{Name: "let-it-be"}
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

func (s *S) TestUnsetEnvRespectsThePublicOnlyFlagKeepPrivateVariablesWhenItsTrue(c *C) {
	a := App{Name: "myapp"}
	err := createApp(&a)
	c.Assert(err, IsNil)
	a.Env = map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:   "DATABASE_HOST",
			Value:  "localhost",
			Public: false,
		},
		"DATABASE_PASSWORD": {
			Name:   "DATABASE_PASSWORD",
			Value:  "123",
			Public: true,
		},
	}
	err = db.Session.Apps().Update(bson.M{"name": "myapp"}, a)
	c.Assert(err, IsNil)
	err = unsetEnvFromApp(&a, []string{"DATABASE_HOST", "DATABASE_PASSWORD"}, true)
	c.Assert(err, IsNil)
	newApp := App{Name: a.Name}
	err = newApp.Get()
	c.Assert(err, IsNil)
	expected := map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:   "DATABASE_HOST",
			Value:  "localhost",
			Public: false,
		},
	}
	c.Assert(newApp.Env, DeepEquals, expected)
}

func (s *S) TestUnsetEnvRespectsThePublicOnlyFlagUnsettingAllVariablesWhenItsFalse(c *C) {
	a := App{Name: "myapp"}
	err := createApp(&a)
	c.Assert(err, IsNil)
	a.Env = map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:   "DATABASE_HOST",
			Value:  "localhost",
			Public: false,
		},
		"DATABASE_PASSWORD": {
			Name:   "DATABASE_PASSWORD",
			Value:  "123",
			Public: true,
		},
	}
	err = db.Session.Apps().Update(bson.M{"name": "myapp"}, a)
	c.Assert(err, IsNil)
	err = unsetEnvFromApp(&a, []string{"DATABASE_HOST", "DATABASE_PASSWORD"}, false)
	c.Assert(err, IsNil)
	newApp := App{Name: a.Name}
	err = newApp.Get()
	c.Assert(err, IsNil)
	c.Assert(newApp.Env, DeepEquals, map[string]bind.EnvVar{})
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
	c.Assert(e, ErrorMatches, "^App not found$")
}

func (s *S) TestUnsetEnvHandlerReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *C) {
	a := App{Name: "mountain-mama"}
	err := createApp(&a)
	c.Assert(err, IsNil)
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
	request, err := http.NewRequest("GET", "/apps/unknown/log/", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AppLog(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^App not found$")
}

func (s *S) TestLogReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *C) {
	a := App{
		Name:      "lost",
		Framework: "vougan",
	}
	err := createApp(&a)
	c.Assert(err, IsNil)
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

func (s *S) TestLogShouldAppLog(c *C) {
	a := App{
		Name:      "lost",
		Framework: "vougan",
		Teams:     []string{s.team.Name},
	}
	err := createApp(&a)
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/log/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = AppLog(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 200)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	logs := []applog{}
	err = json.Unmarshal(body, &logs)
	c.Assert(err, IsNil)
	c.Assert(logs, DeepEquals, logs)
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
	a := App{
		Name:  "painkiller",
		Teams: []string{s.team.Name},
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	a.Units = []Unit{{Ip: "127.0.0.1"}}
	err = db.Session.Apps().Update(bson.M{"name": "painkiller"}, a)
	c.Assert(err, IsNil)
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
	expectedEnv := map[string]bind.EnvVar{
		"DATABASE_USER": {
			Name:         "DATABASE_USER",
			Value:        "root",
			Public:       false,
			InstanceName: instance.Name,
		},
		"DATABASE_PASSWORD": {
			Name:         "DATABASE_PASSWORD",
			Value:        "s3cr3t",
			Public:       false,
			InstanceName: instance.Name,
		},
	}
	c.Assert(a.Env, DeepEquals, expectedEnv)
	var envs []string
	err = json.Unmarshal(recorder.Body.Bytes(), &envs)
	c.Assert(err, IsNil)
	sort.Strings(envs)
	c.Assert(envs, DeepEquals, []string{"DATABASE_PASSWORD", "DATABASE_USER"})
	c.Assert(recorder.Header().Get("Content-Type"), Equals, "application/json")
}

func (s *S) TestBindHandlerReturns404IfTheInstanceDoesNotExist(c *C) {
	a := App{
		Name:      "serviceApp",
		Framework: "django",
		Teams:     []string{s.team.Name},
	}
	err := createApp(&a)
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
	a := App{
		Name:      "serviceApp",
		Framework: "django",
		Teams:     []string{s.team.Name},
	}
	err = createApp(&a)
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
	url := fmt.Sprintf("/services/instances/%s/unknown?:instance=%s&app=unknown", instance.Name, instance.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = BindHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^App not found$")
}

func (s *S) TestBindHandlerReturns403IfTheUserDoesNotHaveAccessToTheApp(c *C) {
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err := instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a := App{
		Name:      "serviceApp",
		Framework: "django",
	}
	err = createApp(&a)
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
	var called bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/hostname/127.0.0.1"
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
	a := App{
		Name:  "painkiller",
		Teams: []string{s.team.Name},
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	a.Env = map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:         "DATABASE_HOST",
			Value:        "arrea",
			Public:       false,
			InstanceName: instance.Name,
		},
		"MY_VAR": {
			Name:  "MY_VAR",
			Value: "123",
		},
	}
	a.Units = []Unit{{Ip: "127.0.0.1"}}
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
	expected := map[string]bind.EnvVar{
		"MY_VAR": {
			Name:  "MY_VAR",
			Value: "123",
		},
	}
	c.Assert(a.Env, DeepEquals, expected)
	ch := make(chan bool)
	go func() {
		t := time.Tick(1)
		for _ = <-t; !called; _ = <-t {
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
	a := App{
		Name:      "serviceApp",
		Framework: "django",
		Teams:     []string{s.team.Name},
	}
	err := createApp(&a)
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
	a := App{
		Name:      "serviceApp",
		Framework: "django",
		Teams:     []string{s.team.Name},
	}
	err = createApp(&a)
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
	url := fmt.Sprintf("/services/instances/%s/unknown?:instance=%s&app=unknown", instance.Name, instance.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = UnbindHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^App not found$")
}

func (s *S) TestUnbindHandlerReturns403IfTheUserDoesNotHaveAccessToTheApp(c *C) {
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err := instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a := App{
		Name:      "serviceApp",
		Framework: "django",
	}
	err = createApp(&a)
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
	tmpdir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	a := App{
		Name:  "stress",
		Teams: []string{s.team.Name},
	}
	err = createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	a.Units = []Unit{
		{AgentState: "started", MachineAgentState: "running", InstanceState: "running", Machine: 10, Ip: "20.20.20.20"},
	}
	db.Session.Apps().Update(bson.M{"name": a.Name}, &a)
	url := fmt.Sprintf("/apps/%s/restart?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RestartHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	expected := fmt.Sprintf("ssh -o StrictHostKeyChecking no -q %d /var/lib/tsuru/hooks/restart", a.unit().Machine)
	c.Assert(string(b), Equals, expected)
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
	a := App{Name: "nightmist"}
	err := createApp(&a)
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

func (s *S) TestRestartHandlerReturns412IfTheUnitOfTheAppDoesNotHaveIp(c *C) {
	a := App{
		Name:  "stress",
		Teams: []string{s.team.Name},
	}
	err := createApp(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	a.Units = []Unit{{Ip: "", Machine: 10}}
	db.Session.Apps().Update(bson.M{"name": a.Name}, &a)
	url := fmt.Sprintf("/apps/%s/restart?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RestartHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusPreconditionFailed)
	c.Assert(e.Message, Equals, "You can't restart this app because it doesn't have an IP yet.")
}
