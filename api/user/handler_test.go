package user

import (
	"bytes"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

func (s *S) TestCreateUserHandler(c *C) {
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
