package consumption

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/service"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/errors"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

func (s *S) makeRequestToServicesHandler(c *C) (*httptest.ResponseRecorder, *http.Request) {
	request, err := http.NewRequest("GET", "/services", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *S) TestServicesHandlerShoudGetAllServicesFromUsersTeam(c *C) {
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
	services := make([]ServiceModel, 1)
	err = json.Unmarshal(b, &services)
	expected := []ServiceModel{
		ServiceModel{Service: "mongodb", Instances: []string{"my_nosql"}},
	}
	c.Assert(services, DeepEquals, expected)
}

func makeRequestToCreateInstanceHandler(c *C) (*httptest.ResponseRecorder, *http.Request) {
	b := bytes.NewBufferString(`{"name": "brainSQL", "service_name": "mysql", "app": "my_app"}`)
	request, err := http.NewRequest("POST", "/services/instances", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (suite *S) TestCreateInstanceHandlerSavesServiceInstanceInDb(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	se := service.Service{Name: "mysql", Teams: []string{suite.team.Name}, Endpoint: map[string]string{"production": ts.URL}}
	se.Create()
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	recorder, request := makeRequestToCreateInstanceHandler(c)
	err := CreateInstanceHandler(recorder, request, suite.user)
	c.Assert(err, IsNil)
	var si service.ServiceInstance
	err = db.Session.ServiceInstances().Find(bson.M{"_id": "brainSQL", "service_name": "mysql"}).One(&si)
	c.Assert(err, IsNil)
	db.Session.ServiceInstances().Update(bson.M{"_id": si.Name}, si)
	c.Assert(si.Name, Equals, "brainSQL")
	c.Assert(si.ServiceName, Equals, "mysql")
}

func (s *S) TestCreateInstanceHandlerSavesAllTeamsThatTheGivenUserIsMemberAndHasAccessToTheServiceInTheInstance(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	t := auth.Team{Name: "judaspriest", Users: []string{s.user.Email}}
	err := db.Session.Teams().Insert(t)
	defer db.Session.Teams().Remove(bson.M{"name": t.Name})
	srv := service.Service{Name: "mysql", Teams: []string{s.team.Name}, IsRestricted: true, Endpoint: map[string]string{"production": ts.URL}}
	err = srv.Create()
	c.Assert(err, IsNil)
	recorder, request := makeRequestToCreateInstanceHandler(c)
	err = CreateInstanceHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	var si service.ServiceInstance
	err = db.Session.ServiceInstances().Find(bson.M{"_id": "brainSQL"}).One(&si)
	c.Assert(err, IsNil)
	db.Session.ServiceInstances().Update(bson.M{"_id": si.Name}, si)
	c.Assert(si.Teams, DeepEquals, []string{s.team.Name})
}

func (s *S) TestCreateInstanceHandlerReturnsErrorWhenUserCannotUseService(c *C) {
	service := service.Service{Name: "mysql", IsRestricted: true}
	service.Create()
	recorder, request := makeRequestToCreateInstanceHandler(c)
	err := CreateInstanceHandler(recorder, request, s.user)
	c.Assert(err, ErrorMatches, "^This user does not have access to this service$")
}

func (s *S) TestCreateInstanceHandlerIgnoresTeamAuthIfServiceIsNotRestricted(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": "mysql"})
	recorder, request := makeRequestToCreateInstanceHandler(c)
	err = CreateInstanceHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	var si service.ServiceInstance
	err = db.Session.ServiceInstances().Find(bson.M{"_id": "brainSQL"}).One(&si)
	c.Assert(err, IsNil)
	c.Assert(si.Name, Equals, "brainSQL")
	c.Assert(si.Teams, DeepEquals, []string{s.team.Name})
}

func (s *S) TestCreateInstanceHandlerReturnsErrorWhenServiceDoesntExists(c *C) {
	recorder, request := makeRequestToCreateInstanceHandler(c)
	err := CreateInstanceHandler(recorder, request, s.user)
	c.Assert(err, ErrorMatches, "^Service mysql does not exist.$")
}

func makeRequestToRemoveInstanceHandler(name string, c *C) (*httptest.ResponseRecorder, *http.Request) {
	url := fmt.Sprintf("/services/c/instances/%s?:name=%s", name, name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *S) TestRemoveServiceInstanceHandler(c *C) {
	se := service.Service{Name: "foo"}
	err := se.Create()
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = si.Create()
	c.Assert(err, IsNil)
	recorder, request := makeRequestToRemoveInstanceHandler("foo-instance", c)
	err = RemoveServiceInstanceHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	c.Assert(string(b), Equals, "service instance successfuly removed")
	n, err := db.Session.ServiceInstances().Find(bson.M{"_id": "foo-instance"}).Count()
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 0)
}

func (s *S) TestRemoveServiceHandlerWithoutPermissionShouldReturn401(c *C) {
	se := service.Service{Name: "foo"}
	err := se.Create()
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo"}
	err = si.Create()
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": si.Name})
	c.Assert(err, IsNil)
	recorder, request := makeRequestToRemoveInstanceHandler("foo-instance", c)
	err = RemoveServiceInstanceHandler(recorder, request, s.user)
	c.Assert(err, ErrorMatches, "^This user does not have access to this service instance$")
}

func (s *S) TestRemoveServiceHandlerWIthAssociatedAppsShouldFailAndReturnError(c *C) {
	se := service.Service{Name: "foo"}
	err := se.Create()
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Apps: []string{"foo-bar"}, Teams: []string{s.team.Name}}
	err = si.Create()
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": si.Name})
	c.Assert(err, IsNil)
	recorder, request := makeRequestToRemoveInstanceHandler("foo-instance", c)
	err = RemoveServiceInstanceHandler(recorder, request, s.user)
	c.Assert(err, ErrorMatches, "^This service instance has binded apps. Unbind them before removing it$")
}

func (s *S) TestRemoveServiceShouldCallTheServiceAPI(c *C) {
	var called bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = r.Method == "DELETE" && r.URL.Path == "/resources/purity-instance"
	}))
	defer ts.Close()
	se := service.Service{Name: "purity", Endpoint: map[string]string{"production": ts.URL}}
	err := se.Create()
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, IsNil)
	si := service.ServiceInstance{Name: "purity-instance", ServiceName: "purity", Teams: []string{s.team.Name}}
	err = si.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": si.Name})
	recorder, request := makeRequestToRemoveInstanceHandler("purity-instance", c)
	err = RemoveServiceInstanceHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(called, Equals, true)
}

