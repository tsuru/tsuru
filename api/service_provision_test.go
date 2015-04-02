// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"path/filepath"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/rec/rectest"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/service"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

const (
	baseManifest = `id: some_service
username: test
password: xxxx
endpoint:
    production: someservice.com
`
	manifestWithoutPassword = `id: some_service
endpoint:
    production: someservice.com
`
	manifestWithoutId = `password: 000000
endpoint:
    production: someservice.com
`
)

type ProvisionSuite struct {
	conn  *db.Storage
	team  *auth.Team
	user  *auth.User
	token auth.Token
}

var _ = check.Suite(&ProvisionSuite{})

func (s *ProvisionSuite) SetUpSuite(c *check.C) {
	repositorytest.Reset()
	var err error
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_api_provision_test")
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("repo-manager", "fake")
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	s.createUserAndTeam(c)
}

func (s *ProvisionSuite) TearDownSuite(c *check.C) {
	dbtest.ClearAllCollections(s.conn.Apps().Database)
}

func (s *ProvisionSuite) TearDownTest(c *check.C) {
	repositorytest.Reset()
	_, err := s.conn.Services().RemoveAll(nil)
	c.Assert(err, check.IsNil)
}

func (s *ProvisionSuite) makeRequestToServicesHandler(c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	request, err := http.NewRequest("GET", "/services", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ProvisionSuite) createUserAndTeam(c *check.C) {
	s.user = &auth.User{Email: "whydidifall@thewho.com", Password: "1234567"}
	_, err := nativeScheme.Create(s.user)
	c.Assert(err, check.IsNil)
	s.team = &auth.Team{Name: "tsuruteam", Users: []string{s.user.Email}}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "1234567"})
	c.Assert(err, check.IsNil)
}

