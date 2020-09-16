// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/servicemanager"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"golang.org/x/crypto/bcrypt"
	check "gopkg.in/check.v1"
)

type DeploySuite struct {
	conn        *db.Storage
	token       auth.Token
	user        *auth.User
	team        *authTypes.Team
	provisioner *provisiontest.FakeProvisioner
	builder     *builder.MockBuilder
	testServer  http.Handler
	mockService servicemock.MockService
}

var _ = check.Suite(&DeploySuite{})

func (s *DeploySuite) createUserAndTeam(c *check.C) {
	user := &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	app.AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	s.team = &authTypes.Team{Name: "tsuruteam"}
	s.token = userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppReadDeploy,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	s.user, err = auth.ConvertNewUser(s.token.User())
	c.Assert(err, check.IsNil)
}

func (s *DeploySuite) reset() {
	s.provisioner.Reset()
	routertest.FakeRouter.Reset()
	repositorytest.Reset()
}

func (s *DeploySuite) SetUpSuite(c *check.C) {
	err := config.ReadConfigFile("testdata/config.yaml")
	c.Assert(err, check.IsNil)
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_deploy_api_tests")
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("repo-manager", "fake")
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	s.testServer = RunServer(true)
}

func (s *DeploySuite) TearDownSuite(c *check.C) {
	config.Unset("docker:router")
	pool.RemovePool("pool1")
	s.conn.Apps().Database.DropDatabase()
	s.conn.Close()
	s.reset()
}

func (s *DeploySuite) SetUpTest(c *check.C) {
	s.provisioner = provisiontest.ProvisionerInstance
	provision.DefaultProvisioner = "fake"
	s.builder = &builder.MockBuilder{}
	builder.Register("fake", s.builder)
	builder.DefaultBuilder = "fake"
	s.reset()
	err := dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
	s.createUserAndTeam(c)
	opts := pool.AddPoolOptions{Name: "pool1", Default: true}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	user, err := s.token.User()
	c.Assert(err, check.IsNil)
	repository.Manager().CreateUser(user.Email)
	config.Set("docker:router", "fake")

	servicemock.SetMockService(&s.mockService)
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	defaultPlan := appTypes.Plan{
		Name:     "default-plan",
		Memory:   1024,
		Swap:     1024,
		CpuShare: 100,
		Default:  true,
	}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{defaultPlan}, nil
	}
	s.mockService.Plan.OnDefaultPlan = func() (*appTypes.Plan, error) {
		return &defaultPlan, nil
	}
}

func newAppVersion(c *check.C, app provision.App) appTypes.AppVersion {
	version, err := servicemanager.AppVersion.NewAppVersion(appTypes.NewVersionArgs{
		App: app,
	})
	c.Assert(err, check.IsNil)
	err = version.CommitBuildImage()
	c.Assert(err, check.IsNil)
	return version
}

func newSuccessfulAppVersion(c *check.C, app provision.App) appTypes.AppVersion {
	version := newAppVersion(c, app)
	err := version.CommitBaseImage()
	c.Assert(err, check.IsNil)
	err = version.CommitSuccessful()
	c.Assert(err, check.IsNil)
	return version
}

func (s *DeploySuite) TestDeployHandler(c *check.C) {
	var builderCalled bool
	s.builder.OnBuild = func(p provision.BuilderDeploy, app provision.App, evt *event.Event, opts *builder.BuildOpts) (appTypes.AppVersion, error) {
		builderCalled = true
		c.Assert(opts.ArchiveURL, check.Equals, "http://something.tar.gz")
		return newAppVersion(c, app), nil
	}
	a := app.App{Name: "otherapp", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/deploy", a.Name)
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
	c.Assert(recorder.Body.String(), check.Matches, ".*Builder deploy called\nOK\n")
	c.Assert(builderCalled, check.Equals, true)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.deploy",
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
			"image": "tsuru/app-" + a.Name + ":v1",
		},
		LogMatches: []string{`.*Builder deploy called`},
	}, eventtest.HasEvent)
}

