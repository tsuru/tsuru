// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
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

type BuildSuite struct {
	conn        *db.Storage
	token       auth.Token
	user        *auth.User
	team        *authTypes.Team
	provisioner *provisiontest.FakeProvisioner
	builder     *builder.MockBuilder
	testServer  http.Handler
	mockService servicemock.MockService
}

var _ = check.Suite(&BuildSuite{})

func (s *BuildSuite) createUserAndTeam(c *check.C) {
	user := &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	app.AuthScheme = nativeScheme
	_, err := nativeScheme.Create(context.TODO(), user)
	c.Assert(err, check.IsNil)
	s.team = &authTypes.Team{Name: "tsuruteam"}
	s.token = userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppReadDeploy,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermAppBuild,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
}

func (s *BuildSuite) reset() {
	s.provisioner.Reset()
	routertest.FakeRouter.Reset()
}

func (s *BuildSuite) SetUpSuite(c *check.C) {
	err := config.ReadConfigFile("testdata/config.yaml")
	c.Assert(err, check.IsNil)
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_deploy_api_tests")
	config.Set("auth:hash-cost", bcrypt.MinCost)
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	storagev2.Reset()
	s.testServer = RunServer(true)
}

func (s *BuildSuite) TearDownSuite(c *check.C) {
	config.Unset("docker:router")
	pool.RemovePool("pool1")
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	s.conn.Close()
	s.reset()
}

func (s *BuildSuite) SetUpTest(c *check.C) {
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
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	s.user, err = auth.ConvertNewUser(s.token.User())
	c.Assert(err, check.IsNil)
	config.Set("docker:router", "fake")
	servicemock.SetMockService(&s.mockService)
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	s.mockService.Team.OnFindByName = func(_ string) (*authTypes.Team, error) {
		return &authTypes.Team{Name: s.team.Name}, nil
	}

	defaultPlan := appTypes.Plan{
		Name:    "default-plan",
		Memory:  1024,
		Default: true,
	}

	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{defaultPlan}, nil
	}
	s.mockService.Plan.OnDefaultPlan = func() (*appTypes.Plan, error) {
		return &defaultPlan, nil
	}

	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		if name == defaultPlan.Name {
			return &defaultPlan, nil
		}
		return nil, appTypes.ErrPlanNotFound
	}
}

func (s *BuildSuite) TestBuildHandler(c *check.C) {
	s.builder.OnBuild = func(app provision.App, evt *event.Event, opts builder.BuildOpts) (appTypes.AppVersion, error) {
		c.Assert(opts.ArchiveFile, check.NotNil)
		c.Assert(opts.Tag, check.Equals, "mytag")
		version, err := servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
			App:            app,
			CustomBuildTag: opts.Tag,
		})
		c.Assert(err, check.IsNil)
		err = version.CommitBuildImage()
		c.Assert(err, check.IsNil)
		return version, nil
	}
	a := app.App{
		Name:      "otherapp",
		Platform:  "python",
		Router:    "fake",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)

	url := fmt.Sprintf("/apps/%s/build?tag=mytag", a.Name)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "archive.tar.gz")
	c.Assert(err, check.IsNil)
	file.Write([]byte("hello world!"))
	writer.Close()
	request, err := http.NewRequest(http.MethodPost, url, &body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "multipart/form-data; boundary="+writer.Boundary())
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "text")
	c.Assert(recorder.Body.String(), check.Equals, "tsuruteam/app-otherapp:mytag\nOK\n")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.build",
		StartCustomData: map[string]interface{}{
			"app.name":   a.Name,
			"commit":     "",
			"filesize":   12,
			"kind":       "uploadbuild",
			"archiveurl": "",
			"user":       s.token.GetUserName(),
			"image":      "",
			"origin":     "",
			"build":      true,
			"rollback":   false,
		},
		EndCustomData: map[string]interface{}{
			"image": "tsuruteam/app-otherapp:mytag",
		},
	}, eventtest.HasEvent)
}

func (s *BuildSuite) TestBuildArchiveURL(c *check.C) {
	s.builder.OnBuild = func(app provision.App, evt *event.Event, opts builder.BuildOpts) (appTypes.AppVersion, error) {
		c.Assert(opts.ArchiveURL, check.Equals, "http://something.tar.gz")
		version, err := servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
			App:            app,
			CustomBuildTag: opts.Tag,
		})
		c.Assert(err, check.IsNil)
		err = version.CommitBuildImage()
		c.Assert(err, check.IsNil)
		return version, nil
	}
	a := app.App{
		Name:      "otherapp",
		Platform:  "python",
		Router:    "fake",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/build", a.Name)
	request, err := http.NewRequest(http.MethodPost, url, strings.NewReader("tag=mytag&archive-url=http://something.tar.gz"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "text")
	c.Assert(recorder.Body.String(), check.Equals, "tsuruteam/app-otherapp:mytag\nOK\n")
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
			"build":      true,
			"rollback":   false,
		},
		EndCustomData: map[string]interface{}{
			"image": "tsuruteam/app-otherapp:mytag",
		},
	}, eventtest.HasEvent)
}

func (s *BuildSuite) TestBuildWithoutTag(c *check.C) {
	a := app.App{Name: "otherapp", Platform: "python", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/build", a.Name)
	request, err := http.NewRequest(http.MethodPost, url, strings.NewReader("archive-url=http://something.tar.gz"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "you must specify the image tag.\n")
}
