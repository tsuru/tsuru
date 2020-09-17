// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockertest

import (
	"errors"
	"testing"

	docker "github.com/fsouza/go-dockerclient"
	dtesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/types"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/provision/provisiontest"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

var _ = check.Suite(&S{})

type S struct{}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "docker_provision_dockertest_tests")
	config.Set("docker:cluster:mongo-url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("docker:cluster:mongo-database", "docker_provision_dockertest_tests_cluster_stor")
}

func (s *S) SetUpTest(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
}

func (s *S) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
}

func (s *S) TestNewFakeDockerProvisioner(c *check.C) {
	server, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server.Stop()
	p, err := NewFakeDockerProvisioner(server.URL())
	c.Assert(err, check.IsNil)
	_, err = p.storage.RetrieveNode(server.URL())
	c.Assert(err, check.IsNil)
	opts := docker.PullImageOptions{Repository: "tsuru/bs"}
	err = p.Cluster().PullImage(opts, dockercommon.RegistryAuthConfig(opts.Repository))
	c.Assert(err, check.IsNil)
	client, err := docker.NewClient(server.URL())
	c.Assert(err, check.IsNil)
	_, err = client.InspectImage("tsuru/bs")
	c.Assert(err, check.IsNil)
}

func (s *S) TestStartMultipleServersCluster(c *check.C) {
	p, err := StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	opts := docker.PullImageOptions{Repository: "tsuru/bs"}
	err = p.Cluster().PullImage(opts, dockercommon.RegistryAuthConfig(opts.Repository))
	c.Assert(err, check.IsNil)
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
}

func (s *S) TestDestroy(c *check.C) {
	p, err := StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	p.Destroy()
	c.Assert(p.servers, check.IsNil)
	opts := docker.PullImageOptions{Repository: "tsuru/bs"}
	err = p.Cluster().PullImage(opts, dockercommon.RegistryAuthConfig(opts.Repository))
	c.Assert(err, check.NotNil)
	e, ok := err.(cluster.DockerNodeError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.BaseError(), check.ErrorMatches, "(cannot connect to Docker endpoint)|(.*connection reset by peer)")
}

func (s *S) TestServers(c *check.C) {
	server, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server.Stop()
	var p FakeDockerProvisioner
	p.servers = append(p.servers, server)
	c.Assert(p.Servers(), check.DeepEquals, p.servers)
}

func (s *S) TestCluster(c *check.C) {
	var p FakeDockerProvisioner
	cluster, err := cluster.New(nil, &cluster.MapStorage{}, "")
	c.Assert(err, check.IsNil)
	p.cluster = cluster
	c.Assert(p.Cluster(), check.Equals, cluster)
}

func (s *S) TestCollection(c *check.C) {
	var p FakeDockerProvisioner
	collection := p.Collection()
	defer collection.Close()
	c.Assert(collection.Name, check.Equals, "fake_docker_provisioner")
}

func (s *S) TestPushImage(c *check.C) {
	var p FakeDockerProvisioner
	err := p.PushImage("tsuru/bs", "v1")
	c.Assert(err, check.IsNil)
	expected := []Push{{Name: "tsuru/bs", Tag: "v1"}}
	c.Assert(p.Pushes(), check.DeepEquals, expected)
}

func (s *S) TestPushImageFailure(c *check.C) {
	p := FakeDockerProvisioner{pushErrors: make(chan error, 1)}
	prepErr := errors.New("fail to push")
	p.FailPush(prepErr)
	err := p.PushImage("tsuru/bs", "v1")
	c.Assert(err, check.Equals, prepErr)
	expected := []Push{{Name: "tsuru/bs", Tag: "v1"}}
	c.Assert(p.Pushes(), check.DeepEquals, expected)
}

func (s *S) TestAllContainers(c *check.C) {
	p, err := NewFakeDockerProvisioner()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	cont1 := container.Container{Container: types.Container{ID: "cont1"}}
	cont2 := container.Container{Container: types.Container{ID: "cont2"}}
	p.SetContainers("localhost", []container.Container{cont1})
	p.SetContainers("remotehost", []container.Container{cont2})
	cont1.HostAddr = "localhost"
	cont2.HostAddr = "remotehost"
	containers := p.AllContainers()
	expected := []container.Container{cont1, cont2}
	if expected[0].HostAddr != containers[0].HostAddr {
		expected = []container.Container{cont2, cont1}
	}
	c.Assert(containers, check.DeepEquals, expected)
}