func (s *DeploySuite) TestDeployOriginDragAndDrop(c *check.C) {
	s.builder.OnBuild = func(p provision.BuilderDeploy, app provision.App, evt *event.Event, opts *builder.BuildOpts) (appTypes.AppVersion, error) {
		c.Assert(opts.ArchiveFile, check.NotNil)
		return newAppVersion(c, app), nil
	}
	a := app.App{Name: "otherapp", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/deploy?origin=drag-and-drop", a.Name)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "archive.tar.gz")
	c.Assert(err, check.IsNil)
	file.Write([]byte("hello world!"))
	writer.Close()
	request, err := http.NewRequest("POST", url, &body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "multipart/form-data; boundary="+writer.Boundary())
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "text")
	c.Assert(recorder.Body.String(), check.Matches, ".*Builder deploy called\nOK\n")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.deploy",
		StartCustomData: map[string]interface{}{
			"app.name":   a.Name,
			"commit":     "",
			"filesize":   12,
			"kind":       "upload",
			"archiveurl": "",
			"user":       s.token.GetUserName(),
			"image":      "",
			"origin":     "drag-and-drop",
			"build":      false,
			"rollback":   false,
		},
		EndCustomData: map[string]interface{}{
			"image": "tsuru/app-" + a.Name + ":v1",
		},
		LogMatches: []string{`.*Builder deploy called`},
	}, eventtest.HasEvent)
}

func (s *DeploySuite) TestDeployInvalidOrigin(c *check.C) {
	a := app.App{Name: "otherapp", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/deploy?:appname=%s&origin=drag", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("archive-url=http://something.tar.gz&user=fulano"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Invalid deployment origin\n")
}

func (s *DeploySuite) TestDeployOriginImage(c *check.C) {
	s.builder.OnBuild = func(p provision.BuilderDeploy, app provision.App, evt *event.Event, opts *builder.BuildOpts) (appTypes.AppVersion, error) {
		return newAppVersion(c, app), nil
	}
	a := app.App{Name: "otherapp", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/deploy?origin=app-deploy", a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("image=127.0.0.1:5000/tsuru/otherapp"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Matches, ".*Builder deploy called\nOK\n")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.deploy",
		StartCustomData: map[string]interface{}{
			"app.name":   a.Name,
			"commit":     "",
			"filesize":   0,
			"kind":       "image",
			"archiveurl": "",
			"user":       s.token.GetUserName(),
			"image":      "127.0.0.1:5000/tsuru/otherapp",
			"origin":     "image",
			"build":      false,
			"rollback":   false,
		},
		EndCustomData: map[string]interface{}{
			"image": "tsuru/app-" + a.Name + ":v1",
		},
		LogMatches: []string{`.*Builder deploy called`},
	}, eventtest.HasEvent)
}

func (s *DeploySuite) TestDeployArchiveURL(c *check.C) {
	s.builder.OnBuild = func(p provision.BuilderDeploy, app provision.App, evt *event.Event, opts *builder.BuildOpts) (appTypes.AppVersion, error) {
		return newAppVersion(c, app), nil
	}
	a := app.App{
		Name:      "otherapp",
		Platform:  "python",
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/deploy?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("archive-url=http://something.tar.gz&user=fulano"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "text")
	c.Assert(recorder.Body.String(), check.Matches, ".*Builder deploy called\nOK\n")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.deploy",
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
			"image": "tsuru/app-" + a.Name + ":v1",
		},
		LogMatches: []string{`Builder deploy called`},
	}, eventtest.HasEvent)
}

func (s *DeploySuite) TestDeployUploadFile(c *check.C) {
	s.builder.OnBuild = func(p provision.BuilderDeploy, app provision.App, evt *event.Event, opts *builder.BuildOpts) (appTypes.AppVersion, error) {
		return newAppVersion(c, app), nil
	}
	a := app.App{
		Name:      "otherapp",
		Platform:  "python",
		Router:    "fake",
		TeamOwner: s.team.Name,
	}

	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/deploy", a.Name)
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
	c.Assert(recorder.Body.String(), check.Matches, ".*Builder deploy called\nOK\n")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.deploy",
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
			"image": "tsuru/app-" + a.Name + ":v1",
		},
		LogMatches: []string{`.*Builder deploy called`},
	}, eventtest.HasEvent)
}

func (s *DeploySuite) TestDeployUploadLargeFile(c *check.C) {
	s.builder.OnBuild = func(p provision.BuilderDeploy, app provision.App, evt *event.Event, opts *builder.BuildOpts) (appTypes.AppVersion, error) {
		return newAppVersion(c, app), nil
	}
	a := app.App{
		Name:      "otherapp",
		Platform:  "python",
		Router:    "fake",
		TeamOwner: s.team.Name,
	}

	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/repository/clone", a.Name)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "archive.tar.gz")
	c.Assert(err, check.IsNil)
	// Must be larger than 32MB to be stored in a tempfile.
	payload := bytes.Repeat([]byte("*"), 33<<20)
	file.Write(payload)
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
	c.Assert(recorder.Body.String(), check.Matches, ".*Builder deploy called\nOK\n")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.deploy",
		StartCustomData: map[string]interface{}{
			"app.name":   a.Name,
			"commit":     "",
			"filesize":   33 << 20,
			"kind":       "upload",
			"archiveurl": "",
			"user":       s.token.GetUserName(),
			"image":      "",
			"origin":     "",
			"build":      false,
			"rollback":   false,
		},
		EndCustomData: map[string]interface{}{
			"image": "tsuru/app-" + a.Name + ":v1",
		},
		LogMatches: []string{`.*Builder deploy called`},
	}, eventtest.HasEvent)
}

