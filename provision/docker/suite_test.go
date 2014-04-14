// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	dtesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	ftesting "github.com/tsuru/tsuru/fs/testing"
	"github.com/tsuru/tsuru/provision"
	_ "github.com/tsuru/tsuru/testing"
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
	s.server, err = dtesting.NewServer("127.0.0.1:0", nil)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) SetUpTest(c *gocheck.C) {
	var err error
	cmutex.Lock()
	defer cmutex.Unlock()
	dCluster, err = cluster.New(nil, &mapStorage{},
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
}

func (s *S) stopMultipleServersCluster(cluster *cluster.Cluster, nodes map[string]string) {
	cmutex.Lock()
	defer cmutex.Unlock()
	clusterNodes = nodes
	dCluster = cluster
}

func (s *S) startMultipleServersCluster() (*cluster.Cluster, map[string]string, error) {
	otherServer, err := dtesting.NewServer("127.0.0.1:0", nil)
	if err != nil {
		return nil, nil, err
	}
	cmutex.Lock()
	defer cmutex.Unlock()
	oldClusterNodes := clusterNodes
	oldCluster := dCluster
	clusterNodes = map[string]string{
		"server1": "http://server1:8888",
		"server2": "http://server2:8888",
	}
	dCluster, err = cluster.New(nil, &mapStorage{},
		cluster.Node{ID: "server1", Address: s.server.URL()},
		cluster.Node{ID: "server2", Address: otherServer.URL()},
	)
	if err != nil {
		return nil, nil, err
	}
	return oldCluster, oldClusterNodes, nil
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
