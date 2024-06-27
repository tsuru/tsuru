// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	authTypes "github.com/tsuru/tsuru/types/auth"
	eventTypes "github.com/tsuru/tsuru/types/event"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/types/quota"
	"golang.org/x/crypto/bcrypt"
	check "gopkg.in/check.v1"
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
	storagev2.Reset()
	s.testServer = RunServer(true)
}

func (s *QuotaSuite) SetUpTest(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
	s.team = &authTypes.Team{Name: "superteam"}
	_, s.token = permissiontest.CustomUserWithPermission(c, nativeScheme, "quotauser", permission.Permission{
		Scheme:  permission.PermAppAdminQuota,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermUserUpdateQuota,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermUserReadQuota,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermTeamReadQuota,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermTeamUpdateQuota,
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
	dbtest.ClearAllCollections(conn.Apps().Database)
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
	_, err = nativeScheme.Create(context.TODO(), user)
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
	_, err = nativeScheme.Create(context.TODO(), user)
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
	s.mockService.UserQuota.OnSetLimit = func(item quota.QuotaItem, limit int) error {
		c.Assert(item.GetName(), check.Equals, "radio@gaga.com")
		c.Assert(limit, check.Equals, 40)
		return nil
	}
	_, err = nativeScheme.Create(context.TODO(), user)
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
		Target: eventTypes.Target{Type: eventTypes.TargetTypeUser, Value: user.Email},
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
	_, err := nativeScheme.Create(context.TODO(), user)
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
	_, err := nativeScheme.Create(context.TODO(), user)
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
			Target: eventTypes.Target{Type: eventTypes.TargetTypeUser, Value: user.Email},
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
	s.mockService.UserQuota.OnSetLimit = func(item quota.QuotaItem, limit int) error {
		c.Assert(item.GetName(), check.Equals, "radio@gaga.com")
		c.Assert(limit, check.Equals, 3)
		return quota.ErrLimitLowerThanAllocated
	}
	_, err = nativeScheme.Create(context.TODO(), user)
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
		Target: eventTypes.Target{Type: eventTypes.TargetTypeUser, Value: user.Email},
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

func (s *QuotaSuite) TestGetTeamQuota(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	team := &authTypes.Team{
		Name:         "avengers",
		CreatingUser: "radio@gaga.com",
		Quota:        quota.Quota{Limit: 4, InUse: 2},
	}
	s.mockService.Team.OnFindByName = func(s string) (*authTypes.Team, error) {
		return team, nil
	}

	request, err := http.NewRequest("GET", "/teams/avengers/quota", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var qt quota.Quota
	err = json.NewDecoder(recorder.Body).Decode(&qt)
	c.Assert(err, check.IsNil)
	c.Assert(qt, check.DeepEquals, team.Quota)
}

func (s *QuotaSuite) TestGetTeamQuotaRequiresPermission(c *check.C) {
	token := userWithPermission(c)
	request, _ := http.NewRequest("GET", "/teams/avengers/quota", nil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *QuotaSuite) TestGetTeamQuotaTeamNotFound(c *check.C) {
	s.mockService.Team.OnFindByName = func(s string) (*authTypes.Team, error) {
		return nil, authTypes.ErrTeamNotFound
	}
	request, _ := http.NewRequest("GET", "/teams/avengers/quota", nil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, authTypes.ErrTeamNotFound.Error()+"\n")
}

func (s *QuotaSuite) TestChangeTeamQuota(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	team := &authTypes.Team{
		Name:         "avengers",
		CreatingUser: "radio@gaga.com",
		Quota:        quota.Quota{Limit: 4, InUse: 2},
	}

	s.mockService.Team.OnFindByName = func(s string) (*authTypes.Team, error) {
		return team, nil
	}
	s.mockService.TeamQuota.OnSetLimit = func(qi quota.QuotaItem, i int) error {
		c.Assert(qi.GetName(), check.Equals, team.Name)
		c.Assert(i, check.Equals, 40)
		return nil
	}

	body := bytes.NewBufferString("limit=40")
	request, _ := http.NewRequest("PUT", "/teams/avengers/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(err, check.IsNil)
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeTeam, Value: team.Name},
		Owner:  s.token.GetUserName(),
		Kind:   "team.update.quota",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": team.Name},
			{"name": "limit", "value": "40"},
		},
	}, eventtest.HasEvent)
}

func (s *QuotaSuite) TestChangeTeamQuotaRequiresPermission(c *check.C) {
	token := userWithPermission(c)
	request, _ := http.NewRequest("PUT", "/teams/avengers/quota", nil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *QuotaSuite) TestChangeTeamQuotaInvalidLimitValue(c *check.C) {
	team := &authTypes.Team{
		Name:         "avengers",
		CreatingUser: "radio@gaga.com",
		Quota:        quota.Quota{Limit: 10, InUse: 5},
	}

	s.mockService.Team.OnFindByName = func(s string) (*authTypes.Team, error) {
		return team, nil
	}

	values := []string{"four", ""}
	for _, value := range values {
		body := bytes.NewBufferString("limit=" + value)
		request, _ := http.NewRequest("PUT", "/teams/avengers/quota", body)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		recorder := httptest.NewRecorder()
		handler := RunServer(true)
		handler.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
		c.Assert(recorder.Body.String(), check.Equals, "Invalid limit\n")
		c.Assert(eventtest.EventDesc{
			Target: eventTypes.Target{Type: eventTypes.TargetTypeTeam, Value: team.Name},
			Owner:  s.token.GetUserName(),
			Kind:   "team.update.quota",
			StartCustomData: []map[string]interface{}{
				{"name": ":name", "value": team.Name},
				{"name": "limit", "value": value},
			},
			ErrorMatches: `Invalid limit`,
		}, eventtest.HasEvent)
	}
}

func (s *QuotaSuite) TestChangeTeamQuotaLimitLowerThanAllocated(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	team := &authTypes.Team{
		Name:         "avengers",
		CreatingUser: "radio@gaga.com",
		Quota:        quota.Quota{Limit: 10, InUse: 5},
	}

	s.mockService.Team.OnFindByName = func(s string) (*authTypes.Team, error) {
		return team, nil
	}
	s.mockService.TeamQuota.OnSetLimit = func(qi quota.QuotaItem, i int) error {
		c.Assert(qi.GetName(), check.Equals, team.Name)
		c.Assert(i, check.Equals, 4)
		return quota.ErrLimitLowerThanAllocated
	}

	body := bytes.NewBufferString("limit=4")
	request, _ := http.NewRequest("PUT", "/teams/avengers/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(err, check.IsNil)
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeTeam, Value: team.Name},
		Owner:  s.token.GetUserName(),
		Kind:   "team.update.quota",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": team.Name},
			{"name": "limit", "value": "4"},
		},
		ErrorMatches: `New limit is less than the current allocated value`,
	}, eventtest.HasEvent)
}