func (s *DeploySuite) TestDeployWithCommit(c *check.C) {
	s.builder.OnBuild = func(p provision.BuilderDeploy, app provision.App, evt *event.Event, opts *builder.BuildOpts) (appTypes.AppVersion, error) {
		return newAppVersion(c, app), nil
	}
	token, err := nativeScheme.AppLogin(app.InternalAppName)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "otherapp",
		Platform:  "python",
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/deploy", a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("archive-url=http://something.tar.gz&user=fulano&commit=123"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "text")
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Matches, ".*Builder deploy called\nOK\n")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  "fulano",
		Kind:   "app.deploy",
		StartCustomData: map[string]interface{}{
			"app.name":   a.Name,
			"commit":     "123",
			"filesize":   0,
			"kind":       "git",
			"archiveurl": "http://something.tar.gz",
			"user":       "fulano",
			"image":      "",
			"origin":     "git",
			"build":      false,
			"rollback":   false,
			"message":    "msg1",
		},
		EndCustomData: map[string]interface{}{
			"image": "tsuru/app-" + a.Name + ":v1",
		},
		LogMatches: []string{`.*Builder deploy called`},
	}, eventtest.HasEvent)
}

func (s *DeploySuite) TestDeployWithCommitUserToken(c *check.C) {
	s.builder.OnBuild = func(p provision.BuilderDeploy, app provision.App, evt *event.Event, opts *builder.BuildOpts) (appTypes.AppVersion, error) {
		return newAppVersion(c, app), nil
	}
	a := app.App{
		Name:      "otherapp",
		Platform:  "python",
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/deploy", a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("archive-url=http://something.tar.gz&user=fulano&commit=123"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "text")
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Matches, ".*Builder deploy called\nOK\n")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.deploy",
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
			"image": "tsuru/app-" + a.Name + ":v1",
		},
		LogMatches: []string{`.*Builder deploy called`},
	}, eventtest.HasEvent)
}

func (s *DeploySuite) TestDeployWithMessage(c *check.C) {
	s.builder.OnBuild = func(p provision.BuilderDeploy, app provision.App, evt *event.Event, opts *builder.BuildOpts) (appTypes.AppVersion, error) {
		return newAppVersion(c, app), nil
	}
	token, err := nativeScheme.AppLogin(app.InternalAppName)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "otherapp",
		Platform:  "python",
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/deploy", a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("archive-url=http://something.tar.gz&message=and when he falleth"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "text")
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Matches, ".*Builder deploy called\nOK\n")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  token.GetUserName(),
		Kind:   "app.deploy",
		StartCustomData: map[string]interface{}{
			"app.name":   a.Name,
			"commit":     "",
			"filesize":   0,
			"kind":       "archive-url",
			"archiveurl": "http://something.tar.gz",
			"user":       token.GetUserName(),
			"image":      "",
			"origin":     "",
			"build":      false,
			"rollback":   false,
			"message":    "and when he falleth",
		},
		EndCustomData: map[string]interface{}{
			"image": "tsuru/app-" + a.Name + ":v1",
		},
		LogMatches: []string{`.*Builder deploy called`},
	}, eventtest.HasEvent)
}

func (s *DeploySuite) TestDeployWithoutPlatformFails(c *check.C) {
	s.builder.OnBuild = func(p provision.BuilderDeploy, app provision.App, evt *event.Event, opts *builder.BuildOpts) (appTypes.AppVersion, error) {
		return newAppVersion(c, app), nil
	}
	token, err := nativeScheme.AppLogin(app.InternalAppName)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "otherapp",
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/deploy", a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("archive-url=http://something.tar.gz"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), check.Matches, "(?s).*can't deploy app without platform, if it's not an image or rollback.*")
}

