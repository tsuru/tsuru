// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/service"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type DeploySuite struct {
	conn        *db.Storage
	logConn     *db.LogStorage
	token       auth.Token
	team        *auth.Team
	provisioner *provisiontest.FakeProvisioner
}

var _ = check.Suite(&DeploySuite{})

func (s *DeploySuite) createUserAndTeam(c *check.C) {
	user := &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	s.team = &auth.Team{Name: "tsuruteam", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
}

func (s *DeploySuite) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_deploy_api_tests")
	config.Set("aut:hash-cost", 4)
	config.Set("admin-team", "tsuruteam")
	config.Set("repo-manager", "fake")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	s.logConn, err = db.LogConn()
	c.Assert(err, check.IsNil)
	s.provisioner = provisiontest.NewFakeProvisioner()
	app.Provisioner = s.provisioner
	s.conn.Platforms().Insert(app.Platform{Name: "python"})
	err = provision.AddPool("pool1", false)
	c.Assert(err, check.IsNil)
}

func (s *DeploySuite) TearDownSuite(c *check.C) {
	provision.RemovePool("pool1")
	s.conn.Close()
	s.logConn.Close()
}

func (s *DeploySuite) SetUpTest(c *check.C) {
	repositorytest.Reset()
	err := dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
	s.createUserAndTeam(c)
	s.conn.Platforms().Insert(app.Platform{Name: "python"})
	err = provision.AddPool("pool1", false)
	c.Assert(err, check.IsNil)
	user, err := s.token.User()
	c.Assert(err, check.IsNil)
	repository.Manager().CreateUser(user.Email)
}

func (s *DeploySuite) TestDeployHandler(c *check.C) {
	a := app.App{Name: "otherapp", Platform: "python", Teams: []string{s.team.Name}}
	user, _ := s.token.User()
	err := app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a)
	url := fmt.Sprintf("/apps/%s/repository/clone?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("version=a345f3e&user=fulano"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "text")
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "Git deploy called\nOK\n")
	c.Assert(s.provisioner.Version(&a), check.Equals, "a345f3e")
}

func (s *DeploySuite) TestDeployArchiveURL(c *check.C) {
	user, _ := s.token.User()
	a := app.App{Name: "otherapp", Platform: "python", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a)
	url := fmt.Sprintf("/apps/%s/repository/clone?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("archive-url=http://something.tar.gz&user=fulano"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "text")
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "Archive deploy called\nOK\n")
}

func (s *DeploySuite) TestDeployUploadFile(c *check.C) {
	user, _ := s.token.User()
	a := app.App{Name: "otherapp", Platform: "python", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a)
	url := fmt.Sprintf("/apps/%s/repository/clone?:appname=%s", a.Name, a.Name)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "archive.tar.gz")
	c.Assert(err, check.IsNil)
	file.Write([]byte("hello world!"))
	writer.Close()
	request, err := http.NewRequest("POST", url, &body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "multipart/form-data; boundary="+writer.Boundary())
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "text")
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "Upload deploy called\nOK\n")
}

