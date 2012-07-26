package provision

import (
	"bytes"
	"fmt"
	"github.com/timeredbull/tsuru/api/service"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/errors"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

func (s *S) TestAddDocHandlerReturns404WhenTheServiceDoesNotExist(c *C) {
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

func (s *S) TestAddDocHandler(c *C) {
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

func (s *S) TestAddDocHandlerReturns403WhenTheUserDoesNotHaveAccessToTheService(c *C) {
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

func (s *S) TestGetDocHandler(c *C) {
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

func (s *S) TestGetDocHandlerReturns403WhenTheUserDoesNotHaveAccessToTheService(c *C) {
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

func (s *S) TestGetDocHandlerReturns404WhenTheServiceDoesNotExist(c *C) {
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
