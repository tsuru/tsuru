package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/errors"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"time"
)

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

func makeRequestToCreateInstanceHandler(c *C) (*httptest.ResponseRecorder, *http.Request) {
	b := bytes.NewBufferString(`{"name": "brainSQL", "service_name": "mysql", "app": "my_app"}`)
	request, err := http.NewRequest("POST", "/services/instances", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *S) TestCreateHandlerSavesNameFromManifestId(c *C) {
	recorder, request := makeRequestToCreateHandler(c)
	err := CreateHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	query := bson.M{"_id": "some_service"}
	var rService Service
	err = db.Session.Services().Find(query).One(&rService)
	c.Assert(err, IsNil)
	c.Assert(rService.Name, Equals, "some_service")
}

func (s *S) TestCreateHandlerSavesEndpointServiceProperty(c *C) {
	recorder, request := makeRequestToCreateHandler(c)
	err := CreateHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	query := bson.M{"_id": "some_service"}
	var rService Service
	err = db.Session.Services().Find(query).One(&rService)
	c.Assert(err, IsNil)
	c.Assert(rService.Endpoint["production"], Equals, "someservice.com")
	c.Assert(rService.Endpoint["test"], Equals, "test.someservice.com")
}

func (s *S) TestCreateHandlerWithContentOfRealYaml(c *C) {
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
	var rService Service
	err = db.Session.Services().Find(query).One(&rService)
	c.Assert(err, IsNil)
	c.Assert(rService.Endpoint["production"], Equals, "mysqlapi.com")
	c.Assert(rService.Endpoint["test"], Equals, "localhost:8000")
	c.Assert(rService.Bootstrap["ami"], Equals, "ami-00000007")
	c.Assert(rService.Bootstrap["when"], Equals, "on-new-instance")
}

func (s *S) TestCreateHandlerShouldReturnErrorWhenNameExists(c *C) {
	recorder, request := makeRequestToCreateHandler(c)
	err := CreateHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	recorder, request = makeRequestToCreateHandler(c)
	err = CreateHandler(recorder, request, s.user)
	c.Assert(err, Not(IsNil))
	c.Assert(err, ErrorMatches, "^Service with name some_service already exists.$")
}

func (s *S) TestCreateHandlerGetAllTeamsFromTheUser(c *C) {
	recorder, request := makeRequestToCreateHandler(c)
	err := CreateHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.String(), Equals, "success")
	c.Assert(recorder.Code, Equals, 200)
	query := bson.M{"_id": "some_service"}
	var rService Service
	err = db.Session.Services().Find(query).One(&rService)
	c.Assert(err, IsNil)
	c.Assert(rService.Name, Equals, "some_service")
	c.Assert(*s.team, HasAccessTo, rService)
}