func (s *DeploySuite) TestDeployWithCommit(c *check.C) {
	user, _ := s.token.User()
	a := app.App{Name: "otherapp", Platform: "python", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a)
	url := fmt.Sprintf("/apps/%s/repository/clone?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("version=a345f3e&user=fulano&commit=123"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "text")
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "Git deploy called\nOK\n")
	deploys, err := s.conn.Deploys().Find(bson.M{"commit": "123"}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(deploys, check.Equals, 1)
	c.Assert(s.provisioner.Version(&a), check.Equals, "a345f3e")
}

func (s *DeploySuite) TestDeployShouldIncrementDeployNumberOnApp(c *check.C) {
	user, _ := s.token.User()
	a := app.App{Name: "otherapp", Platform: "python", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a)
	url := fmt.Sprintf("/apps/%s/repository/clone?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("version=a345f3e"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(a.Deploys, check.Equals, uint(1))
	var result map[string]interface{}
	s.conn.Deploys().Find(bson.M{"app": a.Name}).One(&result)
	c.Assert(result["app"], check.Equals, a.Name)
	now := time.Now()
	diff := now.Sub(result["timestamp"].(time.Time))
	c.Assert(diff < 60*time.Second, check.Equals, true)
}

func (s *DeploySuite) TestDeployShouldReturnNotFoundWhenAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("POST", "/apps/abc/repository/clone", strings.NewReader("version=abcdef"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	message := recorder.Body.String()
	c.Assert(message, check.Equals, "App not found.\n")
}

func (s *DeploySuite) TestDeployShouldReturnForbiddenWhenUserDoesNotHaveAccessToApp(c *check.C) {
	user := &auth.User{Email: "someone@tsuru.io", Password: "123456"}
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	defer nativeScheme.Remove(user)
	token, err := nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	adminUser, _ := s.token.User()
	a := app.App{Name: "otherapp", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, adminUser)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a)
	url := fmt.Sprintf("/apps/%s/repository/clone?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("archive-url=http://something.tar.gz&user=fulano"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "user does not have access to this app\n")
}

func (s *DeploySuite) TestDeployShouldReturnForbiddenWhenTokenIsntFromTheApp(c *check.C) {
	user, _ := s.token.User()
	app1 := app.App{Name: "otherapp", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(&app1, user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&app1)
	app2 := app.App{Name: "superapp", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(&app2, user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&app2)
	token, err := nativeScheme.AppLogin(app2.Name)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/repository/clone?:appname=%s", app1.Name, app2.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("archive-url=http://something.tar.gz&user=fulano"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusUnauthorized)
	c.Assert(recorder.Body.String(), check.Equals, "invalid app token\n")
}

func (s *DeploySuite) TestDeployWithTokenForInternalAppName(c *check.C) {
	token, err := nativeScheme.AppLogin(app.InternalAppName)
	c.Assert(err, check.IsNil)
	a := app.App{Name: "otherapp", Platform: "python", Teams: []string{s.team.Name}}
	user, _ := s.token.User()
	err = app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a)
	url := fmt.Sprintf("/apps/%s/repository/clone?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("version=a345f3e&user=fulano"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "text")
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "Git deploy called\nOK\n")
	c.Assert(s.provisioner.Version(&a), check.Equals, "a345f3e")
}

func (s *DeploySuite) TestDeployWithoutVersionAndArchiveURL(c *check.C) {
	user, _ := s.token.User()
	a := app.App{Name: "abc", Platform: "python", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a)
	defer s.logConn.Logs(a.Name).DropCollection()
	request, err := http.NewRequest("POST", "/apps/abc/repository/clone", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	message := recorder.Body.String()
	c.Assert(message, check.Equals, "you must specify either the version, the archive-url or upload a file\n")
}

func (s *DeploySuite) TestDeployWithVersionAndArchiveURL(c *check.C) {
	user, _ := s.token.User()
	a := app.App{Name: "abc", Platform: "python", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a)
	defer s.logConn.Logs(a.Name).DropCollection()
	body := strings.NewReader("version=abcdef&archive-url=http://google.com")
	request, err := http.NewRequest("POST", "/apps/abc/repository/clone", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	message := recorder.Body.String()
	c.Assert(message, check.Equals, "you must specify either the version or the archive-url, but not both\n")
}

func (s *DeploySuite) TestDeployListNonAdmin(c *check.C) {
	user := &auth.User{Email: "nonadmin@nonadmin.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	team := &auth.Team{Name: "newteam", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	a := app.App{Name: "g1", Platform: "python", Teams: []string{team.Name}}
	err = app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a)
	var result []app.DeployData
	request, err := http.NewRequest("GET", "/deploys", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	timestamp := time.Date(2013, time.November, 1, 0, 0, 0, 0, time.Local)
	duration := time.Since(timestamp)
	err = s.conn.Deploys().Insert(app.DeployData{App: "g1", Timestamp: timestamp.Add(time.Minute), Duration: duration})
	c.Assert(err, check.IsNil)
	err = s.conn.Deploys().Insert(app.DeployData{App: "ge", Timestamp: timestamp.Add(time.Second), Duration: duration})
	c.Assert(err, check.IsNil)
	defer s.conn.Deploys().RemoveAll(nil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result[0].ID, check.NotNil)
	c.Assert(result[0].App, check.Equals, "g1")
	c.Assert(result[0].Timestamp.In(time.UTC), check.DeepEquals, timestamp.Add(time.Minute).In(time.UTC))
	c.Assert(result[0].Duration, check.DeepEquals, duration)
}

func (s *DeploySuite) TestDeployList(c *check.C) {
	user, _ := s.token.User()
	app1 := app.App{Name: "g1", Platform: "python", Teams: []string{s.team.Name}}
	err := app.CreateApp(&app1, user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&app1)
	app2 := app.App{Name: "ge", Platform: "python", Teams: []string{s.team.Name}}
	err = app.CreateApp(&app2, user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&app2)
	var result []app.DeployData
	request, err := http.NewRequest("GET", "/deploys", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	timestamp := time.Date(2013, time.November, 1, 0, 0, 0, 0, time.Local)
	duration := time.Since(timestamp)
	err = s.conn.Deploys().Insert(app.DeployData{App: "g1", Timestamp: timestamp.Add(time.Minute), Duration: duration})
	c.Assert(err, check.IsNil)
	err = s.conn.Deploys().Insert(app.DeployData{App: "ge", Timestamp: timestamp.Add(time.Second), Duration: duration})
	c.Assert(err, check.IsNil)
	defer s.conn.Deploys().RemoveAll(nil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result[0].ID, check.NotNil)
	c.Assert(result[0].App, check.Equals, "g1")
	c.Assert(result[0].Timestamp.In(time.UTC), check.DeepEquals, timestamp.Add(time.Minute).In(time.UTC))
	c.Assert(result[0].Duration, check.DeepEquals, duration)
	c.Assert(result[1].App, check.Equals, "ge")
	c.Assert(result[1].Timestamp.In(time.UTC), check.DeepEquals, timestamp.Add(time.Second).In(time.UTC))
	c.Assert(result[1].Duration, check.DeepEquals, duration)
}

func (s *DeploySuite) TestDeployListByService(c *check.C) {
	user, _ := s.token.User()
	a := app.App{Name: "g1", Platform: "python", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a)
	var result []app.DeployData
	srv := service.Service{Name: "redis", Teams: []string{s.team.Name}}
	err = srv.Create()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "redis-g1",
		ServiceName: "redis",
		Apps:        []string{"g1", "qwerty"},
		Teams:       []string{s.team.Name},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	defer func() {
		srv.Delete()
		service.DeleteInstance(&instance)
	}()
	request, err := http.NewRequest("GET", "/deploys?service=redis", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	timestamp := time.Date(2013, time.November, 1, 0, 0, 0, 0, time.Local)
	duration := time.Since(timestamp)
	err = s.conn.Deploys().Insert(app.DeployData{App: "g1", Timestamp: timestamp, Duration: duration})
	c.Assert(err, check.IsNil)
	err = s.conn.Deploys().Insert(app.DeployData{App: "ge", Timestamp: timestamp, Duration: duration})
	c.Assert(err, check.IsNil)
	defer s.conn.Deploys().RemoveAll(nil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 1)
	c.Assert(result[0].App, check.Equals, "g1")
	c.Assert(result[0].Timestamp.In(time.UTC), check.DeepEquals, timestamp.In(time.UTC))
	c.Assert(result[0].Duration, check.DeepEquals, duration)
}

func (s *DeploySuite) TestDeployListByApp(c *check.C) {
	user, _ := s.token.User()
	a := app.App{Name: "myblog", Platform: "python", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a)
	defer s.logConn.Logs(a.Name).DropCollection()
	timestamp := time.Date(2013, time.November, 1, 0, 0, 0, 0, time.Local)
	duration := time.Since(timestamp)
	deploys := []app.DeployData{
		{App: "myblog", Timestamp: timestamp, Duration: duration},
		{App: "yourblog", Timestamp: timestamp, Duration: duration},
	}
	for _, deploy := range deploys {
		err := s.conn.Deploys().Insert(deploy)
		c.Assert(err, check.IsNil)
	}
	defer s.conn.Deploys().RemoveAll(nil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/deploys?app=myblog", nil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result []app.DeployData
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 1)
	c.Assert(result[0].App, check.Equals, "myblog")
	c.Assert(result[0].Timestamp.In(time.UTC), check.DeepEquals, timestamp.In(time.UTC))
	c.Assert(result[0].Duration, check.DeepEquals, duration)
}

func (s *DeploySuite) TestDeployListAppWithNoDeploys(c *check.C) {
	user, _ := s.token.User()
	a := app.App{Name: "myblog", Platform: "python", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a)
	defer s.logConn.Logs(a.Name).DropCollection()
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/deploys?app=myblog", nil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *DeploySuite) TestDeployListByAppAndService(c *check.C) {
	srv := service.Service{Name: "redis", Teams: []string{s.team.Name}}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "redis-myblog",
		ServiceName: "redis",
		Apps:        []string{"yourblog"},
		Teams:       []string{s.team.Name},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	defer func() {
		srv.Delete()
		service.DeleteInstance(&instance)
	}()
	user, _ := s.token.User()
	a := app.App{Name: "myblog", Platform: "python", Teams: []string{s.team.Name}}
	err = app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a)
	defer s.logConn.Logs(a.Name).DropCollection()
	timestamp := time.Date(2013, time.November, 1, 0, 0, 0, 0, time.Local)
	duration := time.Since(timestamp)
	deploys := []app.DeployData{
		{App: "myblog", Timestamp: timestamp, Duration: duration},
		{App: "yourblog", Timestamp: timestamp, Duration: duration},
	}
	for _, deploy := range deploys {
		err := s.conn.Deploys().Insert(deploy)
		c.Assert(err, check.IsNil)
	}
	defer s.conn.Deploys().RemoveAll(nil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/deploys?app=myblog&service=redis", nil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *DeploySuite) TestDeployInfoByAdminUser(c *check.C) {
	a := app.App{Name: "g1", Platform: "python", Teams: []string{s.team.Name}}
	user, _ := s.token.User()
	err := app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a)
	recorder := httptest.NewRecorder()
	timestamp := time.Now()
	duration := time.Duration(10e9)
	previousDeploy := app.DeployData{App: "g1", Timestamp: timestamp.Add(-3600 * time.Second), Duration: duration, Commit: "e293e3e3me03ejm3puejmp3ej3iejop32", Error: ""}
	err = s.conn.Deploys().Insert(previousDeploy)
	c.Assert(err, check.IsNil)
	lastDeploy := app.DeployData{App: "g1", Timestamp: timestamp, Duration: duration, Commit: "e82nn93nd93mm12o2ueh83dhbd3iu112", Error: ""}
	err = s.conn.Deploys().Insert(lastDeploy)
	c.Assert(err, check.IsNil)
	defer s.conn.Deploys().RemoveAll(nil)
	var d map[string]interface{}
	err = s.conn.Deploys().Find(bson.M{"commit": lastDeploy.Commit}).One(&d)
	c.Assert(err, check.IsNil)
	lastDeployId := d["_id"].(bson.ObjectId).Hex()
	url := fmt.Sprintf("/deploys/%s", lastDeployId)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result app.DeployData
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	lastDeploy.ID = d["_id"].(bson.ObjectId)
	result.Timestamp = lastDeploy.Timestamp
	result.RemoveDate = lastDeploy.RemoveDate
	c.Assert(result, check.DeepEquals, lastDeploy)
}

func (s *DeploySuite) TestDeployInfoDiff(c *check.C) {
	a := app.App{Name: "g1", Platform: "python", Teams: []string{s.team.Name}}
	user, _ := s.token.User()
	err := app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a)
	recorder := httptest.NewRecorder()
	timestamp := time.Now()
	duration := time.Duration(10e9)
	previousDeploy := app.DeployData{App: "g1", Timestamp: timestamp.Add(-3600 * time.Second), Duration: duration, Commit: "e293e3e3me03ejm3puejmp3ej3iejop32", Error: "", Origin: "git"}
	err = s.conn.Deploys().Insert(previousDeploy)
	c.Assert(err, check.IsNil)
	lastDeploy := app.DeployData{App: "g1", Timestamp: timestamp, Duration: duration, Commit: "e82nn93nd93mm12o2ueh83dhbd3iu112", Error: "", Origin: "git"}
	err = s.conn.Deploys().Insert(lastDeploy)
	c.Assert(err, check.IsNil)
	defer s.conn.Deploys().RemoveAll(nil)
	var d map[string]interface{}
	err = s.conn.Deploys().Find(bson.M{"commit": lastDeploy.Commit}).One(&d)
	c.Assert(err, check.IsNil)
	lastDeployId := d["_id"].(bson.ObjectId).Hex()
	url := fmt.Sprintf("/deploys/%s", lastDeployId)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	lastDeploy.ID = d["_id"].(bson.ObjectId)
	expected := app.DiffDeployData{DeployData: lastDeploy, Diff: repositorytest.Diff}
	var result app.DiffDeployData
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	result.Timestamp = lastDeploy.Timestamp
	result.RemoveDate = lastDeploy.RemoveDate
	c.Assert(result, check.DeepEquals, expected)
}

func (s *DeploySuite) TestDeployInfoByNonAdminUser(c *check.C) {
	user, _ := s.token.User()
	a := app.App{Name: "g1", Platform: "python", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a)
	user = &auth.User{Email: "user@user.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	_, err = nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	defer user.Delete()
	team := &auth.Team{Name: "team", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().Remove(team)
	token, err := nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	timestamp := time.Now()
	duration := time.Duration(10e9)
	previousDeploy := app.DeployData{App: "g1", Timestamp: timestamp.Add(-3600 * time.Second), Duration: duration, Commit: "e293e3e3me03ejm3puejmp3ej3iejop32", Error: ""}
	err = s.conn.Deploys().Insert(previousDeploy)
	c.Assert(err, check.IsNil)
	lastDeploy := app.DeployData{App: "g1", Timestamp: timestamp, Duration: duration, Commit: "e82nn93nd93mm12o2ueh83dhbd3iu112", Error: ""}
	err = s.conn.Deploys().Insert(lastDeploy)
	c.Assert(err, check.IsNil)
	defer s.conn.Deploys().RemoveAll(nil)
	var d map[string]interface{}
	err = s.conn.Deploys().Find(bson.M{"commit": lastDeploy.Commit}).One(&d)
	c.Assert(err, check.IsNil)
	lastDeployId := d["_id"].(bson.ObjectId).Hex()
	url := fmt.Sprintf("/deploys/%s", lastDeployId)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	body := recorder.Body.String()
	c.Assert(body, check.Equals, "Deploy not found.\n")
}

func (s *DeploySuite) TestDeployInfoByNonAuthenticated(c *check.C) {
	recorder := httptest.NewRecorder()
	url := fmt.Sprintf("/deploys/xpto")
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusUnauthorized)
}

func (s *DeploySuite) TestDeployInfoByUserWithoutAccess(c *check.C) {
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	defer user.Delete()
	team := &auth.Team{Name: "team", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().Remove(team)
	token, err := nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	timestamp := time.Now()
	duration := time.Duration(10e9)
	previousDeploy := app.DeployData{App: "g1", Timestamp: timestamp.Add(-3600 * time.Second), Duration: duration, Commit: "e293e3e3me03ejm3puejmp3ej3iejop32", Error: ""}
	err = s.conn.Deploys().Insert(previousDeploy)
	c.Assert(err, check.IsNil)
	lastDeploy := app.DeployData{App: "g1", Timestamp: timestamp, Duration: duration, Commit: "e82nn93nd93mm12o2ueh83dhbd3iu112", Error: ""}
	err = s.conn.Deploys().Insert(lastDeploy)
	c.Assert(err, check.IsNil)
	defer s.conn.Deploys().RemoveAll(nil)
	var d map[string]interface{}
	err = s.conn.Deploys().Find(bson.M{"commit": lastDeploy.Commit}).One(&d)
	c.Assert(err, check.IsNil)
	lastDeployId := d["_id"].(bson.ObjectId).Hex()
	url := fmt.Sprintf("/deploys/%s", lastDeployId)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	body := recorder.Body.String()
	c.Assert(body, check.Equals, "Deploy not found.\n")
}

func (s *DeploySuite) TestDeployRollbackHandler(c *check.C) {
	user, _ := s.token.User()
	a := app.App{Name: "otherapp", Platform: "python", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a)
	url := fmt.Sprintf("/apps/%s/deploy/rollback", a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("image=my-image-123"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "{\"Message\":\"Image deploy called\"}\n")
}
