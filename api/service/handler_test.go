package service_test

import (
	"encoding/json"
	"fmt"
	. "github.com/timeredbull/tsuru/api/app"
	. "github.com/timeredbull/tsuru/api/service"
	. "github.com/timeredbull/tsuru/database"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/mgo"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type ServiceSuite struct {
	service     *Service
	serviceType *ServiceType
	serviceApp  *ServiceApp
	session     *mgo.Session
}

var _ = Suite(&ServiceSuite{})

func (s *ServiceSuite) SetUpSuite(c *C) {
	var err error
	s.session, err = mgo.Dial("localhost:27017")
	c.Assert(err, IsNil)
	Mdb = s.session.DB("tsuru_test")
	c.Assert(err, IsNil)
}

func (s *ServiceSuite) TearDownSuite(c *C) {
	err := Mdb.DropDatabase()
	c.Assert(err, IsNil)
	s.session.Close()
}

func (s *ServiceSuite) TearDownTest(c *C) {
	err := Mdb.C("services").DropCollection()
	c.Assert(err, IsNil)

	err = Mdb.C("service_apps").DropCollection()
	c.Assert(err, IsNil)

	err = Mdb.C("service_types").DropCollection()
	c.Assert(err, IsNil)
}

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

	//"SELECT id, service_type_id, name FROM services WHERE name = 'some_service'"
	query := map[string]string{
		"name": "some_service",
	}
	var obtainedService Service

	collection := Mdb.C("services")
	err = collection.Find(query).One(&obtainedService)

	c.Assert(err, IsNil)
	c.Assert(obtainedService.Id, Not(Equals), 0)
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
	c.Assert(results[0].Id, Not(Equals), 0)
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

	/* rows, err := session.Query("SELECT count(*) FROM services WHERE name = 'Mysql'") */
	query := map[string]string{
		"name": "Mysql",
	}

	collection := Mdb.C("services")
	qtd, err := collection.Find(query).Count()
	c.Assert(err, IsNil)
	c.Assert(qtd, Equals, 0)
}

func (s *ServiceSuite) TestDeleteHandlerReturns404(c *C) {
	request, err := http.NewRequest("GET", fmt.Sprintf("/services/%s?:name=%s", "mongobd", "mongodb"), nil)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	err = DeleteHandler(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 404)
}

func (s *ServiceSuite) TestBindHandler(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	st.Create()
	se := Service{ServiceTypeId: st.Id, Name: "my_service"}
	a := App{Name: "someApp", Framework: "django"}
	se.Create()
	a.Create()

	b := strings.NewReader(`{"app":"someApp", "service":"my_service"}`)
	request, err := http.NewRequest("POST", "/services/bind", b)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	err = BindHandler(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 200)

	/* rows, err := session.Query("SELECT count(*) FROM service_apps WHERE service_id = ? AND app_id = ?", se.Id, a.Id) */
	query := map[string]interface{}{
		"service_id": se.Id,
		"app_id": a.Id,
	}
	collection := Mdb.C("services")
	qtd, err := collection.Find(query).Count()
	c.Check(err, IsNil)

	// var qtd int
	// for rows.Next() {
	// 	rows.Scan(&qtd)
	// }

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
	a := App{Name: "someApp", Framework: "django", Ip: "192.168.30.10"}
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

	/* rows, err := session.Query("SELECT count(*) FROM service_apps WHERE service_id = ? AND app_id = ?", se.Id, a.Id) */
	query := map[string]interface{}{
		"service_id": se.Id,
		"app_id": a.Id,
	}
	collection := Mdb.C("services")
	qtd, err := collection.Find(query).Count()
	c.Check(err, IsNil)

	// var qtd int
	// for rows.Next() {
	// 	rows.Scan(&qtd)
	// }

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
