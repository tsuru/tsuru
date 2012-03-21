package service_test

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/timeredbull/tsuru/api/service"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type ServiceSuite struct {
	db *sql.DB
}

var _ = Suite(&ServiceSuite{})

func (s *ServiceSuite) SetUpSuite(c *C) {
	s.db, _ = sql.Open("sqlite3", "./tsuru.db")

	_, err := s.db.Exec("CREATE TABLE 'service' ('id' INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, 'app_id' integer,'name' varchar(255))")
	c.Check(err, IsNil)
}

func (s *ServiceSuite) TearDownSuite(c *C) {
	os.Remove("./tsuru.db")
	s.db.Close()
}

func (s *ServiceSuite) TearDownTest(c *C) {
	s.db.Exec("DELETE FROM service")
}

func (s *ServiceSuite) TestShouldRequestCreateAndBeSuccess(c *C) {
	request, err := http.NewRequest("POST", "services/create", nil)
	recorder := httptest.NewRecorder()
	c.Assert(err, IsNil)

	service.CreateServiceHandler(recorder, request)
	status := recorder.Code

	c.Assert(200, Equals, status)
}

func (s *ServiceSuite) TestShouldRequestCreateAndInsertInTheDatabase(c *C) {
	request, err := http.NewRequest("POST", "services/create", nil)
	request.Header.Set("Content-Type", "application/json")
	request.Form = url.Values{
		"appId":            []string{"1"},
		"name":             []string{"my_mysql"},
	}

	recorder := httptest.NewRecorder()
	c.Assert(err, IsNil)

	service.CreateServiceHandler(recorder, request)
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
