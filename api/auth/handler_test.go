package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/timeredbull/tsuru/db"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/mgo/bson"
	"net/http"
	"net/http/httptest"
)

func (s *S) TestCreateUserHandlerSavesTheUserInTheDatabase(c *C) {
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, IsNil)

	request.Header.Set("Content-type", "application/json")
	response := httptest.NewRecorder()
	err = CreateUser(response, request)
	c.Assert(err, IsNil)

	u := User{Email: "nobody@globo.com"}
	err = u.Get()
	c.Assert(err, IsNil)
}

func (s *S) TestCreateUserHandlerReturnsStatus204AfterCreateTheUser(c *C) {
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, IsNil)

	request.Header.Set("Content-type", "application/json")
	response := httptest.NewRecorder()
	err = CreateUser(response, request)
	c.Assert(err, IsNil)
	c.Assert(response.Code, Equals, 201)
}

func (s *S) TestCreateUserHandlerReturnErrorIfReadingBodyFails(c *C) {
	b := s.getTestData("bodyToBeClosed.txt")
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, IsNil)

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
	c.Assert(err, IsNil)

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
	c.Assert(err, IsNil)

	request.Header.Set("Content-type", "application/json")
	response := httptest.NewRecorder()
	err = Login(response, request)
	c.Assert(err, IsNil)

	var user User
	collection := db.Session.Users()
	err = collection.Find(bson.M{"email": "nobody@globo.com"}).One(&user)

	var responseJson map[string]string
	r, _ := ioutil.ReadAll(response.Body)
	json.Unmarshal(r, &responseJson)
	c.Assert(responseJson["token"], Equals, user.Tokens[0].Token)
}

func (s *S) TestLoginShouldReturnErrorAndBadRequestIfItReceivesAnInvalidJson(c *C) {
	b := bytes.NewBufferString(`"invalid":"json"]`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, IsNil)

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
	c.Assert(err, IsNil)

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
	c.Assert(err, IsNil)

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
	c.Assert(err, IsNil)

	request.Header.Set("Content-type", "application/json")
	response := httptest.NewRecorder()
	err = Login(response, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Authentication failed, wrong password$")
	c.Assert(response.Code, Equals, http.StatusUnauthorized)
}

func (s *S) TestValidateUserTokenReturnJsonRepresentingUser(c *C) {
	var t *Token
	u := User{Email: "nobody@globo.com", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)

	u.Get()
	t, err = u.CreateToken()
	c.Assert(err, IsNil)

	request, err := http.NewRequest("GET", "/users/check-authorization", nil)
	c.Assert(err, IsNil)

	request.Header.Set("Authorization", t.Token)
	response := httptest.NewRecorder()
	err = CheckAuthorization(response, request)
	c.Assert(err, IsNil)

	var expected, got map[string]string
	expected = map[string]string{
		"email": "nobody@globo.com",
	}
	r, _ := ioutil.ReadAll(response.Body)
	json.Unmarshal(r, &got)
	c.Assert(got, DeepEquals, expected)
}

func (s *S) TestValidateUserTokenReturnErrorWhenGetUserByTokenReturnsAny(c *C) {
	request, err := http.NewRequest("GET", "/users/check-authorization", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Authorization", fmt.Sprintf("unexistent token"))
	response := httptest.NewRecorder()
	err = CheckAuthorization(response, request)
	c.Assert(err, NotNil)
}

func (s *S) TestValidateUserTokenReturnErrorAndBadRequestWhenTheAuthorizationHeaderIsNotPresent(c *C) {
	request, err := http.NewRequest("GET", "/users/check-authorization", nil)
	c.Assert(err, IsNil)
	response := httptest.NewRecorder()
	err = CheckAuthorization(response, request)
	c.Assert(response.Code, Equals, http.StatusBadRequest)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^You must provide the Authorization header$")
}

func (s *S) TestCheckTokenReturnBadRequestIfTheTokenIsOmited(c *C) {
	u, e := CheckToken("")
	c.Assert(u, IsNil)
	err, ok := e.(*AuthorizationError)
	c.Assert(ok, Equals, true)
	c.Assert(err.code, Equals, http.StatusBadRequest)
	c.Assert(err, ErrorMatches, "^You must provide the Authorization header$")
}

func (s *S) TestCheckTokenReturnUnauthorizedIfTheTokenIsInvalid(c *C) {
	u, e := CheckToken("invalid")
	c.Assert(u, IsNil)
	err, ok := e.(*AuthorizationError)
	c.Assert(ok, Equals, true)
	c.Assert(err.code, Equals, http.StatusUnauthorized)
	c.Assert(err, ErrorMatches, "^Invalid token$")
}

func (s *S) TestCheckTokenReturnTheUserIfTheTokenIsValid(c *C) {
	u, e := CheckToken(s.token.Token)
	c.Assert(e, IsNil)
	c.Assert(u.Email, Equals, s.user.Email)
}

func (s *S) TestCreateTeamHandlerSavesTheTeamInTheDatabaseWithTheAuthenticatedUser(c *C) {
	b := bytes.NewBufferString(`{"name":"timeredbull"}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	response := httptest.NewRecorder()
	err = CreateTeam(response, request, s.user)
	c.Assert(err, IsNil)

	t := new(Team)
	err = db.Session.Teams().Find(bson.M{"name": "timeredbull"}).One(t)
	c.Assert(err, IsNil)
	c.Assert(t, ContainsUser, s.user)
}

func (s *S) TestCreateTeamHandlerReturnsBadRequestIfTheRequestBodyIsAnInvalidJSON(c *C) {
	b := bytes.NewBufferString(`{"name"["invalidjson"]}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	response := httptest.NewRecorder()
	err = CreateTeam(response, request, s.user)
	c.Assert(response.Code, Equals, http.StatusBadRequest)
	c.Assert(err, NotNil)
}

func (s *S) TestCreateTeamHandlerReturnsBadRequestIfTheNameIsNotGiven(c *C) {
	b := bytes.NewBufferString(`{"genre":"male"}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	response := httptest.NewRecorder()
	err = CreateTeam(response, request, s.user)
	c.Assert(response.Code, Equals, http.StatusBadRequest)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^You must provide the team name$")
}

func (s *S) TestCreateTeamHandlerReturnsInternalServerErrorIfReadAllFails(c *C) {
	b := s.getTestData("bodyToBeClosed.txt")
	err := b.Close()
	c.Assert(err, IsNil)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	response := httptest.NewRecorder()
	err = CreateTeam(response, request, s.user)
	c.Assert(response.Code, Equals, http.StatusInternalServerError)
	c.Assert(err, NotNil)
}
