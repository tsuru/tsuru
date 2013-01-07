// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/service"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"path/filepath"
)

type ProvisionSuite struct {
	team *auth.Team
	user *auth.User
}

var _ = Suite(&ProvisionSuite{})

func (s *ProvisionSuite) SetUpSuite(c *C) {
	var err error
	db.Session, err = db.Open("127.0.0.1:27017", "tsuru_api_provision_test")
	c.Assert(err, IsNil)
	s.createUserAndTeam(c)
}

func (s *ProvisionSuite) TearDownSuite(c *C) {
	defer db.Session.Close()
	db.Session.Apps().Database.DropDatabase()
}

func (s *ProvisionSuite) TearDownTest(c *C) {
	_, err := db.Session.Services().RemoveAll(nil)
	c.Assert(err, IsNil)
}

func (s *ProvisionSuite) makeRequestToServicesHandler(c *C) (*httptest.ResponseRecorder, *http.Request) {
	request, err := http.NewRequest("GET", "/services", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ProvisionSuite) createUserAndTeam(c *C) {
	s.user = &auth.User{Email: "whydidifall@thewho.com", Password: "123"}
	err := s.user.Create()
	c.Assert(err, IsNil)
	s.team = &auth.Team{Name: "tsuruteam", Users: []string{s.user.Email}}
	err = db.Session.Teams().Insert(s.team)
	c.Assert(err, IsNil)
}

func (s *ProvisionSuite) TestServicesHandlerShoudGetAllServicesFromUsersTeam(c *C) {
	srv := service.Service{Name: "mongodb", OwnerTeams: []string{s.team.Name}}
	srv.Create()
	defer db.Session.Services().Remove(bson.M{"_id": srv.Name})
	si := service.ServiceInstance{Name: "my_nosql", ServiceName: srv.Name, Teams: []string{s.team.Name}}
	si.Create()
	defer si.Delete()
	recorder, request := s.makeRequestToServicesHandler(c)
	err := ServicesHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	services := make([]service.ServiceModel, 1)
	err = json.Unmarshal(b, &services)
	expected := []service.ServiceModel{
		{Service: "mongodb", Instances: []string{"my_nosql"}},
	}
	c.Assert(services, DeepEquals, expected)
}

func makeRequestToCreateHandler(c *C) (*httptest.ResponseRecorder, *http.Request) {
	manifest := `id: some_service
endpoint:
    production: someservice.com
    test: test.someservice.com
`
	b := bytes.NewBufferString(manifest)
	request, err := http.NewRequest("POST", "/services", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ProvisionSuite) TestCreateHandlerSavesNameFromManifestId(c *C) {
	recorder, request := makeRequestToCreateHandler(c)
	err := CreateHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	query := bson.M{"_id": "some_service"}
	var rService service.Service
	err = db.Session.Services().Find(query).One(&rService)
	c.Assert(err, IsNil)
	c.Assert(rService.Name, Equals, "some_service")
}

func (s *ProvisionSuite) TestCreateHandlerSavesEndpointServiceProperty(c *C) {
	recorder, request := makeRequestToCreateHandler(c)
	err := CreateHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	query := bson.M{"_id": "some_service"}
	var rService service.Service
	err = db.Session.Services().Find(query).One(&rService)
	c.Assert(err, IsNil)
	c.Assert(rService.Endpoint["production"], Equals, "someservice.com")
	c.Assert(rService.Endpoint["test"], Equals, "test.someservice.com")
}

func (s *ProvisionSuite) TestCreateHandlerWithContentOfRealYaml(c *C) {
	p, err := filepath.Abs("testdata/manifest.yml")
	manifest, err := ioutil.ReadFile(p)
	c.Assert(err, IsNil)
	b := bytes.NewBuffer(manifest)
	request, err := http.NewRequest("POST", "/services", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	query := bson.M{"_id": "mysqlapi"}
	var rService service.Service
	err = db.Session.Services().Find(query).One(&rService)
	c.Assert(err, IsNil)
	c.Assert(rService.Endpoint["production"], Equals, "mysqlapi.com")
	c.Assert(rService.Endpoint["test"], Equals, "localhost:8000")
}

func (s *ProvisionSuite) TestCreateHandlerShouldReturnErrorWhenNameExists(c *C) {
	recorder, request := makeRequestToCreateHandler(c)
	err := CreateHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	recorder, request = makeRequestToCreateHandler(c)
	err = CreateHandler(recorder, request, s.user)
	c.Assert(err, Not(IsNil))
	c.Assert(err, ErrorMatches, "^Service with name some_service already exists.$")
}

func (s *ProvisionSuite) TestCreateHandlerSavesOwnerTeamsFromUserWhoCreated(c *C) {
	recorder, request := makeRequestToCreateHandler(c)
	err := CreateHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.String(), Equals, "success")
	c.Assert(recorder.Code, Equals, http.StatusOK)
	query := bson.M{"_id": "some_service"}
	var rService service.Service
	err = db.Session.Services().Find(query).One(&rService)
	c.Assert(err, IsNil)
	c.Assert("some_service", Equals, rService.Name)
	c.Assert(rService.OwnerTeams, DeepEquals, []string{s.team.Name})
}

func (s *ProvisionSuite) TestCreateHandlerReturnsForbiddenIfTheUserIsNotMemberOfAnyTeam(c *C) {
	u := &auth.User{Email: "enforce@queensryche.com", Password: "123"}
	u.Create()
	defer db.Session.Users().RemoveAll(bson.M{"email": u.Email})
	recorder, request := makeRequestToCreateHandler(c)
	err := CreateHandler(recorder, request, u)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^In order to create a service, you should be member of at least one team$")
}

func (s *ProvisionSuite) TestCreateHandlerReturnsBadRequestIfTheServiceDoesNotHaveAProductionEndpoint(c *C) {
	p, err := filepath.Abs("testdata/manifest-without-endpoint.yml")
	manifest, err := ioutil.ReadFile(p)
	c.Assert(err, IsNil)
	b := bytes.NewBuffer(manifest)
	request, err := http.NewRequest("POST", "/services", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
	c.Assert(e.Message, Equals, "You must provide a production endpoint in the manifest file.")
}

func (s *ProvisionSuite) TestUpdateHandlerShouldUpdateTheServiceWithDataFromManifest(c *C) {
	service := service.Service{Name: "mysqlapi", Endpoint: map[string]string{"production": "sqlapi.com"}, OwnerTeams: []string{s.team.Name}}
	err := service.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": service.Name})
	p, err := filepath.Abs("testdata/manifest.yml")
	manifest, err := ioutil.ReadFile(p)
	c.Assert(err, IsNil)
	request, err := http.NewRequest("PUT", "/services", bytes.NewBuffer(manifest))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = UpdateHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, http.StatusNoContent)
	err = db.Session.Services().Find(bson.M{"_id": service.Name}).One(&service)
	c.Assert(err, IsNil)
	c.Assert(service.Endpoint["production"], Equals, "mysqlapi.com")
}

func (s *ProvisionSuite) TestUpdateHandlerReturns404WhenTheServiceDoesNotExist(c *C) {
	p, err := filepath.Abs("testdata/manifest.yml")
	c.Assert(err, IsNil)
	manifest, err := ioutil.ReadFile(p)
	c.Assert(err, IsNil)
	request, err := http.NewRequest("PUT", "/services", bytes.NewBuffer(manifest))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = UpdateHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^Service not found$")
}

func (s *ProvisionSuite) TestUpdateHandlerReturns404WhenTheServicesIsDeleted(c *C) {
	se := service.Service{Name: "mysqlapi", OwnerTeams: []string{s.team.Name}, Status: "deleted"}
	err := db.Session.Services().Insert(se)
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	p, err := filepath.Abs("testdata/manifest.yml")
	c.Assert(err, IsNil)
	manifest, err := ioutil.ReadFile(p)
	c.Assert(err, IsNil)
	request, err := http.NewRequest("PUT", "/services", bytes.NewBuffer(manifest))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = UpdateHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^Service not found$")
}

func (s *ProvisionSuite) TestUpdateHandlerReturns403WhenTheUserIsNotOwnerOfTheTeam(c *C) {
	se := service.Service{Name: "mysqlapi", Teams: []string{s.team.Name}}
	se.Create()
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	p, err := filepath.Abs("testdata/manifest.yml")
	c.Assert(err, IsNil)
	manifest, err := ioutil.ReadFile(p)
	c.Assert(err, IsNil)
	request, err := http.NewRequest("PUT", "/services", bytes.NewBuffer(manifest))
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = UpdateHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^This user does not have access to this service$")
}

func (s *ProvisionSuite) TestDeleteHandler(c *C) {
	se := service.Service{Name: "Mysql", OwnerTeams: []string{s.team.Name}}
	se.Create()
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/services/%s?:name=%s", se.Name, se.Name), nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = DeleteHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, http.StatusNoContent)
	query := bson.M{"_id": "Mysql"}
	err = db.Session.Services().Find(query).One(&se)
	c.Assert(err, IsNil)
	c.Assert(se.Status, Equals, "deleted")
}

func (s *ProvisionSuite) TestDeleteHandlerReturns404WhenTheServiceDoesNotExist(c *C) {
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

func (s *ProvisionSuite) TestDeleteHandlerReturns404WhenTheServicesIsDeleted(c *C) {
	se := service.Service{Name: "mysql", OwnerTeams: []string{s.team.Name}, Status: "deleted"}
	err := db.Session.Services().Insert(se)
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/services/%s?:name=%s", se.Name, se.Name), nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = DeleteHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^Service not found$")
}

func (s *ProvisionSuite) TestDeleteHandlerReturns403WhenTheUserIsNotOwnerOfTheTeam(c *C) {
	se := service.Service{Name: "Mysql", Teams: []string{s.team.Name}}
	se.Create()
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
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

func (s *ProvisionSuite) TestDeleteHandlerReturns403WhenTheServiceHasInstance(c *C) {
	se := service.Service{Name: "mysql", OwnerTeams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: se.Name}
	err = instance.Create()
	c.Assert(err, IsNil)
	defer instance.Delete()
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/services/%s?:name=%s", se.Name, se.Name), nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = DeleteHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^This service cannot be removed because it has instances.\nPlease remove these instances before removing the service.$")
}

func (s *ProvisionSuite) TestGrantServiceAccessToTeam(c *C) {
	t := &auth.Team{Name: "blaaaa"}
	db.Session.Teams().Insert(t)
	defer db.Session.Teams().Remove(bson.M{"name": t.Name})
	se := service.Service{Name: "my_service", Teams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, t.Name, se.Name, t.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GrantServiceAccessToTeamHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	err = se.Get()
	c.Assert(err, IsNil)
	c.Assert(*s.team, HasAccessTo, se)
}

func (s *ProvisionSuite) TestGrantAccesToTeamReturnNotFoundIfTheServiceDoesNotExist(c *C) {
	url := fmt.Sprintf("/services/nononono/%s?:service=nononono&:team=%s", s.team.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GrantServiceAccessToTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^Service not found$")
}

func (s *ProvisionSuite) TestGrantServiceAccessToTeamReturnForbiddenIfTheGivenUserDoesNotHaveAccessToTheService(c *C) {
	se := service.Service{Name: "my_service"}
	err := se.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, s.team.Name, se.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GrantServiceAccessToTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^This user does not have access to this service$")
}

func (s *ProvisionSuite) TestGrantServiceAccessToTeamReturnNotFoundIfTheTeamDoesNotExist(c *C) {
	se := service.Service{Name: "my_service", Teams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/nonono?:service=%s&:team=nonono", se.Name, se.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GrantServiceAccessToTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^Team not found$")
}

func (s *ProvisionSuite) TestGrantServiceAccessToTeamReturnConflictIfTheTeamAlreadyHasAccessToTheService(c *C) {
	se := service.Service{Name: "my_service", Teams: []string{s.team.Name}}
	err := se.Create()
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, IsNil)
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, s.team.Name, se.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GrantServiceAccessToTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusConflict)
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamRemovesTeamFromService(c *C) {
	t := &auth.Team{Name: "alle-da"}
	se := service.Service{Name: "my_service", Teams: []string{s.team.Name, t.Name}}
	err := se.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, s.team.Name, se.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RevokeServiceAccessFromTeamHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	err = se.Get()
	c.Assert(err, IsNil)
	c.Assert(*s.team, Not(HasAccessTo), se)
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamReturnsNotFoundIfTheServiceDoesNotExist(c *C) {
	url := fmt.Sprintf("/services/nonono/%s?:service=nonono&:team=%s", s.team.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RevokeServiceAccessFromTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^Service not found$")
}

func (s *ProvisionSuite) TestRevokeAccesFromTeamReturnsForbiddenIfTheGivenUserDoesNotHasAccessToTheService(c *C) {
	t := &auth.Team{Name: "alle-da"}
	se := service.Service{Name: "my_service", Teams: []string{t.Name}}
	err := se.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, t.Name, se.Name, t.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RevokeServiceAccessFromTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^This user does not have access to this service$")
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamReturnsNotFoundIfTheTeamDoesNotExist(c *C) {
	se := service.Service{Name: "my_service", Teams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/nonono?:service=%s&:team=nonono", se.Name, se.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RevokeServiceAccessFromTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^Team not found$")
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamReturnsForbiddenIfTheTeamIsTheOnlyWithAccessToTheService(c *C) {
	se := service.Service{Name: "my_service", Teams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, s.team.Name, se.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RevokeServiceAccessFromTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^You can not revoke the access from this team, because it is the unique team with access to this service, and a service can not be orphaned$")
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamReturnNotFoundIfTheTeamDoesNotHasAccessToTheService(c *C) {
	t := &auth.Team{Name: "Rammlied"}
	db.Session.Teams().Insert(t)
	defer db.Session.Teams().RemoveAll(bson.M{"name": t.Name})
	se := service.Service{Name: "my_service", Teams: []string{s.team.Name, s.team.Name}}
	err := se.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, t.Name, se.Name, t.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RevokeServiceAccessFromTeamHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
}

func (s *ProvisionSuite) TestAddDocHandlerReturns404WhenTheServiceDoesNotExist(c *C) {
	b := bytes.NewBufferString("doc")
	request, err := http.NewRequest("PUT", fmt.Sprintf("/services/%s/doc?:name=%s", "mongodb", "mongodb"), b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddDocHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^Service not found$")
}

func (s *ProvisionSuite) TestAddDocHandler(c *C) {
	se := service.Service{Name: "some_service", OwnerTeams: []string{s.team.Name}}
	se.Create()
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	b := bytes.NewBufferString("doc")
	request, err := http.NewRequest("PUT", fmt.Sprintf("/services/%s/doc?:name=%s", se.Name, se.Name), b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddDocHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	query := bson.M{"_id": "some_service"}
	var serv service.Service
	err = db.Session.Services().Find(query).One(&serv)
	c.Assert(err, IsNil)
	c.Assert(serv.Doc, Equals, "doc")
}

func (s *ProvisionSuite) TestAddDocHandlerReturns403WhenTheUserDoesNotHaveAccessToTheService(c *C) {
	se := service.Service{Name: "Mysql"}
	se.Create()
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	b := bytes.NewBufferString("doc")
	request, err := http.NewRequest("PUT", fmt.Sprintf("/services/%s/doc?:name=%s", se.Name, se.Name), b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddDocHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^This user does not have access to this service$")
}

func (s *ProvisionSuite) TestGetDocHandler(c *C) {
	se := service.Service{Name: "some_service", Doc: "some doc", OwnerTeams: []string{s.team.Name}}
	se.Create()
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	request, err := http.NewRequest("GET", fmt.Sprintf("/services/%s/doc?:name=%s", se.Name, se.Name), nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GetDocHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.String(), Equals, "some doc")
}

func (s *ProvisionSuite) TestGetDocHandlerReturns403WhenTheUserDoesNotHaveAccessToTheService(c *C) {
	se := service.Service{Name: "Mysql"}
	se.Create()
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	request, err := http.NewRequest("GET", fmt.Sprintf("/services/%s/doc?:name=%s", se.Name, se.Name), nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GetDocHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^This user does not have access to this service$")
}

func (s *ProvisionSuite) TestGetDocHandlerReturns404WhenTheServiceDoesNotExist(c *C) {
	request, err := http.NewRequest("GET", fmt.Sprintf("/services/%s/doc?:name=%s", "mongodb", "mongodb"), nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = GetDocHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^Service not found$")
}

func (s *ProvisionSuite) TestgetServiceByOwner(c *C) {
	srv := service.Service{Name: "foo", OwnerTeams: []string{s.team.Name}}
	err := srv.Create()
	c.Assert(err, IsNil)
	defer srv.Delete()
	rSrv, err := getServiceByOwner("foo", s.user)
	c.Assert(err, IsNil)
	c.Assert(rSrv.Name, Equals, srv.Name)
}

func (s *ProvisionSuite) TestServicesAndInstancesByOwnerTeams(c *C) {
	srvc := service.Service{Name: "mysql", OwnerTeams: []string{s.team.Name}}
	err := srvc.Create()
	c.Assert(err, IsNil)
	defer srvc.Delete()
	srvc2 := service.Service{Name: "mongodb"}
	err = srvc2.Create()
	c.Assert(err, IsNil)
	defer srvc2.Delete()
	sInstance := service.ServiceInstance{Name: "foo", ServiceName: "mysql"}
	err = sInstance.Create()
	c.Assert(err, IsNil)
	defer sInstance.Delete()
	sInstance2 := service.ServiceInstance{Name: "bar", ServiceName: "mongodb"}
	err = sInstance2.Create()
	defer sInstance2.Delete()
	results := servicesAndInstancesByOwner(s.user)
	expected := []service.ServiceModel{
		{Service: "mysql", Instances: []string{"foo"}},
	}
	c.Assert(results, DeepEquals, expected)
}
