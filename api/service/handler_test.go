package service

import (
	"encoding/json"
	"fmt"
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/errors"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/mgo/bson"
	"net/http"
	"net/http/httptest"
	"strings"
)

func (s *ServiceSuite) TestCreateHandlerGetAllTeamsFromTheUser(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	st.Create()

	b := strings.NewReader(`{"name":"some_service", "type":"mysql"}`)
	request, err := http.NewRequest("POST", "/services", b)
	c.Assert(err, IsNil)

	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.String(), Equals, "success")
	c.Assert(recorder.Code, Equals, 200)

	query := bson.M{"name": "some_service"}
	var obtainedService Service

	err = db.Session.Services().Find(query).One(&obtainedService)
	c.Assert(err, IsNil)
	c.Assert(obtainedService.Name, Equals, "some_service")
	c.Assert(obtainedService.ServiceTypeId, Not(Equals), 0)
	c.Assert(obtainedService.Name, Not(Equals), "")
	c.Assert(*s.team, HasAccessTo, obtainedService)
}

func (s *ServiceSuite) TestCreateHandlerReturnsForbiddenIfTheUserIsNotMemberOfAnyTeam(c *C) {
	u := &auth.User{Email: "enforce@queensryche.com", Password: "123"}
	u.Create()
	defer db.Session.Users().RemoveAll(bson.M{"email": u.Email})
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	st.Create()

	b := strings.NewReader(`{"name":"some_service", "type":"mysql"}`)
	request, err := http.NewRequest("POST", "/services", b)
	c.Assert(err, IsNil)

	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateHandler(recorder, request, u)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^In order to create a service, you should be member of at least one team$")
}

func (s *ServiceSuite) TestServicesHandlerListsOnlyServicesThatTheUserHasAccess(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	st.Create()
	se := Service{ServiceTypeId: st.Id, Name: "myService", Teams: []auth.Team{*s.team}}
	se2 := Service{ServiceTypeId: st.Id, Name: "myOtherService"}
	se.Create()
	se2.Create()

	request, err := http.NewRequest("GET", "/services", nil)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	err = ServicesHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 200)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	var results []serviceT
	err = json.Unmarshal(body, &results)
	c.Assert(err, IsNil)
	c.Assert(len(results), Equals, 1)
	c.Assert(results[0], FitsTypeOf, serviceT{})
	c.Assert(results[0].Name, Not(Equals), "")
	c.Assert(results[0].Type, FitsTypeOf, &ServiceType{})
	c.Assert(results[0].Type.Id, Not(Equals), 0)
}

func (s *ServiceSuite) TestServiceTypesHandler(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	st.Create()

	request, err := http.NewRequest("GET", "/services/types", nil)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	err = ServiceTypesHandler(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 200)

	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)

	var results []ServiceType
	err = json.Unmarshal(body, &results)
	c.Assert(err, IsNil)
	c.Assert(results[0].Id, Not(Equals), 0)
}

func (s *ServiceSuite) TestDeleteHandler(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	st.Create()
	se := Service{ServiceTypeId: st.Id, Name: "Mysql", Teams: []auth.Team{*s.team}}
	se.Create()
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/services/%s?:name=%s", se.Name, se.Name), nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = DeleteHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 200)
	query := bson.M{"name": "Mysql"}
	qtd, err := db.Session.Services().Find(query).Count()
	c.Assert(err, IsNil)
	c.Assert(qtd, Equals, 0)
}

func (s *ServiceSuite) TestDeleteHandlerReturns404(c *C) {
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/services/%s?:name=%s", "mongodb", "mongodb"), nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = DeleteHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^Service not found$")
}

func (s *ServiceSuite) TestDeleteHandlerReturns403IfTheUserDoesNotHaveAccessToTheService(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	st.Create()
	se := Service{ServiceTypeId: st.Id, Name: "Mysql"}
	se.Create()
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/services/%s?:name=%s", se.Name, se.Name), nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = DeleteHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^This user does not have access to this service$")
}

func (s *ServiceSuite) TestBindHandler(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	err := st.Create()
	c.Assert(err, IsNil)
	se := Service{ServiceTypeId: st.Id, Name: "my_service"}
	a := app.App{Name: "serviceApp", Framework: "django"}
	err = se.Create()
	c.Assert(err, IsNil)
	err = a.Create()
	c.Assert(err, IsNil)
	b := strings.NewReader(`{"app":"serviceApp", "service":"my_service"}`)
	request, err := http.NewRequest("POST", "/services/bind", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = BindHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 200)
	query := bson.M{
		"service_name": se.Name,
		"app_name":     a.Name,
	}
	qtd, err := db.Session.ServiceApps().Find(query).Count()
	c.Check(err, IsNil)
	c.Assert(qtd, Equals, 1)
}

func (s *ServiceSuite) TestBindHandlerReturns404(c *C) {
	b := strings.NewReader(`{"app":"serviceApp", "service":"my_service"}`)
	request, err := http.NewRequest("POST", "/services/bind", b)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	err = BindHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 404)
}

func (s *ServiceSuite) TestUnbindHandler(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	st.Create()
	se := Service{ServiceTypeId: st.Id, Name: "my_service"}
	a := app.App{Name: "serviceApp", Framework: "django", Ip: "192.168.30.10"}
	se.Create()
	a.Create()
	se.Bind(&a)

	b := strings.NewReader(`{"app":"serviceApp", "service":"my_service"}`)
	request, err := http.NewRequest("POST", "/services/bind", b)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	err = UnbindHandler(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 200)

	query := bson.M{
		"service_name": se.Name,
		"app_name":     a.Name,
	}
	qtd, err := db.Session.Services().Find(query).Count()
	c.Check(err, IsNil)
	c.Assert(qtd, Equals, 0)
}

