// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/quota"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

type QuotaSuite struct {
	team  *auth.Team
	user  *auth.User
	token auth.Token
}

var _ = gocheck.Suite(&QuotaSuite{})

func (s *QuotaSuite) SetUpSuite(c *gocheck.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_api_auth_test")
	config.Set("admin-team", "superteam")
	config.Set("auth:hash-cost", 4)
	s.user = &auth.User{Email: "unspoken@gotthard.com", Password: "123456"}
	_, err := nativeScheme.Create(s.user)
	c.Assert(err, gocheck.IsNil)
	s.team = &auth.Team{Name: "superteam", Users: []string{s.user.Email}}
	conn, _ := db.Conn()
	defer conn.Close()
	err = conn.Teams().Insert(s.team)
	c.Assert(err, gocheck.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	config.Set("admin-team", s.team.Name)
	app.AuthScheme = nativeScheme
}

func (s *QuotaSuite) TearDownSuite(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}

func (s *QuotaSuite) TestGetUserQuota(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	user := &auth.User{
		Email:    "radio@gaga.com",
		Password: "qwe123",
		Quota:    quota.Quota{Limit: 4, InUse: 2},
	}
	_, err = nativeScheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": user.Email})
	request, _ := http.NewRequest("GET", "/users/radio@gaga.com/quota", nil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	var qt quota.Quota
	err = json.NewDecoder(recorder.Body).Decode(&qt)
	c.Assert(err, gocheck.IsNil)
	c.Assert(qt, gocheck.DeepEquals, user.Quota)
}

func (s *QuotaSuite) TestGetUserQuotaRequiresAdmin(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	user := &auth.User{
		Email:    "radio@gaga.com",
		Password: "qwe123",
	}
	_, err = nativeScheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": user.Email})
	token, err := nativeScheme.Login(map[string]string{"email": user.Email, "password": "qwe123"})
	c.Assert(err, gocheck.IsNil)
	request, _ := http.NewRequest("GET", "/users/radio@gaga.com/quota", nil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, adminRequiredErr.Code)
	c.Assert(recorder.Body.String(), gocheck.Equals, adminRequiredErr.Message+"\n")
}

func (s *QuotaSuite) TestGetUserQuotaUserNotFound(c *gocheck.C) {
	request, _ := http.NewRequest("GET", "/users/radio@gaga.com/quota", nil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), gocheck.Equals, auth.ErrUserNotFound.Error()+"\n")
}

func (s *QuotaSuite) TestChangeUserQuota(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	user := &auth.User{
		Email:    "radio@gaga.com",
		Password: "qwe123",
		Quota:    quota.Quota{Limit: 4, InUse: 2},
	}
	_, err = nativeScheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": user.Email})
	body := bytes.NewBufferString("limit=40")
	request, _ := http.NewRequest("POST", "/users/radio@gaga.com/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	user, err = auth.GetUserByEmail(user.Email)
	c.Assert(err, gocheck.IsNil)
	c.Assert(user.Quota.Limit, gocheck.Equals, 40)
	c.Assert(user.Quota.InUse, gocheck.Equals, 2)
}

func (s *QuotaSuite) TestChangeUserQuotaRequiresAdmin(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	user := &auth.User{
		Email:    "radio@gaga.com",
		Password: "qwe123",
		Quota:    quota.Quota{Limit: 4, InUse: 2},
	}
	_, err = nativeScheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": user.Email})
	token, err := nativeScheme.Login(map[string]string{"email": user.Email, "password": "qwe123"})
	c.Assert(err, gocheck.IsNil)
	body := bytes.NewBufferString("limit=40")
	request, _ := http.NewRequest("POST", "/users/radio@gaga.com/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, adminRequiredErr.Code)
	c.Assert(recorder.Body.String(), gocheck.Equals, adminRequiredErr.Message+"\n")
}

func (s *QuotaSuite) TestChangeUserQuotaInvalidLimitValue(c *gocheck.C) {
	values := []string{"four", ""}
	for _, value := range values {
		body := bytes.NewBufferString("limit=" + value)
		request, _ := http.NewRequest("POST", "/users/radio@gaga.com/quota", body)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		recorder := httptest.NewRecorder()
		handler := RunServer(true)
		handler.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, gocheck.Equals, http.StatusBadRequest)
		c.Assert(recorder.Body.String(), gocheck.Equals, "Invalid limit\n")
	}
}

func (s *QuotaSuite) TestChangeUserQuotaUserNotFound(c *gocheck.C) {
	body := bytes.NewBufferString("limit=2")
	request, _ := http.NewRequest("POST", "/users/radio@gaga.com/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), gocheck.Equals, auth.ErrUserNotFound.Error()+"\n")
}

func (s *QuotaSuite) TestChangeAppQuota(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	a := &app.App{
		Name:  "shangrila",
		Quota: quota.Quota{Limit: 4, InUse: 2},
	}
	err = conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	body := bytes.NewBufferString("limit=40")
	request, _ := http.NewRequest("POST", "/apps/shangrila/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	a, err = app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(a.Quota.InUse, gocheck.Equals, 2)
	c.Assert(a.Quota.Limit, gocheck.Equals, 40)
}

func (s *QuotaSuite) TestChangeAppQuotaRequiresAdmin(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	app := app.App{
		Name:  "shangrila",
		Quota: quota.Quota{Limit: 4, InUse: 2},
	}
	err = conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": app.Name})
	user := &auth.User{
		Email:    "radio@gaga.com",
		Password: "qwe123",
	}
	_, err = nativeScheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": user.Email})
	token, err := nativeScheme.Login(map[string]string{"email": user.Email, "password": "qwe123"})
	c.Assert(err, gocheck.IsNil)
	body := bytes.NewBufferString("limit=40")
	request, _ := http.NewRequest("POST", "/apps/shangrila/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, adminRequiredErr.Code)
	c.Assert(recorder.Body.String(), gocheck.Equals, adminRequiredErr.Message+"\n")
}

func (s *QuotaSuite) TestChangeAppQuotaInvalidLimitValue(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	app := app.App{
		Name:  "shangrila",
		Quota: quota.Quota{Limit: 4, InUse: 2},
	}
	err = conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": app.Name})
	values := []string{"four", ""}
	for _, value := range values {
		body := bytes.NewBufferString("limit=" + value)
		request, _ := http.NewRequest("POST", "/apps/shangrila/quota", body)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		recorder := httptest.NewRecorder()
		handler := RunServer(true)
		handler.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, gocheck.Equals, http.StatusBadRequest)
		c.Assert(recorder.Body.String(), gocheck.Equals, "Invalid limit\n")
	}
}

func (s *QuotaSuite) TestChangeAppQuotaAppNotFound(c *gocheck.C) {
	body := bytes.NewBufferString("limit=2")
	request, _ := http.NewRequest("POST", "/apps/shangrila/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), gocheck.Equals, app.ErrAppNotFound.Error()+"\n")
}
