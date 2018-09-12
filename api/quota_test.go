// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/repository/repositorytest"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/types/quota"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/check.v1"
)

type QuotaSuite struct {
	team        *authTypes.Team
	user        *auth.User
	token       auth.Token
	testServer  http.Handler
	mockService servicemock.MockService
}

var _ = check.Suite(&QuotaSuite{})

func (s *QuotaSuite) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_api_quota_test")
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("repo-manager", "fake")
	s.testServer = RunServer(true)
}

func (s *QuotaSuite) SetUpTest(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
	repositorytest.Reset()
	s.team = &authTypes.Team{Name: "superteam"}
	_, s.token = permissiontest.CustomUserWithPermission(c, nativeScheme, "quotauser", permission.Permission{
		Scheme:  permission.PermAppAdminQuota,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermUserUpdateQuota,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	var err error
	s.user, err = auth.ConvertNewUser(s.token.User())
	c.Assert(err, check.IsNil)
	app.AuthScheme = nativeScheme
	servicemock.SetMockService(&s.mockService)
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
	request, err := http.NewRequest("GET", "/users/radio@gaga.com/quota", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
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
	c.Assert(recorder.Body.String(), check.Equals, authTypes.ErrUserNotFound.Error()+"\n")
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
	s.mockService.UserQuota.OnSetLimit = func(email string, limit int) error {
		c.Assert(email, check.Equals, "radio@gaga.com")
		c.Assert(limit, check.Equals, 40)
		return nil
	}
	_, err = nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	defer conn.Users().Remove(bson.M{"email": user.Email})
	body := bytes.NewBufferString("limit=40")
	request, _ := http.NewRequest("PUT", "/users/radio@gaga.com/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(err, check.IsNil)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeUser, Value: user.Email},
		Owner:  s.token.GetUserName(),
		Kind:   "user.update.quota",
		StartCustomData: []map[string]interface{}{
			{"name": ":email", "value": user.Email},
			{"name": "limit", "value": "40"},
		},
	}, eventtest.HasEvent)
}

func (s *QuotaSuite) TestChangeUserQuotaRequiresPermission(c *check.C) {
	user := &auth.User{
		Email:    "radio@gaga.com",
		Password: "qwe123",
		Quota:    quota.Quota{Limit: 4, InUse: 2},
	}
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c)
	body := bytes.NewBufferString("limit=40")
	request, _ := http.NewRequest("PUT", "/users/radio@gaga.com/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *QuotaSuite) TestChangeUserQuotaInvalidLimitValue(c *check.C) {
	user := &auth.User{
		Email:    "radio@gaga.com",
		Password: "qwe123",
		Quota:    quota.Quota{Limit: 4, InUse: 2},
	}
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	values := []string{"four", ""}
	for _, value := range values {
		body := bytes.NewBufferString("limit=" + value)
		request, _ := http.NewRequest("PUT", "/users/radio@gaga.com/quota", body)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		recorder := httptest.NewRecorder()
		handler := RunServer(true)
		handler.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
		c.Assert(recorder.Body.String(), check.Equals, "Invalid limit\n")
		c.Assert(eventtest.EventDesc{
			Target: event.Target{Type: event.TargetTypeUser, Value: user.Email},
			Owner:  s.token.GetUserName(),
			Kind:   "user.update.quota",
			StartCustomData: []map[string]interface{}{
				{"name": ":email", "value": user.Email},
				{"name": "limit", "value": value},
			},
			ErrorMatches: `Invalid limit`,
		}, eventtest.HasEvent)
	}
}

func (s *QuotaSuite) TestChangeUserQuotaLimitLowerThanAllocated(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	user := &auth.User{
		Email:    "radio@gaga.com",
		Password: "qwe123",
		Quota:    quota.Quota{Limit: 4, InUse: 2},
	}
	s.mockService.UserQuota.OnSetLimit = func(email string, limit int) error {
		c.Assert(email, check.Equals, "radio@gaga.com")
		c.Assert(limit, check.Equals, 3)
		return quota.ErrLimitLowerThanAllocated
	}
	_, err = nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	defer conn.Users().Remove(bson.M{"email": user.Email})
	body := bytes.NewBufferString("limit=3")
	request, _ := http.NewRequest("PUT", "/users/radio@gaga.com/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(err, check.IsNil)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeUser, Value: user.Email},
		Owner:  s.token.GetUserName(),
		Kind:   "user.update.quota",
		StartCustomData: []map[string]interface{}{
			{"name": ":email", "value": user.Email},
			{"name": "limit", "value": "3"},
		},
		ErrorMatches: `New limit is less than the current allocated value`,
	}, eventtest.HasEvent)
}

func (s *QuotaSuite) TestChangeUserQuotaUserNotFound(c *check.C) {
	body := bytes.NewBufferString("limit=2")
	request, _ := http.NewRequest("PUT", "/users/radio@gaga.com/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, authTypes.ErrUserNotFound.Error()+"\n")
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
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request, _ := http.NewRequest("GET", "/apps/civil/quota", nil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var qt quota.Quota
	err = json.NewDecoder(recorder.Body).Decode(&qt)
	c.Assert(err, check.IsNil)
	c.Assert(qt, check.DeepEquals, app.Quota)
}

func (s *QuotaSuite) TestGetAppQuotaRequiresAdmin(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	app := &app.App{
		Name:  "shangrila",
		Quota: quota.Quota{Limit: 4, InUse: 2},
	}
	err = conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": app.Name})
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
	s.mockService.AppQuota.OnSetLimit = func(appName string, limit int) error {
		c.Assert(appName, check.Equals, a.Name)
		c.Assert(limit, check.Equals, 40)
		return nil
	}
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	body := bytes.NewBufferString("limit=40")
	request, _ := http.NewRequest("PUT", "/apps/shangrila/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeApp, Value: a.Name},
		Owner:  s.token.GetUserName(),
		Kind:   "app.admin.quota",
		StartCustomData: []map[string]interface{}{
			{"name": ":appname", "value": a.Name},
			{"name": "limit", "value": "40"},
		},
	}, eventtest.HasEvent)
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
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "other", permission.Permission{
		Scheme:  permission.PermAppAdminQuota,
		Context: permission.Context(permTypes.CtxTeam, "-other-"),
	})
	body := bytes.NewBufferString("limit=40")
	request, _ := http.NewRequest("PUT", "/apps/shangrila/quota", body)
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
		Teams: []string{s.team.Name},
	}
	err = conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": app.Name})
	values := []string{"four", ""}
	for _, value := range values {
		body := bytes.NewBufferString("limit=" + value)
		request, _ := http.NewRequest("PUT", "/apps/shangrila/quota", body)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		recorder := httptest.NewRecorder()
		handler := RunServer(true)
		handler.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
		c.Assert(recorder.Body.String(), check.Equals, "Invalid limit\n")
		c.Assert(eventtest.EventDesc{
			Target: event.Target{Type: event.TargetTypeApp, Value: app.Name},
			Owner:  s.token.GetUserName(),
			Kind:   "app.admin.quota",
			StartCustomData: []map[string]interface{}{
				{"name": ":appname", "value": app.Name},
				{"name": "limit", "value": value},
			},
			ErrorMatches: `Invalid limit`,
		}, eventtest.HasEvent)
	}
}

