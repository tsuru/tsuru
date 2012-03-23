package service_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	. "github.com/timeredbull/tsuru/api/app"
	. "github.com/timeredbull/tsuru/api/service"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type ServiceSuite struct {
	db          *sql.DB
	service     *Service
	serviceType *ServiceType
	serviceApp  *ServiceApp
}

var _ = Suite(&ServiceSuite{})

func (s *ServiceSuite) SetUpSuite(c *C) {
	s.db, _ = sql.Open("sqlite3", "./tsuru.db")

	_, err := s.db.Exec("CREATE TABLE 'services' ('id' INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, 'service_type_id' integer,'name' varchar(255))")
	c.Check(err, IsNil)

	_, err = s.db.Exec("CREATE TABLE 'service_types' ('id' INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, 'name' varchar(255), 'charm' varchar(255))")
	c.Check(err, IsNil)

	_, err = s.db.Exec("CREATE TABLE 'service_apps' ('id' INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, 'service_id' integer, 'app_id' integer)")
	c.Check(err, IsNil)

	_, err = s.db.Exec("CREATE TABLE 'apps' ('id' INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, 'name' varchar(255), 'framework' varchar(255), 'state' varchar(255))")
	c.Check(err, IsNil)
}

func (s *ServiceSuite) TearDownSuite(c *C) {
	os.Remove("./tsuru.db")
	s.db.Close()
}

func (s *ServiceSuite) TearDownTest(c *C) {
	s.db.Exec("DELETE FROM services")
	s.db.Exec("DELETE FROM service_types")
	s.db.Exec("DELETE FROM service_apps")
	s.db.Exec("DELETE FROM apps")
}

func (s *ServiceSuite) TestCreateHandler(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	st.Create()

	b := strings.NewReader(`{"name":"some_service", "type":"mysql"}`)
	request, err := http.NewRequest("POST", "/services", b)

	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	c.Assert(err, IsNil)

	CreateHandler(recorder, request)

	c.Assert(recorder.Body.String(), Equals, "success")
	c.Assert(recorder.Code, Equals, 200)

	rows, err := s.db.Query("SELECT id, service_type_id, name FROM services WHERE name = 'some_service'")

	c.Check(err, IsNil)
	var id, serviceTypeId int64
	var name string
	for rows.Next() {
		rows.Scan(&id, &serviceTypeId, &name)
	}

	c.Assert(id, Not(Equals), int64(0))
	c.Assert(serviceTypeId, Not(Equals), int64(0))
	c.Assert(name, Not(Equals), "")
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
	ServicesHandler(recorder, request)
	c.Assert(recorder.Code, Equals, 200)

	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)

	var results []ServiceT
	err = json.Unmarshal(body, &results)
	c.Assert(err, IsNil)
	c.Assert(len(results), Equals, 2)
	c.Assert(results[0], FitsTypeOf, ServiceT{})
	c.Assert(results[0].Id, Not(Equals), int64(0))
	c.Assert(results[0].Name, Not(Equals), "")

	c.Assert(results[0].Type, FitsTypeOf, &ServiceType{})
	c.Assert(results[0].Type.Id, Not(Equals), int64(0))
}

func (s *ServiceSuite) TestServiceTypesHandler(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	st.Create()

	request, err := http.NewRequest("GET", "/services/types", nil)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	ServiceTypesHandler(recorder, request)
	c.Assert(recorder.Code, Equals, 200)

	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)

	var results []ServiceType
	err = json.Unmarshal(body, &results)
	c.Assert(err, IsNil)
	c.Assert(results[0].Id, Not(Equals), int64(0))
}

func (s *ServiceSuite) TestDeleteHandler(c *C) {
	se := Service{ServiceTypeId: 2, Name: "Mysql"}
	se.Create()
	request, err := http.NewRequest("GET", fmt.Sprintf("/services/%s?:name=%s", se.Name, se.Name), nil)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	DeleteHandler(recorder, request)
	c.Assert(recorder.Code, Equals, 200)

	rows, err := s.db.Query("SELECT count(*) FROM services WHERE name = 'Mysql'")
	c.Check(err, IsNil)

	var qtd int
	for rows.Next() {
		rows.Scan(&qtd)
	}

	c.Assert(qtd, Equals, 0)
}

func (s *ServiceSuite) TestDeleteHandlerReturns404(c *C) {
	request, err := http.NewRequest("GET", fmt.Sprintf("/services/%s?:name=%s", "mongobd", "mongodb"), nil)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	DeleteHandler(recorder, request)
	c.Assert(recorder.Code, Equals, 404)
}

func (s *ServiceSuite) TestBindHandler(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	se := Service{ServiceTypeId: st.Id, Name: "my_service"}
	a := App{Name: "someApp", Framework: "django"}
	st.Create()
	se.Create()
	a.Create()

	b := strings.NewReader(`{"app":"someApp", "service":"my_service"}`)
	request, err := http.NewRequest("POST", "/services/bind", b)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	BindHandler(recorder, request)
	c.Assert(recorder.Code, Equals, 200)

	rows, err := s.db.Query("SELECT count(*) FROM service_apps WHERE service_id = ? AND app_id = ?", se.Id, a.Id)
	c.Check(err, IsNil)

	var qtd int
	for rows.Next() {
		rows.Scan(&qtd)
	}

	c.Assert(qtd, Equals, 1)
}

func (s *ServiceSuite) TestBindHandlerReturns404(c *C) {
	b := strings.NewReader(`{"app":"someApp", "service":"my_service"}`)
	request, err := http.NewRequest("POST", "/services/bind", b)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	BindHandler(recorder, request)
	c.Assert(recorder.Code, Equals, 404)
}

func (s *ServiceSuite) TestUnbindHandler(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	se := Service{ServiceTypeId: st.Id, Name: "my_service"}
	a := App{Name: "someApp", Framework: "django"}
	st.Create()
	se.Create()
	a.Create()
	se.Bind(&a)

	b := strings.NewReader(`{"app":"someApp", "service":"my_service"}`)
	request, err := http.NewRequest("POST", "/services/bind", b)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	UnbindHandler(recorder, request)
	c.Assert(recorder.Code, Equals, 200)

	rows, err := s.db.Query("SELECT count(*) FROM service_apps WHERE service_id = ? AND app_id = ?", se.Id, a.Id)
	c.Check(err, IsNil)

	var qtd int
	for rows.Next() {
		rows.Scan(&qtd)
	}

	c.Assert(qtd, Equals, 0)
}

func (s *ServiceSuite) TestUnbindReturns404(c *C) {
	b := strings.NewReader(`{"app":"someApp", "service":"my_service"}`)
	request, err := http.NewRequest("POST", "/services/bind", b)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	UnbindHandler(recorder, request)
	c.Assert(recorder.Code, Equals, 404)
}
