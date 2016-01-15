// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/api"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/iaas"
	tsuruIo "github.com/tsuru/tsuru/io"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/bs"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/healer"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/tsurutest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type TestIaaS struct{}

func (TestIaaS) DeleteMachine(m *iaas.Machine) error {
	return nil
}

func createToken(c *check.C) auth.Token {
	user := &auth.User{Email: "provisioner-docker@groundcontrol.com", Password: "123456", Quota: quota.Unlimited}
	nativeScheme.Remove(user)
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	return createTokenForUser(user, c)
}

func createTokenForUser(user *auth.User, c *check.C) auth.Token {
	token, err := nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	role, err := permission.NewRole("provisioner-docker", string(permission.CtxGlobal))
	c.Assert(err, check.IsNil)
	err = role.AddPermissions("*")
	c.Assert(err, check.IsNil)
	err = user.AddRole(role.Name, "")
	c.Assert(err, check.IsNil)
	return token
}

func (TestIaaS) CreateMachine(params map[string]string) (*iaas.Machine, error) {
	m := iaas.Machine{
		Id:      params["id"],
		Status:  "running",
		Address: "127.0.0.1",
	}
	return &m, nil
}

func (TestIaaS) Describe() string {
	return "my iaas description"
}

func newTestIaaS(string) iaas.IaaS {
	return TestIaaS{}
}

type HandlersSuite struct {
	conn        *db.Storage
	user        *auth.User
	token       auth.Token
	team        *auth.Team
	clusterSess *mgo.Session
}

var _ = check.Suite(&HandlersSuite{})
var nativeScheme = auth.ManagedScheme(native.NativeScheme{})

func (s *HandlersSuite) SetUpSuite(c *check.C) {
	config.Set("database:name", "docker_provision_handlers_tests_s")
	config.Set("docker:collection", "docker_handler_suite")
	config.Set("docker:run-cmd:port", 8888)
	config.Set("docker:router", "fake")
	config.Set("docker:cluster:mongo-url", "127.0.0.1:27017")
	config.Set("docker:cluster:mongo-database", "docker_provision_handlers_tests_cluster_stor")
	config.Set("docker:repository-namespace", "tsuru")
	config.Set("queue:mongo-url", "127.0.0.1:27017")
	config.Set("queue:mongo-database", "queue_provision_docker_tests_handlers")
	config.Set("iaas:default", "test-iaas")
	config.Set("iaas:node-protocol", "http")
	config.Set("iaas:node-port", 1234)
	config.Set("routers:fake:type", "fake")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	clusterDbUrl, _ := config.GetString("docker:cluster:mongo-url")
	s.clusterSess, err = mgo.Dial(clusterDbUrl)
	c.Assert(err, check.IsNil)
	pools, err := provision.ListPools(nil)
	c.Assert(err, check.IsNil)
	for _, pool := range pools {
		err = provision.RemovePool(pool.Name)
		c.Assert(err, check.IsNil)
	}
	app.AuthScheme = nativeScheme
	s.team = &auth.Team{Name: "admin"}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, check.IsNil)
}

func (s *HandlersSuite) SetUpTest(c *check.C) {
	config.Set("docker:api-timeout", 2)
	queue.ResetQueue()
	err := clearClusterStorage(s.clusterSess)
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = &dockerProvisioner{}
	err = mainDockerProvisioner.Initialize()
	c.Assert(err, check.IsNil)
	coll := mainDockerProvisioner.Collection()
	defer coll.Close()
	err = dbtest.ClearAllCollectionsExcept(coll.Database, []string{"users", "teams"})
	c.Assert(err, check.IsNil)
	s.token = createToken(c)
	s.user, err = s.token.User()
	c.Assert(err, check.IsNil)
}

func (s *HandlersSuite) TearDownSuite(c *check.C) {
	defer s.clusterSess.Close()
	defer s.conn.Close()
	coll := mainDockerProvisioner.Collection()
	defer coll.Close()
	coll.Database.DropDatabase()
	databaseName, _ := config.GetString("docker:cluster:mongo-database")
	s.clusterSess.DB(databaseName).DropDatabase()
}

func (s *HandlersSuite) startFakeDockerNode(c *check.C) (*testing.DockerServer, func()) {
	pong := make(chan struct{})
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		if strings.Contains(r.URL.Path, "ping") {
			close(pong)
		}
	})
	c.Assert(err, check.IsNil)
	url, err := url.Parse(server.URL())
	c.Assert(err, check.IsNil)
	_, portString, _ := net.SplitHostPort(url.Host)
	port, _ := strconv.Atoi(portString)
	config.Set("iaas:node-port", port)
	return server, func() {
		<-pong
		queue.ResetQueue()
	}
}

func (s *HandlersSuite) TestAddNodeHandler(c *check.C) {
	server, waitQueue := s.startFakeDockerNode(c)
	defer server.Stop()
	mainDockerProvisioner.cluster, _ = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{})
	opts := provision.AddPoolOptions{Name: "pool1"}
	err := provision.AddPool(opts)
	defer provision.RemovePool("pool1")
	json := fmt.Sprintf(`{"address": "%s", "pool": "pool1"}`, server.URL())
	b := bytes.NewBufferString(json)
	req, err := http.NewRequest("POST", "/docker/node?register=true", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = addNodeHandler(rec, req, s.token)
	c.Assert(err, check.IsNil)
	waitQueue()
	nodes, err := mainDockerProvisioner.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address, check.Equals, server.URL())
	c.Assert(nodes[0].Metadata, check.DeepEquals, map[string]string{
		"pool":        "pool1",
		"LastSuccess": nodes[0].Metadata["LastSuccess"],
	})
	c.Assert(nodes[0].CreationStatus, check.Equals, cluster.NodeCreationStatusCreated)
}

func (s *HandlersSuite) TestAddNodeHandlerCreatingAnIaasMachine(c *check.C) {
	server, waitQueue := s.startFakeDockerNode(c)
	defer server.Stop()
	iaas.RegisterIaasProvider("test-iaas", newTestIaaS)
	mainDockerProvisioner.cluster, _ = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{})
	opts := provision.AddPoolOptions{Name: "pool1"}
	err := provision.AddPool(opts)
	defer provision.RemovePool("pool1")
	b := bytes.NewBufferString(`{"pool": "pool1", "id": "test1"}`)
	req, err := http.NewRequest("POST", "/docker/node?register=false", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = addNodeHandler(rec, req, s.token)
	c.Assert(err, check.IsNil)
	var result map[string]string
	err = json.NewDecoder(rec.Body).Decode(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, map[string]string{"description": "my iaas description"})
	nodes, err := mainDockerProvisioner.Cluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address, check.Equals, strings.TrimRight(server.URL(), "/"))
	c.Assert(nodes[0].CreationStatus, check.Equals, cluster.NodeCreationStatusPending)
	waitQueue()
	nodes, err = mainDockerProvisioner.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address, check.Equals, strings.TrimRight(server.URL(), "/"))
	c.Assert(nodes[0].Metadata, check.DeepEquals, map[string]string{
		"id":          "test1",
		"pool":        "pool1",
		"iaas":        "test-iaas",
		"iaas-id":     "test1",
		"LastSuccess": nodes[0].Metadata["LastSuccess"],
	})
	c.Assert(nodes[0].CreationStatus, check.Equals, cluster.NodeCreationStatusCreated)
}

