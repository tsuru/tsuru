// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/servicemanager"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/types/quota"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

var _ = check.Suite(&S{})

type S struct {
	conn        *db.Storage
	team        authTypes.Team
	user        *authTypes.User
	plan        *appTypes.Plan
	plans       []appTypes.Plan
	defaultPlan *appTypes.Plan
	provisioner *provisiontest.JobProvisioner
	Pool        string
	zeroLock    map[string]interface{}
	mockService servicemock.MockService
}

func (s *S) createUserAndTeam(c *check.C) {
	user := &auth.User{
		Email: "august@tswift.com",
		Quota: quota.UnlimitedQuota,
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	s.user = &authTypes.User{
		Email: user.Email,
		Quota: quota.UnlimitedQuota,
	}
	s.team = authTypes.Team{
		Name:  "tsuruteam",
		Quota: quota.UnlimitedQuota,
	}
}

func (s *S) TearDownSuite(c *check.C) {
	provision.Unregister("jobProv")
	defer s.conn.Close()
	dbtest.ClearAllCollections(s.conn.Apps().Database)
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
		return s.plans, nil
	}
	s.mockService.Plan.OnDefaultPlan = func() (*appTypes.Plan, error) {
		return s.mockService.Plan.OnFindByName("default-plan")
	}
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		for _, p := range s.plans {
			if p.Name == name {
				return &p, nil
			}
		}
		return nil, appTypes.ErrPlanNotFound
	}
	s.mockService.AppQuota.OnGet = func(_ quota.QuotaItem) (*quota.Quota, error) {
		return &quota.UnlimitedQuota, nil
	}
	s.mockService.TeamQuota.OnGet = func(_ quota.QuotaItem) (*quota.Quota, error) {
		return &quota.UnlimitedQuota, nil
	}
	s.mockService.Pool.OnServices = func(pool string) ([]string, error) {
		return []string{
			"my",
			"mysql",
			"healthcheck",
		}, nil
	}
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("queue:mongo-url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("queue:mongo-database", "queue_job_pkg_tests")
	config.Set("queue:mongo-polling-interval", 0.01)
	config.Set("docker:registry", "registry.somewhere")
	config.Set("auth:hash-cost", bcrypt.MinCost)
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	s.provisioner = &provisiontest.JobProvisioner{
		FakeProvisioner: provisiontest.ProvisionerInstance,
	}
	provision.Register("jobProv", func() (provision.Provisioner, error) {
		return s.provisioner, nil
	})
	provision.DefaultProvisioner = "jobProv"
	data, err := json.Marshal(appTypes.AppLock{})
	c.Assert(err, check.IsNil)
	err = json.Unmarshal(data, &s.zeroLock)
	c.Assert(err, check.IsNil)
}

func (s *S) SetUpTest(c *check.C) {
	s.provisioner.Reset()
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	s.createUserAndTeam(c)
	s.plans = []appTypes.Plan{
		{
			Name:     "c2m1",
			Memory:   1024,
			CPUMilli: 2000,
		},
		{
			Name:     "c4m2",
			Memory:   2048,
			CPUMilli: 4000,
		},
		{
			Name:     "default-plan",
			Memory:   1024,
			CPUMilli: 1000,
			Default:  true,
		},
	}
	var err error
	s.Pool = "pool1"
	opts := pool.AddPoolOptions{Name: s.Pool, Default: true}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "pool2"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AppendPoolConstraint(&pool.PoolConstraint{
		PoolExpr: "pool2",
		Field:    pool.ConstraintTypeTeam,
		Values:   []string{"team-2"},
	})
	c.Assert(err, check.IsNil)
	err = pool.AppendPoolConstraint(&pool.PoolConstraint{
		PoolExpr: "pool1",
		Field:    pool.ConstraintTypePlan,
		Values:   []string{"default-plan", "c2m1"},
	})
	c.Assert(err, check.IsNil)
	builder.DefaultBuilder = "fake"
	setupMocks(s)
	s.defaultPlan, err = s.mockService.Plan.OnDefaultPlan()
	c.Assert(err, check.IsNil)
	s.plan, err = s.mockService.Plan.OnFindByName("c4m2")
	c.Assert(err, check.IsNil)
	servicemanager.Job, err = JobService()
	c.Assert(err, check.IsNil)
}
