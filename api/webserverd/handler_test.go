package webserverd

import (
	"errors"
	"fmt"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})

func errorHandler(w http.ResponseWriter, r *http.Request) error {
	return errors.New("some error")
}

func simpleHandler(w http.ResponseWriter, r *http.Request) error {
	fmt.Fprint(w, "success")
	return nil
}

// func anotherSimpleHandler(w http.ResponseWriter, r *http.Request, db *DB) error {
// 	fmt.Fprint(w, "success")
// 	return nil
// }

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

// func (s *S) TestHandlerRepassesDbSession(c *C) {

// }
