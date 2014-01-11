// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"code.google.com/p/go.crypto/bcrypt"
	"encoding/json"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/service"
	"github.com/globocom/tsuru/testing"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"path/filepath"
)

type ProvisionSuite struct {
	conn  *db.TsrStorage
	team  *auth.Team
	user  *auth.User
	token *auth.Token
}

var _ = gocheck.Suite(&ProvisionSuite{})

func (s *ProvisionSuite) SetUpSuite(c *gocheck.C) {
	var err error
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_api_provision_test")
	config.Set("auth:hash-cost", bcrypt.MinCost)
	s.conn, err = db.NewStorage()
	c.Assert(err, gocheck.IsNil)
	s.createUserAndTeam(c)
}

func (s *ProvisionSuite) TearDownSuite(c *gocheck.C) {
	s.conn.Apps().Database.DropDatabase()
}

func (s *ProvisionSuite) TearDownTest(c *gocheck.C) {
	_, err := s.conn.Services().RemoveAll(nil)
	c.Assert(err, gocheck.IsNil)
}

func (s *ProvisionSuite) makeRequestToServicesHandler(c *gocheck.C) (*httptest.ResponseRecorder, *http.Request) {
	request, err := http.NewRequest("GET", "/services", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ProvisionSuite) createUserAndTeam(c *gocheck.C) {
	s.user = &auth.User{Email: "whydidifall@thewho.com", Password: "1234567"}
	err := s.user.Create()
	c.Assert(err, gocheck.IsNil)
	s.team = &auth.Team{Name: "tsuruteam", Users: []string{s.user.Email}}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, gocheck.IsNil)
	s.token, err = s.user.CreateToken("1234567")
	c.Assert(err, gocheck.IsNil)
}

