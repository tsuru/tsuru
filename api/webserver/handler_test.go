package webserver

import (
	"errors"
	"fmt"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/db"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{
	u *auth.User
	t *auth.Token
}

var _ = Suite(&S{})

func errorHandler(w http.ResponseWriter, r *http.Request) error {
	return errors.New("some error")
}

func simpleHandler(w http.ResponseWriter, r *http.Request) error {
	fmt.Fprint(w, "success")
	return nil
}

func (s *S) SetUpSuite(c *C) {
	db.Session, _ = db.Open("127.0.0.1:27017", "tsuru_handler_test")
	s.u = &auth.User{Email: "handler@tsuru.globo.com", Password: "123"}
	s.u.Create()
	s.t, _ = s.u.CreateToken()
}

func (s *S) TearDownSuite(c *C) {
	defer db.Session.Close()
	db.Session.DropDB()
}

func (s *S) TestHandlerReturns500WhenInternalHandlerReturnsAnError(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)

	Handler(errorHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, Equals, 500)
	c.Assert(recorder.Body.String(), Equals, "some error\n")
}

func (s *S) TestHandlerShouldPassAnHandlerWithoutError(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)

	Handler(simpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, Equals, 200)
	c.Assert(recorder.Body.String(), Equals, "success")
}

func (s *S) TestAuthorizationRequiredHandlerShouldReturnBadRequestIfTheAuthorizationHeadIsNotPresent(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)

	AuthorizationRequiredHandler(simpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), Equals, "You must provide the Authorization header\n")
}

func (s *S) TestAuthorizationRequiredHandlerShouldReturnUnauthorizedIfTheTokenIsInvalid(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Authorization", "what the token?!")

	AuthorizationRequiredHandler(simpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, Equals, http.StatusUnauthorized)
	c.Assert(recorder.Body.String(), Equals, "Invalid token\n")
}

func (s *S) TestAuthorizationRequiredHandlerShouldReturnTheHandlerResultIfTheTokenIsOk(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Authorization", s.t.Token)

	AuthorizationRequiredHandler(simpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), Equals, "success")
}

func (s *S) TestAuthorizationRequiredHandlerShouldReturnTheHandlerErrorIfAnyHappen(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Authorization", s.t.Token)

	AuthorizationRequiredHandler(errorHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), Equals, "some error\n")
}