func (s *HandlersSuite) TestAddNodeHandlerCreatingAnIaasMachineExplicit(c *check.C) {
	server, waitQueue := s.startFakeDockerNode(c)
	defer server.Stop()
	iaas.RegisterIaasProvider("test-iaas", newTestIaaS)
	iaas.RegisterIaasProvider("another-test-iaas", newTestIaaS)
	mainDockerProvisioner.cluster, _ = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{})
	opts := provision.AddPoolOptions{Name: "pool1"}
	err := provision.AddPool(opts)
	defer provision.RemovePool("pool1")
	b := bytes.NewBufferString(`{"pool": "pool1", "id": "test1", "iaas": "another-test-iaas"}`)
	req, err := http.NewRequest("POST", "/docker/node?register=false", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = addNodeHandler(rec, req, s.token)
	c.Assert(err, check.IsNil)
	waitQueue()
	nodes, err := mainDockerProvisioner.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address, check.Equals, strings.TrimRight(server.URL(), "/"))
	c.Assert(nodes[0].Metadata, check.DeepEquals, map[string]string{
		"id":          "test1",
		"pool":        "pool1",
		"iaas":        "another-test-iaas",
		"iaas-id":     "test1",
		"LastSuccess": nodes[0].Metadata["LastSuccess"],
	})
}

func (s *HandlersSuite) TestAddNodeHandlerWithoutCluster(c *check.C) {
	server, waitQueue := s.startFakeDockerNode(c)
	defer server.Stop()
	opts := provision.AddPoolOptions{Name: "pool1"}
	err := provision.AddPool(opts)
	defer provision.RemovePool("pool1")
	config.Set("docker:cluster:redis-server", "127.0.0.1:6379")
	defer config.Unset("docker:cluster:redis-server")
	b := bytes.NewBufferString(fmt.Sprintf(`{"address": "%s", "pool": "pool1"}`, server.URL()))
	req, err := http.NewRequest("POST", "/docker/node?register=true", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = addNodeHandler(rec, req, s.token)
	c.Assert(err, check.IsNil)
	waitQueue()
	nodes, err := mainDockerProvisioner.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address, check.Equals, server.URL())
	c.Assert(nodes[0].Metadata, check.DeepEquals, map[string]string{
		"pool":        "pool1",
		"LastSuccess": nodes[0].Metadata["LastSuccess"],
	})
}

