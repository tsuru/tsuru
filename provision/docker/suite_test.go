// Copyright 2014 tsuru authors. All rights reserved.
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
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
	rtesting "github.com/tsuru/tsuru/router/testing"
	"github.com/tsuru/tsuru/service"
	tTesting "github.com/tsuru/tsuru/testing"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct {
	collName       string
	imageCollName  string
	gitHost        string
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
}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpSuite(c *gocheck.C) {
	s.collName = "docker_unit"
	s.imageCollName = "docker_image"
	s.gitHost = "my.gandalf.com"
	s.repoNamespace = "tsuru"
	s.sshUser = "root"
	config.Set("git:ro-host", s.gitHost)
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "docker_provision_tests_s")
	config.Set("docker:repository-namespace", s.repoNamespace)
	config.Set("docker:router", "fake")
	config.Set("docker:collection", s.collName)
	config.Set("docker:deploy-cmd", "/var/lib/tsuru/deploy")
	config.Set("docker:run-cmd:bin", "/usr/local/bin/circusd /etc/circus/circus.ini")
	config.Set("docker:run-cmd:port", "8888")
	config.Set("docker:ssh:add-key-cmd", "/var/lib/tsuru/add-key")
	config.Set("docker:ssh:user", s.sshUser)
	config.Set("docker:cluster:mongo-url", "127.0.0.1:27017")
	config.Set("docker:cluster:mongo-database", "docker_provision_tests_cluster_stor")
	config.Set("queue", "fake")
	s.deployCmd = "/var/lib/tsuru/deploy"
	s.runBin = "/usr/local/bin/circusd"
	s.runArgs = "/etc/circus/circus.ini"
	s.port = "8888"
	var err error
	s.server, err = dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, gocheck.IsNil)
	s.targetRecover = tTesting.SetTargetFile(c, []byte("http://localhost"))
	s.storage, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
	s.oldProvisioner = app.Provisioner
	app.Provisioner = &dockerProvisioner{}
}

func (s *S) SetUpTest(c *gocheck.C) {
	var err error
	cmutex.Lock()
	defer cmutex.Unlock()
	dCluster, err = cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: s.server.URL()},
	)
	c.Assert(err, gocheck.IsNil)
	coll := collection()
	defer coll.Close()
	coll.RemoveAll(nil)
	err = clearClusterStorage()
	c.Assert(err, gocheck.IsNil)
	rtesting.FakeRouter.Reset()
	healingColl, err := healingCollection()
	c.Assert(err, gocheck.IsNil)
	defer healingColl.Close()
	healingColl.RemoveAll(nil)
}

func clearClusterStorage() error {
	clusterDbUrl, _ := config.GetString("docker:cluster:mongo-url")
	clusterDbName, _ := config.GetString("docker:cluster:mongo-database")
	session, err := mgo.Dial(clusterDbUrl)
	if err != nil {
		return err
	}
	defer session.Close()
	return clearAllCollections(session.DB(clusterDbName))
}

func clearAllCollections(db *mgo.Database) error {
	colls, err := db.CollectionNames()
	if err != nil {
		return err
	}
	for _, collName := range colls {
		if strings.Index(collName, ".") != -1 {
			continue
		}
		db.C(collName).RemoveAll(nil)
	}
	return nil
}

func (s *S) TearDownSuite(c *gocheck.C) {
	coll := collection()
	defer coll.Close()
	err := clearAllCollections(coll.Database)
	c.Assert(err, gocheck.IsNil)
	tTesting.RollbackFile(s.targetRecover)
	s.storage.Apps().Database.DropDatabase()
	s.storage.Close()
	app.Provisioner = s.oldProvisioner
}

func (s *S) stopMultipleServersCluster(cluster *cluster.Cluster) {
	cmutex.Lock()
	defer cmutex.Unlock()
	dCluster = cluster
}

func (s *S) startMultipleServersCluster() (*cluster.Cluster, error) {
	otherServer, err := dtesting.NewServer("localhost:0", nil, nil)
	if err != nil {
		return nil, err
	}
	cmutex.Lock()
	defer cmutex.Unlock()
	oldCluster := dCluster
	otherUrl := strings.Replace(otherServer.URL(), "127.0.0.1", "localhost", 1)
	dCluster, err = cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: s.server.URL()},
		cluster.Node{Address: otherUrl},
	)
	if err != nil {
		return nil, err
	}
	return oldCluster, nil
}

func (s *S) addServiceInstance(c *gocheck.C, appName string, fn http.HandlerFunc) func() {
	ts := httptest.NewServer(fn)
	ret := func() {
		ts.Close()
		s.storage.Services().Remove(bson.M{"_id": "mysql"})
		s.storage.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	}
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{}}
	err = instance.Create()
	c.Assert(err, gocheck.IsNil)
	err = instance.AddApp(appName)
	c.Assert(err, gocheck.IsNil)
	err = s.storage.ServiceInstances().Update(bson.M{"name": instance.Name}, instance)
	c.Assert(err, gocheck.IsNil)
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
