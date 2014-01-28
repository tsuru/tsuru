// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	dtesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/garyburd/redigo/redis"
	"github.com/globocom/config"
	"github.com/globocom/docker-cluster/cluster"
	"github.com/globocom/docker-cluster/storage"
	ftesting "github.com/globocom/tsuru/fs/testing"
	"github.com/globocom/tsuru/provision"
	_ "github.com/globocom/tsuru/testing"
	"launchpad.net/gocheck"
	"os"
	"sort"
	"testing"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct {
	collName      string
	imageCollName string
	gitHost       string
	repoNamespace string
	deployCmd     string
	runBin        string
	runArgs       string
	port          string
	sshUser       string
	server        *dtesting.DockerServer
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
	config.Set("queue", "fake")
	s.deployCmd = "/var/lib/tsuru/deploy"
	s.runBin = "/usr/local/bin/circusd"
	s.runArgs = "/etc/circus/circus.ini"
	s.port = "8888"
	fsystem = &ftesting.RecordingFs{}
	f, err := fsystem.Create(os.ExpandEnv("${HOME}/.ssh/id_rsa.pub"))
	c.Assert(err, gocheck.IsNil)
	f.Write([]byte("key-content"))
	f.Close()
	s.server, err = dtesting.NewServer(nil)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) SetUpTest(c *gocheck.C) {
	var err error
	dCluster, err = cluster.New(nil, storage.Redis("localhost:6379", "tests"),
		cluster.Node{ID: "server", Address: s.server.URL()},
	)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TearDownSuite(c *gocheck.C) {
	coll := collection()
	defer coll.Close()
	err := coll.Database.DropDatabase()
	c.Assert(err, gocheck.IsNil)
	fsystem = nil
	clearSchedStorage(c)
}

func removeClusterNodes(ids []string, c *gocheck.C) {
	conn, err := redis.Dial("tcp", "localhost:6379")
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	c.Assert(err, gocheck.IsNil)
	err = conn.Send("multi")
	c.Assert(err, gocheck.IsNil)
	for _, id := range ids {
		conn.Send("del", "tests:node:"+id)
		conn.Send("lrem", "tests:nodes", "0", id)
	}
	_, err = conn.Do("exec")
	c.Assert(err, gocheck.IsNil)
}

func clearSchedStorage(c *gocheck.C) {
	conn, err := redis.Dial("tcp", "localhost:6379")
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	keys, err := conn.Do("keys", "*")
	c.Assert(err, gocheck.IsNil)
	for _, key := range keys.([]interface{}) {
		k := string(key.([]byte))
		_, err := conn.Do("del", k)
		c.Assert(err, gocheck.IsNil)
	}
}

func insertImage(repo, nodeID string, c *gocheck.C) func() {
	conn, err := redis.Dial("tcp", "localhost:6379")
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	_, err = conn.Do("set", "tests:image:"+repo, nodeID)
	c.Assert(err, gocheck.IsNil)
	return func() {
		conn, err := redis.Dial("tcp", "localhost:6379")
		c.Assert(err, gocheck.IsNil)
		defer conn.Close()
		conn.Do("del", "tests:image:"+repo)
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

func createFakeContainers(ids []string, c *gocheck.C) func() {
	conn, err := redis.Dial("tcp", "localhost:6379")
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	filter := []interface{}{}
	for _, id := range ids {
		key := "tests:" + id
		_, err = conn.Do("SET", key, "server")
		c.Assert(err, gocheck.IsNil)
		filter = append(filter, key)
	}
	return func() {
		conn.Do("DEL", filter...)
	}
}
