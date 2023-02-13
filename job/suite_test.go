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
	user        *auth.User
	plan        appTypes.Plan
	defaultPlan appTypes.Plan
	provisioner *provisiontest.JobProvisioner
	Pool        string
	zeroLock    map[string]interface{}
	mockService servicemock.MockService
}

func (s *S) createUserAndTeam(c *check.C) {
	s.user = &auth.User{
		Email: "august@tswift.com",
		Quota: quota.UnlimitedQuota,
	}
	err := s.user.Create()
	c.Assert(err, check.IsNil)
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
	s.defaultPlan = appTypes.Plan{
		Name:     "default-plan",
		Memory:   1024,
		Swap:     1024,
		CpuShare: 100,
		Default:  true,
	}
	s.plan = appTypes.Plan{}
	s.Pool = "pool1"
	opts := pool.AddPoolOptions{Name: s.Pool, Default: true}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	builder.DefaultBuilder = "fake"
	setupMocks(s)
}
