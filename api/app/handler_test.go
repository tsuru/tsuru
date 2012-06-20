package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/unit"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/errors"
	"github.com/timeredbull/tsuru/log"
	"github.com/timeredbull/tsuru/repository"
	"io"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/mgo/bson"
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

func (s *S) TestCloneRepositoryHandler(c *C) {
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
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
	u := unit.Unit{Name: "someapp/0", Type: "django"}
	a := App{Name: "someApp", Framework: "django", Teams: []auth.Team{s.team}, Units: []unit.Unit{u}}
	err = a.Create()
	c.Assert(err, IsNil)
	defer a.Destroy()
	url := fmt.Sprintf("/apps/%s/repository/clone?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = CloneRepositoryHandler(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 200)
	c.Assert(recorder.Body.String(), Not(Equals), "success")
}

func (s *S) TestCloneRepositoryRunsCloneOrPullThenPreRestartThenRestartThenPosRestartHooksInOrder(c *C) {
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
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
	u := unit.Unit{Name: "someapp/0", Type: "django"}
	a := App{Name: "someApp", Framework: "django", Teams: []auth.Team{s.team}, Units: []unit.Unit{u}}
	err = a.Create()
	c.Assert(err, IsNil)
	defer a.Destroy()
	url := fmt.Sprintf("/apps/%s/repository/clone?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = CloneRepositoryHandler(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 200)
	str := w.String()
	cloneIndex := strings.Index(str, "git clone")
	c.Assert(cloneIndex, Not(Equals), -1)
	reloadIndex := strings.Index(str, "reload-gunicorn")
	c.Assert(reloadIndex, Not(Equals), -1)
	preRstIndex := strings.Index(str, "pre-restart hook")
	c.Assert(preRstIndex, Not(Equals), -1)
	posRstIndex := strings.Index(str, "pos-restart hook")
	c.Assert(posRstIndex, Not(Equals), -1)
	c.Assert(preRstIndex, Greater, cloneIndex)  // clone/pull runs before pre-restart
	c.Assert(reloadIndex, Greater, preRstIndex) // pre-restart runs before reload
	c.Assert(posRstIndex, Greater, reloadIndex) // pos-restart runs after reload
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
	apps := []App{}
	expected := []App{}
	app1 := App{Name: "app1", Teams: []auth.Team{s.team}}
	app1.Create()
	defer app1.Destroy()
	expected = append(expected, app1)
	app2 := App{Name: "app2", Teams: []auth.Team{s.team}}
	app2.Create()
	defer app2.Destroy()
	expected = append(expected, app2)
	app3 := App{Name: "app3", Framework: "django", Teams: []auth.Team{}}
	app3.Create()
	defer app3.Destroy()
	expected = append(expected)

	request, err := http.NewRequest("GET", "/apps/", nil)
	c.Assert(err, IsNil)

	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = AppList(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 200)

	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)

	err = json.Unmarshal(body, &apps)
	c.Assert(err, IsNil)
	c.Assert(len(apps), Equals, len(expected))
	for i, app := range apps {
		c.Assert(app.Name, DeepEquals, expected[i].Name)
	}
}

func (s *S) TestListShouldReturnStatusNoContentWhenAppListIsNil(c *C) {
	err := db.Session.Apps().RemoveAll(nil)
	c.Assert(err, IsNil)
	request, err := http.NewRequest("GET", "/apps/", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = AppList(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, http.StatusNoContent)
}

func (s *S) TestDelete(c *C) {
	myApp := App{Name: "MyAppToDelete", Framework: "django", Teams: []auth.Team{s.team}}
	myApp.Create()
	request, err := http.NewRequest("DELETE", "/apps/"+myApp.Name+"?:name="+myApp.Name, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AppDelete(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 200)
}

func (s *S) TestDeleteShouldReturnForbiddenIfTheGivenUserDoesNotHaveAccesToTheapp(c *C) {
	myApp := App{Name: "MyAppToDelete", Framework: "django"}
	myApp.Create()
	defer myApp.Destroy()
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
	myApp := &App{Name: "MyAppToDelete", Framework: "django"}
	_, err := createApp(myApp, s.user)
	c.Assert(err, IsNil)
	request, err := http.NewRequest("DELETE", "/apps/"+myApp.Name+"?:name="+myApp.Name, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AppDelete(recorder, request, s.user)
	c.Assert(err, IsNil)
	time.Sleep(1e9)
	c.Assert("writable = "+myApp.Name, NotInGitosis)
}

func (s *S) TestAppInfo(c *C) {
	expectedApp := App{Name: "NewApp", Framework: "django", Teams: []auth.Team{s.team}}
	expectedApp.Create()
	defer expectedApp.Destroy()

	var myApp App
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
	c.Assert(myApp.Name, Equals, expectedApp.Name)
}

func (s *S) TestAppInfoReturnsForbiddenWhenTheUserDoesNotHaveAccessToTheApp(c *C) {
	expectedApp := App{Name: "NewApp", Framework: "django", Teams: []auth.Team{}}
	expectedApp.Create()
	defer expectedApp.Destroy()
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

func (s *S) TestCreateApp(c *C) {
	a := App{Name: "someApp"}
	defer a.Destroy()

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
	c.Assert(s.team, HasAccessTo, gotApp)
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
	_, err := createApp(app, s.user)
	c.Assert(err, IsNil)
	time.Sleep(1e9)
	c.Assert("writable = "+app.Name, IsInGitosis)
}

func (s *S) TestAddTeamToTheApp(c *C) {
	t := auth.Team{Name: "itshardteam", Users: []*auth.User{s.user}}
	a := App{Name: "itshard", Framework: "django", Teams: []auth.Team{t}}
	err := a.Create()
	c.Assert(err, IsNil)
	defer a.Destroy()
	url := fmt.Sprintf("/apps/%s/%s?:app=%s&:team=%s", a.Name, s.team.Name, a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GrantAccessToTeamHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	err = a.Get()
	c.Assert(err, IsNil)
	c.Assert(s.team, HasAccessTo, a)
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
	a := App{Name: "itshard", Framework: "django"}
	err := a.Create()
	c.Assert(err, IsNil)
	defer a.Destroy()
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
	a := App{Name: "itshard", Framework: "django", Teams: []auth.Team{s.team}}
	err := a.Create()
	c.Assert(err, IsNil)
	defer a.Destroy()
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
	a := App{Name: "itshard", Framework: "django", Teams: []auth.Team{s.team}}
	err := a.Create()
	c.Assert(err, IsNil)
	defer a.Destroy()
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
	t := &auth.Team{Name: "anything", Users: []*auth.User{s.user}}
	s.addGroup()
	a := &App{Name: "tsuru", Framework: "golang", Teams: []auth.Team{*t}}
	err := a.Create()
	c.Assert(err, IsNil)
	err = grantAccessToTeam(a.Name, s.team.Name, s.user)
	c.Assert(err, IsNil)
	time.Sleep(1e9)
	c.Assert("writable = "+a.Name, IsInGitosis)
}

func (s *S) TestRevokeAccessFromTeam(c *C) {
	t := auth.Team{Name: "abcd"}
	a := App{Name: "itshard", Framework: "django", Teams: []auth.Team{s.team, t}}
	err := a.Create()
	c.Assert(err, IsNil)
	defer a.Destroy()
	url := fmt.Sprintf("/apps/%s/%s?:app=%s&:team=%s", a.Name, s.team.Name, a.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RevokeAccessFromTeamHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	a.Get()
	c.Assert(s.team, Not(HasAccessTo), a)
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
	a := App{Name: "itshard", Framework: "django"}
	err := a.Create()
	c.Assert(err, IsNil)
	defer a.Destroy()
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
	a := App{Name: "itshard", Framework: "django", Teams: []auth.Team{s.team}}
	err := a.Create()
	c.Assert(err, IsNil)
	defer a.Destroy()
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
	defer db.Session.Teams().Remove(bson.M{"name": "blaaa"})
	a := App{Name: "itshard", Framework: "django", Teams: []auth.Team{s.team, t2}}
	err := a.Create()
	c.Assert(err, IsNil)
	defer a.Destroy()
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
	a := App{Name: "itshard", Framework: "django", Teams: []auth.Team{s.team}}
	err := a.Create()
	c.Assert(err, IsNil)
	defer a.Destroy()
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
	t := auth.Team{Name: "anything", Users: []*auth.User{s.user}}
	s.addGroup()
	a := &App{Name: "tsuru", Framework: "golang", Teams: []auth.Team{t}}
	err := a.Create()
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
	u := unit.Unit{Name: "someapp/0", Type: "django", Machine: 10}
	a := &App{Name: "secrets", Framework: "arch enemy", Teams: []auth.Team{s.team}, Units: []unit.Unit{u}}
	err = a.Create()
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/run/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("ls"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RunCommand(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.String(), Equals, "ssh -o StrictHostKeyChecking no 10 cd /home/application/current; ls")
}

func (s *S) TestRunHandlerShouldFilterOutputFromJuju(c *C) {
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	u := unit.Unit{Name: "someapp/0", Type: "django", Machine: 10}
	a := &App{Name: "unspeakable", Framework: "vougan", Teams: []auth.Team{s.team}, Units: []unit.Unit{u}}
	err = a.Create()
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
	a := &App{Name: "secrets", Framework: "arch enemy"}
	err := a.Create()
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
	a := &App{Name: "everything-i-want", Framework: "gotthard", Teams: []auth.Team{s.team}}
	a.Env = map[string]string{
		"DATABASE_HOST":     "localhost",
		"DATABASE_USER":     "root",
		"DATABASE_PASSWORD": "secret",
	}
	err := a.Create()
	c.Assert(err, IsNil)
	defer a.Destroy()
	url := fmt.Sprintf("/apps/%s/env/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, strings.NewReader("DATABASE_HOST"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.String(), Equals, "DATABASE_HOST=localhost\n")
}

func (s *S) TestGetEnvHandlerShouldAcceptMultipleVariables(c *C) {
	a := &App{Name: "four-sticks", Teams: []auth.Team{s.team}}
	a.Env = map[string]string{
		"DATABASE_HOST":     "localhost",
		"DATABASE_USER":     "root",
		"DATABASE_PASSWORD": "secret",
	}
	err := a.Create()
	c.Assert(err, IsNil)
	defer a.Destroy()
	url := fmt.Sprintf("/apps/%s/env/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, strings.NewReader("DATABASE_HOST DATABASE_USER"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.String(), Equals, "DATABASE_HOST=localhost\nDATABASE_USER=root\n")
}

func (s *S) TestGetEnvHandlerReturnsAllVariablesIfEnvironmentVariablesAreMissing(c *C) {
	a := &App{Name: "time", Framework: "pink-floyd", Teams: []auth.Team{s.team}}
	a.Env = map[string]string{
		"DATABASE_HOST":     "localhost",
		"DATABASE_USER":     "root",
		"DATABASE_PASSWORD": "secret",
	}
	err := a.Create()
	c.Assert(err, IsNil)
	expected := []string{"", "DATABASE_HOST=localhost", "DATABASE_PASSWORD=secret", "DATABASE_USER=root"}
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
	a := &App{Name: "lost", Framework: "vougan"}
	err := a.Create()
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

func (s *S) TestSetEnvHandlerShouldSetAnEnvironmentVariableInTheApp(c *C) {
	a := &App{Name: "black-dog", Teams: []auth.Team{s.team}}
	err := a.Create()
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
	expected := map[string]string{
		"DATABASE_HOST": "localhost",
	}
	c.Assert(app.Env, DeepEquals, expected)
}

func (s *S) TestSetEnvHandlerShouldSetMultipleEnvironmentVariablesInTheApp(c *C) {
	a := &App{Name: "vigil", Teams: []auth.Team{s.team}}
	err := a.Create()
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
	expected := map[string]string{
		"DATABASE_HOST": "localhost",
		"DATABASE_USER": "root",
	}
	c.Assert(app.Env, DeepEquals, expected)
}

func (s *S) TestSetEnvHandlerShouldSupportSpacesInTheEnvironmentVariableValue(c *C) {
	a := &App{Name: "loser", Teams: []auth.Team{s.team}}
	err := a.Create()
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
	expected := map[string]string{
		"DATABASE_HOST": "local host",
		"DATABASE_USER": "root",
	}
	c.Assert(app.Env, DeepEquals, expected)
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
	a := &App{Name: "rock-and-roll"}
	err := a.Create()
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
	a := &App{
		Name:  "swift",
		Env:   map[string]string{"DATABASE_HOST": "localhost", "DATABASE_PASSWORD": "123"},
		Teams: []auth.Team{s.team},
	}
	err := a.Create()
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/env/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("DELETE", url, strings.NewReader("DATABASE_HOST"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = UnsetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := App{Name: "swift"}
	err = app.Get()
	c.Assert(err, IsNil)
	c.Assert(app.Env, DeepEquals, map[string]string{"DATABASE_PASSWORD": "123"})
}

func (s *S) TestUnsetEnvHandlerRemovesAllGivenEnvironmentVariables(c *C) {
	a := &App{
		Name:  "let-it-be",
		Env:   map[string]string{"DATABASE_HOST": "localhost", "DATABASE_USER": "root", "DATABASE_PASSWORD": "123"},
		Teams: []auth.Team{s.team},
	}
	err := a.Create()
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/apps/%s/env/?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("DELETE", url, strings.NewReader("DATABASE_HOST DATABASE_USER"))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = UnsetEnv(recorder, request, s.user)
	c.Assert(err, IsNil)
	app := App{Name: "let-it-be"}
	err = app.Get()
	c.Assert(err, IsNil)
	c.Assert(app.Env, DeepEquals, map[string]string{"DATABASE_PASSWORD": "123"})
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
	a := &App{Name: "mountain-mama"}
	err := a.Create()
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