func (s *DeploySuite) TestDeployDockerImage(c *check.C) {
	s.builder.OnBuild = func(p provision.BuilderDeploy, app provision.App, evt *event.Event, opts *builder.BuildOpts) (appTypes.AppVersion, error) {
		return newAppVersion(c, app), nil
	}
	a := app.App{Name: "myapp", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/deploy", a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("image=127.0.0.1:5000/tsuru/otherapp"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Matches, ".*Builder deploy called\nOK\n")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.deploy",
		StartCustomData: map[string]interface{}{
			"app.name":   a.Name,
			"commit":     "",
			"filesize":   0,
			"kind":       "image",
			"archiveurl": "",
			"user":       s.token.GetUserName(),
			"image":      "127.0.0.1:5000/tsuru/otherapp",
			"origin":     "image",
			"build":      false,
			"rollback":   false,
		},
		EndCustomData: map[string]interface{}{
			"image": "tsuru/app-" + a.Name + ":v1",
		},
		LogMatches: []string{`.*Builder deploy called`},
	}, eventtest.HasEvent)
}

func (s *DeploySuite) TestDeployShouldIncrementDeployNumberOnApp(c *check.C) {
	s.builder.OnBuild = func(p provision.BuilderDeploy, app provision.App, evt *event.Event, opts *builder.BuildOpts) (appTypes.AppVersion, error) {
		return newAppVersion(c, app), nil
	}
	a := app.App{Name: "otherapp", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/deploy?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("archive-url=http://something.tar.gz"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(a.Deploys, check.Equals, uint(1))
}

func (s *DeploySuite) TestDeployShouldReturnNotFoundWhenAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("POST", "/apps/abc/deploy", strings.NewReader("archive-url=http://something.tar.gz"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	message := recorder.Body.String()
	c.Assert(message, check.Equals, "App not found\n")
}

func (s *DeploySuite) TestDeployShouldReturnForbiddenWhenUserDoesNotHaveAccessToApp(c *check.C) {
	user := &auth.User{Email: "someone@tsuru.io", Password: "123456"}
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	a := app.App{Name: "otherapp", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/deploy?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("archive-url=http://something.tar.gz&user=fulano"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "User does not have permission to do this action in this app\n")
}

func (s *DeploySuite) TestDeployShouldReturnForbiddenWhenTokenIsntFromTheApp(c *check.C) {
	app1 := app.App{Name: "otherapp", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := app.App{Name: "superapp", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &app2, s.user)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.AppLogin(app2.Name)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/deploy?:appname=%s", app1.Name, app2.Name)
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
	s.builder.OnBuild = func(p provision.BuilderDeploy, app provision.App, evt *event.Event, opts *builder.BuildOpts) (appTypes.AppVersion, error) {
		return newAppVersion(c, app), nil
	}
	token, err := nativeScheme.AppLogin(app.InternalAppName)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "otherapp",
		Platform:  "python",
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/deploy?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("archive-url=http://something.tar.gz&user=fulano"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "text")
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Matches, ".*Builder deploy called\nOK\n")
}

func (s *DeploySuite) TestDeployWithoutArchiveURL(c *check.C) {
	a := app.App{Name: "abc", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/apps/abc/deploy", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	message := recorder.Body.String()
	c.Assert(message, check.Equals, "you must specify either the archive-url, a image url or upload a file.\n")
}

func (s *DeploySuite) TestPermSchemeForDeploy(c *check.C) {
	var tests = []struct {
		input    app.DeployOptions
		expected *permission.PermissionScheme
	}{
		{
			app.DeployOptions{Commit: "abc123"},
			permission.PermAppDeployGit,
		},
		{
			app.DeployOptions{Image: "quay.io/tsuru/python"},
			permission.PermAppDeployImage,
		},
		{
			app.DeployOptions{File: ioutil.NopCloser(bytes.NewReader(nil))},
			permission.PermAppDeployUpload,
		},
		{
			app.DeployOptions{File: ioutil.NopCloser(bytes.NewReader(nil)), Build: true},
			permission.PermAppDeployBuild,
		},
		{
			app.DeployOptions{},
			permission.PermAppDeployArchiveUrl,
		},
	}
	for _, t := range tests {
		c.Check(permSchemeForDeploy(t.input), check.Equals, t.expected)
	}
}

func insertDeploysAsEvents(data []app.DeployData, c *check.C) []*event.Event {
	evts := make([]*event.Event, len(data))
	for i, d := range data {
		evt, err := event.New(&event.Opts{
			Target:   event.Target{Type: "app", Value: d.App},
			Kind:     permission.PermAppDeploy,
			RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: d.User},
			CustomData: app.DeployOptions{
				Commit: d.Commit,
				Origin: d.Origin,
			},
			Allowed: event.Allowed(permission.PermAppReadEvents, permission.Context(permTypes.CtxApp, d.App)),
		})
		c.Assert(err, check.IsNil)
		evt.StartTime = d.Timestamp
		evt.Logf(d.Log)
		err = evt.SetOtherCustomData(map[string]string{"diff": d.Diff})
		c.Assert(err, check.IsNil)
		err = evt.DoneCustomData(nil, map[string]string{"image": d.Image})
		c.Assert(err, check.IsNil)
		evts[i] = evt
	}
	return evts
}

func (s *DeploySuite) TestDeployListNonAdmin(c *check.C) {
	user := &auth.User{Email: "nonadmin@nonadmin.com", Password: "123456"}
	app.AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	team := authTypes.Team{Name: "newteam"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: team.Name}}, nil
	}
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "apponlyg1", permission.Permission{
		Scheme:  permission.PermAppReadDeploy,
		Context: permission.Context(permTypes.CtxApp, "g1"),
	})
	a := app.App{Name: "g1", Platform: "python", TeamOwner: team.Name}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	var result []app.DeployData
	request, err := http.NewRequest("GET", "/deploys", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	timestamp := time.Date(2013, time.November, 1, 0, 0, 0, 0, time.Local)
	insertDeploysAsEvents([]app.DeployData{
		{App: "g1", Timestamp: timestamp.Add(time.Minute)},
		{App: "ge", Timestamp: timestamp.Add(time.Second)},
	}, c)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 1)
	c.Assert(result[0].ID, check.NotNil)
	c.Assert(result[0].App, check.Equals, "g1")
	c.Assert(result[0].Timestamp.In(time.UTC), check.DeepEquals, timestamp.Add(time.Minute).In(time.UTC))
}

func (s *DeploySuite) TestDeployList(c *check.C) {
	app1 := app.App{Name: "g1", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := app.App{Name: "ge", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &app2, s.user)
	c.Assert(err, check.IsNil)
	var result []app.DeployData
	request, err := http.NewRequest("GET", "/deploys", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	timestamp := time.Date(2013, time.November, 1, 0, 0, 0, 0, time.Local)
	deps := []app.DeployData{
		{App: "g1", Timestamp: timestamp.Add(time.Minute)},
		{App: "ge", Timestamp: timestamp.Add(time.Second)},
	}
	insertDeploysAsEvents(deps, c)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 2)
	c.Assert(result[0].ID, check.NotNil)
	c.Assert(result[0].App, check.Equals, "g1")
	c.Assert(result[0].Timestamp.In(time.UTC), check.DeepEquals, timestamp.Add(time.Minute).In(time.UTC))
	c.Assert(result[1].App, check.Equals, "ge")
	c.Assert(result[1].Timestamp.In(time.UTC), check.DeepEquals, timestamp.Add(time.Second).In(time.UTC))
}

func (s *DeploySuite) TestDeployListByApp(c *check.C) {
	a := app.App{Name: "myblog", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	timestamp := time.Date(2013, time.November, 1, 0, 0, 0, 0, time.Local)
	deploys := []app.DeployData{
		{App: "myblog", Timestamp: timestamp},
		{App: "yourblog", Timestamp: timestamp},
	}
	insertDeploysAsEvents(deploys, c)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/deploys?app=myblog", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var result []app.DeployData
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 1)
	c.Assert(result[0].App, check.Equals, "myblog")
	c.Assert(result[0].Timestamp.In(time.UTC), check.DeepEquals, timestamp.In(time.UTC))
}

func (s *DeploySuite) TestDeployListByAppWithImage(c *check.C) {
	a := app.App{Name: "myblog", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	timestamp := time.Date(2013, time.November, 1, 0, 0, 0, 0, time.Local)
	deploys := []app.DeployData{
		{App: "myblog", Timestamp: timestamp, Image: "registry.tsuru.globoi.com/tsuru/app-example:v2", CanRollback: true},
		{App: "yourblog", Timestamp: timestamp, Image: "127.0.0.1:5000/tsuru/app-tsuru-dashboard:v1", CanRollback: true},
	}
	insertDeploysAsEvents(deploys, c)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/deploys?app=myblog", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var result []app.DeployData
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 1)
	c.Assert(result[0].Image, check.Equals, "v2")
	c.Assert(result[0].App, check.Equals, "myblog")
	c.Assert(result[0].Timestamp.In(time.UTC), check.DeepEquals, timestamp.In(time.UTC))
}

func (s *DeploySuite) TestDeployListAppWithNoDeploys(c *check.C) {
	a := app.App{Name: "myblog", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/deploys?app=myblog", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *DeploySuite) TestDeployInfoByAdminUser(c *check.C) {
	a := app.App{Name: "g1", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	timestamp := time.Now()
	depData := []app.DeployData{
		{App: "g1", Timestamp: timestamp.Add(-3600 * time.Second), Commit: "e293e3e3me03ejm3puejmp3ej3iejop32", Error: ""},
		{App: "g1", Timestamp: timestamp, Commit: "e82nn93nd93mm12o2ueh83dhbd3iu112", Error: ""},
	}
	lastDeploy := depData[1]
	lastDeploy.Origin = "git"
	evts := insertDeploysAsEvents(depData, c)
	url := fmt.Sprintf("/deploys/%s", evts[1].UniqueID.Hex())
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myadmin", permission.Permission{
		Scheme:  permission.PermAppReadDeploy,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var result app.DeployData
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	lastDeploy.ID = evts[1].UniqueID
	result.Timestamp = lastDeploy.Timestamp
	result.RemoveDate = lastDeploy.RemoveDate
	result.Duration = 0
	result.Log = ""
	c.Assert(result, check.DeepEquals, lastDeploy)
}

func (s *DeploySuite) TestDeployInfoDiff(c *check.C) {
	a := app.App{Name: "g1", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	timestamp := time.Now()
	depData := []app.DeployData{
		{App: "g1", Timestamp: timestamp.Add(-3600 * time.Second), Commit: "e293e3e3me03ejm3puejmp3ej3iejop32", Error: "", Origin: "git"},
		{App: "g1", Timestamp: timestamp, Commit: "e82nn93nd93mm12o2ueh83dhbd3iu112", Error: "", Origin: "git", Diff: "fake-diff"},
	}
	lastDeploy := depData[1]
	evts := insertDeploysAsEvents(depData, c)
	url := fmt.Sprintf("/deploys/%s", evts[1].UniqueID.Hex())
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	lastDeploy.ID = evts[1].UniqueID
	var result app.DeployData
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	result.Timestamp = lastDeploy.Timestamp
	result.RemoveDate = lastDeploy.RemoveDate
	result.Duration = 0
	result.Log = ""
	c.Assert(result, check.DeepEquals, lastDeploy)
}

func (s *DeploySuite) TestDeployInfoByNonAdminUser(c *check.C) {
	a := app.App{Name: "g1", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	app.AuthScheme = nativeScheme
	_, err = nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	timestamp := time.Now()
	depData := []app.DeployData{
		{App: "g1", Timestamp: timestamp.Add(-3600 * time.Second), Commit: "e293e3e3me03ejm3puejmp3ej3iejop32", Error: "", Origin: "git"},
		{App: "g1", Timestamp: timestamp, Commit: "e82nn93nd93mm12o2ueh83dhbd3iu112", Error: "", Origin: "git", Diff: "fake-diff"},
	}
	evts := insertDeploysAsEvents(depData, c)
	url := fmt.Sprintf("/deploys/%s", evts[1].UniqueID.Hex())
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
	url := "/deploys/xpto"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusUnauthorized)
}

func (s *DeploySuite) TestDeployInfoByUserWithoutAccess(c *check.C) {
	user := &auth.User{Email: "user@user.com", Password: "123456"}
	app.AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	team := authTypes.Team{Name: "team"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: team.Name}}, nil
	}
	a := app.App{Name: "g1", Platform: "python", TeamOwner: team.Name}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	timestamp := time.Now()
	depData := []app.DeployData{
		{App: "g1", Timestamp: timestamp.Add(-3600 * time.Second), Commit: "e293e3e3me03ejm3puejmp3ej3iejop32", Error: "", Origin: "git"},
		{App: "g1", Timestamp: timestamp, Commit: "e82nn93nd93mm12o2ueh83dhbd3iu112", Error: "", Origin: "git", Diff: "fake-diff"},
	}
	evts := insertDeploysAsEvents(depData, c)
	url := fmt.Sprintf("/deploys/%s", evts[1].UniqueID.Hex())
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
	a := app.App{Name: "otherapp", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	version := newSuccessfulAppVersion(c, &a)
	v := url.Values{}
	v.Set("origin", "rollback")
	v.Set("image", version.BaseImageName())
	u := fmt.Sprintf("/apps/%s/deploy/rollback", a.Name)
	request, err := http.NewRequest("POST", u, strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Matches, "{\"Message\":\".*Builder deploy called\",\"Timestamp\":\".*\"}\n")
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.deploy",
		StartCustomData: map[string]interface{}{
			"app.name":   a.Name,
			"commit":     "",
			"filesize":   0,
			"kind":       "rollback",
			"archiveurl": "",
			"user":       s.token.GetUserName(),
			"image":      version.BaseImageName(),
			"origin":     "rollback",
			"build":      false,
			"rollback":   true,
		},
		EndCustomData: map[string]interface{}{
			"image": version.BaseImageName(),
		},
	}, eventtest.HasEvent)
}

func (s *DeploySuite) TestDeployRollbackHandlerWithOnlyVersionImage(c *check.C) {
	a := app.App{Name: "otherapp", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	version := newSuccessfulAppVersion(c, &a)
	v := url.Values{}
	v.Set("origin", "rollback")
	v.Set("image", fmt.Sprintf("v%d", version.Version()))
	u := fmt.Sprintf("/apps/%s/deploy/rollback", a.Name)
	request, err := http.NewRequest("POST", u, strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Matches, "{\"Message\":\".*Builder deploy called\",\"Timestamp\":\".*\"}\n")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.deploy",
		StartCustomData: map[string]interface{}{
			"app.name":   a.Name,
			"commit":     "",
			"filesize":   0,
			"kind":       "rollback",
			"archiveurl": "",
			"user":       s.token.GetUserName(),
			"image":      "v1",
			"origin":     "rollback",
			"build":      false,
			"rollback":   true,
		},
		EndCustomData: map[string]interface{}{
			"image": "tsuru/app-otherapp:v1",
		},
		LogMatches: []string{`Builder deploy called`},
	}, eventtest.HasEvent)
}

func (s *DeploySuite) TestDeployRollbackHandlerWithInexistVersion(c *check.C) {
	a := app.App{
		Name:      "otherapp",
		Platform:  "python",
		TeamOwner: s.team.Name,
		Teams:     []string{s.team.Name},
		Router:    "fake",
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	v := url.Values{}
	v.Set("origin", "rollback")
	v.Set("image", "v9")
	u := fmt.Sprintf("/apps/%s/deploy/rollback", a.Name)
	request, err := http.NewRequest("POST", u, strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), check.Matches, `(?s).*Invalid version: v9.*`)
}

func (s *DeploySuite) TestDiffDeploy(c *check.C) {
	diff := `--- hello.go	2015-11-25 16:04:22.409241045 +0000
+++ hello.go	2015-11-18 18:40:21.385697080 +0000
@@ -1,10 +1,7 @@
 package main

-import (
-    "fmt"
-)
+import "fmt"

-func main() {
-	fmt.Println("Hello")
+func main2() {
+	fmt.Println("Hello World!")
 }
`
	a := app.App{Name: "otherapp", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("customdata", diff)
	body := strings.NewReader(v.Encode())
	url := fmt.Sprintf("/apps/%s/diff", a.Name)
	request, err := http.NewRequest("POST", url, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	evt, err := event.New(&event.Opts{
		Target:  appTarget(a.Name),
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "Saving the difference between the old and new code\n")
	err = evt.Done(nil)
	c.Assert(err, check.IsNil)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.deploy",
	}, eventtest.HasEvent)
}

func (s *DeploySuite) TestDiffDeployWhenUserDoesNotHaveAccessToApp(c *check.C) {
	diff := `--- hello.go	2015-11-25 16:04:22.409241045 +0000
+++ hello.go	2015-11-18 18:40:21.385697080 +0000
@@ -1,10 +1,7 @@
 package main

-import (
-    "fmt"
-)
+import "fmt"

-func main() {
-	fmt.Println("Hello")
+func main2() {
+	fmt.Println("Hello World!")
 }
	`

	user1 := &auth.User{Email: "someone@tsuru.io", Password: "user123"}
	_, err := nativeScheme.Create(user1)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(map[string]string{"email": user1.Email, "password": "user123"})
	c.Assert(err, check.IsNil)
	a := app.App{Name: "otherapp", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("customdata", diff)
	body := strings.NewReader(v.Encode())
	url := fmt.Sprintf("/apps/%s/diff?:appname=%s", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	expected := `Saving the difference between the old and new code
`
	c.Assert(recorder.Body.String(), check.Equals, expected+permission.ErrUnauthorized.Error()+"\n")
}

func (s *DeploySuite) TestDeployRebuildHandler(c *check.C) {
	s.builder.OnBuild = func(p provision.BuilderDeploy, app provision.App, evt *event.Event, opts *builder.BuildOpts) (appTypes.AppVersion, error) {
		c.Assert(opts.Rebuild, check.Equals, true)
		return newAppVersion(c, app), nil
	}
	a := app.App{Name: "otherapp", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("origin", "rebuild")
	u := fmt.Sprintf("/apps/%s/deploy/rebuild", a.Name)
	request, err := http.NewRequest("POST", u, strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Matches, "{\"Message\":\".*Builder deploy called\",\"Timestamp\":\".*\"}\n")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.deploy",
		StartCustomData: map[string]interface{}{
			"app.name":   a.Name,
			"commit":     "",
			"filesize":   0,
			"kind":       "rebuild",
			"archiveurl": "",
			"user":       s.token.GetUserName(),
			"image":      "",
			"origin":     "rebuild",
			"build":      false,
			"rollback":   false,
		},
		EndCustomData: map[string]interface{}{
			"image": "tsuru/app-" + a.Name + ":v1",
		},
	}, eventtest.HasEvent)
}

func (s *DeploySuite) TestRollbackUpdate(c *check.C) {
	fakeApp := app.App{Name: "otherapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &fakeApp, s.user)
	c.Assert(err, check.IsNil)
	version := newSuccessfulAppVersion(c, &fakeApp)
	v := url.Values{}
	v.Set("disable", "true")
	v.Set("reason", "because of reasons")
	v.Set("image", fmt.Sprintf("v%d", version.Version()))
	url := fmt.Sprintf("/apps/%s/deploy/rollback/update", fakeApp.Name)
	request, err := http.NewRequest(http.MethodPut, url, strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myadmin", permission.Permission{
		Scheme:  permission.PermAppUpdateDeployRollback,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	versions, err := servicemanager.AppVersion.AppVersions(&fakeApp)
	c.Assert(err, check.IsNil)
	disabledVersion := versions.Versions[version.Version()]
	c.Assert(disabledVersion.Disabled, check.Equals, true)
	c.Assert(disabledVersion.DisabledReason, check.Equals, "because of reasons")
}

func (s *DeploySuite) TestRollbackUpdateInvalidImage(c *check.C) {
	fakeApp := app.App{Name: "otherapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &fakeApp, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &fakeApp)
	v := url.Values{}
	v.Set("disable", "false")
	v.Set("reason", "")
	v.Set("image", "v10")
	url := fmt.Sprintf("/apps/%s/deploy/rollback/update", fakeApp.Name)
	request, err := http.NewRequest(http.MethodPut, url, strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myadmin", permission.Permission{
		Scheme:  permission.PermAppUpdateDeployRollback,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Equals, "Invalid version: v10\n")
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *DeploySuite) TestRollbackUpdateImageNotFound(c *check.C) {
	fakeApp := app.App{Name: "otherapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &fakeApp, s.user)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("disable", "false")
	v.Set("reason", "")
	v.Set("image", "v1")
	url := fmt.Sprintf("/apps/%s/deploy/rollback/update", fakeApp.Name)
	request, err := http.NewRequest(http.MethodPut, url, strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myadmin", permission.Permission{
		Scheme:  permission.PermAppUpdateDeployRollback,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Equals, "no versions available for app\n")
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *DeploySuite) TestRollbackUpdateEmptyImage(c *check.C) {
	fakeApp := app.App{Name: "rimworld", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &fakeApp, s.user)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("disable", "false")
	url := fmt.Sprintf("/apps/%s/deploy/rollback/update", fakeApp.Name)
	request, err := http.NewRequest(http.MethodPut, url, strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myadmin", permission.Permission{
		Scheme:  permission.PermAppUpdateDeployRollback,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Equals, "you must specify an image\n")
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *DeploySuite) TestRollbackUpdateErrEmptyReason(c *check.C) {
	fakeApp := app.App{Name: "xayah", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &fakeApp, s.user)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("disable", "true")
	v.Set("reason", "")
	v.Set("image", "v1")
	url := fmt.Sprintf("/apps/%s/deploy/rollback/update", fakeApp.Name)
	request, err := http.NewRequest(http.MethodPut, url, strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myadmin", permission.Permission{
		Scheme:  permission.PermAppUpdateDeployRollback,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Equals, "Reason cannot be empty while disabling a image rollback\n")
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *DeploySuite) TestRollbackUpdateErrNoPerms(c *check.C) {
	user := &auth.User{Email: "janna@zaun.com", Password: "jannazaun123"}
	err := user.Create()
	c.Assert(err, check.IsNil)
	fakeApp := app.App{Name: "xayah", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &fakeApp, user)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("disable", "false")
	v.Set("reason", "Zaun is under attack!")
	v.Set("image", "v1")
	url := fmt.Sprintf("/apps/%s/deploy/rollback/update", fakeApp.Name)
	request, err := http.NewRequest(http.MethodPut, url, strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myadmin")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Equals, "User does not have permission to do this action in this app\n")
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}
