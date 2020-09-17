// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"

	dtesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/version"
	"github.com/tsuru/tsuru/applog"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/safe"
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
	collName      string
	imageCollName string
	repoNamespace string
	deployCmd     string
	runBin        string
	runArgs       string
	port          string
	sshUser       string
	server        *dtesting.DockerServer
	extraServer   *dtesting.DockerServer
	conn          *db.Storage
	p             *dockerProvisioner
	user          *auth.User
	token         auth.Token
	team          *authTypes.Team
	clusterSess   *mgo.Session
	logBuf        *safe.Buffer
	mockService   servicemock.MockService
}

var _ = check.Suite(&S{})

var nativeScheme = auth.ManagedScheme(native.NativeScheme{})

func (s *S) SetUpSuite(c *check.C) {
	s.collName = "docker_unit"
	s.imageCollName = "docker_image"
	s.repoNamespace = "tsuru"
	s.sshUser = "root"
	s.port = "8888"
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "docker_provision_tests_s")
	config.Set("docker:repository-namespace", s.repoNamespace)
	config.Set("docker:router", "fake")
	config.Set("docker:collection", s.collName)
	config.Set("docker:deploy-cmd", "/var/lib/tsuru/deploy")
	config.Set("docker:run-cmd:bin", "/usr/local/bin/circusd /etc/circus/circus.ini")
	config.Set("docker:run-cmd:port", s.port)
	config.Set("docker:user", s.sshUser)
	config.Set("docker:cluster:mongo-url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("docker:cluster:mongo-database", "docker_provision_tests_cluster_stor")
	config.Set("queue:mongo-url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("queue:mongo-database", "queue_provision_docker_tests")
	config.Set("queue:mongo-polling-interval", 0.01)
	config.Set("routers:fake:type", "fake")
	config.Set("repo-manager", "fake")
	config.Set("docker:registry-max-try", 1)
	config.Set("auth:hash-cost", bcrypt.MinCost)
	s.deployCmd = "/var/lib/tsuru/deploy"
	s.runBin = "/usr/local/bin/circusd"
	s.runArgs = "/etc/circus/circus.ini"
	os.Setenv("TSURU_TARGET", "http://localhost")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	clusterDbURL, _ := config.GetString("docker:cluster:mongo-url")
	s.clusterSess, err = mgo.Dial(clusterDbURL)
	c.Assert(err, check.IsNil)
	err = dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
	repositorytest.Reset()
	s.user = &auth.User{Email: "myadmin@arrakis.com", Password: "123456", Quota: quota.UnlimitedQuota}
	nScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nScheme
	_, err = nScheme.Create(s.user)
	c.Assert(err, check.IsNil)
	s.token = permissiontest.ExistingUserWithPermission(c, nativeScheme, s.user, permission.Permission{
		Scheme:  permission.PermAll,
		Context: permTypes.PermissionContext{CtxType: permTypes.CtxGlobal},
	})
}

func (s *S) SetUpTest(c *check.C) {
	pool.ResetCache()
	config.Set("docker:api-timeout", 2)
	iaas.ResetAll()
	repositorytest.Reset()
	queue.ResetQueue()
	repository.Manager().CreateUser(s.user.Email)
	s.p = &dockerProvisioner{storage: &cluster.MapStorage{}}
	err := s.p.Initialize()
	c.Assert(err, check.IsNil)
	queue.ResetQueue()
	s.server, err = dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	s.p.cluster, err = cluster.New(nil, s.p.storage, "",
		cluster.Node{Address: s.server.URL(), Metadata: map[string]string{"pool": "test-default"}},
	)
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = s.p
	err = dbtest.ClearAllCollectionsExcept(s.conn.Apps().Database, []string{"users", "tokens"})
	c.Assert(err, check.IsNil)
	err = clearClusterStorage(s.clusterSess)
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.Reset()
	opts := pool.AddPoolOptions{Name: "test-default", Default: true}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	s.conn.Tokens().Remove(bson.M{"appname": bson.M{"$ne": ""}})
	s.logBuf = safe.NewBuffer(nil)
	log.SetLogger(log.NewWriterLogger(s.logBuf, true))
	servicemock.SetMockService(&s.mockService)
	s.team = &authTypes.Team{Name: "admin"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{*s.team}, nil
	}
	s.mockService.Team.OnFindByName = func(_ string) (*authTypes.Team, error) {
		return s.team, nil
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
	s.mockService.UserQuota.OnGet = func(email string) (*quota.Quota, error) {
		c.Assert(email, check.Equals, s.user.Email)
		return &s.user.Quota, nil
	}
	s.mockService.UserQuota.OnInc = func(email string, n int) error {
		c.Assert(email, check.Equals, s.user.Email)
		return nil
	}

	s.mockService.PlatformImage.OnCurrentImage = func(name string) (string, error) {
		return "tsuru/" + name + ":v1", nil
	}
	servicemanager.AppVersion, err = version.AppVersionService()
	c.Assert(err, check.IsNil)
	servicemanager.AppLog, err = applog.AppLogService()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownTest(c *check.C) {
	app.GetAppRouterUpdater().Shutdown(context.Background())
	log.SetLogger(nil)
	s.server.Stop()
	if s.extraServer != nil {
		s.extraServer.Stop()
		s.extraServer = nil
	}
}

func (s *S) TearDownSuite(c *check.C) {
	defer s.clusterSess.Close()
	defer s.conn.Close()
	os.Unsetenv("TSURU_TARGET")
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
	clusterDbName, _ := config.GetString("docker:cluster:mongo-database")
	dbtest.ClearAllCollections(conn.Apps().Database.Session.DB(clusterDbName))
}

func clearClusterStorage(sess *mgo.Session) error {
	clusterDbName, _ := config.GetString("docker:cluster:mongo-database")
	return dbtest.ClearAllCollections(sess.DB(clusterDbName))
}

func (s *S) startMultipleServersCluster() (*dockerProvisioner, error) {
	var err error
	s.extraServer, err = dtesting.NewServer("localhost:0", nil, nil)
	if err != nil {
		return nil, err
	}
	otherURL := strings.Replace(s.extraServer.URL(), "127.0.0.1", "localhost", 1)
	var p dockerProvisioner
	err = p.Initialize()
	if err != nil {
		return nil, err
	}
	p.storage = &cluster.MapStorage{}
	p.cluster, err = cluster.New(nil, p.storage, "",
		cluster.Node{Address: s.server.URL(), Metadata: map[string]string{"pool": "test-default", "m1": "v1"}},
		cluster.Node{Address: otherURL, Metadata: map[string]string{"pool": "test-default"}},
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *S) addServiceInstance(c *check.C, appName string, unitsIDs []string, fn http.HandlerFunc) func() {
	ts := httptest.NewServer(fn)
	ret := func() {
		ts.Close()
	}
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := service.Create(srvc)
	c.Assert(err, check.IsNil)
	var units []service.Unit
	for _, s := range unitsIDs {
		units = append(units, service.Unit{ID: s})
	}
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{},
		BoundUnits:  units,
		Apps:        []string{appName},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	return ret
}

type unitSlice []provision.Unit

func (s unitSlice) Len() int {
	return len(s)
}

func (s unitSlice) Less(i, j int) bool {
	return s[i].ID < s[j].ID
}

func (s unitSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func sortUnits(units []provision.Unit) {
	sort.Sort(unitSlice(units))
}

func (s *S) newApp(name string) app.App {
	return app.App{
		Name:      name,
		Platform:  "python",
		TeamOwner: s.team.Name,
		Router:    "fake",
	}
}

func (s *S) newAppFromFake(fake *provisiontest.FakeApp) app.App {
	return app.App{
		Name:     fake.GetName(),
		Platform: fake.GetPlatform(),
		Routers:  fake.GetRouters(),
	}
}
