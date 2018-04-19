// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/router/routertest"
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
	team        authTypes.Team
	user        *auth.User
	provisioner *provisiontest.FakeProvisioner
	builder     *builder.MockBuilder
	defaultPlan appTypes.Plan
	Pool        string
	zeroLock    map[string]interface{}
	mockService servicemock.MockService
}

var _ = check.Suite(&S{})

type greaterChecker struct{}

func (c *greaterChecker) Info() *check.CheckerInfo {
	return &check.CheckerInfo{Name: "Greater", Params: []string{"expected", "obtained"}}
}

func (c *greaterChecker) Check(params []interface{}, names []string) (bool, string) {
	if len(params) != 2 {
		return false, "you should pass two values to compare"
	}
	n1, ok := params[0].(int)
	if !ok {
		return false, "first parameter should be int"
	}
	n2, ok := params[1].(int)
	if !ok {
		return false, "second parameter should be int"
	}
	if n1 > n2 {
		return true, ""
	}
	err := fmt.Sprintf("%d is not greater than %d", params[0], params[1])
	return false, err
}

var Greater check.Checker = &greaterChecker{}

func (s *S) createUserAndTeam(c *check.C) {
	s.user = &auth.User{
		Email: "whydidifall@thewho.com",
		Quota: authTypes.AuthQuota{Limit: -1},
	}
	err := s.user.Create()
	c.Assert(err, check.IsNil)
	s.team = authTypes.Team{Name: "tsuruteam"}
}

var nativeScheme = auth.Scheme(native.NativeScheme{})

func (s *S) SetUpSuite(c *check.C) {
	err := config.ReadConfigFile("testdata/config.yaml")
	c.Assert(err, check.IsNil)
	config.Set("log:disable-syslog", true)
	config.Set("queue:mongo-url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("queue:mongo-database", "queue_app_pkg_tests")
	config.Set("queue:mongo-polling-interval", 0.01)
	config.Set("docker:registry", "registry.somewhere")
	config.Set("routers:fake-tls:type", "fake-tls")
	config.Set("auth:hash-cost", bcrypt.MinCost)
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	s.logConn, err = db.LogConn()
	c.Assert(err, check.IsNil)
	s.provisioner = provisiontest.ProvisionerInstance
	provision.DefaultProvisioner = "fake"
	AuthScheme = nativeScheme
	data, err := json.Marshal(AppLock{})
	c.Assert(err, check.IsNil)
	err = json.Unmarshal(data, &s.zeroLock)
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	defer s.conn.Close()
	defer s.logConn.Close()
	s.conn.Apps().Database.DropDatabase()
	s.logConn.Logs("myapp").Database.DropDatabase()
}

func (s *S) SetUpTest(c *check.C) {
	// Reset fake routers twice, first time will remove registered failures and
	// allow pending enqueued tasks to run, second time (after queue is reset)
	// will remove any routes added by executed queue tasks.
	routertest.FakeRouter.Reset()
	routertest.HCRouter.Reset()
	routertest.TLSRouter.Reset()
	routertest.OptsRouter.Reset()
	queue.ResetQueue()
	routertest.FakeRouter.Reset()
	routertest.HCRouter.Reset()
	routertest.TLSRouter.Reset()
	routertest.OptsRouter.Reset()
	pool.ResetCache()
	err := rebuild.RegisterTask(func(appName string) (rebuild.RebuildApp, error) {
		a, err := GetByName(appName)
		if err == ErrAppNotFound {
			return nil, nil
		}
		return a, err
	})
	c.Assert(err, check.IsNil)
	config.Set("docker:router", "fake")
	s.provisioner.Reset()
	repositorytest.Reset()
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	s.createUserAndTeam(c)
	s.defaultPlan = appTypes.Plan{
		Name:     "default-plan",
		Memory:   1024,
		Swap:     1024,
		CpuShare: 100,
		Default:  true,
	}
	s.Pool = "pool1"
	opts := pool.AddPoolOptions{Name: s.Pool, Default: true}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	repository.Manager().CreateUser(s.user.Email)
	s.builder = &builder.MockBuilder{}
	builder.Register("fake", s.builder)
	builder.DefaultBuilder = "fake"
	setupMocks(s)
}

func (s *S) TearDownTest(c *check.C) {
	GetAppRouterUpdater().Shutdown(context.Background())
}

func setupMocks(s *S) {
	servicemock.SetMockService(&s.mockService)

	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		if name == s.team.Name {
			return &authTypes.Team{Name: s.team.Name}, nil
		}
		return nil, authTypes.ErrTeamNotFound
	}
	s.mockService.Team.OnFindByNames = func(names []string) ([]authTypes.Team, error) {
		if len(names) == 1 && names[0] == s.team.Name {
			return []authTypes.Team{{Name: s.team.Name}}, nil
		}
		return []authTypes.Team{}, nil
	}

	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{s.defaultPlan}, nil
	}
	s.mockService.Plan.OnDefaultPlan = func() (*appTypes.Plan, error) {
		return &s.defaultPlan, nil
	}

	s.mockService.Platform.OnFindByName = func(name string) (*appTypes.Platform, error) {
		if name == "python" || name == "heimerdinger" {
			return &appTypes.Platform{Name: name}, nil
		}
		return nil, appTypes.ErrPlatformNotFound
	}
}