func (s *ProvisionSuite) TestServicesHandlerShoudGetAllServicesFromUsersTeam(c *gocheck.C) {
	srv := service.Service{Name: "mongodb", OwnerTeams: []string{s.team.Name}}
	srv.Create()
	defer s.conn.Services().Remove(bson.M{"_id": srv.Name})
	si := service.ServiceInstance{Name: "my_nosql", ServiceName: srv.Name, Teams: []string{s.team.Name}}
	si.Create()
	defer service.DeleteInstance(&si)
	recorder, request := s.makeRequestToServicesHandler(c)
	err := serviceList(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	services := make([]service.ServiceModel, 1)
	err = json.Unmarshal(b, &services)
	expected := []service.ServiceModel{
		{Service: "mongodb", Instances: []string{"my_nosql"}},
	}
	c.Assert(services, gocheck.DeepEquals, expected)
	action := testing.Action{Action: "list-services", User: s.user.Email}
	c.Assert(action, testing.IsRecorded)
}

func makeRequestToCreateHandler(c *gocheck.C) (*httptest.ResponseRecorder, *http.Request) {
	manifest := `id: some_service
endpoint:
    production: someservice.com
    test: test.someservice.com
`
	b := bytes.NewBufferString(manifest)
	request, err := http.NewRequest("POST", "/services", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ProvisionSuite) TestCreateHandlerSavesNameFromManifestID(c *gocheck.C) {
	recorder, request := makeRequestToCreateHandler(c)
	err := serviceCreate(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	query := bson.M{"_id": "some_service"}
	var rService service.Service
	err = s.conn.Services().Find(query).One(&rService)
	c.Assert(err, gocheck.IsNil)
	c.Assert(rService.Name, gocheck.Equals, "some_service")
	endpoints := map[string]string{
		"production": "someservice.com",
		"test":       "test.someservice.com",
	}
	action := testing.Action{
		Action: "create-service",
		User:   s.user.Email,
		Extra:  []interface{}{"some_service", endpoints},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *ProvisionSuite) TestCreateHandlerSavesEndpointServiceProperty(c *gocheck.C) {
	recorder, request := makeRequestToCreateHandler(c)
	err := serviceCreate(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	query := bson.M{"_id": "some_service"}
	var rService service.Service
	err = s.conn.Services().Find(query).One(&rService)
	c.Assert(err, gocheck.IsNil)
	c.Assert(rService.Endpoint["production"], gocheck.Equals, "someservice.com")
	c.Assert(rService.Endpoint["test"], gocheck.Equals, "test.someservice.com")
}

func (s *ProvisionSuite) TestCreateHandlerWithContentOfRealYaml(c *gocheck.C) {
	p, err := filepath.Abs("testdata/manifest.yml")
	manifest, err := ioutil.ReadFile(p)
	c.Assert(err, gocheck.IsNil)
	b := bytes.NewBuffer(manifest)
	request, err := http.NewRequest("POST", "/services", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = serviceCreate(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	query := bson.M{"_id": "mysqlapi"}
	var rService service.Service
	err = s.conn.Services().Find(query).One(&rService)
	c.Assert(err, gocheck.IsNil)
	c.Assert(rService.Endpoint["production"], gocheck.Equals, "mysqlapi.com")
	c.Assert(rService.Endpoint["test"], gocheck.Equals, "localhost:8000")
}

func (s *ProvisionSuite) TestCreateHandlerShouldReturnErrorWhenNameExists(c *gocheck.C) {
	recorder, request := makeRequestToCreateHandler(c)
	err := serviceCreate(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	recorder, request = makeRequestToCreateHandler(c)
	err = serviceCreate(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^Service with name some_service already exists.$")
}

func (s *ProvisionSuite) TestCreateHandlerSavesOwnerTeamsFromUserWhoCreated(c *gocheck.C) {
	recorder, request := makeRequestToCreateHandler(c)
	err := serviceCreate(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Body.String(), gocheck.Equals, "success")
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	query := bson.M{"_id": "some_service"}
	var rService service.Service
	err = s.conn.Services().Find(query).One(&rService)
	c.Assert(err, gocheck.IsNil)
	c.Assert("some_service", gocheck.Equals, rService.Name)
	c.Assert(rService.OwnerTeams, gocheck.DeepEquals, []string{s.team.Name})
}

func (s *ProvisionSuite) TestCreateHandlerReturnsForbiddenIfTheUserIsNotMemberOfAnyTeam(c *gocheck.C) {
	u := &auth.User{Email: "enforce@queensryche.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().RemoveAll(bson.M{"email": u.Email})
	token, err := u.CreateToken("123456")
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.Token})
	recorder, request := makeRequestToCreateHandler(c)
	err = serviceCreate(recorder, request, token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e, gocheck.ErrorMatches, "^In order to create a service, you should be member of at least one team$")
}

func (s *ProvisionSuite) TestCreateHandlerReturnsBadRequestIfTheServiceDoesNotHaveAProductionEndpoint(c *gocheck.C) {
	p, err := filepath.Abs("testdata/manifest-without-endpoint.yml")
	manifest, err := ioutil.ReadFile(p)
	c.Assert(err, gocheck.IsNil)
	b := bytes.NewBuffer(manifest)
	request, err := http.NewRequest("POST", "/services", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = serviceCreate(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e.Message, gocheck.Equals, "You must provide a production endpoint in the manifest file.")
}

func (s *ProvisionSuite) TestUpdateHandlerShouldUpdateTheServiceWithDataFromManifest(c *gocheck.C) {
	service := service.Service{
		Name:       "mysqlapi",
		Endpoint:   map[string]string{"production": "sqlapi.com"},
		OwnerTeams: []string{s.team.Name},
	}
	err := service.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": service.Name})
	p, err := filepath.Abs("testdata/manifest.yml")
	manifest, err := ioutil.ReadFile(p)
	c.Assert(err, gocheck.IsNil)
	request, err := http.NewRequest("PUT", "/services", bytes.NewBuffer(manifest))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceUpdate(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusNoContent)
	err = s.conn.Services().Find(bson.M{"_id": service.Name}).One(&service)
	c.Assert(err, gocheck.IsNil)
	c.Assert(service.Endpoint["production"], gocheck.Equals, "mysqlapi.com")
	endpoints := map[string]string{"production": "mysqlapi.com", "test": "localhost:8000"}
	action := testing.Action{
		Action: "update-service",
		User:   s.user.Email,
		Extra:  []interface{}{service.Name, endpoints},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *ProvisionSuite) TestUpdateHandlerReturns404WhenTheServiceDoesNotExist(c *gocheck.C) {
	p, err := filepath.Abs("testdata/manifest.yml")
	c.Assert(err, gocheck.IsNil)
	manifest, err := ioutil.ReadFile(p)
	c.Assert(err, gocheck.IsNil)
	request, err := http.NewRequest("PUT", "/services", bytes.NewBuffer(manifest))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceUpdate(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^Service not found$")
}

func (s *ProvisionSuite) TestUpdateHandlerReturns403WhenTheUserIsNotOwnerOfTheTeam(c *gocheck.C) {
	se := service.Service{Name: "mysqlapi", Teams: []string{s.team.Name}}
	se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	p, err := filepath.Abs("testdata/manifest.yml")
	c.Assert(err, gocheck.IsNil)
	manifest, err := ioutil.ReadFile(p)
	c.Assert(err, gocheck.IsNil)
	request, err := http.NewRequest("PUT", "/services", bytes.NewBuffer(manifest))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceUpdate(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e, gocheck.ErrorMatches, "^This user does not have access to this service$")
}

func (s *ProvisionSuite) TestDeleteHandler(c *gocheck.C) {
	se := service.Service{Name: "Mysql", OwnerTeams: []string{s.team.Name}}
	se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/services/%s?:name=%s", se.Name, se.Name), nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceDelete(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusNoContent)
	query := bson.M{"_id": se.Name}
	err = s.conn.Services().Find(query).One(&se)
	count, err := s.conn.Services().Find(query).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 0)
	action := testing.Action{
		Action: "delete-service",
		User:   s.user.Email,
		Extra:  []interface{}{se.Name},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *ProvisionSuite) TestDeleteHandlerReturns404WhenTheServiceDoesNotExist(c *gocheck.C) {
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/services/%s?:name=%s", "mongodb", "mongodb"), nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceDelete(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^Service not found$")
}

func (s *ProvisionSuite) TestDeleteHandlerReturns403WhenTheUserIsNotOwnerOfTheTeam(c *gocheck.C) {
	se := service.Service{Name: "Mysql", Teams: []string{s.team.Name}}
	se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/services/%s?:name=%s", se.Name, se.Name), nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceDelete(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e, gocheck.ErrorMatches, "^This user does not have access to this service$")
}

func (s *ProvisionSuite) TestDeleteHandlerReturns403WhenTheServiceHasInstance(c *gocheck.C) {
	se := service.Service{Name: "mysql", OwnerTeams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: se.Name}
	err = instance.Create()
	c.Assert(err, gocheck.IsNil)
	defer service.DeleteInstance(&instance)
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/services/%s?:name=%s", se.Name, se.Name), nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceDelete(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e, gocheck.ErrorMatches, "^This service cannot be removed because it has instances.\nPlease remove these instances before removing the service.$")
}

func (s *ProvisionSuite) TestGrantServiceAccessToTeam(c *gocheck.C) {
	t := &auth.Team{Name: "blaaaa"}
	s.conn.Teams().Insert(t)
	defer s.conn.Teams().Remove(bson.M{"name": t.Name})
	se := service.Service{Name: "my_service", Teams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, t.Name, se.Name, t.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = grantServiceAccess(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	err = se.Get()
	c.Assert(err, gocheck.IsNil)
	c.Assert(*s.team, HasAccessTo, se)
	action := testing.Action{
		Action: "grant-service-access",
		User:   s.user.Email,
		Extra:  []interface{}{"service=" + se.Name, "team=" + t.Name},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *ProvisionSuite) TestGrantAccesToTeamReturnNotFoundIfTheServiceDoesNotExist(c *gocheck.C) {
	url := fmt.Sprintf("/services/nononono/%s?:service=nononono&:team=%s", s.team.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = grantServiceAccess(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^Service not found$")
}

func (s *ProvisionSuite) TestGrantServiceAccessToTeamReturnForbiddenIfTheGivenUserDoesNotHaveAccessToTheService(c *gocheck.C) {
	se := service.Service{Name: "my_service"}
	err := se.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, s.team.Name, se.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = grantServiceAccess(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e, gocheck.ErrorMatches, "^This user does not have access to this service$")
}

func (s *ProvisionSuite) TestGrantServiceAccessToTeamReturnNotFoundIfTheTeamDoesNotExist(c *gocheck.C) {
	se := service.Service{Name: "my_service", Teams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/nonono?:service=%s&:team=nonono", se.Name, se.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = grantServiceAccess(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^Team not found$")
}

func (s *ProvisionSuite) TestGrantServiceAccessToTeamReturnConflictIfTheTeamAlreadyHasAccessToTheService(c *gocheck.C) {
	se := service.Service{Name: "my_service", Teams: []string{s.team.Name}}
	err := se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, gocheck.IsNil)
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, s.team.Name, se.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = grantServiceAccess(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusConflict)
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamRemovesTeamFromService(c *gocheck.C) {
	t := &auth.Team{Name: "alle-da"}
	se := service.Service{Name: "my_service", Teams: []string{s.team.Name, t.Name}}
	err := se.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, s.team.Name, se.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = revokeServiceAccess(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	err = se.Get()
	c.Assert(err, gocheck.IsNil)
	c.Assert(*s.team, gocheck.Not(HasAccessTo), se)
	action := testing.Action{
		Action: "revoke-service-access",
		User:   s.user.Email,
		Extra:  []interface{}{"service=" + se.Name, "team=" + s.team.Name},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamReturnsNotFoundIfTheServiceDoesNotExist(c *gocheck.C) {
	url := fmt.Sprintf("/services/nonono/%s?:service=nonono&:team=%s", s.team.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = revokeServiceAccess(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^Service not found$")
}

func (s *ProvisionSuite) TestRevokeAccesFromTeamReturnsForbiddenIfTheGivenUserDoesNotHasAccessToTheService(c *gocheck.C) {
	t := &auth.Team{Name: "alle-da"}
	se := service.Service{Name: "my_service", Teams: []string{t.Name}}
	err := se.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, t.Name, se.Name, t.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = revokeServiceAccess(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e, gocheck.ErrorMatches, "^This user does not have access to this service$")
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamReturnsNotFoundIfTheTeamDoesNotExist(c *gocheck.C) {
	se := service.Service{Name: "my_service", Teams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/nonono?:service=%s&:team=nonono", se.Name, se.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = revokeServiceAccess(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^Team not found$")
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamReturnsForbiddenIfTheTeamIsTheOnlyWithAccessToTheService(c *gocheck.C) {
	se := service.Service{Name: "my_service", Teams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, s.team.Name, se.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = revokeServiceAccess(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e, gocheck.ErrorMatches, "^You can not revoke the access from this team, because it is the unique team with access to this service, and a service can not be orphaned$")
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamReturnNotFoundIfTheTeamDoesNotHasAccessToTheService(c *gocheck.C) {
	t := &auth.Team{Name: "Rammlied"}
	s.conn.Teams().Insert(t)
	defer s.conn.Teams().RemoveAll(bson.M{"name": t.Name})
	se := service.Service{Name: "my_service", Teams: []string{s.team.Name, s.team.Name}}
	err := se.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, t.Name, se.Name, t.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = revokeServiceAccess(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
}

func (s *ProvisionSuite) TestAddDocHandlerReturns404WhenTheServiceDoesNotExist(c *gocheck.C) {
	b := bytes.NewBufferString("doc")
	request, err := http.NewRequest("PUT", fmt.Sprintf("/services/%s/doc?:name=%s", "mongodb", "mongodb"), b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceAddDoc(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^Service not found$")
}

func (s *ProvisionSuite) TestAddDocHandler(c *gocheck.C) {
	se := service.Service{Name: "some_service", OwnerTeams: []string{s.team.Name}}
	se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	b := bytes.NewBufferString("doc")
	request, err := http.NewRequest("PUT", fmt.Sprintf("/services/%s/doc?:name=%s", se.Name, se.Name), b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceAddDoc(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	query := bson.M{"_id": "some_service"}
	var serv service.Service
	err = s.conn.Services().Find(query).One(&serv)
	c.Assert(err, gocheck.IsNil)
	c.Assert(serv.Doc, gocheck.Equals, "doc")
	action := testing.Action{
		Action: "service-add-doc",
		User:   s.user.Email,
		Extra:  []interface{}{"some_service", "doc"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *ProvisionSuite) TestAddDocHandlerReturns403WhenTheUserDoesNotHaveAccessToTheService(c *gocheck.C) {
	se := service.Service{Name: "Mysql"}
	se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	b := bytes.NewBufferString("doc")
	request, err := http.NewRequest("PUT", fmt.Sprintf("/services/%s/doc?:name=%s", se.Name, se.Name), b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceAddDoc(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e, gocheck.ErrorMatches, "^This user does not have access to this service$")
}

func (s *ProvisionSuite) TestgetServiceByOwner(c *gocheck.C) {
	srv := service.Service{Name: "foo", OwnerTeams: []string{s.team.Name}}
	err := srv.Create()
	c.Assert(err, gocheck.IsNil)
	defer srv.Delete()
	rSrv, err := getServiceByOwner("foo", s.user)
	c.Assert(err, gocheck.IsNil)
	c.Assert(rSrv.Name, gocheck.Equals, srv.Name)
}

func (s *ProvisionSuite) TestServicesAndInstancesByOwnerTeams(c *gocheck.C) {
	srvc := service.Service{Name: "mysql", OwnerTeams: []string{s.team.Name}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer srvc.Delete()
	srvc2 := service.Service{Name: "mongodb"}
	err = srvc2.Create()
	c.Assert(err, gocheck.IsNil)
	defer srvc2.Delete()
	sInstance := service.ServiceInstance{Name: "foo", ServiceName: "mysql"}
	err = sInstance.Create()
	c.Assert(err, gocheck.IsNil)
	defer service.DeleteInstance(&sInstance)
	sInstance2 := service.ServiceInstance{Name: "bar", ServiceName: "mongodb"}
	err = sInstance2.Create()
	defer service.DeleteInstance(&sInstance2)
	results := servicesAndInstancesByOwner(s.user)
	expected := []service.ServiceModel{
		{Service: "mysql", Instances: []string{"foo"}},
	}
	c.Assert(results, gocheck.DeepEquals, expected)
}