func (s *HandlersSuite) TestAddNodeHandlerWithoutAddress(c *check.C) {
	config.Set("docker:cluster:redis-server", "127.0.0.1:6379")
	defer config.Unset("docker:cluster:redis-server")
	b := bytes.NewBufferString(`{"pool": "pool1"}`)
	req, err := http.NewRequest("POST", "/docker/node?register=true", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = addNodeHandler(rec, req, s.token)
	var result map[string]string
	err = json.NewDecoder(rec.Body).Decode(&result)
	c.Assert(err, check.IsNil)
	c.Assert(rec.Code, check.Equals, http.StatusBadRequest)
	c.Assert(result["error"], check.Matches, "address=url parameter is required")
}

func (s *HandlersSuite) TestAddNodeHandlerWithInvalidURLAddress(c *check.C) {
	config.Set("docker:cluster:redis-server", "127.0.0.1:6379")
	defer config.Unset("docker:cluster:redis-server")
	b := bytes.NewBufferString(`{"address": "/invalid", "pool": "pool1"}`)
	req, err := http.NewRequest("POST", "/docker/node?register=true", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = addNodeHandler(rec, req, s.token)
	c.Assert(err, check.IsNil)
	var result map[string]string
	err = json.NewDecoder(rec.Body).Decode(&result)
	c.Assert(err, check.IsNil)
	c.Assert(rec.Code, check.Equals, http.StatusBadRequest)
	c.Assert(result["error"], check.Matches, "Invalid address url: host cannot be empty")
	b = bytes.NewBufferString(`{"address": "xxx://abc/invalid", "pool": "pool1"}`)
	req, err = http.NewRequest("POST", "/docker/node?register=true", b)
	c.Assert(err, check.IsNil)
	rec = httptest.NewRecorder()
	err = addNodeHandler(rec, req, s.token)
	c.Assert(err, check.IsNil)
	err = json.NewDecoder(rec.Body).Decode(&result)
	c.Assert(err, check.IsNil)
	c.Assert(rec.Code, check.Equals, http.StatusBadRequest)
	c.Assert(result["error"], check.Matches, `Invalid address url: scheme must be http\[s\]`)
}

func (s *HandlersSuite) TestAddNodeHandlerNoPool(c *check.C) {
	config.Set("docker:cluster:redis-server", "127.0.0.1:6379")
	defer config.Unset("docker:cluster:redis-server")
	b := bytes.NewBufferString(`{"address": "http://192.168.50.4:2375"}`)
	req, err := http.NewRequest("POST", "/docker/node?register=true", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = addNodeHandler(rec, req, s.token)
	c.Assert(err, check.IsNil)
	var result map[string]string
	err = json.NewDecoder(rec.Body).Decode(&result)
	c.Assert(err, check.IsNil)
	c.Assert(rec.Code, check.Equals, http.StatusBadRequest)
	c.Assert(result["error"], check.Matches, `pool is required`)
}

func (s *HandlersSuite) TestValidateNodeAddress(c *check.C) {
	err := validateNodeAddress("/invalid")
	c.Assert(err, check.ErrorMatches, "Invalid address url: host cannot be empty")
	err = validateNodeAddress("xxx://abc/invalid")
	c.Assert(err, check.ErrorMatches, `Invalid address url: scheme must be http\[s\]`)
	err = validateNodeAddress("")
	c.Assert(err, check.ErrorMatches, "address=url parameter is required")
}

func (s *HandlersSuite) TestRemoveNodeHandler(c *check.C) {
	var err error
	mainDockerProvisioner.cluster, err = cluster.New(nil, &cluster.MapStorage{})
	c.Assert(err, check.IsNil)
	err = mainDockerProvisioner.Cluster().Register(cluster.Node{Address: "host.com:2375"})
	c.Assert(err, check.IsNil)
	b := bytes.NewBufferString(`{"address": "host.com:2375"}`)
	req, err := http.NewRequest("POST", "/node/remove", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = removeNodeHandler(rec, req, s.token)
	c.Assert(err, check.IsNil)
	nodes, err := mainDockerProvisioner.Cluster().Nodes()
	c.Assert(len(nodes), check.Equals, 0)
}

func (s *HandlersSuite) TestRemoveNodeHandlerWithoutRemoveIaaS(c *check.C) {
	iaas.RegisterIaasProvider("some-iaas", newTestIaaS)
	machine, err := iaas.CreateMachineForIaaS("some-iaas", map[string]string{})
	c.Assert(err, check.IsNil)
	mainDockerProvisioner.cluster, err = cluster.New(nil, &cluster.MapStorage{})
	c.Assert(err, check.IsNil)
	err = mainDockerProvisioner.Cluster().Register(cluster.Node{
		Address: fmt.Sprintf("http://%s:2375", machine.Address),
	})
	c.Assert(err, check.IsNil)
	b := bytes.NewBufferString(fmt.Sprintf(`{"address": "http://%s:2375", "remove_iaas": "false"}`, machine.Address))
	req, err := http.NewRequest("POST", "/node/remove", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = removeNodeHandler(rec, req, s.token)
	c.Assert(err, check.IsNil)
	nodes, err := mainDockerProvisioner.Cluster().Nodes()
	c.Assert(len(nodes), check.Equals, 0)
	dbM, err := iaas.FindMachineById(machine.Id)
	c.Assert(err, check.IsNil)
	c.Assert(dbM.Id, check.Equals, machine.Id)
}

func (s *HandlersSuite) TestRemoveNodeHandlerRemoveIaaS(c *check.C) {
	iaas.RegisterIaasProvider("my-xxx-iaas", newTestIaaS)
	machine, err := iaas.CreateMachineForIaaS("my-xxx-iaas", map[string]string{})
	c.Assert(err, check.IsNil)
	mainDockerProvisioner.cluster, err = cluster.New(nil, &cluster.MapStorage{})
	c.Assert(err, check.IsNil)
	err = mainDockerProvisioner.Cluster().Register(cluster.Node{
		Address: fmt.Sprintf("http://%s:2375", machine.Address),
	})
	c.Assert(err, check.IsNil)
	b := bytes.NewBufferString(fmt.Sprintf(`{"address": "http://%s:2375", "remove_iaas": "true"}`, machine.Address))
	req, err := http.NewRequest("POST", "/node/remove", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = removeNodeHandler(rec, req, s.token)
	c.Assert(err, check.IsNil)
	nodes, err := mainDockerProvisioner.Cluster().Nodes()
	c.Assert(len(nodes), check.Equals, 0)
	_, err = iaas.FindMachineById(machine.Id)
	c.Assert(err, check.Equals, mgo.ErrNotFound)
}

func (s *S) TestRemoveNodeHandlerRebalanceContainers(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = p
	err = s.newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	coll := p.Collection()
	defer coll.Close()
	coll.Insert(container.Container{ID: "container-id", AppName: appInstance.GetName(), Version: "container-version", Image: "tsuru/python", ProcessName: "web"})
	defer coll.RemoveAll(bson.M{"appname": appInstance.GetName()})
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	nodes, err := mainDockerProvisioner.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(len(nodes), check.Equals, 2)
	units, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "127.0.0.1",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 5}},
		app:         appInstance,
		imageId:     imageId,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	appStruct := &app.App{
		Name:     appInstance.GetName(),
		Platform: appInstance.GetPlatform(),
	}
	err = s.storage.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	err = s.storage.Apps().Update(
		bson.M{"name": appStruct.Name},
		bson.M{"$set": bson.M{"units": units}},
	)
	c.Assert(err, check.IsNil)
	b := bytes.NewBufferString(fmt.Sprintf(`{"address": "%s"}`, nodes[0].Address))
	req, err := http.NewRequest("POST", "/node/remove", b)
	c.Assert(err, check.IsNil)
	rec := tsurutest.NewSafeResponseRecorder()
	err = removeNodeHandler(rec, req, s.token)
	c.Assert(err, check.IsNil)
	nodes, err = mainDockerProvisioner.Cluster().Nodes()
	c.Assert(len(nodes), check.Equals, 1)
	containerList, err := mainDockerProvisioner.listContainersByHost(tsuruNet.URLToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	c.Assert(len(containerList), check.Equals, 5)
}

func (s *S) TestRemoveNodeHandlerNoRebalanceContainers(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = p
	err = s.newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	coll := p.Collection()
	defer coll.Close()
	coll.Insert(container.Container{
		ID:          "container-id",
		AppName:     appInstance.GetName(),
		Version:     "container-version",
		Image:       "tsuru/python",
		ProcessName: "web",
	})
	defer coll.RemoveAll(bson.M{"appname": appInstance.GetName()})
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	nodes, err := mainDockerProvisioner.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(len(nodes), check.Equals, 2)
	units, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "127.0.0.1",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 5}},
		app:         appInstance,
		imageId:     imageId,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	appStruct := &app.App{
		Name:     appInstance.GetName(),
		Platform: appInstance.GetPlatform(),
	}
	err = s.storage.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	err = s.storage.Apps().Update(
		bson.M{"name": appStruct.Name},
		bson.M{"$set": bson.M{"units": units}},
	)
	c.Assert(err, check.IsNil)
	b := bytes.NewBufferString(fmt.Sprintf(`{"address": "%s"}`, nodes[0].Address))
	req, err := http.NewRequest("POST", "/node/remove?no-rebalance=true", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = removeNodeHandler(rec, req, s.token)
	c.Assert(err, check.IsNil)
	nodes, err = mainDockerProvisioner.Cluster().Nodes()
	c.Assert(len(nodes), check.Equals, 1)
	containerList, err := mainDockerProvisioner.listContainersByHost(tsuruNet.URLToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	c.Assert(len(containerList), check.Equals, 0)
}

func (s *HandlersSuite) TestListNodeHandler(c *check.C) {
	var result struct {
		Nodes    []cluster.Node `json:"nodes"`
		Machines []iaas.Machine `json:"machines"`
	}
	var err error
	mainDockerProvisioner.cluster, err = cluster.New(nil, &cluster.MapStorage{})
	c.Assert(err, check.IsNil)
	err = mainDockerProvisioner.Cluster().Register(cluster.Node{
		Address:  "host1.com:2375",
		Metadata: map[string]string{"pool": "pool1"},
	})
	c.Assert(err, check.IsNil)
	err = mainDockerProvisioner.Cluster().Register(cluster.Node{
		Address:  "host2.com:2375",
		Metadata: map[string]string{"pool": "pool2", "foo": "bar"},
	})
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest("GET", "/node/", nil)
	rec := httptest.NewRecorder()
	err = listNodesHandler(rec, req, s.token)
	c.Assert(err, check.IsNil)
	body, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, check.IsNil)
	err = json.Unmarshal(body, &result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Nodes[0].Address, check.Equals, "host1.com:2375")
	c.Assert(result.Nodes[0].Metadata, check.DeepEquals, map[string]string{"pool": "pool1"})
	c.Assert(result.Nodes[1].Address, check.Equals, "host2.com:2375")
	c.Assert(result.Nodes[1].Metadata, check.DeepEquals, map[string]string{"pool": "pool2", "foo": "bar"})
}

func (s *HandlersSuite) TestListContainersByHostHandler(c *check.C) {
	var result []container.Container
	var err error
	mainDockerProvisioner.cluster, err = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{})
	c.Assert(err, check.IsNil)
	mainDockerProvisioner.cluster.Register(cluster.Node{Address: "http://node1.company:4243"})
	coll := mainDockerProvisioner.Collection()
	defer coll.Close()
	err = coll.Insert(container.Container{ID: "blabla", Type: "python", HostAddr: "node1.company"})
	c.Assert(err, check.IsNil)
	defer coll.Remove(bson.M{"id": "blabla"})
	err = coll.Insert(container.Container{ID: "bleble", Type: "java", HostAddr: "node1.company"})
	c.Assert(err, check.IsNil)
	defer coll.Remove(bson.M{"id": "bleble"})
	req, err := http.NewRequest("GET", "/docker/node/http://node1.company:4243/containers", nil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	server := api.RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, check.IsNil)
	err = json.Unmarshal(body, &result)
	c.Assert(err, check.IsNil)
	c.Assert(result[0].ID, check.Equals, "blabla")
	c.Assert(result[0].Type, check.Equals, "python")
	c.Assert(result[0].HostAddr, check.Equals, "node1.company")
	c.Assert(result[1].ID, check.Equals, "bleble")
	c.Assert(result[1].Type, check.Equals, "java")
	c.Assert(result[1].HostAddr, check.Equals, "node1.company")
}

func (s *HandlersSuite) TestListContainersByAppHandler(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Insert(app.App{Name: "appbla", Platform: "python"})
	var result []container.Container
	mainDockerProvisioner.cluster, err = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{})
	coll := mainDockerProvisioner.Collection()
	defer coll.Close()
	err = coll.Insert(container.Container{ID: "blabla", AppName: "appbla", HostAddr: "http://node.company"})
	c.Assert(err, check.IsNil)
	defer coll.Remove(bson.M{"id": "blabla"})
	err = coll.Insert(container.Container{ID: "bleble", AppName: "appbla", HostAddr: "http://node.company"})
	c.Assert(err, check.IsNil)
	defer coll.Remove(bson.M{"id": "bleble"})
	req, err := http.NewRequest("GET", "/node/appbla/containers?:appname=appbla", nil)
	rec := httptest.NewRecorder()
	err = listContainersHandler(rec, req, s.token)
	c.Assert(err, check.IsNil)
	body, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, check.IsNil)
	err = json.Unmarshal(body, &result)
	c.Assert(err, check.IsNil)
	c.Assert(result[0].ID, check.DeepEquals, "blabla")
	c.Assert(result[0].AppName, check.DeepEquals, "appbla")
	c.Assert(result[0].HostAddr, check.DeepEquals, "http://node.company")
	c.Assert(result[1].ID, check.DeepEquals, "bleble")
	c.Assert(result[1].AppName, check.DeepEquals, "appbla")
	c.Assert(result[1].HostAddr, check.DeepEquals, "http://node.company")
}

func (s *HandlersSuite) TestMoveContainersEmptyBodyHandler(c *check.C) {
	recorder := httptest.NewRecorder()
	b := bytes.NewBufferString(``)
	request, err := http.NewRequest("POST", "/docker/containers/move", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	c.Assert(string(body), check.Equals, "unexpected end of JSON input\n")
}

func (s *HandlersSuite) TestMoveContainersEmptyToHandler(c *check.C) {
	recorder := httptest.NewRecorder()
	b := bytes.NewBufferString(`{"from": "fromhost", "to": ""}`)
	request, err := http.NewRequest("POST", "/docker/containers/move", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	c.Assert(string(body), check.Equals, "Invalid params: from: fromhost - to: \n")
}

func (s *HandlersSuite) TestMoveContainersHandler(c *check.C) {
	recorder := httptest.NewRecorder()
	b := bytes.NewBufferString(`{"from": "localhost", "to": "127.0.0.1"}`)
	request, err := http.NewRequest("POST", "/docker/containers/move", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	mainDockerProvisioner.Cluster().Register(cluster.Node{Address: "http://localhost:2375"})
	mainDockerProvisioner.Cluster().Register(cluster.Node{Address: "http://127.0.0.1:2375"})
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	validJson := fmt.Sprintf("[%s]", strings.Replace(strings.Trim(string(body), "\n "), "\n", ",", -1))
	var result []tsuruIo.SimpleJsonMessage
	err = json.Unmarshal([]byte(validJson), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, []tsuruIo.SimpleJsonMessage{
		{Message: "No units to move in localhost\n"},
		{Message: "Containers moved successfully!\n"},
	})
}

func (s *HandlersSuite) TestMoveContainerHandlerNotFound(c *check.C) {
	recorder := httptest.NewRecorder()
	mainDockerProvisioner.Cluster().Register(cluster.Node{Address: "http://127.0.0.1:2375"})
	b := bytes.NewBufferString(`{"to": "127.0.0.1"}`)
	request, err := http.NewRequest("POST", "/docker/container/myid/move", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestRebalanceContainersEmptyBodyHandler(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = p
	err = s.newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	coll := p.Collection()
	defer coll.Close()
	coll.Insert(container.Container{ID: "container-id", AppName: appInstance.GetName(), Version: "container-version", Image: "tsuru/python", ProcessName: "web"})
	defer coll.RemoveAll(bson.M{"appname": appInstance.GetName()})
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	units, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 5}},
		app:         appInstance,
		imageId:     imageId,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)

	appStruct := &app.App{
		Name:     appInstance.GetName(),
		Platform: appInstance.GetPlatform(),
	}
	err = s.storage.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	err = s.storage.Apps().Update(
		bson.M{"name": appStruct.Name},
		bson.M{"$set": bson.M{"units": units}},
	)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	b := bytes.NewBufferString("")
	request, err := http.NewRequest("POST", "/docker/containers/rebalance", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	validJson := fmt.Sprintf("[%s]", strings.Replace(strings.Trim(string(body), "\n "), "\n", ",", -1))
	var result []tsuruIo.SimpleJsonMessage
	err = json.Unmarshal([]byte(validJson), &result)
	c.Assert(err, check.IsNil)
	c.Assert(len(result), check.Equals, 14)
	c.Assert(result[0].Message, check.Equals, "Rebalancing 6 units...\n")
	c.Assert(result[1].Message, check.Matches, "(?s)Moving unit .*")
	c.Assert(result[13].Message, check.Equals, "Containers successfully rebalanced!\n")
}

func (s *S) TestRebalanceContainersFilters(c *check.C) {
	p, err := s.startMultipleServersClusterSeggregated()
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = p
	err = s.newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	coll := p.Collection()
	defer coll.Close()
	defer coll.RemoveAll(bson.M{"appname": appInstance.GetName()})
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	units, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 5}},
		app:         appInstance,
		imageId:     imageId,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	appStruct := &app.App{
		Name:     appInstance.GetName(),
		Platform: appInstance.GetPlatform(),
	}
	err = s.storage.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	err = s.storage.Apps().Update(
		bson.M{"name": appStruct.Name},
		bson.M{"$set": bson.M{"units": units}},
	)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	b := bytes.NewBufferString(`{"metadataFilter": {"pool": "pool1"}}`)
	request, err := http.NewRequest("POST", "/docker/containers/rebalance", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	validJson := fmt.Sprintf("[%s]", strings.Replace(strings.Trim(string(body), "\n "), "\n", ",", -1))
	var result []tsuruIo.SimpleJsonMessage
	err = json.Unmarshal([]byte(validJson), &result)
	c.Assert(err, check.IsNil)
	c.Assert(len(result), check.Equals, 2)
	c.Assert(result[0].Message, check.Equals, "No containers found to rebalance\n")
	c.Assert(result[1].Message, check.Equals, "Containers successfully rebalanced!\n")
}

func (s *S) TestRebalanceContainersDryBodyHandler(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = p
	err = s.newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	coll := p.Collection()
	defer coll.Close()
	coll.Insert(container.Container{ID: "container-id", AppName: appInstance.GetName(), Version: "container-version", Image: "tsuru/python", ProcessName: "web"})
	defer coll.RemoveAll(bson.M{"appname": appInstance.GetName()})
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	units, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 5}},
		app:         appInstance,
		imageId:     imageId,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	appStruct := &app.App{
		Name:     appInstance.GetName(),
		Platform: appInstance.GetPlatform(),
	}
	err = s.storage.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	err = s.storage.Apps().Update(
		bson.M{"name": appStruct.Name},
		bson.M{"$set": bson.M{"units": units}},
	)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	b := bytes.NewBufferString(`{"dry": "true"}`)
	request, err := http.NewRequest("POST", "/docker/containers/rebalance", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	validJson := fmt.Sprintf("[%s]", strings.Replace(strings.Trim(string(body), "\n "), "\n", ",", -1))
	var result []tsuruIo.SimpleJsonMessage
	err = json.Unmarshal([]byte(validJson), &result)
	c.Assert(err, check.IsNil)
	c.Assert(len(result), check.Equals, 8)
	c.Assert(result[0].Message, check.Equals, "Rebalancing 6 units...\n")
	c.Assert(result[1].Message, check.Matches, "(?s)Would move unit .*")
	c.Assert(result[7].Message, check.Equals, "Containers successfully rebalanced!\n")
}

func (s *HandlersSuite) TestHealingHistoryHandler(c *check.C) {
	evt1, err := healer.NewHealingEvent(cluster.Node{Address: "addr1"})
	c.Assert(err, check.IsNil)
	evt1.Update(cluster.Node{Address: "addr2"}, nil)
	evt2, err := healer.NewHealingEvent(cluster.Node{Address: "addr3"})
	evt2.Update(cluster.Node{}, errors.New("some error"))
	evt3, err := healer.NewHealingEvent(container.Container{ID: "1234"})
	evt3.Update(container.Container{ID: "9876"}, nil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/docker/healing", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body := recorder.Body.Bytes()
	var healings []healer.HealingEvent
	err = json.Unmarshal(body, &healings)
	c.Assert(err, check.IsNil)
	c.Assert(healings, check.HasLen, 3)
	c.Assert(healings[2].StartTime.UTC().Format(time.Stamp), check.Equals, evt1.StartTime.UTC().Format(time.Stamp))
	c.Assert(healings[2].EndTime.UTC().Format(time.Stamp), check.Equals, evt1.EndTime.UTC().Format(time.Stamp))
	c.Assert(healings[2].FailingNode.Address, check.Equals, "addr1")
	c.Assert(healings[2].CreatedNode.Address, check.Equals, "addr2")
	c.Assert(healings[2].Error, check.Equals, "")
	c.Assert(healings[2].Successful, check.Equals, true)
	c.Assert(healings[2].Action, check.Equals, "node-healing")
	c.Assert(healings[1].FailingNode.Address, check.Equals, "addr3")
	c.Assert(healings[1].CreatedNode.Address, check.Equals, "")
	c.Assert(healings[1].Error, check.Equals, "some error")
	c.Assert(healings[1].Successful, check.Equals, false)
	c.Assert(healings[1].Action, check.Equals, "node-healing")
	c.Assert(healings[0].FailingContainer.ID, check.Equals, "1234")
	c.Assert(healings[0].CreatedContainer.ID, check.Equals, "9876")
	c.Assert(healings[0].Successful, check.Equals, true)
	c.Assert(healings[0].Error, check.Equals, "")
	c.Assert(healings[0].Action, check.Equals, "container-healing")
}

func (s *HandlersSuite) TestHealingHistoryHandlerFilterContainer(c *check.C) {
	evt1, err := healer.NewHealingEvent(cluster.Node{Address: "addr1"})
	c.Assert(err, check.IsNil)
	evt1.Update(cluster.Node{Address: "addr2"}, nil)
	evt2, err := healer.NewHealingEvent(cluster.Node{Address: "addr3"})
	evt2.Update(cluster.Node{}, errors.New("some error"))
	evt3, err := healer.NewHealingEvent(container.Container{ID: "1234"})
	evt3.Update(container.Container{ID: "9876"}, nil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/docker/healing?filter=container", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body := recorder.Body.Bytes()
	var healings []healer.HealingEvent
	err = json.Unmarshal(body, &healings)
	c.Assert(err, check.IsNil)
	c.Assert(healings, check.HasLen, 1)
	c.Assert(healings[0].FailingContainer.ID, check.Equals, "1234")
	c.Assert(healings[0].CreatedContainer.ID, check.Equals, "9876")
	c.Assert(healings[0].Successful, check.Equals, true)
	c.Assert(healings[0].Error, check.Equals, "")
	c.Assert(healings[0].Action, check.Equals, "container-healing")
}

func (s *HandlersSuite) TestHealingHistoryHandlerFilterNode(c *check.C) {
	evt1, err := healer.NewHealingEvent(cluster.Node{Address: "addr1"})
	c.Assert(err, check.IsNil)
	evt1.Update(cluster.Node{Address: "addr2"}, nil)
	evt2, err := healer.NewHealingEvent(cluster.Node{Address: "addr3"})
	evt2.Update(cluster.Node{}, errors.New("some error"))
	evt3, err := healer.NewHealingEvent(container.Container{ID: "1234"})
	evt3.Update(container.Container{ID: "9876"}, nil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/docker/healing?filter=node", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body := recorder.Body.Bytes()
	var healings []healer.HealingEvent
	err = json.Unmarshal(body, &healings)
	c.Assert(err, check.IsNil)
	c.Assert(healings, check.HasLen, 2)
	c.Assert(healings[0].Action, check.Equals, "node-healing")
	c.Assert(healings[0].ID, check.Equals, evt2.ID)
	c.Assert(healings[1].Action, check.Equals, "node-healing")
	c.Assert(healings[1].ID, check.Equals, evt1.ID)
}

func (s *HandlersSuite) TestAutoScaleHistoryHandler(c *check.C) {
	evt1, err := newAutoScaleEvent("poolx", nil)
	c.Assert(err, check.IsNil)
	err = evt1.update("add", "reason 1")
	c.Assert(err, check.IsNil)
	err = evt1.finish(nil)
	c.Assert(err, check.IsNil)
	evt1.logMsg("my evt1")
	time.Sleep(100 * time.Millisecond)
	evt2, err := newAutoScaleEvent("pooly", nil)
	c.Assert(err, check.IsNil)
	err = evt2.update("rebalance", "reason 2")
	c.Assert(err, check.IsNil)
	err = evt2.finish(errors.New("my error"))
	c.Assert(err, check.IsNil)
	evt2.logMsg("my evt2")
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/docker/autoscale", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body := recorder.Body.Bytes()
	history := []autoScaleEvent{}
	err = json.Unmarshal(body, &history)
	c.Assert(err, check.IsNil)
	c.Assert(history, check.HasLen, 2)
	c.Assert(evt1.StartTime.Sub(history[1].StartTime) < time.Second, check.Equals, true)
	c.Assert(evt2.StartTime.Sub(history[0].StartTime) < time.Second, check.Equals, true)
	c.Assert(evt1.MetadataValue, check.Equals, history[1].MetadataValue)
	c.Assert(evt2.MetadataValue, check.Equals, history[0].MetadataValue)
	c.Assert(evt1.Action, check.Equals, history[1].Action)
	c.Assert(evt2.Action, check.Equals, history[0].Action)
	c.Assert(evt1.Reason, check.Equals, history[1].Reason)
	c.Assert(evt2.Reason, check.Equals, history[0].Reason)
	c.Assert(evt1.Log, check.Equals, history[1].Log)
	c.Assert(evt2.Log, check.Equals, history[0].Log)
}

func (s *HandlersSuite) TestUpdateNodeHandler(c *check.C) {
	mainDockerProvisioner.cluster, _ = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{},
		cluster.Node{Address: "localhost:1999", Metadata: map[string]string{
			"m1": "v1",
			"m2": "v2",
		}},
	)
	opts := provision.AddPoolOptions{Name: "pool1"}
	err := provision.AddPool(opts)
	defer provision.RemovePool("pool1")
	json := `{"address": "localhost:1999", "m1": "", "m2": "v9", "m3": "v8"}`
	b := bytes.NewBufferString(json)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/docker/node", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	nodes, err := mainDockerProvisioner.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Metadata, check.DeepEquals, map[string]string{
		"m2": "v9",
		"m3": "v8",
	})
}

func (s *HandlersSuite) TestUpdateNodeHandlerNoAddress(c *check.C) {
	mainDockerProvisioner.cluster, _ = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{},
		cluster.Node{Address: "localhost:1999", Metadata: map[string]string{
			"m1": "v1",
			"m2": "v2",
		}},
	)
	opts := provision.AddPoolOptions{Name: "pool1"}
	err := provision.AddPool(opts)
	defer provision.RemovePool("pool1")
	json := `{"m1": "", "m2": "v9", "m3": "v8"}`
	b := bytes.NewBufferString(json)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/docker/node", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *HandlersSuite) TestUpdateNodeDisableNodeHandler(c *check.C) {
	mainDockerProvisioner.cluster, _ = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{},
		cluster.Node{Address: "localhost:1999", CreationStatus: cluster.NodeCreationStatusCreated},
	)
	opts := provision.AddPoolOptions{Name: "pool1"}
	err := provision.AddPool(opts)
	defer provision.RemovePool("pool1")
	json := `{"address": "localhost:1999"}`
	b := bytes.NewBufferString(json)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/docker/node?disabled=true", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	nodes, err := mainDockerProvisioner.Cluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].CreationStatus, check.DeepEquals, cluster.NodeCreationStatusDisabled)
}

