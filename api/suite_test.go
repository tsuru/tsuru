// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	stdcontext "context"
	"math/rand"
	"net/http"
	"testing"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/autoscale"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/healer"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/service"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	conn        *db.Storage
	logConn     *db.LogStorage
	team        *authTypes.Team
	user        *auth.User
	token       auth.Token
	provisioner *provisiontest.FakeProvisioner
	Pool        string
	testServer  http.Handler
	mockService servicemock.MockService
}

var _ = check.Suite(&S{})

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
		Context: permission.Context(permission.CtxGlobal, ""),
	})
	var err error
	s.user, err = auth.ConvertNewUser(s.token.User())
	c.Assert(err, check.IsNil)
	s.team = &authTypes.Team{Name: "tsuruteam"}
}

var nativeScheme = auth.ManagedScheme(native.NativeScheme{})

func (s *S) SetUpSuite(c *check.C) {
	rand.Seed(time.Now().UnixNano())
	err := config.ReadConfigFile("testdata/config.yaml")
	c.Assert(err, check.IsNil)
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_api_base_test")
	config.Set("auth:hash-cost", bcrypt.MinCost)
	s.testServer = RunServer(true)
}

func (s *S) SetUpTest(c *check.C) {
	config.Set("routers:fake:default", true)
	config.Set("routers:fake-tls:type", "fake-tls")
	routertest.FakeRouter.Reset()
	routertest.TLSRouter.Reset()
	repositorytest.Reset()
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	s.logConn, err = db.LogConn()
	c.Assert(err, check.IsNil)
	dbtest.ClearAllCollections(s.logConn.Logs("myapp").Database)
	s.createUserAndTeam(c)
	s.provisioner = provisiontest.ProvisionerInstance
	s.provisioner.Reset()
	provision.DefaultProvisioner = "fake"
	app.AuthScheme = nativeScheme
	s.Pool = "test1"
	opts := pool.AddPoolOptions{Name: "test1", Default: true}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	repository.Manager().CreateUser(s.user.Email)
	s.setupMocks()
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
	s.mockService.UserQuota.OnReserveApp = func(email string) error {
		return nil
	}
	s.mockService.UserQuota.OnReleaseApp = func(email string) error {
		return nil
	}
	s.mockService.UserQuota.OnFindByUserEmail = func(email string) (*authTypes.Quota, error) {
		return &s.user.Quota, nil
	}
}

func (s *S) TearDownTest(c *check.C) {
	app.GetAppRouterUpdater().Shutdown(stdcontext.Background())
	cfg, _ := autoscale.CurrentConfig()
	if cfg != nil {
		cfg.Shutdown(stdcontext.Background())
	}
	s.provisioner.Reset()
	s.conn.Close()
	s.logConn.Close()
	healer.HealerInstance = nil
	queue.ResetQueue()
	config.Unset("listen")
	config.Unset("tls:listen")
}

func (s *S) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
	logConn, err := db.LogConn()
	c.Assert(err, check.IsNil)
	defer logConn.Close()
	logConn.Logs("myapp").Database.DropDatabase()
}

func userWithPermission(c *check.C, perm ...permission.Permission) auth.Token {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "majortom", perm...)
	return token
}

func resetHandlers() {
	tsuruHandlerList = []TsuruHandler{}
}