func (s *S) TestremoveServiceShouldNotRemoveTheServiceIfTheServiceAPICallFail(c *C) {
	var called bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = r.Method == "DELETE" && r.URL.Path == "/resources/purity-instance"
		w.WriteHeader(500)
		fmt.Fprint(w, "it's a test!")
	}))
	defer ts.Close()
	se := service.Service{Name: "deepercut", Endpoint: map[string]string{"production": ts.URL}}
	err := se.Create()
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, IsNil)
	si := service.ServiceInstance{Name: "deepercut-instance", ServiceName: "deepercut", Teams: []string{s.team.Name}}
	err = si.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": si.Name})
	recorder, request := makeRequestToRemoveInstanceHandler("deepercut-instance", c)
	err = RemoveServiceInstanceHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusInternalServerError)
	c.Assert(e.Message, Equals, "Failed to destroy the instance deepercut-instance: it's a test!")
}

func (s *S) TestServicesInstancesHandler(c *C) {
	srv := service.Service{Name: "redis", Teams: []string{s.team.Name}}
	err := srv.Create()
	c.Assert(err, IsNil)
	instance := service.ServiceInstance{
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
	var instances []ServiceModel
	err = json.Unmarshal(body, &instances)
	c.Assert(err, IsNil)
	expected := []ServiceModel{
		ServiceModel{Service: "redis", Instances: []string{"redis-globo"}},
	}
	c.Assert(instances, DeepEquals, expected)
}

func (s *S) TestServicesInstancesHandlerReturnsOnlyServicesThatTheUserHasAccess(c *C) {
	u := &auth.User{Email: "me@globo.com", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)
	defer db.Session.Users().Remove(bson.M{"email": u.Email})
	srv := service.Service{Name: "redis", IsRestricted: true}
	err = db.Session.Services().Insert(srv)
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": "redis"})
	instance := service.ServiceInstance{
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
	var instances []ServiceModel
	err = json.Unmarshal(body, &instances)
	c.Assert(err, IsNil)
	c.Assert(instances, DeepEquals, []ServiceModel{})
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
		srv := service.Service{Name: name, Teams: []string{s.team.Name}}
		err = srv.Create()
		c.Assert(err, IsNil)
		instance := service.ServiceInstance{
			Name:        srv.Name + "1",
			ServiceName: srv.Name,
			Teams:       []string{s.team.Name},
		}
		err = instance.Create()
		c.Assert(err, IsNil)
		instance = service.ServiceInstance{
			Name:        srv.Name + "2",
			ServiceName: srv.Name,
			Teams:       []string{s.team.Name},
		}
		err = instance.Create()
	}
	srv := service.Service{Name: "oracle", Teams: []string{s.team.Name}}
	err = srv.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"name": "oracle"})
	request, err := http.NewRequest("GET", "/services/instances", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = ServicesInstancesHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	var instances []ServiceModel
	err = json.Unmarshal(body, &instances)
	c.Assert(err, IsNil)
	expected := []ServiceModel{
		ServiceModel{Service: "redis", Instances: []string{"redis1", "redis2"}},
		ServiceModel{Service: "mysql", Instances: []string{"mysql1", "mysql2"}},
		ServiceModel{Service: "pgsql", Instances: []string{"pgsql1", "pgsql2"}},
		ServiceModel{Service: "memcached", Instances: []string{"memcached1", "memcached2"}},
		ServiceModel{Service: "oracle", Instances: []string(nil)},
	}
	c.Assert(instances, DeepEquals, expected)
}