func (s *ProvisionSuite) TestServicesHandlerShoudGetAllServicesFromUsersTeam(c *check.C) {
	srv := service.Service{Name: "mongodb", OwnerTeams: []string{s.team.Name}}
	srv.Create()
	defer s.conn.Services().Remove(bson.M{"_id": srv.Name})
	si := service.ServiceInstance{Name: "my_nosql", ServiceName: srv.Name, Teams: []string{s.team.Name}}
	si.Create()
	defer service.DeleteInstance(&si)
	recorder, request := s.makeRequestToServicesHandler(c)
	err := serviceList(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	services := make([]service.ServiceModel, 1)
	err = json.Unmarshal(b, &services)
	expected := []service.ServiceModel{
		{Service: "mongodb", Instances: []string{"my_nosql"}},
	}
	c.Assert(services, check.DeepEquals, expected)
	action := rectest.Action{Action: "list-services", User: s.user.Email}
	c.Assert(action, rectest.IsRecorded)
}

func makeRequestToCreateHandler(c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	return makeRequestWithManifest(baseManifest, c)
}

func makeRequestWithManifest(manifest string, c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	b := bytes.NewBufferString(manifest)
	request, err := http.NewRequest("POST", "/services", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ProvisionSuite) TestCreateHandlerSavesNameFromManifestID(c *check.C) {
	recorder, request := makeRequestToCreateHandler(c)
	err := serviceCreate(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	query := bson.M{"_id": "some_service"}
	var rService service.Service
	err = s.conn.Services().Find(query).One(&rService)
	c.Assert(err, check.IsNil)
	c.Assert(rService.Name, check.Equals, "some_service")
	endpoints := map[string]string{
		"production": "someservice.com",
	}
	action := rectest.Action{
		Action: "create-service",
		User:   s.user.Email,
		Extra:  []interface{}{"some_service", endpoints},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *ProvisionSuite) TestCreateHandlerSavesServiceMetadata(c *check.C) {
	recorder, request := makeRequestToCreateHandler(c)
	err := serviceCreate(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	query := bson.M{"_id": "some_service"}
	var rService service.Service
	err = s.conn.Services().Find(query).One(&rService)
	c.Assert(err, check.IsNil)
	c.Assert(rService.Endpoint["production"], check.Equals, "someservice.com")
	c.Assert(rService.Password, check.Equals, "xxxx")
	c.Assert(rService.Username, check.Equals, "test")
}

func (s *ProvisionSuite) TestCreateHandlerWithContentOfRealYaml(c *check.C) {
	p, err := filepath.Abs("testdata/manifest.yml")
	manifest, err := ioutil.ReadFile(p)
	recorder, request := makeRequestWithManifest(string(manifest), c)
	err = serviceCreate(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	query := bson.M{"_id": "mysqlapi"}
	var rService service.Service
	err = s.conn.Services().Find(query).One(&rService)
	c.Assert(err, check.IsNil)
	c.Assert(rService.Endpoint["production"], check.Equals, "mysqlapi.com")
}

func (s *ProvisionSuite) TestCreateHandlerShouldReturnErrorWhenNameExists(c *check.C) {
	recorder, request := makeRequestToCreateHandler(c)
	err := serviceCreate(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	recorder, request = makeRequestToCreateHandler(c)
	err = serviceCreate(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^Service already exists.$")
}

func (s *ProvisionSuite) TestCreateHandlerSavesOwnerTeamsFromUserWhoCreated(c *check.C) {
	recorder, request := makeRequestToCreateHandler(c)
	err := serviceCreate(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Body.String(), check.Equals, "success")
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	query := bson.M{"_id": "some_service"}
	var rService service.Service
	err = s.conn.Services().Find(query).One(&rService)
	c.Assert(err, check.IsNil)
	c.Assert("some_service", check.Equals, rService.Name)
	c.Assert(rService.OwnerTeams, check.DeepEquals, []string{s.team.Name})
}

func (s *ProvisionSuite) TestCreateHandlerReturnsForbiddenIfTheUserIsNotMemberOfAnyTeam(c *check.C) {
	u := &auth.User{Email: "enforce@queensryche.com", Password: "123456"}
	_, err := nativeScheme.Create(u)
	c.Assert(err, check.IsNil)
	defer s.conn.Users().RemoveAll(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	recorder, request := makeRequestToCreateHandler(c)
	err = serviceCreate(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e, check.ErrorMatches, "^In order to create a service, you should be member of at least one team$")
}

func (s *ProvisionSuite) TestCreateHandlerReturnsBadRequestIfTheServiceDoesNotHaveAProductionEndpoint(c *check.C) {
	p, err := filepath.Abs("testdata/manifest-without-endpoint.yml")
	manifest, err := ioutil.ReadFile(p)
	recorder, request := makeRequestWithManifest(string(manifest), c)
	err = serviceCreate(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, "You must provide a production endpoint in the manifest file.")
}

func (s *ProvisionSuite) TestCreateHandlerReturnsBadRequestWithoutPassword(c *check.C) {
	recorder, request := makeRequestWithManifest(manifestWithoutPassword, c)
	err := serviceCreate(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, "You must provide a password in the manifest file.")
}

func (s *ProvisionSuite) TestCreateHandlerReturnsBadRequestWithoutId(c *check.C) {
	recorder, request := makeRequestWithManifest(manifestWithoutId, c)
	err := serviceCreate(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, "You must provide an id in the manifest file.")
}

func (s *ProvisionSuite) TestUpdateHandlerShouldUpdateTheServiceWithDataFromManifest(c *check.C) {
	service := service.Service{
		Name:       "mysqlapi",
		Endpoint:   map[string]string{"production": "sqlapi.com"},
		OwnerTeams: []string{s.team.Name},
		Password:   "oldold",
	}
	err := service.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": service.Name})
	p, err := filepath.Abs("testdata/manifest.yml")
	manifest, err := ioutil.ReadFile(p)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", "/services", bytes.NewBuffer(manifest))
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceUpdate(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
	err = s.conn.Services().Find(bson.M{"_id": service.Name}).One(&service)
	c.Assert(err, check.IsNil)
	c.Assert(service.Endpoint["production"], check.Equals, "mysqlapi.com")
	c.Assert(service.Password, check.Equals, "yyyy")
	c.Assert(service.Username, check.Equals, "mysqltest")
	endpoints := map[string]string{"production": "mysqlapi.com"}
	action := rectest.Action{
		Action: "update-service",
		User:   s.user.Email,
		Extra:  []interface{}{service.Name, endpoints},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *ProvisionSuite) TestUpdateHandlerReturnsBadRequestWithoutPassword(c *check.C) {
	recorder, request := makeRequestWithManifest(manifestWithoutPassword, c)
	err := serviceUpdate(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, "You must provide a password in the manifest file.")
}

func (s *ProvisionSuite) TestUpdateHandlerReturnsBadRequestWithoutProductionEndpoint(c *check.C) {
	p, err := filepath.Abs("testdata/manifest-without-endpoint.yml")
	manifest, err := ioutil.ReadFile(p)
	recorder, request := makeRequestWithManifest(string(manifest), c)
	err = serviceUpdate(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, "You must provide a production endpoint in the manifest file.")
}

func (s *ProvisionSuite) TestUpdateHandlerReturns404WhenTheServiceDoesNotExist(c *check.C) {
	p, err := filepath.Abs("testdata/manifest.yml")
	c.Assert(err, check.IsNil)
	manifest, err := ioutil.ReadFile(p)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", "/services", bytes.NewBuffer(manifest))
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceUpdate(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^Service not found$")
}

func (s *ProvisionSuite) TestUpdateHandlerReturns403WhenTheUserIsNotOwnerOfTheTeam(c *check.C) {
	se := service.Service{Name: "mysqlapi", Teams: []string{s.team.Name}}
	se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	p, err := filepath.Abs("testdata/manifest.yml")
	c.Assert(err, check.IsNil)
	manifest, err := ioutil.ReadFile(p)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", "/services", bytes.NewBuffer(manifest))
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceUpdate(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e, check.ErrorMatches, "^This user does not have access to this service$")
}

func (s *ProvisionSuite) TestDeleteHandler(c *check.C) {
	se := service.Service{Name: "Mysql", OwnerTeams: []string{s.team.Name}}
	se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/services/%s?:name=%s", se.Name, se.Name), nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceDelete(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
	query := bson.M{"_id": se.Name}
	err = s.conn.Services().Find(query).One(&se)
	count, err := s.conn.Services().Find(query).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
	action := rectest.Action{
		Action: "delete-service",
		User:   s.user.Email,
		Extra:  []interface{}{se.Name},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *ProvisionSuite) TestDeleteHandlerReturns404WhenTheServiceDoesNotExist(c *check.C) {
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/services/%s?:name=%s", "mongodb", "mongodb"), nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceDelete(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^Service not found$")
}

func (s *ProvisionSuite) TestDeleteHandlerReturns403WhenTheUserIsNotOwnerOfTheTeam(c *check.C) {
	se := service.Service{Name: "Mysql", Teams: []string{s.team.Name}}
	se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/services/%s?:name=%s", se.Name, se.Name), nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceDelete(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e, check.ErrorMatches, "^This user does not have access to this service$")
}

func (s *ProvisionSuite) TestDeleteHandlerReturns403WhenTheServiceHasInstance(c *check.C) {
	se := service.Service{Name: "mysql", OwnerTeams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: se.Name}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	defer service.DeleteInstance(&instance)
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/services/%s?:name=%s", se.Name, se.Name), nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceDelete(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e, check.ErrorMatches, "^This service cannot be removed because it has instances.\nPlease remove these instances before removing the service.$")
}

func (s *ProvisionSuite) TestGrantServiceAccessToTeam(c *check.C) {
	t := &auth.Team{Name: "blaaaa"}
	s.conn.Teams().Insert(t)
	defer s.conn.Teams().Remove(bson.M{"name": t.Name})
	se := service.Service{Name: "my_service", Teams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, t.Name, se.Name, t.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = grantServiceAccess(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	err = se.Get()
	c.Assert(err, check.IsNil)
	c.Assert(*s.team, HasAccessTo, se)
	action := rectest.Action{
		Action: "grant-service-access",
		User:   s.user.Email,
		Extra:  []interface{}{"service=" + se.Name, "team=" + t.Name},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *ProvisionSuite) TestGrantAccesToTeamReturnNotFoundIfTheServiceDoesNotExist(c *check.C) {
	url := fmt.Sprintf("/services/nononono/%s?:service=nononono&:team=%s", s.team.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = grantServiceAccess(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^Service not found$")
}

func (s *ProvisionSuite) TestGrantServiceAccessToTeamReturnForbiddenIfTheGivenUserDoesNotHaveAccessToTheService(c *check.C) {
	se := service.Service{Name: "my_service"}
	err := se.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, s.team.Name, se.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = grantServiceAccess(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e, check.ErrorMatches, "^This user does not have access to this service$")
}

func (s *ProvisionSuite) TestGrantServiceAccessToTeamReturnNotFoundIfTheTeamDoesNotExist(c *check.C) {
	se := service.Service{Name: "my_service", Teams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/nonono?:service=%s&:team=nonono", se.Name, se.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = grantServiceAccess(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^Team not found$")
}

func (s *ProvisionSuite) TestGrantServiceAccessToTeamReturnConflictIfTheTeamAlreadyHasAccessToTheService(c *check.C) {
	se := service.Service{Name: "my_service", Teams: []string{s.team.Name}}
	err := se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, s.team.Name, se.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = grantServiceAccess(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusConflict)
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamRemovesTeamFromService(c *check.C) {
	t := &auth.Team{Name: "alle-da"}
	se := service.Service{Name: "my_service", Teams: []string{s.team.Name, t.Name}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, s.team.Name, se.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = revokeServiceAccess(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	err = se.Get()
	c.Assert(err, check.IsNil)
	c.Assert(*s.team, check.Not(HasAccessTo), se)
	action := rectest.Action{
		Action: "revoke-service-access",
		User:   s.user.Email,
		Extra:  []interface{}{"service=" + se.Name, "team=" + s.team.Name},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamReturnsNotFoundIfTheServiceDoesNotExist(c *check.C) {
	url := fmt.Sprintf("/services/nonono/%s?:service=nonono&:team=%s", s.team.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = revokeServiceAccess(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^Service not found$")
}

func (s *ProvisionSuite) TestRevokeAccesFromTeamReturnsForbiddenIfTheGivenUserDoesNotHasAccessToTheService(c *check.C) {
	t := &auth.Team{Name: "alle-da"}
	se := service.Service{Name: "my_service", Teams: []string{t.Name}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, t.Name, se.Name, t.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = revokeServiceAccess(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e, check.ErrorMatches, "^This user does not have access to this service$")
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamReturnsNotFoundIfTheTeamDoesNotExist(c *check.C) {
	se := service.Service{Name: "my_service", Teams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/nonono?:service=%s&:team=nonono", se.Name, se.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = revokeServiceAccess(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^Team not found$")
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamReturnsForbiddenIfTheTeamIsTheOnlyWithAccessToTheService(c *check.C) {
	se := service.Service{Name: "my_service", Teams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, s.team.Name, se.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = revokeServiceAccess(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e, check.ErrorMatches, "^You can not revoke the access from this team, because it is the unique team with access to this service, and a service can not be orphaned$")
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamReturnNotFoundIfTheTeamDoesNotHasAccessToTheService(c *check.C) {
	t := &auth.Team{Name: "Rammlied"}
	s.conn.Teams().Insert(t)
	defer s.conn.Teams().RemoveAll(bson.M{"name": t.Name})
	se := service.Service{Name: "my_service", Teams: []string{s.team.Name, s.team.Name}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	url := fmt.Sprintf("/services/%s/%s?:service=%s&:team=%s", se.Name, t.Name, se.Name, t.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = revokeServiceAccess(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
}

func (s *ProvisionSuite) TestAddDocHandlerReturns404WhenTheServiceDoesNotExist(c *check.C) {
	b := bytes.NewBufferString("doc")
	request, err := http.NewRequest("PUT", fmt.Sprintf("/services/%s/doc?:name=%s", "mongodb", "mongodb"), b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceAddDoc(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^Service not found$")
}

func (s *ProvisionSuite) TestAddDocHandler(c *check.C) {
	se := service.Service{Name: "some_service", OwnerTeams: []string{s.team.Name}}
	se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	b := bytes.NewBufferString("doc")
	request, err := http.NewRequest("PUT", fmt.Sprintf("/services/%s/doc?:name=%s", se.Name, se.Name), b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceAddDoc(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	query := bson.M{"_id": "some_service"}
	var serv service.Service
	err = s.conn.Services().Find(query).One(&serv)
	c.Assert(err, check.IsNil)
	c.Assert(serv.Doc, check.Equals, "doc")
	action := rectest.Action{
		Action: "service-add-doc",
		User:   s.user.Email,
		Extra:  []interface{}{"some_service", "doc"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *ProvisionSuite) TestAddDocHandlerReturns403WhenTheUserDoesNotHaveAccessToTheService(c *check.C) {
	se := service.Service{Name: "Mysql"}
	se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	b := bytes.NewBufferString("doc")
	request, err := http.NewRequest("PUT", fmt.Sprintf("/services/%s/doc?:name=%s", se.Name, se.Name), b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceAddDoc(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e, check.ErrorMatches, "^This user does not have access to this service$")
}

func (s *ProvisionSuite) TestgetServiceByOwner(c *check.C) {
	srv := service.Service{Name: "foo", OwnerTeams: []string{s.team.Name}}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	defer srv.Delete()
	rSrv, err := getServiceByOwner("foo", s.user)
	c.Assert(err, check.IsNil)
	c.Assert(rSrv.Name, check.Equals, srv.Name)
}

func (s *ProvisionSuite) TestServicesAndInstancesByOwnerTeams(c *check.C) {
	srvc := service.Service{Name: "mysql", OwnerTeams: []string{s.team.Name}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer srvc.Delete()
	srvc2 := service.Service{Name: "mongodb"}
	err = srvc2.Create()
	c.Assert(err, check.IsNil)
	defer srvc2.Delete()
	sInstance := service.ServiceInstance{Name: "foo", ServiceName: "mysql"}
	err = sInstance.Create()
	c.Assert(err, check.IsNil)
	defer service.DeleteInstance(&sInstance)
	sInstance2 := service.ServiceInstance{Name: "bar", ServiceName: "mongodb"}
	err = sInstance2.Create()
	defer service.DeleteInstance(&sInstance2)
	results := servicesAndInstancesByOwner(s.user)
	expected := []service.ServiceModel{
		{Service: "mysql", Instances: []string{"foo"}},
	}
	c.Assert(results, check.DeepEquals, expected)
}
