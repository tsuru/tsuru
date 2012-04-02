package user

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
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
	c.Assert(err, ErrorMatches, "This email is already registered")
}

func (s *S) TestLoginShouldCreateTokenInTheDatabaseAndReturnItWithinTheResponse(c *C) {
	u := User{Email: "nobody@globo.com", Password: "123"}
	u.Create()

	b := bytes.NewBufferString(`{"password":"123"}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	if err != nil {
		panic(err)
	}
	request.Header.Set("Content-type", "application/json")
	response := httptest.NewRecorder()
	err = Login(response, request)
	c.Assert(err, IsNil)

	var id int
	var token string
	row := s.db.QueryRow("SELECT t.id, t.token FROM usertokens t INNER JOIN users u ON t.user_id = u.id WHERE u.email = ?", u.Email)
	row.Scan(&id, &token)
	c.Assert(id > 0, Equals, true)

	var responseJson map[string]string
	r, _ := ioutil.ReadAll(response.Body)
	json.Unmarshal(r, &responseJson)
	c.Assert(responseJson["token"], Equals, token)
}

func (s *S) TestLoginShouldReturnErrorAndBadRequestIfItReceivesAnInvalidJson(c *C) {
	b := bytes.NewBufferString(`"invalid":"json"]`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	if err != nil {
		panic(err)
	}
	request.Header.Set("Content-type", "application/json")
	response := httptest.NewRecorder()
	err = Login(response, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Invalid JSON$")
	c.Assert(response.Code, Equals, http.StatusBadRequest)
}

func (s *S) TestLoginShouldReturnErrorAndBadRequestIfTheJSONDoesNotContainsAPassword(c *C) {
	b := bytes.NewBufferString(`{}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	if err != nil {
		panic(err)
	}
	request.Header.Set("Content-type", "application/json")
	response := httptest.NewRecorder()
	err = Login(response, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^You must provide a password to login$")
	c.Assert(response.Code, Equals, http.StatusBadRequest)
}

func (s *S) TestLoginShouldReturnErrorAndNotFoundIfTheUserDoesNotExist(c *C) {
	b := bytes.NewBufferString(`{"password":"123"}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	if err != nil {
		panic(err)
	}
	request.Header.Set("Content-type", "application/json")
	response := httptest.NewRecorder()
	err = Login(response, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^User not found$")
	c.Assert(response.Code, Equals, http.StatusNotFound)
}

func (s *S) TestLoginShouldreturnErrorIfThePasswordDoesNotMatch(c *C) {
	u := User{Email: "nobody@globo.com", Password: "123"}
	u.Create()

	b := bytes.NewBufferString(`{"password":"1234"}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	if err != nil {
		panic(err)
	}
	request.Header.Set("Content-type", "application/json")
	response := httptest.NewRecorder()
	err = Login(response, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Authentication failed, wrong password$")
	c.Assert(response.Code, Equals, http.StatusUnauthorized)
}