func makeRequestToStatusHandler(name string, c *C) (*httptest.ResponseRecorder, *http.Request) {
	url := fmt.Sprintf("/services/instances/%s/status/?:instance=%s", name, name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *S) TestServiceInstanceStatusHandler(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		w.Write([]byte(`Service instance "my_nosql" is up`))
	}))
	defer ts.Close()
	srv := service.Service{Name: "mongodb", OwnerTeams: []string{s.team.Name}, Endpoint: map[string]string{"production": ts.URL}}
	err := srv.Create()
	c.Assert(err, IsNil)
	defer srv.Delete()
	si := service.ServiceInstance{Name: "my_nosql", ServiceName: srv.Name}
	err = si.Create()
	c.Assert(err, IsNil)
	defer si.Delete()
	recorder, request := makeRequestToStatusHandler("my_nosql", c)
	err = ServiceInstanceStatusHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(string(b), Equals, "Service instance \"my_nosql\" is up")
}

func (s *S) TestServiceInstanceStatusHandlerShouldReturnErrorWHenNameIsNotProvided(c *C) {
	recorder, request := makeRequestToStatusHandler("", c)
	err := ServiceInstanceStatusHandler(recorder, request, s.user)
	c.Assert(err, ErrorMatches, "^Service instance name not provided.$")
}

func (s *S) TestServiceInstanceStatusHandlerShouldReturnErrorWhenServiceInstanceNotExists(c *C) {
	recorder, request := makeRequestToStatusHandler("inexistent-instance", c)
	err := ServiceInstanceStatusHandler(recorder, request, s.user)
	c.Assert(err, ErrorMatches, "^Service instance does not exists, error: not found$")
}

func (s *S) TestServiceInstanceStatusHandlerShouldReturnErrorWhenServiceHasNoProductionEndpoint(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`Service instance "my_nosql" is up`))
	}))
	defer ts.Close()
	srv := service.Service{Name: "mongodb", OwnerTeams: []string{s.team.Name}}
	err := srv.Create()
	c.Assert(err, IsNil)
	defer srv.Delete()
	si := service.ServiceInstance{Name: "my_nosql", ServiceName: srv.Name}
	err = si.Create()
	c.Assert(err, IsNil)
	defer si.Delete()
	recorder, request := makeRequestToStatusHandler("my_nosql", c)
	err = ServiceInstanceStatusHandler(recorder, request, s.user)
	c.Assert(err, ErrorMatches, "^Unknown endpoint: production$")
}

