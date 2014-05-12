// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oauth

import (
	"bytes"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestOAuthLoginWithoutCode(c *gocheck.C) {
	scheme := OAuthScheme{}
	params := make(map[string]string)
	_, err := scheme.Login(params)
	c.Assert(err, gocheck.Equals, ErrMissingCodeError)
}

func (s *S) TestOAuthLogin(c *gocheck.C) {
	scheme := OAuthScheme{}
	s.rsps["/token"] = `access_token=my_token`
	s.rsps["/user"] = `{"email":"rand@althor.com"}`
	params := make(map[string]string)
	params["code"] = "abcdefg"
	token, err := scheme.Login(params)
	c.Assert(err, gocheck.IsNil)
	c.Assert(token.GetValue(), gocheck.Equals, "my_token")
	c.Assert(token.GetUserName(), gocheck.Equals, "rand@althor.com")
	c.Assert(token.IsAppToken(), gocheck.Equals, false)
	u, err := token.User()
	c.Assert(err, gocheck.IsNil)
	c.Assert(u.Email, gocheck.Equals, "rand@althor.com")
	c.Assert(len(s.reqs), gocheck.Equals, 2)
	c.Assert(s.reqs[0].URL.Path, gocheck.Equals, "/token")
	c.Assert(s.reqs[1].URL.Path, gocheck.Equals, "/user")
	c.Assert(s.bodies[0], gocheck.Matches, "client_id=clientid&client_secret=clientsecret&code=abcdefg.*")
	c.Assert(s.reqs[1].Header.Get("Authorization"), gocheck.Equals, "Bearer my_token")
}

func (s *S) TestOAuthLoginEmptyToken(c *gocheck.C) {
	scheme := OAuthScheme{}
	s.rsps["/token"] = `access_token=`
	params := make(map[string]string)
	params["code"] = "abcdefg"
	_, err := scheme.Login(params)
	c.Assert(err, gocheck.Equals, ErrEmptyAccessToken)
	c.Assert(len(s.reqs), gocheck.Equals, 1)
	c.Assert(s.reqs[0].URL.Path, gocheck.Equals, "/token")
}

func (s *S) TestOAuthLoginEmptyEmail(c *gocheck.C) {
	scheme := OAuthScheme{}
	s.rsps["/token"] = `access_token=my_token`
	s.rsps["/user"] = `{"email":""}`
	params := make(map[string]string)
	params["code"] = "abcdefg"
	_, err := scheme.Login(params)
	c.Assert(err, gocheck.Equals, ErrEmptyUserEmail)
	c.Assert(len(s.reqs), gocheck.Equals, 2)
	c.Assert(s.reqs[0].URL.Path, gocheck.Equals, "/token")
	c.Assert(s.reqs[1].URL.Path, gocheck.Equals, "/user")
}

func (s *S) TestOAuthName(c *gocheck.C) {
	scheme := OAuthScheme{}
	name := scheme.Name()
	c.Assert(name, gocheck.Equals, "oauth")
}

func (s *S) TestOAuthInfo(c *gocheck.C) {
	scheme := OAuthScheme{}
	info, err := scheme.Info()
	c.Assert(err, gocheck.IsNil)
	c.Assert(info["authorizeUrl"], gocheck.Matches, s.server.URL+"/auth.*")
	c.Assert(info["authorizeUrl"], gocheck.Matches, ".*client_id=clientid.*")
	c.Assert(info["authorizeUrl"], gocheck.Matches, ".*redirect_uri=redirect_url_placeholder.*")
}

func (s *S) TestOAuthParse(c *gocheck.C) {
	b := ioutil.NopCloser(bytes.NewBufferString(`{"email":"x@x.com"}`))
	rsp := &http.Response{Body: b}
	parser := OAuthParser(&OAuthScheme{})
	email, err := parser.Parse(rsp)
	c.Assert(err, gocheck.IsNil)
	c.Assert(email, gocheck.Equals, "x@x.com")
}

func (s *S) TestOAuthParseInvalid(c *gocheck.C) {
	b := ioutil.NopCloser(bytes.NewBufferString(`{xxxxxxx}`))
	rsp := &http.Response{Body: b}
	parser := OAuthParser(&OAuthScheme{})
	_, err := parser.Parse(rsp)
	c.Assert(err, gocheck.NotNil)
}