func (s *S) TestMoveOneContainer(c *check.C) {
	p, err := NewFakeDockerProvisioner()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	cont := container.Container{Container: types.Container{ID: "cont1"}}
	p.SetContainers("localhost", []container.Container{cont})
	p.SetContainers("remotehost", []container.Container{{Container: types.Container{ID: "cont2"}}})
	errors := make(chan error, 1)
	result := p.MoveOneContainer(cont, "remotehost", errors, nil, nil, nil)
	expected := container.Container{Container: types.Container{ID: "cont1-moved", HostAddr: "remotehost"}}
	c.Assert(result, check.DeepEquals, expected)
	select {
	case err := <-errors:
		c.Fatal(err)
	default:
	}
	containers := p.Containers("localhost")
	c.Assert(containers, check.HasLen, 0)
	containers = p.Containers("remotehost")
	expectedContainers := []container.Container{{Container: types.Container{ID: "cont2", HostAddr: "remotehost"}}, result}
	c.Assert(containers, check.DeepEquals, expectedContainers)
	expectedMovings := []ContainerMoving{
		{HostFrom: "localhost", HostTo: "remotehost", ContainerID: cont.ID},
	}
	c.Assert(p.Movings(), check.DeepEquals, expectedMovings)
}

func (s *S) TestMoveOneContainerRecreate(c *check.C) {
	p, err := NewFakeDockerProvisioner()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	cont := container.Container{Container: types.Container{ID: "cont1"}}
	p.SetContainers("localhost", []container.Container{cont})
	p.SetContainers("remotehost", []container.Container{{Container: types.Container{ID: "cont2"}}})
	errors := make(chan error, 1)
	result := p.MoveOneContainer(cont, "", errors, nil, nil, nil)
	expected := container.Container{Container: types.Container{ID: "cont1-moved", HostAddr: "remotehost"}}
	c.Assert(result, check.DeepEquals, expected)
	select {
	case err := <-errors:
		c.Fatal(err)
	default:
	}
	containers := p.Containers("localhost")
	c.Assert(containers, check.HasLen, 0)
	containers = p.Containers("remotehost")
	expectedContainers := []container.Container{{Container: types.Container{ID: "cont2", HostAddr: "remotehost"}}, result}
	c.Assert(containers, check.DeepEquals, expectedContainers)
	expectedMovings := []ContainerMoving{
		{HostFrom: "localhost", HostTo: "remotehost", ContainerID: cont.ID},
	}
	c.Assert(p.Movings(), check.DeepEquals, expectedMovings)
}

func (s *S) TestMoveOneContainerNotFound(c *check.C) {
	p, err := NewFakeDockerProvisioner()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	cont := container.Container{Container: types.Container{ID: "something", HostAddr: "localhost"}}
	errors := make(chan error, 1)
	result := p.MoveOneContainer(cont, "remotehost", errors, nil, nil, nil)
	c.Assert(result, check.DeepEquals, container.Container{})
	err = <-errors
	c.Assert(err, check.NotNil)
	e, ok := err.(*provision.UnitNotFoundError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.ID, check.Equals, cont.ID)
}

func (s *S) TestMoveOneContainerNoActionNeeded(c *check.C) {
	p, err := NewFakeDockerProvisioner()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	cont := container.Container{Container: types.Container{ID: "something"}}
	p.SetContainers("localhost", []container.Container{cont})
	errors := make(chan error, 1)
	result := p.MoveOneContainer(cont, "localhost", errors, nil, nil, nil)
	cont.HostAddr = "localhost"
	c.Assert(result, check.DeepEquals, cont)
	select {
	case err := <-errors:
		c.Error(err)
	default:
	}
}

func (s *S) TestMoveOneContainerError(c *check.C) {
	p, err := NewFakeDockerProvisioner()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	cont := container.Container{Container: types.Container{ID: "something"}}
	p.SetContainers("localhost", []container.Container{cont})
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")
	p.FailMove(err1, err2)
	errors := make(chan error, 1)
	result := p.MoveOneContainer(cont, "localhost", errors, nil, nil, nil)
	c.Assert(result.ID, check.Equals, "")
	c.Assert(<-errors, check.Equals, err1)
	result = p.MoveOneContainer(cont, "localhost", errors, nil, nil, nil)
	c.Assert(result.ID, check.Equals, "")
	c.Assert(<-errors, check.Equals, err2)
}