func (s *S) TestCreateHandlerReturnsForbiddenIfTheUserIsNotMemberOfAnyTeam(c *C) {
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

func (s *S) TestCreateInstanceHandlerVMOnNewInstanceWhenManifestSaysSo(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	service := Service{
		Name: "mysql",
		Bootstrap: map[string]string{
			"ami":  "ami-0000007",
			"when": OnNewInstance,
		},
		Teams: []string{s.team.Name},
		Endpoint: map[string]string{
			"production": ts.URL,
		},
	}
	err := service.Create()
	c.Assert(err, IsNil)
	recorder, request := makeRequestToCreateInstanceHandler(c)
	err = CreateInstanceHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	q := bson.M{"_id": "brainSQL", "instances": bson.M{"$ne": ""}}
	var si ServiceInstance
	err = db.Session.ServiceInstances().Find(q).One(&si)
	c.Assert(err, IsNil)
	si.Host = "192.168.0.110"
	si.State = "running"
	db.Session.ServiceInstances().Update(bson.M{"_id": si.Name}, si)
	err = db.Session.ServiceInstances().Find(q).One(&si)
	c.Assert(err, IsNil)
	c.Assert(si.Instance, Equals, "i-0")
}

func (suite *S) TestCreateInstanceHandlerSavesServiceInstanceInDb(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	s := Service{Name: "mysql", Teams: []string{suite.team.Name}, Endpoint: map[string]string{"production": ts.URL}}
	s.Create()
	defer s.Delete()
	recorder, request := makeRequestToCreateInstanceHandler(c)
	err := CreateInstanceHandler(recorder, request, suite.user)
	c.Assert(err, IsNil)
	var si ServiceInstance
	err = db.Session.ServiceInstances().Find(bson.M{"_id": "brainSQL", "service_name": "mysql"}).One(&si)
	c.Assert(err, IsNil)
	si.Host = "192.168.0.110"
	si.State = "running"
	db.Session.ServiceInstances().Update(bson.M{"_id": si.Name}, si)
	c.Assert(si.Name, Equals, "brainSQL")
	c.Assert(si.ServiceName, Equals, "mysql")
}

func (s *S) TestCreateInstanceHandlerCallsTheServiceAPIAndSaveEnvironmentVariablesInTheInstance(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	service := Service{Name: "mysql", Teams: []string{s.team.Name}, Endpoint: map[string]string{"production": ts.URL}}
	service.Create()
	recorder, request := makeRequestToCreateInstanceHandler(c)
	err := CreateInstanceHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	var si ServiceInstance
	db.Session.ServiceInstances().Find(bson.M{"_id": "brainSQL"}).One(&si)
	si.Host = "192.168.0.110"
	si.State = "running"
	db.Session.ServiceInstances().Update(bson.M{"_id": si.Name}, si)
	time.Sleep(2e9)
	db.Session.ServiceInstances().Find(bson.M{"_id": "brainSQL"}).One(&si)
	c.Assert(si.Env, DeepEquals, map[string]string{"DATABASE_HOST": "localhost"})
}

func (s *S) TestCreateInstanceHandlerSavesAllTeamsThatTheGivenUserIsMemberAndHasAccessToTheServiceInTheInstance(c *C) {
	t := auth.Team{Name: "judaspriest", Users: []auth.User{*s.user}}
	err := db.Session.Teams().Insert(t)
	defer db.Session.Teams().Remove(bson.M{"name": t.Name})
	service := Service{Name: "mysql", Teams: []string{s.team.Name}}
	err = service.Create()
	c.Assert(err, IsNil)
	recorder, request := makeRequestToCreateInstanceHandler(c)
	err = CreateInstanceHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	var si ServiceInstance
	err = db.Session.ServiceInstances().Find(bson.M{"_id": "brainSQL"}).One(&si)
	c.Assert(err, IsNil)
	si.Host = "192.168.0.110"
	si.State = "running"
	db.Session.ServiceInstances().Update(bson.M{"_id": si.Name}, si)
	c.Assert(si.Teams, DeepEquals, []string{s.team.Name})
}

func (s *S) TestCreateInstanceHandlerDoesNotFailIfTheServiceDoesNotDeclareEndpoint(c *C) {
	service := Service{Name: "mysql", Teams: []string{s.team.Name}}
	service.Create()
	recorder, request := makeRequestToCreateInstanceHandler(c)
	err := CreateInstanceHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	var si ServiceInstance
	err = db.Session.ServiceInstances().Find(bson.M{"_id": "brainSQL"}).One(&si)
	c.Assert(err, IsNil)
	err = db.Session.ServiceInstances().Find(bson.M{"_id": "brainSQL"}).One(&si)
	c.Assert(err, IsNil)
	si.Host = "192.168.0.110"
	si.State = "running"
	db.Session.ServiceInstances().Update(bson.M{"_id": si.Name}, si)
}

func (s *S) TestCreateInstanceHandlerReturnsErrorWhenUserCannotUseService(c *C) {
	service := Service{Name: "mysql"}
	service.Create()
	recorder, request := makeRequestToCreateInstanceHandler(c)
	err := CreateInstanceHandler(recorder, request, s.user)
	c.Assert(err, ErrorMatches, "^You don't have access to service mysql$")
}

func (s *S) TestCreateInstanceHandlerReturnsErrorWhenServiceDoesntExists(c *C) {
	recorder, request := makeRequestToCreateInstanceHandler(c)
	err := CreateInstanceHandler(recorder, request, s.user)
	c.Assert(err, ErrorMatches, "^Service mysql does not exists.$")
}

func (s *S) TestCallServiceApi(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	service := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := service.Create()
	si := ServiceInstance{Name: "brainSQL", Host: "192.168.0.110", State: "running"}
	si.Create()
	defer si.Delete()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": "mysql"})
	callServiceApi(service, si)
	db.Session.ServiceInstances().Find(bson.M{"_id": si.Name}).One(&si)
	c.Assert(si.Env, DeepEquals, map[string]string{"DATABASE_USER": "root", "DATABASE_PASSWORD": "s3cr3t"})

}

func (s *S) TestAsyncCAllServiceApi(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	service := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := service.Create()
	si := ServiceInstance{Name: "brainSQL"}
	si.Create()
	defer si.Delete()
	go callServiceApi(service, si)
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": "mysql"})
	si.State = "running"
	si.Host = "192.168.0.110"
	err = db.Session.ServiceInstances().Update(bson.M{"_id": si.Name}, si)
	c.Assert(err, IsNil)
	time.Sleep(2e9)
	db.Session.ServiceInstances().Find(bson.M{"_id": si.Name}).One(&si)
	c.Assert(si.Env, DeepEquals, map[string]string{"DATABASE_USER": "root", "DATABASE_PASSWORD": "s3cr3t"})
}

func (s *S) TestDeleteHandler(c *C) {
	se := Service{Name: "Mysql", Teams: []string{s.team.Name}}
	se.Create()
	defer se.Delete()
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/services/%s?:name=%s", se.Name, se.Name), nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = DeleteHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, http.StatusNoContent)
	query := bson.M{"_id": "Mysql"}
	qtd, err := db.Session.Services().Find(query).Count()
	c.Assert(err, IsNil)
	c.Assert(qtd, Equals, 0)
}

func (s *S) TestDeleteHandlerReturns404WhenTheServiceDoesNotExist(c *C) {
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

func (s *S) TestDeleteHandlerReturns403WhenTheUserDoesNotHaveAccessToTheService(c *C) {
	se := Service{Name: "Mysql"}
	se.Create()
	defer se.Delete()
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

func (s *S) TestDeleteHandlerReturns403WhenTheServiceHasInstance(c *C) {
	se := Service{Name: "mysql", Teams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, IsNil)
	defer se.Delete()
	instance := ServiceInstance{Name: "my-mysql", ServiceName: se.Name}
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

func (s *S) TestGrantAccessToTeam(c *C) {
	t := &auth.Team{Name: "blaaaa"}
	db.Session.Teams().Insert(t)
	defer db.Session.Teams().Remove(bson.M{"name": t.Name})
	se := Service{Name: "my_service", Teams: []string{s.team.Name}}
	err := se.Create()
	defer se.Delete()
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

func (s *S) TestGrantAccesToTeamReturnNotFoundIfTheServiceDoesNotExist(c *C) {
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

func (s *S) TestGrantAccessToTeamReturnForbiddenIfTheGivenUserDoesNotHaveAccessToTheService(c *C) {
	se := Service{Name: "my_service"}
	err := se.Create()
	c.Assert(err, IsNil)
	defer se.Delete()
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

func (s *S) TestGrantAccessToTeamReturnNotFoundIfTheTeamDoesNotExist(c *C) {
	se := Service{Name: "my_service", Teams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, IsNil)
	defer se.Delete()
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

func (s *S) TestGrantAccessToTeamReturnConflictIfTheTeamAlreadyHasAccessToTheService(c *C) {
	se := Service{Name: "my_service", Teams: []string{s.team.Name}}
	err := se.Create()
	defer se.Delete()
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

func (s *S) TestRevokeAccessFromTeamRemovesTeamFromService(c *C) {
	t := &auth.Team{Name: "alle-da"}
	se := Service{Name: "my_service", Teams: []string{s.team.Name, t.Name}}
	err := se.Create()
	c.Assert(err, IsNil)
	defer se.Delete()
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

func (s *S) TestRevokeAccessFromTeamReturnsNotFoundIfTheServiceDoesNotExist(c *C) {
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

func (s *S) TestRevokeAccesFromTeamReturnsForbiddenIfTheGivenUserDoesNotHasAccessToTheService(c *C) {
	t := &auth.Team{Name: "alle-da"}
	se := Service{Name: "my_service", Teams: []string{t.Name}}
	err := se.Create()
	c.Assert(err, IsNil)
	defer se.Delete()
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

func (s *S) TestRevokeAccessFromTeamReturnsNotFoundIfTheTeamDoesNotExist(c *C) {
	se := Service{Name: "my_service", Teams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, IsNil)
	defer se.Delete()
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

func (s *S) TestRevokeAccessFromTeamReturnsForbiddenIfTheTeamIsTheOnlyWithAccessToTheService(c *C) {
	se := Service{Name: "my_service", Teams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, IsNil)
	defer se.Delete()
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

func (s *S) TestRevokeAccessFromTeamReturnNotFoundIfTheTeamDoesNotHasAccessToTheService(c *C) {
	t := &auth.Team{Name: "Rammlied"}
	db.Session.Teams().Insert(t)
	defer db.Session.Teams().RemoveAll(bson.M{"name": t.Name})
	se := Service{Name: "my_service", Teams: []string{s.team.Name, s.team.Name}}
	err := se.Create()
	c.Assert(err, IsNil)
	defer se.Delete()
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

func (s *S) TestServicesHandler(c *C) {
	service := Service{Name: "redis", Teams: []string{s.team.Name}}
	err := service.Create()
	c.Assert(err, IsNil)
	instance := ServiceInstance{
		Name:        "redis-globo",
		ServiceName: "redis",
		Apps:        []string{"globo"},
		Teams:       []string{s.team.Name},
	}
	err = instance.Create()
	c.Assert(err, IsNil)
	request, err := http.NewRequest("GET", "/services/instances", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = ServicesInstancesHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	var instances map[string][]string
	err = json.Unmarshal(body, &instances)
	c.Assert(err, IsNil)
	expected := map[string][]string{
		"redis": []string{"redis-globo"},
	}
	c.Assert(instances, DeepEquals, expected)
}

func (s *S) TestServicesInstancesHandlerReturnsOnlyServicesThatTheUserHasAccess(c *C) {
	u := &auth.User{Email: "me@globo.com", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)
	defer db.Session.Users().Remove(bson.M{"email": u.Email})
	service := Service{Name: "redis"}
	err = service.Create()
	c.Assert(err, IsNil)
	instance := ServiceInstance{
		Name:        "redis-globo",
		ServiceName: "redis",
		Apps:        []string{"globo"},
	}
	err = instance.Create()
	c.Assert(err, IsNil)
	request, err := http.NewRequest("GET", "/services/instances", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = ServicesInstancesHandler(recorder, request, u)
	c.Assert(err, IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	var instances map[string][]string
	err = json.Unmarshal(body, &instances)
	c.Assert(err, IsNil)
	c.Assert(instances, DeepEquals, map[string][]string(nil))
}

func (s *S) TestServicesInstancesHandlerFilterInstancesPerServiceIncludingServicesThatDoesNotHaveInstances(c *C) {
	u := &auth.User{Email: "me@globo.com", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)
	defer db.Session.Users().Remove(bson.M{"email": u.Email})
	serviceNames := []string{"redis", "mysql", "pgsql", "memcached"}
	defer db.Session.Services().RemoveAll(bson.M{"name": bson.M{"$in": serviceNames}})
	defer db.Session.ServiceInstances().RemoveAll(bson.M{"service_name": bson.M{"$in": serviceNames}})
	for _, name := range serviceNames {
		service := Service{Name: name, Teams: []string{s.team.Name}}
		err = service.Create()
		c.Assert(err, IsNil)
		instance := ServiceInstance{
			Name:        service.Name + "1",
			ServiceName: service.Name,
			Teams:       []string{s.team.Name},
		}
		err = instance.Create()
		c.Assert(err, IsNil)
		instance = ServiceInstance{
			Name:        service.Name + "2",
			ServiceName: service.Name,
			Teams:       []string{s.team.Name},
		}
		err = instance.Create()
	}
	service := Service{Name: "oracle", Teams: []string{s.team.Name}}
	err = service.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"name": "oracle"})
	request, err := http.NewRequest("GET", "/services/instances", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = ServicesInstancesHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	var instances map[string][]string
	err = json.Unmarshal(body, &instances)
	c.Assert(err, IsNil)
	expected := map[string][]string{
		"redis":     []string{"redis1", "redis2"},
		"mysql":     []string{"mysql1", "mysql2"},
		"pgsql":     []string{"pgsql1", "pgsql2"},
		"memcached": []string{"memcached1", "memcached2"},
		"oracle":    []string{},
	}
	c.Assert(instances, DeepEquals, expected)
}

func (s *S) makeRequestToServicesHandler(c *C) (*httptest.ResponseRecorder, *http.Request) {
	request, err := http.NewRequest("GET", "/services", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *S) TestServicesHandlerShoudGetAllServicesFromUsersTeam(c *C) {
	srv := Service{Name: "mongodb", Teams: []string{s.team.Name}}
	srv.Create()
	defer srv.Delete()
	si := ServiceInstance{Name: "my_nosql", ServiceName: srv.Name}
	si.Create()
	defer si.Delete()
	recorder, request := s.makeRequestToServicesHandler(c)
	err := ServicesHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	services := make([]ServiceModel, 1)
	err = json.Unmarshal(b, &services)
	expected := []ServiceModel{
		ServiceModel{Service: "mongodb", Instances: []string{"my_nosql"}},
	}
	c.Assert(services, DeepEquals, expected)
}

func (s *S) TestServiceAndServiceInstancesByTeams(c *C) {
	srv := Service{Name: "mongodb", Teams: []string{s.team.Name}}
	srv.Create()
	defer srv.Delete()
	si := ServiceInstance{Name: "my_nosql", ServiceName: srv.Name}
	si.Create()
	defer si.Delete()
	obtained := serviceAndServiceInstancesByTeams(s.user)
	expected := []ServiceModel{
		ServiceModel{Service: "mongodb", Instances: []string{"my_nosql"}},
	}
	c.Assert(obtained, DeepEquals, expected)
}
