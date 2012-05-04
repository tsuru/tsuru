package service

import (
	"encoding/json"
	"fmt"
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/db"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/mgo/bson"
	"net/http"
	"net/http/httptest"
	"strings"
)

func (s *ServiceSuite) TestCreateHandler(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	st.Create()

	b := strings.NewReader(`{"name":"some_service", "type":"mysql"}`)
	request, err := http.NewRequest("POST", "/services", b)
	c.Assert(err, IsNil)

	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateHandler(recorder, request)
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
}

func (s *ServiceSuite) TestServicesHandler(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	st.Create()
	se := Service{ServiceTypeId: st.Id, Name: "myService"}
	se2 := Service{ServiceTypeId: st.Id, Name: "myOtherService"}
	se.Create()
	se2.Create()

	request, err := http.NewRequest("GET", "/services", nil)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	err = ServicesHandler(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 200)

	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)

	var results []ServiceT
	err = json.Unmarshal(body, &results)
	c.Assert(err, IsNil)
	c.Assert(len(results), Equals, 2)
	c.Assert(results[0], FitsTypeOf, ServiceT{})
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
	se := Service{ServiceTypeId: st.Id, Name: "Mysql"}
	se.Create()
	request, err := http.NewRequest("GET", fmt.Sprintf("/services/%s?:name=%s", se.Name, se.Name), nil)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	err = DeleteHandler(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 200)

	query := bson.M{"name": "Mysql"}

	qtd, err := db.Session.Services().Find(query).Count()
	c.Assert(err, IsNil)
	c.Assert(qtd, Equals, 0)
}

func (s *ServiceSuite) TestDeleteHandlerReturns404(c *C) {
	request, err := http.NewRequest("GET", fmt.Sprintf("/services/%s?:name=%s", "mongodb", "mongodb"), nil)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	err = DeleteHandler(recorder, request)
	c.Assert(err, NotNil)
	c.Assert(recorder.Code, Equals, 404)
}

func (s *ServiceSuite) TestBindHandler(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	err := st.Create()
	c.Assert(err, IsNil)
	se := Service{ServiceTypeId: st.Id, Name: "my_service"}
	a := app.App{Name: "someApp", Framework: "django"}
	err = se.Create()
	c.Assert(err, IsNil)
	err = a.Create()
	c.Assert(err, IsNil)
	b := strings.NewReader(`{"app":"someApp", "service":"my_service"}`)
	request, err := http.NewRequest("POST", "/services/bind", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = BindHandler(recorder, request)
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
	b := strings.NewReader(`{"app":"someApp", "service":"my_service"}`)
	request, err := http.NewRequest("POST", "/services/bind", b)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	err = BindHandler(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 404)
}

func (s *ServiceSuite) TestUnbindHandler(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	st.Create()
	se := Service{ServiceTypeId: st.Id, Name: "my_service"}
	a := app.App{Name: "someApp", Framework: "django", Ip: "192.168.30.10"}
	se.Create()
	a.Create()
	se.Bind(&a)

	b := strings.NewReader(`{"app":"someApp", "service":"my_service"}`)
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
	b := strings.NewReader(`{"app":"someApp", "service":"my_service"}`)
	request, err := http.NewRequest("POST", "/services/bind", b)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	err = UnbindHandler(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 404)
}