func (s *S) TestServiceInfoHandler(c *C) {
	srv := service.Service{Name: "mongodb", Teams: []string{s.team.Name}}
	err := srv.Create()
	c.Assert(err, IsNil)
	defer srv.Delete()
	si1 := service.ServiceInstance{
		Name:        "my_nosql",
		ServiceName: srv.Name,
		Apps:        []string{},
		Teams:       []string{},
	}
	err = si1.Create()
	c.Assert(err, IsNil)
	defer si1.Delete()
	si2 := service.ServiceInstance{
		Name:        "your_nosql",
		ServiceName: srv.Name,
		Apps:        []string{"wordpress"},
		Teams:       []string{},
	}
	err = si2.Create()
	c.Assert(err, IsNil)
	defer si2.Delete()
	request, err := http.NewRequest("GET", fmt.Sprintf("/services/%s?:name=%s", "mongodb", "mongodb"), nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = ServiceInfoHandler(recorder, request, s.user)
	c.Assert(err, IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	var instances []service.ServiceInstance
	err = json.Unmarshal(body, &instances)
	c.Assert(err, IsNil)
	expected := []service.ServiceInstance{si1, si2}
	c.Assert(instances, DeepEquals, expected)
}

func (s *S) TestServiceInfoHandlerReturns404WhenTheServiceDoesNotExist(c *C) {
	request, err := http.NewRequest("GET", fmt.Sprintf("/services/%s?:name=%s", "mongodb", "mongodb"), nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = ServiceInfoHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^Service not found$")
}

func (s *S) TestServiceInfoHandlerReturns403WhenTheUserDoesNotHaveAccessToTheService(c *C) {
	se := service.Service{Name: "Mysql", IsRestricted: true}
	se.Create()
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/services/%s?:name=%s", se.Name, se.Name), nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = ServiceInfoHandler(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^This user does not have access to this service$")
}

func (s *S) makeRequestToGetDocHandler(name string, c *C) (*httptest.ResponseRecorder, *http.Request) {
	url := fmt.Sprintf("/services/%s/doc/?:name=%s", name, name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *S) TestDocHandler(c *C) {
	doc := `Doc for coolnosql
Collnosql is a really really cool nosql`
	srv := service.Service{
		Name:  "coolnosql",
		Doc:   doc,
		Teams: []string{s.team.Name},
	}
	err := srv.Create()
	c.Assert(err, IsNil)
	recorder, request := s.makeRequestToGetDocHandler("coolnosql", c)
	err = Doc(recorder, request, s.user)
	c.Assert(err, IsNil)
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	c.Assert(string(b), Equals, doc)
}

func (s *S) TestDocHandlerReturns401WhenUserHasNoAccessToService(c *C) {
	srv := service.Service{
		Name:         "coolnosql",
		Doc:          "some doc...",
		IsRestricted: true,
	}
	err := srv.Create()
	c.Assert(err, IsNil)
	recorder, request := s.makeRequestToGetDocHandler("coolnosql", c)
	err = Doc(recorder, request, s.user)
	c.Assert(err, ErrorMatches, "^This user does not have access to this service$")
}

func (s *S) TestDocHandlerReturns404WhenServiceDoesNotExists(c *C) {
	recorder, request := s.makeRequestToGetDocHandler("inexistentsql", c)
	err := Doc(recorder, request, s.user)
	c.Assert(err, ErrorMatches, "^Service not found$")
}

func (s *S) TestServiceAndServiceInstancesByTeams(c *C) {
	srv := service.Service{Name: "mongodb", Teams: []string{s.team.Name}}
	srv.Create()
	defer db.Session.Services().Remove(bson.M{"_id": srv.Name})
	si := service.ServiceInstance{Name: "my_nosql", ServiceName: srv.Name, Teams: []string{s.team.Name}}
	si.Create()
	defer si.Delete()
	obtained := ServiceAndServiceInstancesByTeams("teams", s.user)
	expected := []ServiceModel{
		ServiceModel{Service: "mongodb", Instances: []string{"my_nosql"}},
	}
	c.Assert(obtained, DeepEquals, expected)
}

func (s *S) TestServiceAndServiceInstancesByOwnerTeams(c *C) {
	srv := service.Service{Name: "mongodb", OwnerTeams: []string{s.team.Name}}
	srv.Create()
	defer srv.Delete()
	si := service.ServiceInstance{Name: "my_nosql", ServiceName: srv.Name, Teams: []string{s.team.Name}}
	si.Create()
	defer si.Delete()
	obtained := ServiceAndServiceInstancesByTeams("owner_teams", s.user)
	expected := []ServiceModel{
		ServiceModel{Service: "mongodb", Instances: []string{"my_nosql"}},
	}
	c.Assert(obtained, DeepEquals, expected)
}

func (s *S) TestServiceAndServiceInstancesByTeamsShouldAlsoReturnServicesWithIsRestrictedFalse(c *C) {
	srv := service.Service{Name: "mongodb", OwnerTeams: []string{s.team.Name}}
	srv.Create()
	defer srv.Delete()
	srv2 := service.Service{Name: "mysql"}
	srv2.Create()
	defer srv2.Delete()
	si := service.ServiceInstance{Name: "my_nosql", ServiceName: srv.Name, Teams: []string{s.team.Name}}
	si.Create()
	defer si.Delete()
	obtained := ServiceAndServiceInstancesByTeams("owner_teams", s.user)
	expected := []ServiceModel{
		ServiceModel{Service: "mongodb", Instances: []string{"my_nosql"}},
		ServiceModel{Service: "mysql"},
	}
	c.Assert(obtained, DeepEquals, expected)
}

func (s *S) TestServiceAndServiceInstancesByTeamsShouldReturnServiceInstancesByTeam(c *C) {
	srv := service.Service{Name: "mongodb"}
	srv.Create()
	defer srv.Delete()
	si := service.ServiceInstance{Name: "my_nosql", ServiceName: srv.Name, Teams: []string{s.team.Name}}
	si.Create()
	defer si.Delete()
	si2 := service.ServiceInstance{Name: "some_nosql", ServiceName: srv.Name}
	si2.Create()
	defer si2.Delete()
	obtained := ServiceAndServiceInstancesByTeams("teams", s.user)
	expected := []ServiceModel{
		ServiceModel{Service: "mongodb", Instances: []string{"my_nosql"}},
	}
	c.Assert(obtained, DeepEquals, expected)
}
