// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	dtesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/cmd/cmdtest"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/service"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	collName       string
	imageCollName  string
	repoNamespace  string
	deployCmd      string
	runBin         string
	runArgs        string
	port           string
	sshUser        string
	server         *dtesting.DockerServer
	targetRecover  []string
	storage        *db.Storage
	oldProvisioner provision.Provisioner
	p              *dockerProvisioner
	user           *auth.User
	token          auth.Token
	team           *auth.Team
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	s.collName = "docker_unit"
	s.imageCollName = "docker_image"
	s.repoNamespace = "tsuru"
	s.sshUser = "root"
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "docker_provision_tests_s")
	config.Set("docker:repository-namespace", s.repoNamespace)
	config.Set("docker:router", "fake")
	config.Set("docker:collection", s.collName)
	config.Set("docker:deploy-cmd", "/var/lib/tsuru/deploy")
	config.Set("docker:run-cmd:bin", "/usr/local/bin/circusd /etc/circus/circus.ini")
	config.Set("docker:run-cmd:port", "8888")
	config.Set("docker:ssh:user", s.sshUser)
	config.Set("docker:cluster:mongo-url", "127.0.0.1:27017")
	config.Set("docker:cluster:mongo-database", "docker_provision_tests_cluster_stor")
	config.Set("routers:fake:type", "fake")
	config.Set("queue", "fake")
	config.Set("repo-manager", "fake")
	config.Set("admin-team", "admin")
	s.deployCmd = "/var/lib/tsuru/deploy"
	s.runBin = "/usr/local/bin/circusd"
	s.runArgs = "/etc/circus/circus.ini"
	s.port = "8888"
	var err error
	s.targetRecover = cmdtest.SetTargetFile(c, []byte("http://localhost"))
	s.storage, err = db.Conn()
	c.Assert(err, check.IsNil)
	s.p = &dockerProvisioner{storage: &cluster.MapStorage{}}
	err = s.p.Initialize()
	c.Assert(err, check.IsNil)
	s.oldProvisioner = app.Provisioner
	app.Provisioner = s.p
	s.user = &auth.User{Email: "myadmin@arrakis.com", Password: "123456", Quota: quota.Unlimited}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	_, err = nativeScheme.Create(s.user)
	c.Assert(err, check.IsNil)
	s.team = &auth.Team{Name: "admin", Users: []string{s.user.Email}}
	c.Assert(err, check.IsNil)
	err = s.storage.Teams().Insert(s.team)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
}

func (s *S) SetUpTest(c *check.C) {
	iaas.ResetAll()
	config.Set("docker:registry-max-try", 1)
	repositorytest.Reset()
	var err error
	if s.server != nil {
		s.server.Stop()
	}
	s.server, err = dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	s.p.cluster, err = cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: s.server.URL()},
	)
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = s.p
	coll := s.p.collection()
	defer coll.Close()
	err = dbtest.ClearAllCollectionsExcept(coll.Database, []string{"users", "tokens", "teams"})
	c.Assert(err, check.IsNil)
	err = clearClusterStorage()
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.Reset()
}

func clearClusterStorage() error {
	clusterDbUrl, _ := config.GetString("docker:cluster:mongo-url")
	clusterDbName, _ := config.GetString("docker:cluster:mongo-database")
	session, err := mgo.Dial(clusterDbUrl)
	if err != nil {
		return err
	}
	defer session.Close()
	return dbtest.ClearAllCollections(session.DB(clusterDbName))
}

func (s *S) TearDownSuite(c *check.C) {
	coll := s.p.collection()
	defer coll.Close()
	err := dbtest.ClearAllCollections(coll.Database)
	c.Assert(err, check.IsNil)
	cmdtest.RollbackFile(s.targetRecover)
	dbtest.ClearAllCollections(s.storage.Apps().Database)
	s.storage.Close()
	app.Provisioner = s.oldProvisioner
}

func (s *S) stopMultipleServersCluster(p *dockerProvisioner) {
}

func (s *S) startMultipleServersCluster() (*dockerProvisioner, error) {
	otherServer, err := dtesting.NewServer("localhost:0", nil, nil)
	if err != nil {
		return nil, err
	}
	otherUrl := strings.Replace(otherServer.URL(), "127.0.0.1", "localhost", 1)
	var p dockerProvisioner
	err = p.Initialize()
	if err != nil {
		return nil, err
	}
	p.storage = &cluster.MapStorage{}
	p.cluster, err = cluster.New(nil, p.storage,
		cluster.Node{Address: s.server.URL()},
		cluster.Node{Address: otherUrl},
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *S) startMultipleServersClusterSeggregated() (*dockerProvisioner, error) {
	otherServer, err := dtesting.NewServer("localhost:0", nil, nil)
	if err != nil {
		return nil, err
	}
	otherUrl := strings.Replace(otherServer.URL(), "127.0.0.1", "localhost", 1)
	var p dockerProvisioner
	err = p.Initialize()
	if err != nil {
		return nil, err
	}
	p.storage = &cluster.MapStorage{}
	sched := segregatedScheduler{provisioner: &p}
	p.cluster, err = cluster.New(&sched, p.storage,
		cluster.Node{Address: s.server.URL(), Metadata: map[string]string{"pool": "pool1"}},
		cluster.Node{Address: otherUrl, Metadata: map[string]string{"pool": "pool2"}},
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *S) addServiceInstance(c *check.C, appName string, fn http.HandlerFunc) func() {
	ts := httptest.NewServer(fn)
	ret := func() {
		ts.Close()
		s.storage.Services().Remove(bson.M{"_id": "mysql"})
		s.storage.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	}
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{}}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	err = instance.AddApp(appName)
	c.Assert(err, check.IsNil)
	err = s.storage.ServiceInstances().Update(bson.M{"name": instance.Name}, instance)
	c.Assert(err, check.IsNil)
	return ret
}

func startTestRepositoryServer() func() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	repoUrl := strings.Replace(server.URL, "http://", "", 1)
	config.Set("docker:registry", repoUrl)
	return func() {
		config.Unset("docker:registry")
		server.Close()
	}
}

type unitSlice []provision.Unit

func (s unitSlice) Len() int {
	return len(s)
}

func (s unitSlice) Less(i, j int) bool {
	return s[i].Name < s[j].Name
}

func (s unitSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func sortUnits(units []provision.Unit) {
	sort.Sort(unitSlice(units))
}
