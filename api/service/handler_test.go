package service_test

import (
	"fmt"
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	. "github.com/timeredbull/tsuru/api/service"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"strings"
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

	_, err := s.db.Exec("CREATE TABLE 'service' ('id' INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, 'service_type_id' integer,'name' varchar(255))")
	c.Check(err, IsNil)

	_, err = s.db.Exec("CREATE TABLE 'service_type' ('id' INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, 'name' varchar(255), 'charm' varchar(255))")
	c.Check(err, IsNil)

	_, err = s.db.Exec("CREATE TABLE 'service_app' ('id' INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, 'service_id' integer, 'app_id' integer)")
	c.Check(err, IsNil)

	_, err = s.db.Exec("CREATE TABLE 'apps' ('id' INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, 'name' varchar(255), 'framework' varchar(255), 'state' varchar(255))")
	c.Check(err, IsNil)
}

func (s *ServiceSuite) TearDownSuite(c *C) {
	os.Remove("./tsuru.db")
	s.db.Close()
}

func (s *ServiceSuite) TearDownTest(c *C) {
	s.db.Exec("DELETE FROM service")
	s.db.Exec("DELETE FROM service_type")
	s.db.Exec("DELETE FROM service_app")
	s.db.Exec("DELETE FROM apps")
}

func (s *ServiceSuite) TestShouldRequestCreateAndBeSuccess(c *C) {
	request, err := http.NewRequest("POST", "services/create", nil)
	recorder := httptest.NewRecorder()
	c.Assert(err, IsNil)

	CreateHandler(recorder, request)
	status := recorder.Code

	c.Assert(200, Equals, status)
}

func (s *ServiceSuite) TestShouldRequestCreateAndInsertInTheDatabase(c *C) {
	request, err := http.NewRequest("POST", "/services", nil)
	c.Assert(err, IsNil)

	request.Header.Set("Content-Type", "application/json")
	request.Form = url.Values{
		"serviceTypeId": []string{"1"},
		"name":          []string{"my_mysql"},
	}

	recorder := httptest.NewRecorder()
	CreateHandler(recorder, request)
	body := recorder.Body
	c.Assert(body.String(), Equals, "success")

	rows, err := s.db.Query("SELECT count(*) FROM service WHERE name = 'my_mysql'")

	c.Check(err, IsNil)
	var qtd int

	for rows.Next() {
		rows.Scan(&qtd)
	}

	c.Assert(1, Equals, qtd)
}

func (s *ServiceSuite) TestDeleteHandler(c *C) {
	se := Service{ServiceTypeId: 2, Name: "Mysql"}
	se.Create()
	request, err := http.NewRequest("GET", fmt.Sprintf("/services/%s?:name=%s", se.Name, se.Name), nil)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	DeleteHandler(recorder, request)
	c.Assert(recorder.Code, Equals, 200)

	rows, err := s.db.Query("SELECT count(*) FROM service WHERE name = 'Mysql'")
	c.Check(err, IsNil)

	var qtd int
	for rows.Next() {
		rows.Scan(&qtd)
	}

	c.Assert(qtd, Equals, 0)
}

//func (s *ServiceSuite) TestListHandler(c *C) {}

func (s *ServiceSuite) TestBindHandler(c *C) {
	se := Service{ServiceTypeId: 2, Name: "Mysql"}
	se.Create()
	a := App{Name: "someApp", Framework: "django"}
	b := strings.NewReader(`{"app":"someApp", "service":"mysql"}`)
	request, err := http.NewRequest("POST", "/services/bind", b)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	Bind(recorder, request)
	c.Assert(recorder.Code, Equals, 200)

	rows, err := s.db.Query("SELECT count(*) FROM service_app WHERE name = 'Mysql'")
	c.Check(err, IsNil)

	var qtd int
	for rows.Next() {
		rows.Scan(&qtd)
	}

	c.Assert(qtd, Equals, 0)
}