func (s *S) TestMoveContainers(c *check.C) {
	p, err := NewFakeDockerProvisioner()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	cont1 := container.Container{Container: types.Container{ID: "cont1"}}
	cont2 := container.Container{Container: types.Container{ID: "cont2"}}
	p.SetContainers("localhost", []container.Container{cont1, cont2})
	cont3 := container.Container{Container: types.Container{ID: "cont3"}}
	p.SetContainers("remotehost", []container.Container{cont3})
	err = p.MoveContainers("localhost", "remotehost", nil)
	c.Assert(err, check.IsNil)
	containers := p.Containers("localhost")
	c.Assert(containers, check.HasLen, 0)
	containers = p.Containers("remotehost")
	expected := []container.Container{
		{Container: types.Container{ID: "cont3", HostAddr: "remotehost"}},
		{Container: types.Container{ID: "cont1-moved", HostAddr: "remotehost"}},
		{Container: types.Container{ID: "cont2-moved", HostAddr: "remotehost"}},
	}
	c.Assert(containers, check.DeepEquals, expected)
	expectedMovings := []ContainerMoving{
		{HostFrom: "localhost", HostTo: "remotehost", ContainerID: "cont1"},
		{HostFrom: "localhost", HostTo: "remotehost", ContainerID: "cont2"},
	}
	c.Assert(p.Movings(), check.DeepEquals, expectedMovings)
}

func (s *S) TestMoveContainersRecreation(c *check.C) {
	p, err := NewFakeDockerProvisioner()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	cont1 := container.Container{Container: types.Container{ID: "cont1"}}
	cont2 := container.Container{Container: types.Container{ID: "cont2"}}
	p.SetContainers("localhost", []container.Container{cont1, cont2})
	err = p.MoveContainers("localhost", "", nil)
	c.Assert(err, check.IsNil)
	containers := p.Containers("localhost")
	expected := []container.Container{
		{Container: types.Container{ID: "cont1-recreated", HostAddr: "localhost"}},
		{Container: types.Container{ID: "cont2-recreated", HostAddr: "localhost"}},
	}
	c.Assert(containers, check.DeepEquals, expected)
	expectedMovings := []ContainerMoving{
		{HostFrom: "localhost", HostTo: "", ContainerID: "cont1"},
		{HostFrom: "localhost", HostTo: "", ContainerID: "cont2"},
	}
	c.Assert(p.Movings(), check.DeepEquals, expectedMovings)
}

func (s *S) TestMoveContainersEmptyDestination(c *check.C) {
	p, err := NewFakeDockerProvisioner()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	cont1 := container.Container{Container: types.Container{ID: "cont1"}}
	cont2 := container.Container{Container: types.Container{ID: "cont2"}}
	p.SetContainers("localhost", []container.Container{cont1, cont2})
	cont3 := container.Container{Container: types.Container{ID: "cont3"}}
	p.SetContainers("remotehost", []container.Container{cont3})
	err = p.MoveContainers("localhost", "", nil)
	c.Assert(err, check.IsNil)
	containers := p.Containers("remotehost")
	expected := []container.Container{
		{Container: types.Container{ID: "cont3", HostAddr: "remotehost"}},
		{Container: types.Container{ID: "cont1-moved", HostAddr: "remotehost"}},
		{Container: types.Container{ID: "cont2-moved", HostAddr: "remotehost"}},
	}
	c.Assert(containers, check.DeepEquals, expected)
	containers = p.Containers("localhost")
	c.Assert(containers, check.HasLen, 0)
	expectedMovings := []ContainerMoving{
		{HostFrom: "localhost", HostTo: "remotehost", ContainerID: "cont1"},
		{HostFrom: "localhost", HostTo: "remotehost", ContainerID: "cont2"},
	}
	c.Assert(p.Movings(), check.DeepEquals, expectedMovings)
}

