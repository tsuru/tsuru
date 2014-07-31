// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	dtesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/garyburd/redigo/redis"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/service"
	tTesting "github.com/tsuru/tsuru/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
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
	config.Set("docker:scheduler:redis-prefix", "redis-scheduler-storage-test")
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
	clearRedisKeys("redis-scheduler-storage-test*", c)
}

func clearRedisKeys(keysPattern string, c *gocheck.C) {
	redisConn, err := redis.Dial("tcp", "127.0.0.1:6379")
	c.Assert(err, gocheck.IsNil)
	defer redisConn.Close()
	result, err := redisConn.Do("KEYS", keysPattern)
	c.Assert(err, gocheck.IsNil)
	keys := result.([]interface{})
	for _, key := range keys {
		keyName := string(key.([]byte))
		redisConn.Do("DEL", keyName)
	}
}

func (s *S) TearDownSuite(c *gocheck.C) {
	coll := collection()
	defer coll.Close()
	err := coll.Database.DropDatabase()
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
