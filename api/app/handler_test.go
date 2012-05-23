package app

import (
	"encoding/json"
	"fmt"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/errors"
	"github.com/timeredbull/tsuru/repository"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/mgo/bson"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
)

func (s *S) TestCloneRepositoryHandler(c *C) {
	a := App{Name: "someApp", Framework: "django", Teams: []auth.Team{s.team}}
	err := a.Create()
	c.Assert(err, IsNil)
	defer a.Destroy()
	url := fmt.Sprintf("/apps/%s/repository/clone?:name=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = CloneRepositoryHandler(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 200)
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
	app3 := App{Name: "app3", Framework: "django", Ip: "122222", Teams: []auth.Team{}}
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