func (s *S) TestMoveContainersHostNotFound(c *check.C) {
	p, err := NewFakeDockerProvisioner()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	err = p.MoveContainers("localhost", "remotehost", nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "host not found: localhost")
}

func (s *S) TestHandleMoveErrors(c *check.C) {
	originalError := errors.New("something went wrong")
	errs := make(chan error, 1)
	errs <- originalError
	var p FakeDockerProvisioner
	err := p.HandleMoveErrors(errs, nil)
	c.Check(err, check.Equals, originalError)
	err = p.HandleMoveErrors(errs, nil)
	c.Check(err, check.IsNil)
}

func (s *S) TestListContainersNoResult(c *check.C) {
	p, err := NewFakeDockerProvisioner()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	query := bson.M{"id": "abc123"}
	containers, err := p.ListContainers(query)
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 0)
	c.Assert(p.Queries(), check.DeepEquals, []bson.M{query})
}

func (s *S) TestListContainersPreparedResult(c *check.C) {
	p, err := NewFakeDockerProvisioner()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	p.PrepareListResult([]container.Container{{Container: types.Container{ID: "cont1"}}}, nil)
	query := bson.M{"id": "cont1"}
	containers, err := p.ListContainers(query)
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	containers[0].MongoID = bson.ObjectId("")
	c.Assert(containers, check.DeepEquals, []container.Container{{Container: types.Container{ID: "cont1"}}})
	c.Assert(p.Queries(), check.DeepEquals, []bson.M{query})
}

func (s *S) TestListContainersPreparedError(c *check.C) {
	p, err := NewFakeDockerProvisioner()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	prepErr := errors.New("something went not fine")
	p.PrepareListResult([]container.Container{{Container: types.Container{ID: "cont1"}}}, prepErr)
	query := bson.M{"id": "abc123", "hostaddr": "127.0.0.1"}
	containers, err := p.ListContainers(query)
	c.Assert(err, check.Equals, prepErr)
	c.Assert(containers, check.HasLen, 0)
	c.Assert(p.Queries(), check.DeepEquals, []bson.M{query})
}

func (s *S) TestListContainersPreparedNoResultNorError(c *check.C) {
	p, err := NewFakeDockerProvisioner()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	p.PrepareListResult(nil, nil)
	query := bson.M{"id": "abc123"}
	containers, err := p.ListContainers(query)
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 0)
	c.Assert(p.Queries(), check.DeepEquals, []bson.M{query})
}

func (s *S) TestListContainersStartedContainers(c *check.C) {
	p, err := StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	containers, err := p.StartContainers(StartContainersArgs{
		Endpoint:  p.Servers()[0].URL(),
		App:       provisiontest.NewFakeApp("myapp", "plat", 2),
		Amount:    map[string]int{"web": 2},
		Image:     "tsuru/python",
		PullImage: true,
	})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	p.PrepareListResult(nil, nil)
	containers, err = p.ListContainers(nil)
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	c.Assert(p.Queries(), check.DeepEquals, []bson.M{nil})
	coll := p.Collection()
	defer coll.Close()
	count, err := coll.Find(nil).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
}

func (s *S) TestListContainersStartedAndPreparedContainers(c *check.C) {
	p, err := StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	containers, err := p.StartContainers(StartContainersArgs{
		Endpoint:  p.Servers()[0].URL(),
		App:       provisiontest.NewFakeApp("myapp", "plat", 2),
		Amount:    map[string]int{"web": 2},
		Image:     "tsuru/python",
		PullImage: true,
	})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	p.PrepareListResult([]container.Container{{Container: types.Container{ID: containers[0].ID}}}, nil)
	containers, err = p.ListContainers(nil)
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	c.Assert(p.Queries(), check.DeepEquals, []bson.M{nil})
	coll := p.Collection()
	defer coll.Close()
	count, err := coll.Find(nil).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 1)
}

func (s *S) TestStartContainers(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	p, err := StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	containers, err := p.StartContainers(StartContainersArgs{
		Amount:    map[string]int{"web": 2, "worker": 1},
		Image:     "tsuru/python",
		PullImage: true,
		Endpoint:  p.Servers()[0].URL(),
		App:       app,
	})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 3)
	c.Assert(p.Containers(net.URLToHost(p.Servers()[0].URL())), check.DeepEquals, containers)
}
