// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	stdcontext "context"
	"net/http"
	"os"
	"testing"

	"github.com/ajg/form"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/version"
	"github.com/tsuru/tsuru/applog"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/job"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/servicemanager"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/types/quota"
	"golang.org/x/crypto/bcrypt"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	conn        *db.Storage
	team        *authTypes.Team
	user        *auth.User
	token       auth.Token
	plan        appTypes.Plan
	defaultPlan appTypes.Plan
	provisioner *provisiontest.FakeProvisioner
	Pool        string
	testServer  http.Handler
	mockService servicemock.MockService
}

var (
	_ = check.Suite(&S{})

	testCert, testKey string
)

type hasAccessToChecker struct{}

func (c *hasAccessToChecker) Info() *check.CheckerInfo {
	return &check.CheckerInfo{Name: "HasAccessTo", Params: []string{"team", "service"}}
}

func (c *hasAccessToChecker) Check(params []interface{}, names []string) (bool, string) {
	if len(params) != 2 {
		return false, "you must provide two parameters"
	}
	team, ok := params[0].(authTypes.Team)
	if !ok {
		return false, "first parameter should be a team instance"
	}
	srv, ok := params[1].(service.Service)
	if !ok {
		return false, "second parameter should be service instance"
	}
	return srv.HasTeam(&team), ""
}

var HasAccessTo check.Checker = &hasAccessToChecker{}

func (s *S) createUserAndTeam(c *check.C) {
	// TODO: remove this token from the suite, each test should create their
	// own user with specific permissions.
	_, s.token = permissiontest.CustomUserWithPermission(c, nativeScheme, "super-root-toremove", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	var err error
	s.user, err = auth.ConvertNewUser(s.token.User())
	c.Assert(err, check.IsNil)
	s.team = &authTypes.Team{Name: "tsuruteam"}
}

var nativeScheme = native.NativeScheme{}

func (s *S) SetUpSuite(c *check.C) {
	form.DefaultEncoder = form.DefaultEncoder.UseJSONTags(false)
	app.TestLogWriterWaitOnClose = true
	s.testServer = RunServer(true)
	testCertData, err := os.ReadFile("./testdata/cert.pem")
	c.Assert(err, check.IsNil)
	testKeyData, err := os.ReadFile("./testdata/key.pem")
	c.Assert(err, check.IsNil)
	testCert = string(testCertData)
	testKey = string(testKeyData)
}

func (s *S) SetUpTest(c *check.C) {
	resetConfig(c)
	config.Set("routers:fake:default", true)
	config.Set("routers:fake-tls:type", "fake-tls")
	routertest.FakeRouter.Reset()
	routertest.TLSRouter.Reset()
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	s.createUserAndTeam(c)
	s.provisioner = provisiontest.ProvisionerInstance
	s.provisioner.Reset()
	pool.ResetCache()
	provision.DefaultProvisioner = "fake"
	app.AuthScheme = nativeScheme
	s.Pool = "test1"
	opts := pool.AddPoolOptions{Name: "test1", Default: true}
	err = pool.AddPool(stdcontext.TODO(), opts)
	c.Assert(err, check.IsNil)
	s.setupMocks()
	servicemanager.App, err = app.AppService()
	c.Assert(err, check.IsNil)
	servicemanager.LogService, err = applog.AppLogService()
	c.Assert(err, check.IsNil)
	servicemanager.AppVersion, err = version.AppVersionService()
	c.Assert(err, check.IsNil)
	servicemanager.AuthGroup, err = auth.GroupService()
	c.Assert(err, check.IsNil)
	servicemanager.Job, err = job.JobService()
	c.Assert(err, check.IsNil)
}

func (s *S) setupMocks() {
	servicemock.SetMockService(&s.mockService)
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	s.mockService.Team.OnFindByName = func(_ string) (*authTypes.Team, error) {
		return &authTypes.Team{Name: s.team.Name}, nil
	}
	s.mockService.Team.OnFindByNames = func(_ []string) ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	s.defaultPlan = appTypes.Plan{
		Name:    "default-plan",
		Memory:  1024,
		Default: true,
	}
	s.plan = appTypes.Plan{}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		if s.plan.Name != "" {
			return []appTypes.Plan{s.defaultPlan, s.plan}, nil
		}
		return []appTypes.Plan{s.defaultPlan}, nil
	}
	s.mockService.Plan.OnDefaultPlan = func() (*appTypes.Plan, error) {
		return &s.defaultPlan, nil
	}
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		if name == s.defaultPlan.Name {
			return &s.defaultPlan, nil
		}
		if s.plan.Name == name {
			return &s.plan, nil
		}
		return nil, appTypes.ErrPlanNotFound
	}
	s.mockService.UserQuota.OnInc = func(item quota.QuotaItem, q int) error {
		return nil
	}
	s.mockService.UserQuota.OnGet = func(item quota.QuotaItem) (*quota.Quota, error) {
		return &s.user.Quota, nil
	}
	s.mockService.AppQuota.OnGet = func(item quota.QuotaItem) (*quota.Quota, error) {
		return &quota.UnlimitedQuota, nil
	}
	s.mockService.Pool.OnServices = func(pool string) ([]string, error) {
		return []string{"varus", "mysql", "mysql2"}, nil
	}
}

func (s *S) TearDownTest(c *check.C) {
	app.GetAppRouterUpdater().Shutdown(stdcontext.Background())
	s.provisioner.Reset()
	s.conn.Close()
	config.Unset("listen")
	config.Unset("tls:listen")
}

func (s *S) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
}

func userWithPermission(c *check.C, perm ...permission.Permission) auth.Token {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "majortom", perm...)
	return token
}

func resetHandlers() {
	tsuruHandlerList = []TsuruHandler{}
}

func resetConfig(c *check.C) {
	err := config.ReadConfigFile("testdata/config.yaml")
	c.Assert(err, check.IsNil)
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_api_base_test")
	config.Set("auth:hash-cost", bcrypt.MinCost)
}
