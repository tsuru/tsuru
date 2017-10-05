// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/builder/fake"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/router/routertest"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	serviceTypes "github.com/tsuru/tsuru/types/service"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/check.v1"
)

type BuildSuite struct {
	conn        *db.Storage
	logConn     *db.LogStorage
	token       auth.Token
	team        *authTypes.Team
	provisioner *provisiontest.FakeProvisioner
	builder     *fake.FakeBuilder
	testServer  http.Handler
}

var _ = check.Suite(&BuildSuite{})

func (s *BuildSuite) createUserAndTeam(c *check.C) {
	user := &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	app.AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	s.team = &authTypes.Team{Name: "tsuruteam"}
	err = serviceTypes.Team().Insert(*s.team)
	c.Assert(err, check.IsNil)
	s.token = userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppReadDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermAppBuild,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
}

func (s *BuildSuite) reset() {
	s.provisioner.Reset()
	s.builder.Reset()
	routertest.FakeRouter.Reset()
	repositorytest.Reset()
}

func (s *BuildSuite) SetUpSuite(c *check.C) {
	err := config.ReadConfigFile("testdata/config.yaml")
	c.Assert(err, check.IsNil)
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_deploy_api_tests")
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("repo-manager", "fake")
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	s.logConn, err = db.LogConn()
	c.Assert(err, check.IsNil)
	s.builder = fake.NewFakeBuilder()
	builder.Register("fake", s.builder)
	s.testServer = RunServer(true)
}

func (s *BuildSuite) TearDownSuite(c *check.C) {
	config.Unset("docker:router")
	pool.RemovePool("pool1")
	s.conn.Apps().Database.DropDatabase()
	s.logConn.Logs("myapp").Database.DropDatabase()
	s.conn.Close()
	s.logConn.Close()
	s.reset()
}

func (s *BuildSuite) SetUpTest(c *check.C) {
	s.provisioner = provisiontest.ProvisionerInstance
	provision.DefaultProvisioner = "fake"
	builder.DefaultBuilder = "fake"
	s.reset()
	err := dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
	s.createUserAndTeam(c)
	app.PlatformService().Insert(appTypes.Platform{Name: "python"})
	opts := pool.AddPoolOptions{Name: "pool1", Default: true}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	user, err := s.token.User()
	c.Assert(err, check.IsNil)
	repository.Manager().CreateUser(user.Email)
	config.Set("docker:router", "fake")
}

func (s *BuildSuite) TestBuildHandler(c *check.C) {
	user, _ := s.token.User()
	a := app.App{
		Name:      "otherapp",
		Platform:  "python",
		Router:    "fake",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/build?tag=mytag", a.Name)
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
	c.Assert(recorder.Body.String(), check.Equals, "tsuruteam/app-otherapp:mytag\n")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.build",
		StartCustomData: map[string]interface{}{
			"app.name":   a.Name,
			"commit":     "",
			"filesize":   12,
			"kind":       "upload",
			"archiveurl": "",
			"user":       s.token.GetUserName(),
			"image":      "",
			"origin":     "",
			"build":      false,
			"rollback":   false,
		},
		EndCustomData: map[string]interface{}{
			"image": "tsuruteam/app-otherapp:mytag",
		},
	}, eventtest.HasEvent)
}

func (s *BuildSuite) TestBuildArchiveURL(c *check.C) {
	user, _ := s.token.User()
	a := app.App{
		Name:      "otherapp",
		Platform:  "python",
		Router:    "fake",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/build", a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("tag=mytag&archive-url=http://something.tar.gz"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "text")
	c.Assert(recorder.Body.String(), check.Equals, "tsuruteam/app-otherapp:mytag\n")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.build",
		StartCustomData: map[string]interface{}{
			"app.name":   a.Name,
			"commit":     "",
			"filesize":   0,
			"kind":       "archive-url",
			"archiveurl": "http://something.tar.gz",
			"user":       s.token.GetUserName(),
			"image":      "",
			"origin":     "",
			"build":      false,
			"rollback":   false,
		},
		EndCustomData: map[string]interface{}{
			"image": "tsuruteam/app-otherapp:mytag",
		},
	}, eventtest.HasEvent)
}

func (s *BuildSuite) TestBuildWithoutTag(c *check.C) {
	a := app.App{Name: "otherapp", Platform: "python", TeamOwner: s.team.Name}
	user, _ := s.token.User()
	err := app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/build", a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("archive-url=http://something.tar.gz"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "you must specify the image tag.\n")
}