func (s *QuotaSuite) TestChangeAppQuotaAppNotFound(c *check.C) {
	body := bytes.NewBufferString("limit=2")
	request, _ := http.NewRequest("PUT", "/apps/shangrila/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, appTypes.ErrAppNotFound.Error()+"\n")
}

func (s *QuotaSuite) TestChangeAppQuotaLimitLowerThanAllocated(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	a := &app.App{
		Name:  "shangrila",
		Quota: quota.Quota{Limit: 4, InUse: 2},
		Teams: []string{s.team.Name},
	}
	s.mockService.AppQuota.OnSetLimit = func(appName string, limit int) error {
		c.Assert(appName, check.Equals, a.Name)
		c.Assert(limit, check.Equals, 3)
		return quota.ErrLimitLowerThanAllocated
	}
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	body := bytes.NewBufferString("limit=3")
	request, _ := http.NewRequest("PUT", "/apps/shangrila/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeApp, Value: a.Name},
		Owner:  s.token.GetUserName(),
		Kind:   "app.admin.quota",
		StartCustomData: []map[string]interface{}{
			{"name": ":appname", "value": a.Name},
			{"name": "limit", "value": "3"},
		},
		ErrorMatches: `New limit is less than the current allocated value`,
	}, eventtest.HasEvent)
}
