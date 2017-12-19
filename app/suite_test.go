// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"encoding/json"
	"fmt"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/builder/fake"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/router/routertest"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	"github.com/tsuru/tsuru/types"
	appTypes "github.com/tsuru/tsuru/types/app"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	conn        *db.Storage
	logConn     *db.LogStorage
	team        types.Team
	user        *auth.User
	provisioner *provisiontest.FakeProvisioner
	defaultPlan appTypes.Plan
	Pool        string
	zeroLock    map[string]interface{}
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
		Quota: quota.Unlimited,
	}
	err := s.user.Create()
	c.Assert(err, check.IsNil)
	s.team = types.Team{Name: "tsuruteam"}
	err = auth.TeamService().Insert(s.team)
	c.Assert(err, check.IsNil)
}

var nativeScheme = auth.Scheme(native.NativeScheme{})

func (s *S) SetUpSuite(c *check.C) {
	err := config.ReadConfigFile("testdata/config.yaml")
	c.Assert(err, check.IsNil)
	config.Set("log:disable-syslog", true)
	config.Set("queue:mongo-url", "127.0.0.1:27017")
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
	builder.DefaultBuilder = "fake"
	builder.Register("fake", fake.NewFakeBuilder())
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
	platform := appTypes.Platform{Name: "python"}
	PlatformService().Insert(platform)
	PlatformService().Insert(appTypes.Platform{Name: "heimerdinger"})
	s.defaultPlan = appTypes.Plan{
		Name:     "default-plan",
		Memory:   1024,
		Swap:     1024,
		CpuShare: 100,
		Default:  true,
	}
	err = PlanService().Insert(s.defaultPlan)
	c.Assert(err, check.IsNil)
	s.Pool = "pool1"
	opts := pool.AddPoolOptions{Name: s.Pool, Default: true}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	repository.Manager().CreateUser(s.user.Email)
}
