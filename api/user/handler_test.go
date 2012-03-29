package user

import (
	"bytes"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

func (s *S) TestCreateUserHandlerSavesTheUserInTheDatabase(c *C) {
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123"}`)
	request, err := http.NewRequest("POST", "/users", b)
	if err != nil {
		panic(err)
	}
	request.Header.Set("Content-type", "application/json")
	response := httptest.NewRecorder()
	err = CreateUser(response, request)
	c.Assert(err, IsNil)

	u := User{Email: "nobody@globo.com"}
	err = u.Get()
	c.Assert(err, IsNil)

	c.Assert(u.Id > 0, Equals, true)
	s.db.Exec(`DELETE FROM users WHERE email = "nobody@globo.com"`)
}

func (s *S) TestCreateUserHandlerReturnsStatus204AfterCreateTheUser(c *C) {
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123"}`)
	request, err := http.NewRequest("POST", "/users", b)
	if err != nil {
		panic(err)
	}
	request.Header.Set("Content-type", "application/json")
	response := httptest.NewRecorder()
	err = CreateUser(response, request)
	c.Assert(err, IsNil)
	c.Assert(response.Code, Equals, 201)

	s.db.Exec(`DELETE FROM users WHERE email = "nobody@globo.com"`)
}

func (s *S) TestCreateUserHandlerReturnErrorIfReadingBodyFails(c *C) {
	b := s.getTestData("bodyToBeClosed.txt")
	request, err := http.NewRequest("POST", "/users", b)
	if err != nil {
		panic(err)
	}
	request.Header.Set("Content-type", "application/json")
	request.Body.Close()
	response := httptest.NewRecorder()
	err = CreateUser(response, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^.*bad file descriptor$")
}

func (s *S) TestCreateUserHandlerReturnErrorIfInvalidJSONIsGiven(c *C) {
	b := bytes.NewBufferString(`["invalid json":"i'm invalid"]`)
	request, err := http.NewRequest("POST", "/users", b)
	if err != nil {
		panic(err)
	}
	request.Header.Set("Content-type", "application/json")
	response := httptest.NewRecorder()
	err = CreateUser(response, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^invalid character.*$")
}

func (s *S) TestCreateUserHandlerReturnErrorIfItFailsToCreateUser(c *C) {
	u := User{Email: "nobody@globo.com", Password: "123"}
	u.Create()

	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123"}`)
	request, err := http.NewRequest("POST", "/users", b)
	if err != nil {
		panic(err)
	}
	request.Header.Set("Content-type", "application/json")
	response := httptest.NewRecorder()
	err = CreateUser(response, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, ".*unique$")

	s.db.Exec(`DELETE FROM users WHERE email = "nobody@globo.com"`)
}
