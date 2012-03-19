package api_test

import (
	"github.com/timeredbull/tsuru/api"
	. "launchpad.net/gocheck"
	"testing"
	"net/http"
	"net/http/httptest"
)

func Test(t *testing.T) { TestingT(t) }

type ServicesSuite struct{}
var _ = Suite(&ServicesSuite{})

func (s *ServicesSuite) TestShouldRequestCreate(c *C) {
	request, err := http.NewRequest("POST", "services/create", nil)
	recorder := httptest.NewRecorder()
	c.Assert(err, IsNil)

	api.CreateService(recorder, request)
	status := recorder.Code

	c.Assert(200, Equals, status)
}