func (s *QuotaSuite) TestChangeTeamQuotaTeamNotFound(c *check.C) {
	s.mockService.Team.OnFindByName = func(s string) (*authTypes.Team, error) {
		return nil, authTypes.ErrTeamNotFound
	}
	body := bytes.NewBufferString("limit=2")
	request, _ := http.NewRequest("PUT", "/teams/avengers/quota", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, authTypes.ErrTeamNotFound.Error()+"\n")
}

func (s *QuotaSuite) TestGetAppQuota(c *check.C) {
	s.mockService.AppQuota.OnGet = func(item quota.QuotaItem) (*quota.Quota, error) {
		c.Assert(item.GetName(), check.Equals, "civil")
		return &quota.Quota{Limit: 4, InUse: 2}, nil
	}
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	app := &app.App{
		Name:  "civil",
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
	c.Assert(qt, check.DeepEquals, quota.Quota{Limit: 4, InUse: 2})
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
	_, err = nativeScheme.Create(context.TODO(), user)
	c.Assert(err, check.IsNil)
	defer conn.Users().Remove(bson.M{"email": user.Email})
	token, err := nativeScheme.Login(context.TODO(), map[string]string{"email": user.Email, "password": "qwe123"})
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
	s.mockService.AppQuota.OnSetLimit = func(item quota.QuotaItem, limit int) error {
		c.Assert(item.GetName(), check.Equals, a.Name)
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
		Target: eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Owner:  s.token.GetUserName(),
		Kind:   "app.admin.quota",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
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
			Target: eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: app.Name},
			Owner:  s.token.GetUserName(),
			Kind:   "app.admin.quota",
			StartCustomData: []map[string]interface{}{
				{"name": ":app", "value": app.Name},
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
	c.Assert(recorder.Body.String(), check.Equals, "App shangrila not found.\n")
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
	s.mockService.AppQuota.OnSetLimit = func(item quota.QuotaItem, limit int) error {
		c.Assert(item.GetName(), check.Equals, a.Name)
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
		Target: eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Owner:  s.token.GetUserName(),
		Kind:   "app.admin.quota",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "limit", "value": "3"},
		},
		ErrorMatches: `New limit is less than the current allocated value`,
	}, eventtest.HasEvent)
}