func (s *HandlersSuite) TestUpdateNodeEnableNodeHandler(c *check.C) {
	mainDockerProvisioner.cluster, _ = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{},
		cluster.Node{Address: "localhost:1999", CreationStatus: cluster.NodeCreationStatusDisabled},
	)
	opts := provision.AddPoolOptions{Name: "pool1"}
	err := provision.AddPool(opts)
	defer provision.RemovePool("pool1")
	json := `{"address": "localhost:1999"}`
	b := bytes.NewBufferString(json)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/docker/node?enabled=true", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	nodes, err := mainDockerProvisioner.Cluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].CreationStatus, check.DeepEquals, cluster.NodeStatusReady)
}

func (s *HandlersSuite) TestUpdateNodeEnableAndDisableCantBeDone(c *check.C) {
	mainDockerProvisioner.cluster, _ = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{},
		cluster.Node{Address: "localhost:1999", CreationStatus: cluster.NodeCreationStatusDisabled},
	)
	opts := provision.AddPoolOptions{Name: "pool1"}
	err := provision.AddPool(opts)
	defer provision.RemovePool("pool1")
	jsonBody := `{"address": "localhost:1999"}`
	b := bytes.NewBufferString(jsonBody)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/docker/node?enabled=true&disabled=true", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *HandlersSuite) TestAutoScaleRunHandler(c *check.C) {
	mainDockerProvisioner.cluster, _ = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{},
		cluster.Node{Address: "localhost:1999", Metadata: map[string]string{
			"pool": "pool1",
		}},
	)
	config.Set("docker:auto-scale:enabled", true)
	defer config.Unset("docker:auto-scale:enabled")
	config.Set("docker:auto-scale:group-by-metadata", "pool")
	config.Set("docker:auto-scale:max-container-count", 2)
	defer config.Unset("docker:auto-scale:max-container-count")
	defer config.Unset("docker:auto-scale:group-by-metadata")
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/docker/autoscale/run", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body := recorder.Body.String()
	parts := strings.Split(body, "\n")
	c.Assert(parts, check.DeepEquals, []string{
		`{"Message":"running scaler *docker.countScaler for \"pool\": \"pool1\"\n"}`,
		`{"Message":"nothing to do for \"pool\": \"pool1\"\n"}`,
		``,
	})
}