func (s *ServiceSuite) TestUnbindReturns404(c *C) {
	b := strings.NewReader(`{"app":"serviceApp", "service":"my_service"}`)
	request, err := http.NewRequest("POST", "/services/bind", b)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	err = UnbindHandler(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 404)
}

func (s *ServiceSuite) TestGrantAccessToTeam(c *C) {
	t := &auth.Team{Name: "blaaaa"}
	db.Session.Teams().Insert(t)
	defer db.Session.Teams().Remove(bson.M{"name": t.Name})
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	err := st.Create()
	c.Assert(err, IsNil)
	se := Service{ServiceTypeId: st.Id, Name: "my_service", Teams: []auth.Team{*s.team}}
	err = se.Create()
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, t.Name, se.Name, t.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GrantAccessToTeamHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	err = se.Get()
	c.Assert(err, IsNil)
	c.Assert(*s.team, HasAccessTo, se)
}

func (s *ServiceSuite) TestGrantAccesToTeamReturnNotFoundIfTheServiceDoesNotExist(c *C) {
	url := fmt.Sprintf("/services/nononono/%s?:service=nononono&:team=%s", s.team.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GrantAccessToTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^Service not found$")
}

func (s *ServiceSuite) TestGrantAccessToTeamReturnForbiddenIfTheGivenUserDoesNotHaveAccessToTheService(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	err := st.Create()
	c.Assert(err, IsNil)
	se := Service{ServiceTypeId: st.Id, Name: "my_service"}
	err = se.Create()
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, s.team.Name, se.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GrantAccessToTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^This user does not have access to this service$")
}

func (s *ServiceSuite) TestGrantAccessToTeamReturnNotFoundIfTheTeamDoesNotExist(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	err := st.Create()
	c.Assert(err, IsNil)
	se := Service{ServiceTypeId: st.Id, Name: "my_service", Teams: []auth.Team{*s.team}}
	err = se.Create()
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/services/%s/nonono?:service=%s&:team=nonono", se.Name, se.Name)
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

func (s *ServiceSuite) TestGrantAccessToTeamReturnConflictIfTheTeamAlreadyHasAccessToTheService(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	err := st.Create()
	c.Assert(err, IsNil)
	se := Service{ServiceTypeId: st.Id, Name: "my_service", Teams: []auth.Team{*s.team}}
	err = se.Create()
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, s.team.Name, se.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GrantAccessToTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusConflict)
}

func (s *ServiceSuite) TestRevokeAccessFromTeamRemovesTeamFromService(c *C) {
	t := &auth.Team{Name: "alle-da"}
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	err := st.Create()
	c.Assert(err, IsNil)
	se := Service{ServiceTypeId: st.Id, Name: "my_service", Teams: []auth.Team{*s.team, *t}}
	err = se.Create()
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, s.team.Name, se.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RevokeAccessFromTeamHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	err = se.Get()
	c.Assert(err, IsNil)
	c.Assert(*s.team, Not(HasAccessTo), se)
}

func (s *ServiceSuite) TestRevokeAccessFromTeamReturnsNotFoundIfTheServiceDoesNotExist(c *C) {
	url := fmt.Sprintf("/services/nonono/%s?:service=nonono&:team=%s", s.team.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RevokeAccessFromTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^Service not found$")
}

func (s *ServiceSuite) TestRevokeAccesFromTeamReturnsForbiddenIfTheGivenUserDoesNotHasAccessToTheService(c *C) {
	t := &auth.Team{Name: "alle-da"}
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	err := st.Create()
	c.Assert(err, IsNil)
	se := Service{ServiceTypeId: st.Id, Name: "my_service", Teams: []auth.Team{*t}}
	err = se.Create()
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, t.Name, se.Name, t.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RevokeAccessFromTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^This user does not have access to this service$")
}

func (s *ServiceSuite) TestRevokeAccessFromTeamReturnsNotFoundIfTheTeamDoesNotExist(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	err := st.Create()
	c.Assert(err, IsNil)
	se := Service{ServiceTypeId: st.Id, Name: "my_service", Teams: []auth.Team{*s.team}}
	err = se.Create()
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/services/%s/nonono?:service=%s&:team=nonono", se.Name, se.Name)
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

func (s *ServiceSuite) TestRevokeAccessFromTeamReturnsForbiddenIfTheTeamIsTheOnlyWithAccessToTheService(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	err := st.Create()
	c.Assert(err, IsNil)
	se := Service{ServiceTypeId: st.Id, Name: "my_service", Teams: []auth.Team{*s.team}}
	err = se.Create()
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, s.team.Name, se.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RevokeAccessFromTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^You can not revoke the access from this team, because it is the unique team with access to this service, and a service can not be orphaned$")
}

func (s *ServiceSuite) TestRevokeAccessFromTeamReturnNotFoundIfTheTeamDoesNotHasAccessToTheService(c *C) {
	t := &auth.Team{Name: "Rammlied"}
	db.Session.Teams().Insert(t)
	defer db.Session.Teams().RemoveAll(bson.M{"name": t.Name})
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	err := st.Create()
	c.Assert(err, IsNil)
	se := Service{ServiceTypeId: st.Id, Name: "my_service", Teams: []auth.Team{*s.team, *s.team}}
	err = se.Create()
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, t.Name, se.Name, t.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RevokeAccessFromTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
}
