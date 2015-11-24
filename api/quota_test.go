// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type QuotaSuite struct {
	team  *auth.Team
	user  *auth.User
	token auth.Token
}

var _ = check.Suite(&QuotaSuite{})

func (s *QuotaSuite) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_api_quota_test")
	config.Set("auth:hash-cost", 4)
	config.Set("repo-manager", "fake")
}

func (s *QuotaSuite) SetUpTest(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
	repositorytest.Reset()
	s.team = &auth.Team{Name: "superteam"}
	err := conn.Teams().Insert(s.team)
	c.Assert(err, check.IsNil)
	s.token = customUserWithPermission(c, "quotauser", permission.Permission{
		Scheme:  permission.PermAppAdminQuota,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermUserUpdateQuota,
		Context: permission.Context(permission.CtxGlobal, ""),
	})
	s.user, err = s.token.User()
	c.Assert(err, check.IsNil)
	app.AuthScheme = nativeScheme
}

func (s *QuotaSuite) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}

func (s *QuotaSuite) TestGetUserQuota(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	user := &auth.User{
		Email:    "radio@gaga.com",
		Password: "qwe123",
		Quota:    quota.Quota{Limit: 4, InUse: 2},
	}
	_, err = nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	defer conn.Users().Remove(bson.M{"email": user.Email})
	request, _ := http.NewRequest("GET", "/users/radio@gaga.com/quota", nil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var qt quota.Quota
	err = json.NewDecoder(recorder.Body).Decode(&qt)
	c.Assert(err, check.IsNil)
	c.Assert(qt, check.DeepEquals, user.Quota)
}

func (s *QuotaSuite) TestGetUserQuotaRequiresPermission(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	user := &auth.User{
		Email:    "radio@gaga.com",
		Password: "qwe123",
	}
	_, err = nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c)
	request, _ := http.NewRequest("GET", "/users/radio@gaga.com/quota", nil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *QuotaSuite) TestGetUserQuotaUserNotFound(c *check.C) {
	request, _ := http.NewRequest("GET", "/users/radio@gaga.com/quota", nil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, auth.ErrUserNotFound.Error()+"\n")
}

func (s *QuotaSuite) TestChangeUserQuota(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	user := &auth.User{
		Email:    "radio@gaga.com",
		Password: "qwe123",
		Quota:    quota.Quota{Limit: 4, InUse: 2},
	}
	_, err = nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	defer conn.Users().Remove(bson.M{"email": user.Email})
	body := bytes.NewBufferString("limit=40")
	request, _ := http.NewRequest("POST", "/users/radio@gaga.com/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	user, err = auth.GetUserByEmail(user.Email)
	c.Assert(err, check.IsNil)
	c.Assert(user.Quota.Limit, check.Equals, 40)
	c.Assert(user.Quota.InUse, check.Equals, 2)
}

func (s *QuotaSuite) TestChangeUserQuotaRequiresPermission(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	user := &auth.User{
		Email:    "radio@gaga.com",
		Password: "qwe123",
		Quota:    quota.Quota{Limit: 4, InUse: 2},
	}
	_, err = nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c)
	body := bytes.NewBufferString("limit=40")
	request, _ := http.NewRequest("POST", "/users/radio@gaga.com/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *QuotaSuite) TestChangeUserQuotaInvalidLimitValue(c *check.C) {
	values := []string{"four", ""}
	for _, value := range values {
		body := bytes.NewBufferString("limit=" + value)
		request, _ := http.NewRequest("POST", "/users/radio@gaga.com/quota", body)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		recorder := httptest.NewRecorder()
		handler := RunServer(true)
		handler.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
		c.Assert(recorder.Body.String(), check.Equals, "Invalid limit\n")
	}
}

func (s *QuotaSuite) TestChangeUserQuotaUserNotFound(c *check.C) {
	body := bytes.NewBufferString("limit=2")
	request, _ := http.NewRequest("POST", "/users/radio@gaga.com/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, auth.ErrUserNotFound.Error()+"\n")
}

func (s *QuotaSuite) TestGetAppQuota(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	app := &app.App{
		Name:  "civil",
		Quota: quota.Quota{Limit: 4, InUse: 2},
		Teams: []string{s.team.Name},
	}
	err = conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": app.Name})
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	request, _ := http.NewRequest("GET", "/apps/civil/quota", nil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var qt quota.Quota
	err = json.NewDecoder(recorder.Body).Decode(&qt)
	c.Assert(err, check.IsNil)
	c.Assert(qt, check.DeepEquals, app.Quota)
}

func (s *QuotaSuite) TestGetAppQuotaRequiresAdmin(c *check.C) {
	conn, err := db.Conn()
	app := &app.App{
		Name:  "shangrila",
		Quota: quota.Quota{Limit: 4, InUse: 2},
	}
	err = conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	defer conn.Close()
	user := &auth.User{
		Email:    "radio@gaga.com",
		Password: "qwe123",
	}
	_, err = nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	defer conn.Users().Remove(bson.M{"email": user.Email})
	token, err := nativeScheme.Login(map[string]string{"email": user.Email, "password": "qwe123"})
	c.Assert(err, check.IsNil)
	request, _ := http.NewRequest("GET", "/apps/shangrila/quota", nil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, permission.ErrUnauthorized.Code)
	c.Assert(recorder.Body.String(), check.Equals, permission.ErrUnauthorized.Message+"\n")
}

func (s *QuotaSuite) TestGetAppQuotaAppNotFound(c *check.C) {
	request, _ := http.NewRequest("GET", "/apps/shangrila/quota", nil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App shangrila not found.\n")
}

func (s *QuotaSuite) TestChangeAppQuota(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	a := &app.App{
		Name:  "shangrila",
		Quota: quota.Quota{Limit: 4, InUse: 2},
		Teams: []string{s.team.Name},
	}
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	body := bytes.NewBufferString("limit=40")
	request, _ := http.NewRequest("POST", "/apps/shangrila/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	a, err = app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(a.Quota.InUse, check.Equals, 2)
	c.Assert(a.Quota.Limit, check.Equals, 40)
}

func (s *QuotaSuite) TestChangeAppQuotaRequiresAdmin(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	app := app.App{
		Name:  "shangrila",
		Quota: quota.Quota{Limit: 4, InUse: 2},
		Teams: []string{s.team.Name},
	}
	err = conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": app.Name})
	token := customUserWithPermission(c, "other", permission.Permission{
		Scheme:  permission.PermAppAdminQuota,
		Context: permission.Context(permission.CtxTeam, "-other-"),
	})
	body := bytes.NewBufferString("limit=40")
	request, _ := http.NewRequest("POST", "/apps/shangrila/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *QuotaSuite) TestChangeAppQuotaInvalidLimitValue(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	app := app.App{
		Name:  "shangrila",
		Quota: quota.Quota{Limit: 4, InUse: 2},
	}
	err = conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
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
		c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
		c.Assert(recorder.Body.String(), check.Equals, "Invalid limit\n")
	}
}

func (s *QuotaSuite) TestChangeAppQuotaAppNotFound(c *check.C) {
	body := bytes.NewBufferString("limit=2")
	request, _ := http.NewRequest("POST", "/apps/shangrila/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, app.ErrAppNotFound.Error()+"\n")
}