type bsEnvList []bs.Env

func (l bsEnvList) Len() int           { return len(l) }
func (l bsEnvList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l bsEnvList) Less(i, j int) bool { return l[i].Name < l[j].Name }

type bsPoolEnvsList []bs.PoolEnvs

func (l bsPoolEnvsList) Len() int           { return len(l) }
func (l bsPoolEnvsList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l bsPoolEnvsList) Less(i, j int) bool { return l[i].Name < l[j].Name }

func (s *HandlersSuite) TestBsEnvSetHandler(c *check.C) {
	recorder := httptest.NewRecorder()
	json := `{
		"image": "ignored",
		"envs": [
			{"name": "VAR1", "value": "VALUE1"},
			{"name": "VAR2", "value": "VALUE2"}
		],
		"pools": [
			{
				"name": "POOL1",
				"envs": [
					{"name": "VAR3", "value": "VALUE3"},
					{"name": "VAR4", "value": "VALUE4"}
				]
			},
			{
				"name": "POOL2",
				"envs": [
					{"name": "VAR5", "value": "VALUE5"},
					{"name": "VAR6", "value": "VALUE6"}
				]
			}
		]
	}`
	body := bytes.NewBufferString(json)
	request, err := http.NewRequest("POST", "/docker/bs/env", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	conf, err := bs.LoadConfig(nil)
	c.Assert(err, check.IsNil)
	c.Assert(conf.Image, check.Equals, "")
	sort.Sort(bsEnvList(conf.Envs))
	c.Assert(conf.Envs, check.DeepEquals, []bs.Env{{Name: "VAR1", Value: "VALUE1"}, {Name: "VAR2", Value: "VALUE2"}})
	c.Assert(conf.Pools, check.HasLen, 2)
	sort.Sort(bsPoolEnvsList(conf.Pools))
	sort.Sort(bsEnvList(conf.Pools[0].Envs))
	sort.Sort(bsEnvList(conf.Pools[1].Envs))
	c.Assert(conf.Pools, check.DeepEquals, []bs.PoolEnvs{
		{Name: "POOL1", Envs: []bs.Env{{Name: "VAR3", Value: "VALUE3"}, {Name: "VAR4", Value: "VALUE4"}}},
		{Name: "POOL2", Envs: []bs.Env{{Name: "VAR5", Value: "VALUE5"}, {Name: "VAR6", Value: "VALUE6"}}},
	})
}

func (s *HandlersSuite) TestBsEnvSetHandlerForbiddenVar(c *check.C) {
	recorder := httptest.NewRecorder()
	json := `{
		"image": "ignored",
		"envs": [
			{"name": "TSURU_ENDPOINT", "value": "VAL"}
		]
	}`
	body := bytes.NewBufferString(json)
	request, err := http.NewRequest("POST", "/docker/bs/env", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "cannot set TSURU_ENDPOINT variable\n")
	_, err = bs.LoadConfig(nil)
	c.Assert(err, check.ErrorMatches, "not found")
}

func (s *HandlersSuite) TestBsEnvSetHandlerUpdateExisting(c *check.C) {
	err := bs.SaveImage("myimg")
	c.Assert(err, check.IsNil)
	envMap := bs.EnvMap{"VAR1": "VAL1", "VAR2": "VAL2"}
	poolEnvMap := bs.PoolEnvMap{
		"POOL1": bs.EnvMap{"VAR3": "VAL3", "VAR4": "VAL4"},
	}
	err = bs.SaveEnvs(envMap, poolEnvMap)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	json := `{
		"image": "ignored",
		"envs": [
			{"name": "VAR1", "value": ""},
			{"name": "VAR3", "value": "VAL3"}
		],
		"pools": [
			{
				"name": "POOL1",
				"envs": [
					{"name": "VAR3", "value": ""}
				]
			}
		]
	}`
	body := bytes.NewBufferString(json)
	request, err := http.NewRequest("POST", "/docker/bs/env", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	conf, err := bs.LoadConfig(nil)
	c.Assert(err, check.IsNil)
	c.Assert(conf.Image, check.Equals, "myimg")
	sort.Sort(bsEnvList(conf.Envs))
	c.Assert(conf.Envs, check.DeepEquals, []bs.Env{{Name: "VAR2", Value: "VAL2"}, {Name: "VAR3", Value: "VAL3"}})
	c.Assert(conf.Pools, check.DeepEquals, []bs.PoolEnvs{
		{Name: "POOL1", Envs: []bs.Env{{Name: "VAR4", Value: "VAL4"}}},
	})
}

func (s *HandlersSuite) TestBsConfigGetHandler(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/docker/bs", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	expected := &bs.Config{}
	var conf bs.Config
	err = json.Unmarshal(recorder.Body.Bytes(), &conf)
	c.Assert(err, check.IsNil)
	c.Assert(conf, check.DeepEquals, *expected)
	err = bs.SaveImage("myimg")
	c.Assert(err, check.IsNil)
	envMap := bs.EnvMap{"VAR1": "VAL1", "VAR2": "VAL2"}
	poolEnvMap := bs.PoolEnvMap{
		"POOL1": bs.EnvMap{"VAR3": "VAL3", "VAR4": "VAL4"},
	}
	err = bs.SaveEnvs(envMap, poolEnvMap)
	c.Assert(err, check.IsNil)
	expected, err = bs.LoadConfig(nil)
	c.Assert(err, check.IsNil)
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	err = json.Unmarshal(recorder.Body.Bytes(), &conf)
	c.Assert(err, check.IsNil)
	c.Assert(conf, check.DeepEquals, *expected)
}

func (s *HandlersSuite) TestBsConfigGetFilteringPools(c *check.C) {
	role, err := permission.NewRole("bs-config-get", string(permission.CtxPool))
	c.Assert(err, check.IsNil)
	err = role.AddPermissions(permission.PermNodeBs.FullName())
	c.Assert(err, check.IsNil)
	user := &auth.User{Email: "provisioner-docker-bs-env@groundcontrol.com", Password: "123456", Quota: quota.Unlimited}
	nativeScheme.Remove(user)
	_, err = nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	user.AddRole(role.Name, "POOL1")
	user.AddRole(role.Name, "POOL3")
	token, err := nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/docker/bs", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	err = bs.SaveImage("myimg")
	c.Assert(err, check.IsNil)
	envMap := bs.EnvMap{"VAR1": "VAL1", "VAR2": "VAL2"}
	poolEnvMap := bs.PoolEnvMap{
		"POOL1": bs.EnvMap{"VAR3": "VAL3", "VAR4": "VAL4"},
		"POOL2": bs.EnvMap{"VAR3": "VAL4", "VAR4": "VAL5"},
		"POOL3": bs.EnvMap{"VAR3": "VAL5", "VAR4": "VAL6"},
		"POOL4": bs.EnvMap{"VAR3": "VAL7", "VAR4": "VAL7"},
	}
	err = bs.SaveEnvs(envMap, poolEnvMap)
	c.Assert(err, check.IsNil)
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	expected, err := bs.LoadConfig([]string{"POOL1", "POOL3"})
	c.Assert(err, check.IsNil)
	var conf bs.Config
	err = json.Unmarshal(recorder.Body.Bytes(), &conf)
	c.Assert(err, check.IsNil)
	c.Assert(conf, check.DeepEquals, *expected)
}

func (s *HandlersSuite) TestBsUpgradeHandler(c *check.C) {
	err := bs.SaveImage("tsuru/bs@sha256:abcef384829283eff")
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/docker/bs/upgrade", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	conf, err := bs.LoadConfig(nil)
	c.Assert(err, check.IsNil)
	c.Assert(conf.Image, check.Equals, "")
}

func (s *HandlersSuite) TestAutoScaleConfigHandler(c *check.C) {
	config.Set("docker:auto-scale:enabled", true)
	defer config.Unset("docker:auto-scale:enabled")
	expected, err := json.Marshal(mainDockerProvisioner.initAutoScaleConfig())
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/docker/autoscale/config", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, string(expected)+"\n")
}

func (s *HandlersSuite) TestAutoScaleListRulesEmpty(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/docker/autoscale/rules", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var rules []autoScaleRule
	err = json.Unmarshal(recorder.Body.Bytes(), &rules)
	c.Assert(err, check.IsNil)
	c.Assert(rules, check.DeepEquals, []autoScaleRule{
		{Enabled: true, ScaleDownRatio: 1.333, Error: "invalid rule, either memory information or max container count must be set"},
	})
}

func (s *HandlersSuite) TestAutoScaleListRulesWithLegacyConfig(c *check.C) {
	config.Set("docker:auto-scale:metadata-filter", "mypool")
	config.Set("docker:auto-scale:max-container-count", 4)
	config.Set("docker:auto-scale:scale-down-ratio", 1.5)
	config.Set("docker:auto-scale:prevent-rebalance", true)
	config.Set("docker:scheduler:max-used-memory", 0.9)
	defer config.Unset("docker:auto-scale:metadata-filter")
	defer config.Unset("docker:auto-scale:max-container-count")
	defer config.Unset("docker:auto-scale:scale-down-ratio")
	defer config.Unset("docker:auto-scale:prevent-rebalance")
	defer config.Unset("docker:scheduler:max-used-memory")
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/docker/autoscale/rules", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var rules []autoScaleRule
	err = json.Unmarshal(recorder.Body.Bytes(), &rules)
	c.Assert(err, check.IsNil)
	c.Assert(rules, check.DeepEquals, []autoScaleRule{
		{MetadataFilter: "mypool", Enabled: true, MaxContainerCount: 4, ScaleDownRatio: 1.5, PreventRebalance: true, MaxMemoryRatio: 0.9},
	})
}

func (s *HandlersSuite) TestAutoScaleListRulesWithDBConfig(c *check.C) {
	config.Set("docker:auto-scale:scale-down-ratio", 2.0)
	defer config.Unset("docker:auto-scale:max-container-count")
	config.Set("docker:scheduler:total-memory-metadata", "maxmemory")
	defer config.Unset("docker:scheduler:total-memory-metadata")
	rules := []autoScaleRule{
		{MetadataFilter: "", Enabled: true, MaxContainerCount: 10, ScaleDownRatio: 1.2},
		{MetadataFilter: "pool1", Enabled: true, ScaleDownRatio: 1.1, MaxMemoryRatio: 2.0},
	}
	for _, r := range rules {
		err := r.update()
		c.Assert(err, check.IsNil)
	}
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/docker/autoscale/rules", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var reqRules []autoScaleRule
	err = json.Unmarshal(recorder.Body.Bytes(), &reqRules)
	c.Assert(err, check.IsNil)
	c.Assert(reqRules, check.DeepEquals, rules)
}

func (s *HandlersSuite) TestAutoScaleSetRule(c *check.C) {
	config.Set("docker:scheduler:total-memory-metadata", "maxmemory")
	defer config.Unset("docker:scheduler:total-memory-metadata")
	rule := autoScaleRule{MetadataFilter: "pool1", Enabled: true, ScaleDownRatio: 1.1, MaxMemoryRatio: 2.0}
	data, err := json.Marshal(rule)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/docker/autoscale/rules", bytes.NewBuffer(data))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	rules, err := listAutoScaleRules()
	c.Assert(err, check.IsNil)
	c.Assert(rules, check.DeepEquals, []autoScaleRule{
		{Enabled: true, ScaleDownRatio: 1.333, Error: "invalid rule, either memory information or max container count must be set"},
		rule,
	})
}

func (s *HandlersSuite) TestAutoScaleSetRuleInvalidRule(c *check.C) {
	rule := autoScaleRule{MetadataFilter: "pool1", Enabled: true, ScaleDownRatio: 0.9, MaxMemoryRatio: 2.0}
	data, err := json.Marshal(rule)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/docker/autoscale/rules", bytes.NewBuffer(data))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), check.Matches, "(?s).*invalid rule, scale down ratio needs to be greater than 1.0, got 0.9.*")
}

func (s *HandlersSuite) TestAutoScaleSetRuleExisting(c *check.C) {
	rule := autoScaleRule{MetadataFilter: "", Enabled: true, ScaleDownRatio: 1.1, MaxContainerCount: 5}
	err := rule.update()
	c.Assert(err, check.IsNil)
	rule.MaxContainerCount = 9
	data, err := json.Marshal(rule)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/docker/autoscale/rules", bytes.NewBuffer(data))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	rules, err := listAutoScaleRules()
	c.Assert(err, check.IsNil)
	c.Assert(rules, check.DeepEquals, []autoScaleRule{rule})
}

func (s *HandlersSuite) TestAutoScaleDeleteRule(c *check.C) {
	rule := autoScaleRule{MetadataFilter: "", Enabled: true, ScaleDownRatio: 1.1, MaxContainerCount: 5}
	err := rule.update()
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/docker/autoscale/rules/", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	rules, err := listAutoScaleRules()
	c.Assert(err, check.IsNil)
	c.Assert(rules, check.DeepEquals, []autoScaleRule{
		{Enabled: true, ScaleDownRatio: 1.333, Error: "invalid rule, either memory information or max container count must be set"},
	})
}

func (s *HandlersSuite) TestAutoScaleDeleteRuleNonDefault(c *check.C) {
	rule := autoScaleRule{MetadataFilter: "mypool", Enabled: true, ScaleDownRatio: 1.1, MaxContainerCount: 5}
	err := rule.update()
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/docker/autoscale/rules/mypool", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	rules, err := listAutoScaleRules()
	c.Assert(err, check.IsNil)
	c.Assert(rules, check.DeepEquals, []autoScaleRule{
		{Enabled: true, ScaleDownRatio: 1.333, Error: "invalid rule, either memory information or max container count must be set"},
	})
}

func (s *HandlersSuite) TestAutoScaleDeleteRuleNotFound(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/docker/autoscale/rules/mypool", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "rule not found\n")
}
