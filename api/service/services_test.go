package service_test

import (
	"bytes"
	"encoding/json"
	"github.com/timeredbull/tsuru/api/service"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type ServicesSuite struct{}

var _ = Suite(&ServicesSuite{})

func (s *ServicesSuite) TestShouldRequestCreateAndBeSuccess(c *C) {
	request, err := http.NewRequest("POST", "services/create", nil)
	recorder := httptest.NewRecorder()
	c.Assert(err, IsNil)

	service.CreateService(recorder, request)
	status := recorder.Code

	c.Assert(200, Equals, status)
}

func (s *ServicesSuite) TestShouldRequestCreateAndInsertInTheDatabase(c *C) {
	service_binding := service.ServiceBindings{
		ServiceConfigId: 1,
		AppId:           1,
		UserId:          1,
		BindingToken:    123,
		Name:            "mysql",
	}
	jsonData, err := json.Marshal(service_binding)
	c.Assert(err, IsNil)

	request, err := http.NewRequest("POST", "services/create", bytes.NewBuffer(jsonData))
	recorder := httptest.NewRecorder()
	c.Assert(err, IsNil)

	service.CreateService(recorder, request)
	body := recorder.Body
	c.Assert(body.String(), Equals, "success")
}
