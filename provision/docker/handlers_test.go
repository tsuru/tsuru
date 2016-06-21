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

	"github.com/cezarsa/form"
	"github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/api"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/iaas"
	tsuruIo "github.com/tsuru/tsuru/io"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/healer"
	"github.com/tsuru/tsuru/provision/docker/nodecontainer"
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
	return createTokenForUser(user, "*", string(permission.CtxGlobal), "", c)
}

func createTokenForUser(user *auth.User, perm, contextType, contextValue string, c *check.C) auth.Token {
	token, err := nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	role, err := permission.NewRole("provisioner-docker-"+user.Email+perm, contextType, "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions(perm)
	c.Assert(err, check.IsNil)
	err = user.AddRole(role.Name, contextValue)
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
	params := addNodeOptions{
		Register: true,
		Metadata: map[string]string{
			"address": server.URL(),
			"pool":    "pool1",
		},
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest("POST", "/1.0/docker/node", strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", s.token.GetValue())
	rec := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusCreated)
	c.Assert(rec.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
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
	params := addNodeOptions{
		Register: false,
		Metadata: map[string]string{
			"id":   "test1",
			"pool": "pool1",
		},
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	req, err := http.NewRequest("POST", "/docker/node", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", s.token.GetValue())
	rec := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusCreated)
	validJson := fmt.Sprintf("[%s]", strings.Replace(strings.Trim(rec.Body.String(), "\n "), "\n", ",", -1))
	var result []tsuruIo.SimpleJsonMessage
	err = json.Unmarshal([]byte(validJson), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 0)
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
	params := addNodeOptions{
		Register: false,
		Metadata: map[string]string{
			"pool": "pool1",
			"id":   "test1",
			"iaas": "another-test-iaas",
		},
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	req, err := http.NewRequest("POST", "/docker/node?register=false", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", s.token.GetValue())
	rec := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusCreated)
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
	params := addNodeOptions{
		Register: true,
		Metadata: map[string]string{
			"pool":    "pool1",
			"address": server.URL(),
		},
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	req, err := http.NewRequest("POST", "/docker/node", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", s.token.GetValue())
	rec := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusCreated)
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
	params := addNodeOptions{
		Register: true,
		Metadata: map[string]string{
			"pool": "pool1",
		},
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	req, err := http.NewRequest("POST", "/docker/node", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", s.token.GetValue())
	rec := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusCreated)
	var result map[string]string
	err = json.NewDecoder(rec.Body).Decode(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result["Error"], check.Equals, "address=url parameter is required\n\n")
}

func (s *HandlersSuite) TestAddNodeHandlerWithInvalidURLAddress(c *check.C) {
	config.Set("docker:cluster:redis-server", "127.0.0.1:6379")
	defer config.Unset("docker:cluster:redis-server")
	params := addNodeOptions{
		Register: true,
		Metadata: map[string]string{
			"address": "/invalid",
			"pool":    "pool1",
		},
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	req, err := http.NewRequest("POST", "/docker/node", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", s.token.GetValue())
	rec := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusCreated)
	var result map[string]string
	err = json.NewDecoder(rec.Body).Decode(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result["Error"], check.Equals, "Invalid address url: host cannot be empty\n\n")
	params = addNodeOptions{
		Register: true,
		Metadata: map[string]string{
			"address": "xxx://abc/invalid",
			"pool":    "pool1",
		},
	}
	v, err = form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	b = strings.NewReader(v.Encode())
	req, err = http.NewRequest("POST", "/docker/node", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", s.token.GetValue())
	rec = httptest.NewRecorder()
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusCreated)
	err = json.NewDecoder(rec.Body).Decode(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result["Error"], check.Equals, "Invalid address url: scheme must be http[s]\n\n")
}

func (s *HandlersSuite) TestAddNodeHandlerNoPool(c *check.C) {
	config.Set("docker:cluster:redis-server", "127.0.0.1:6379")
	defer config.Unset("docker:cluster:redis-server")
	b := bytes.NewBufferString(`{"address": "http://192.168.50.4:2375"}`)
	req, err := http.NewRequest("POST", "/docker/node?register=true", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = addNodeHandler(rec, req, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*tsuruErrors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, "pool is required")
}

func (s *HandlersSuite) TestValidateNodeAddress(c *check.C) {
	err := validateNodeAddress("/invalid")
	c.Assert(err, check.ErrorMatches, "Invalid address url: host cannot be empty")
	err = validateNodeAddress("xxx://abc/invalid")
	c.Assert(err, check.ErrorMatches, `Invalid address url: scheme must be http\[s\]`)
	err = validateNodeAddress("")
	c.Assert(err, check.ErrorMatches, "address=url parameter is required")
}

func (s *HandlersSuite) TestRemoveNodeHandlerNotFound(c *check.C) {
	req, err := http.NewRequest("DELETE", "/docker/node/host.com:2375", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	server := api.RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusNotFound)
}

func (s *HandlersSuite) TestRemoveNodeHandler(c *check.C) {
	var err error
	mainDockerProvisioner.cluster, err = cluster.New(nil, &cluster.MapStorage{})
	c.Assert(err, check.IsNil)
	err = mainDockerProvisioner.Cluster().Register(cluster.Node{Address: "host.com:2375"})
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest("DELETE", "/docker/node/host.com:2375", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	server := api.RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	nodes, err := mainDockerProvisioner.Cluster().Nodes()
	c.Assert(nodes, check.HasLen, 0)
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
	u := fmt.Sprintf("/docker/node/http://%s:2375?remove-iaas=false", machine.Address)
	req, err := http.NewRequest("DELETE", u, nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	server := api.RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Body.String(), check.Equals, "")
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	nodes, err := mainDockerProvisioner.Cluster().Nodes()
	c.Assert(nodes, check.HasLen, 0)
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
	u := fmt.Sprintf("/docker/node/http://%s:2375?remove-iaas=true", machine.Address)
	req, err := http.NewRequest("DELETE", u, nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	server := api.RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	nodes, err := mainDockerProvisioner.Cluster().Nodes()
	c.Assert(nodes, check.HasLen, 0)
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
	c.Assert(nodes, check.HasLen, 2)
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
	u := fmt.Sprintf("/docker/node/%s", nodes[0].Address)
	req, err := http.NewRequest("DELETE", u, nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := tsurutest.NewSafeResponseRecorder()
	server := api.RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	nodes, err = mainDockerProvisioner.Cluster().Nodes()
	c.Assert(nodes, check.HasLen, 1)
	containerList, err := mainDockerProvisioner.listContainersByHost(tsuruNet.URLToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containerList, check.HasLen, 5)
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
	c.Assert(nodes, check.HasLen, 2)
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
	u := fmt.Sprintf("/docker/node/%s?no-rebalance=true", nodes[0].Address)
	req, err := http.NewRequest("DELETE", u, nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := tsurutest.NewSafeResponseRecorder()
	server := api.RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	nodes, err = mainDockerProvisioner.Cluster().Nodes()
	c.Assert(nodes, check.HasLen, 1)
	containerList, err := mainDockerProvisioner.listContainersByHost(tsuruNet.URLToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containerList, check.HasLen, 0)
}

func (s *HandlersSuite) TestListNodeHandlerNoContent(c *check.C) {
	req, err := http.NewRequest("GET", "/docker/node", nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	req.Header.Set("Authorization", s.token.GetValue())
	m := api.RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusNoContent)
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
	req, err := http.NewRequest("GET", "/docker/node", nil)
	rec := httptest.NewRecorder()
	req.Header.Set("Authorization", s.token.GetValue())
	m := api.RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	c.Assert(rec.Header().Get("Content-Type"), check.Equals, "application/json")
	err = json.Unmarshal(rec.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Nodes[0].Address, check.Equals, "host1.com:2375")
	c.Assert(result.Nodes[0].Metadata, check.DeepEquals, map[string]string{"pool": "pool1"})
	c.Assert(result.Nodes[1].Address, check.Equals, "host2.com:2375")
	c.Assert(result.Nodes[1].Metadata, check.DeepEquals, map[string]string{"pool": "pool2", "foo": "bar"})
}

func (s *HandlersSuite) TestListContainersByHostNotFound(c *check.C) {
	req, err := http.NewRequest("GET", "/docker/node/http://notfound.com:4243/containers", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	server := api.RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusNotFound)
}

func (s *HandlersSuite) TestListContainersByHostNoContent(c *check.C) {
	var err error
	mainDockerProvisioner.cluster, err = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{})
	c.Assert(err, check.IsNil)
	mainDockerProvisioner.cluster.Register(cluster.Node{Address: "http://node1.company:4243"})
	req, err := http.NewRequest("GET", "/docker/node/http://node1.company:4243/containers", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	server := api.RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusNoContent)
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

func (s *HandlersSuite) TestListContainersByAppNotFound(c *check.C) {
	req, err := http.NewRequest("GET", "/docker/node/apps/notfound/containers", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", s.token.GetValue())
	rec := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusNotFound)
}

func (s *HandlersSuite) TestListContainersByAppNoContent(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Insert(app.App{Name: "appbla", Platform: "python"})
	req, err := http.NewRequest("GET", "/docker/node/apps/appbla/containers", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", s.token.GetValue())
	rec := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusNoContent)
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
	req, err := http.NewRequest("GET", "/docker/node/apps/appbla/containers", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", s.token.GetValue())
	rec := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	c.Assert(rec.Header().Get("Content-Type"), check.Equals, "application/json")
	err = json.Unmarshal(rec.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result[0].ID, check.DeepEquals, "blabla")
	c.Assert(result[0].AppName, check.DeepEquals, "appbla")
	c.Assert(result[0].HostAddr, check.DeepEquals, "http://node.company")
	c.Assert(result[1].ID, check.DeepEquals, "bleble")
	c.Assert(result[1].AppName, check.DeepEquals, "appbla")
	c.Assert(result[1].HostAddr, check.DeepEquals, "http://node.company")
}

func (s *HandlersSuite) TestListContainersByAppHandlerNotAdminUser(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Insert(app.App{Name: "appbla", Platform: "python", Pool: "mypool"})
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
	req, err := http.NewRequest("GET", "/docker/node/apps/appbla/containers", nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	limitedUser := &auth.User{Email: "mylimited@groundcontrol.com", Password: "123456"}
	_, err = nativeScheme.Create(limitedUser)
	c.Assert(err, check.IsNil)
	defer nativeScheme.Remove(limitedUser)
	t := createTokenForUser(limitedUser, "node", string(permission.CtxPool), "mypool", c)
	req.Header.Set("Authorization", t.GetValue())
	m := api.RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	c.Assert(rec.Header().Get("Content-Type"), check.Equals, "application/json")
	err = json.Unmarshal(rec.Body.Bytes(), &result)
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
	request, err := http.NewRequest("POST", "/docker/containers/move", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *HandlersSuite) TestMoveContainersEmptyToHandler(c *check.C) {
	recorder := httptest.NewRecorder()
	v := url.Values{}
	v.Set("from", "fromhost")
	v.Set("to", "")
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", "/docker/containers/move", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), check.Equals, "Invalid params: from: fromhost - to: \n")
}

func (s *HandlersSuite) TestMoveContainersHandler(c *check.C) {
	recorder := httptest.NewRecorder()
	v := url.Values{}
	v.Set("from", "localhost")
	v.Set("to", "127.0.0.1")
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", "/docker/containers/move", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	mainDockerProvisioner.Cluster().Register(cluster.Node{Address: "http://localhost:2375"})
	mainDockerProvisioner.Cluster().Register(cluster.Node{Address: "http://127.0.0.1:2375"})
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	validJson := fmt.Sprintf("[%s]", strings.Replace(strings.Trim(recorder.Body.String(), "\n "), "\n", ",", -1))
	var result []tsuruIo.SimpleJsonMessage
	err = json.Unmarshal([]byte(validJson), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, []tsuruIo.SimpleJsonMessage{
		{Message: "No units to move in localhost\n"},
		{Message: "Containers moved successfully!\n"},
	})
}

func (s *HandlersSuite) TestMoveContainerNotFound(c *check.C) {
	recorder := httptest.NewRecorder()
	mainDockerProvisioner.Cluster().Register(cluster.Node{Address: "http://127.0.0.1:2375"})
	v := url.Values{}
	v.Set("to", "127.0.0.1")
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", "/docker/container/myid/move", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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
	request, err := http.NewRequest("POST", "/docker/containers/rebalance", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	validJson := fmt.Sprintf("[%s]", strings.Replace(strings.Trim(string(body), "\n "), "\n", ",", -1))
	var result []tsuruIo.SimpleJsonMessage
	err = json.Unmarshal([]byte(validJson), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 14)
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
	opts := rebalanceOptions{
		MetadataFilter: map[string]string{"pool": "pool1"},
	}
	v, err := form.EncodeToValues(&opts)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/docker/containers/rebalance", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	validJson := fmt.Sprintf("[%s]", strings.Replace(strings.Trim(string(body), "\n "), "\n", ",", -1))
	var result []tsuruIo.SimpleJsonMessage
	err = json.Unmarshal([]byte(validJson), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 2)
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
	opts := rebalanceOptions{Dry: true}
	v, err := form.EncodeToValues(&opts)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", "/docker/containers/rebalance", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	validJson := fmt.Sprintf("[%s]", strings.Replace(strings.Trim(string(body), "\n "), "\n", ",", -1))
	var result []tsuruIo.SimpleJsonMessage
	err = json.Unmarshal([]byte(validJson), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 8)
	c.Assert(result[0].Message, check.Equals, "Rebalancing 6 units...\n")
	c.Assert(result[1].Message, check.Matches, "(?s)Would move unit .*")
	c.Assert(result[7].Message, check.Equals, "Containers successfully rebalanced!\n")
}

func (s *HandlersSuite) TestHealingHistoryNoContent(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/docker/healing", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
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
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
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
	err = evt1.Update(cluster.Node{Address: "addr2"}, nil)
	c.Assert(err, check.IsNil)
	evt2, err := healer.NewHealingEvent(cluster.Node{Address: "addr3"})
	c.Assert(err, check.IsNil)
	err = evt2.Update(cluster.Node{}, errors.New("some error"))
	c.Assert(err, check.IsNil)
	evt3, err := healer.NewHealingEvent(container.Container{ID: "1234"})
	c.Assert(err, check.IsNil)
	err = evt3.Update(container.Container{ID: "9876"}, nil)
	c.Assert(err, check.IsNil)
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
	c.Assert(healings[0].ID, check.Equals, evt2.ID.(bson.ObjectId).Hex())
	c.Assert(healings[0].FailingNode.Address, check.Equals, evt2.FailingNode.Address)
	c.Assert(healings[1].Action, check.Equals, "node-healing")
	c.Assert(healings[1].ID, check.Equals, evt1.ID.(bson.ObjectId).Hex())
	c.Assert(healings[1].FailingNode.Address, check.Equals, evt1.FailingNode.Address)
}

func (s *HandlersSuite) TestAutoScaleHistoryNoContent(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/docker/autoscale", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
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
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
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
	params := updateNodeOptions{
		Address: "localhost:1999",
		Enable:  true,
		Metadata: map[string]string{
			"m1": "",
			"m2": "v9",
			"m3": "v8",
		},
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/1.0/docker/node", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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
	v := url.Values{}
	v.Set("m1", "")
	v.Set("m2", "v9")
	v.Set("m3", "v8")
	b := strings.NewReader(v.Encode())
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/docker/node", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *HandlersSuite) TestUpdateNodeHandlerNodeDoesNotExist(c *check.C) {
	mainDockerProvisioner.cluster, _ = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{},
		cluster.Node{Address: "127.0.0.1:1999", Metadata: map[string]string{
			"m1": "v1",
			"m2": "v2",
		}},
	)
	opts := provision.AddPoolOptions{Name: "pool1"}
	err := provision.AddPool(opts)
	defer provision.RemovePool("pool1")
	params := updateNodeOptions{
		Address: "127.0.0.2:1999",
		Metadata: map[string]string{
			"m1": "",
			"m2": "v9",
			"m3": "v8",
		},
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/docker/node", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "No such node in storage\n")
	nodes, err := mainDockerProvisioner.Cluster().Nodes()
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Metadata, check.DeepEquals, map[string]string{
		"m1": "v1",
		"m2": "v2",
	})
}

func (s *HandlersSuite) TestUpdateNodeDisableNodeHandler(c *check.C) {
	mainDockerProvisioner.cluster, _ = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{},
		cluster.Node{Address: "localhost:1999", CreationStatus: cluster.NodeCreationStatusCreated},
	)
	opts := provision.AddPoolOptions{Name: "pool1"}
	err := provision.AddPool(opts)
	defer provision.RemovePool("pool1")
	params := updateNodeOptions{
		Address: "localhost:1999",
		Disable: true,
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/docker/node", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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
	params := updateNodeOptions{
		Address: "localhost:1999",
		Enable:  true,
	}
	v, err := form.EncodeToValues(&params)
	b := strings.NewReader(v.Encode())
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/docker/node", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	nodes, err := mainDockerProvisioner.Cluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].CreationStatus, check.DeepEquals, cluster.NodeCreationStatusCreated)
}

func (s *HandlersSuite) TestUpdateNodeEnableAndDisableCantBeDone(c *check.C) {
	mainDockerProvisioner.cluster, _ = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{},
		cluster.Node{Address: "localhost:1999", CreationStatus: cluster.NodeCreationStatusDisabled},
	)
	opts := provision.AddPoolOptions{Name: "pool1"}
	err := provision.AddPool(opts)
	defer provision.RemovePool("pool1")
	params := updateNodeOptions{Address: "localhost:1999"}
	v, err := form.EncodeToValues(&params)
	b := strings.NewReader(v.Encode())
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/docker/node?enabled=true&disabled=true", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *HandlersSuite) TestUpdateNodeHandlerEnableCanMoveContainers(c *check.C) {
	mainDockerProvisioner.cluster, _ = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{},
		cluster.Node{Address: "localhost:2375", CreationStatus: cluster.NodeCreationStatusDisabled},
	)
	opts := provision.AddPoolOptions{Name: "pool1"}
	err := provision.AddPool(opts)
	defer provision.RemovePool("pool1")
	params := updateNodeOptions{
		Address: "localhost:2375",
		Enable:  true,
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/docker/node", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	nodes, err := mainDockerProvisioner.Cluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].CreationStatus, check.DeepEquals, cluster.NodeCreationStatusCreated)
	recorder = httptest.NewRecorder()
	v = url.Values{}
	v.Set("from", "localhost")
	v.Set("to", "127.0.0.1")
	b = strings.NewReader(v.Encode())
	request, err = http.NewRequest("POST", "/docker/containers/move", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	mainDockerProvisioner.Cluster().Register(cluster.Node{Address: "http://127.0.0.1:2375"})
	server = api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	validJson := fmt.Sprintf("[%s]", strings.Replace(strings.Trim(recorder.Body.String(), "\n "), "\n", ",", -1))
	var result []tsuruIo.SimpleJsonMessage
	err = json.Unmarshal([]byte(validJson), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, []tsuruIo.SimpleJsonMessage{
		{Message: "No units to move in localhost\n"},
		{Message: "Containers moved successfully!\n"},
	})
}

func (s *HandlersSuite) TestUpdateNodeHandlerDisableCannotMoveContainers(c *check.C) {
	mainDockerProvisioner.cluster, _ = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{},
		cluster.Node{Address: "localhost:2375", CreationStatus: cluster.NodeCreationStatusCreated},
	)
	opts := provision.AddPoolOptions{Name: "pool1"}
	err := provision.AddPool(opts)
	defer provision.RemovePool("pool1")
	params := updateNodeOptions{
		Address: "localhost:2375",
		Disable: true,
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/docker/node", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	nodes, err := mainDockerProvisioner.Cluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].CreationStatus, check.DeepEquals, cluster.NodeCreationStatusDisabled)
	recorder = httptest.NewRecorder()
	v = url.Values{}
	v.Set("from", "localhost")
	v.Set("to", "127.0.0.1")
	b = strings.NewReader(v.Encode())
	request, err = http.NewRequest("POST", "/docker/containers/move", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	mainDockerProvisioner.Cluster().Register(cluster.Node{Address: "http://127.0.0.1:2375"})
	server = api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	validJson := fmt.Sprintf("[%s]", strings.Replace(strings.Trim(recorder.Body.String(), "\n "), "\n", ",", -1))
	c.Assert(validJson, check.Equals, "[Host `localhost` not found]")
}

func (s *HandlersSuite) TestAutoScaleRunHandler(c *check.C) {
	mainDockerProvisioner.cluster, _ = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{},
		cluster.Node{Address: "localhost:1999", Metadata: map[string]string{
			"pool": "pool1",
		}},
	)
	config.Set("docker:auto-scale:enabled", true)
	defer config.Unset("docker:auto-scale:enabled")
	config.Set("docker:auto-scale:max-container-count", 2)
	defer config.Unset("docker:auto-scale:max-container-count")
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/docker/autoscale/run", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	body := recorder.Body.String()
	parts := strings.Split(body, "\n")
	c.Assert(parts, check.DeepEquals, []string{
		`{"Message":"running scaler *docker.countScaler for \"pool\": \"pool1\"\n"}`,
		`{"Message":"nothing to do for \"pool\": \"pool1\"\n"}`,
		``,
	})
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
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
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
	rule := autoScaleRule{
		MetadataFilter: "pool1",
		Enabled:        true,
		ScaleDownRatio: 1.1,
		MaxMemoryRatio: 2.0,
	}
	v, err := form.EncodeToValues(&rule)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", "/docker/autoscale/rules", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
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
	v, err := form.EncodeToValues(&rule)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(v.Encode())
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/docker/autoscale/rules", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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
	v, err := form.EncodeToValues(&rule)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(v.Encode())
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/docker/autoscale/rules", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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
	request, err := http.NewRequest("DELETE", "/docker/autoscale/rules", nil)
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

func (s *HandlersSuite) TestDockerLogsUpdateHandler(c *check.C) {
	values1 := url.Values{
		"Driver":                 []string{"awslogs"},
		"LogOpts.awslogs-region": []string{"sa-east1"},
	}
	values2 := url.Values{
		"pool":   []string{"POOL1"},
		"Driver": []string{"bs"},
	}
	values3 := url.Values{
		"pool":                    []string{"POOL2"},
		"Driver":                  []string{"fluentd"},
		"LogOpts.fluentd-address": []string{"localhost:2222"},
	}
	doReq := func(val url.Values) {
		reader := strings.NewReader(val.Encode())
		recorder := httptest.NewRecorder()
		request, err := http.NewRequest("POST", "/docker/logs", reader)
		c.Assert(err, check.IsNil)
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		server := api.RunServer(true)
		server.ServeHTTP(recorder, request)
		c.Assert(recorder.Body.String(), check.Equals, "{\"Message\":\"Log config successfully updated.\\n\"}\n")
		c.Assert(recorder.Code, check.Equals, http.StatusOK)
		c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	}
	doReq(values1)
	entries, err := container.LogLoadAll()
	c.Assert(err, check.IsNil)
	c.Assert(entries, check.DeepEquals, map[string]container.DockerLogConfig{
		"": {Driver: "awslogs", LogOpts: map[string]string{"awslogs-region": "sa-east1"}},
	})
	doReq(values2)
	entries, err = container.LogLoadAll()
	c.Assert(err, check.IsNil)
	c.Assert(entries, check.DeepEquals, map[string]container.DockerLogConfig{
		"":      {Driver: "awslogs", LogOpts: map[string]string{"awslogs-region": "sa-east1"}},
		"POOL1": {Driver: "bs", LogOpts: map[string]string{}},
	})
	doReq(values3)
	entries, err = container.LogLoadAll()
	c.Assert(err, check.IsNil)
	c.Assert(entries, check.DeepEquals, map[string]container.DockerLogConfig{
		"":      {Driver: "awslogs", LogOpts: map[string]string{"awslogs-region": "sa-east1"}},
		"POOL1": {Driver: "bs", LogOpts: map[string]string{}},
		"POOL2": {Driver: "fluentd", LogOpts: map[string]string{"fluentd-address": "localhost:2222"}},
	})
}

func (s *HandlersSuite) TestDockerLogsUpdateHandlerWithRestartNoApps(c *check.C) {
	values := url.Values{
		"restart":                []string{"true"},
		"Driver":                 []string{"awslogs"},
		"LogOpts.awslogs-region": []string{"sa-east1"},
	}
	recorder := httptest.NewRecorder()
	reader := strings.NewReader(values.Encode())
	request, err := http.NewRequest("POST", "/docker/logs", reader)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Equals, "{\"Message\":\"Log config successfully updated.\\n\"}\n")
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	entries, err := container.LogLoadAll()
	c.Assert(err, check.IsNil)
	c.Assert(entries, check.DeepEquals, map[string]container.DockerLogConfig{
		"": {Driver: "awslogs", LogOpts: map[string]string{"awslogs-region": "sa-east1"}},
	})
}

func (s *S) TestDockerLogsUpdateHandlerWithRestartSomeApps(c *check.C) {
	appPools := [][]string{{"app1", "POOL1"}, {"app2", "POOL2"}, {"app3", "POOL2"}}
	for _, appPool := range appPools {
		opts := provision.AddPoolOptions{Name: appPool[1]}
		provision.AddPool(opts)
		err := s.newFakeImage(s.p, "tsuru/app-"+appPool[0], nil)
		c.Assert(err, check.IsNil)
		appInstance := provisiontest.NewFakeApp(appPool[0], "python", 0)
		appStruct := &app.App{
			Name:     appInstance.GetName(),
			Platform: appInstance.GetPlatform(),
			Pool:     opts.Name,
		}
		err = s.storage.Apps().Insert(appStruct)
		c.Assert(err, check.IsNil)
		err = s.p.Provision(appStruct)
		c.Assert(err, check.IsNil)
	}
	values := url.Values{
		"pool":                   []string{"POOL2"},
		"restart":                []string{"true"},
		"Driver":                 []string{"awslogs"},
		"LogOpts.awslogs-region": []string{"sa-east1"},
	}
	recorder := httptest.NewRecorder()
	reader := strings.NewReader(values.Encode())
	request, err := http.NewRequest("POST", "/docker/logs", reader)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	responseParts := strings.Split(recorder.Body.String(), "\n")
	c.Assert(responseParts, check.HasLen, 17)
	c.Assert(responseParts[0], check.Equals, "{\"Message\":\"Log config successfully updated.\\n\"}")
	c.Assert(responseParts[1], check.Equals, "{\"Message\":\"Restarting 2 applications: [app2, app3]\\n\"}")
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	entries, err := container.LogLoadAll()
	c.Assert(err, check.IsNil)
	c.Assert(entries, check.DeepEquals, map[string]container.DockerLogConfig{
		"":      {},
		"POOL2": {Driver: "awslogs", LogOpts: map[string]string{"awslogs-region": "sa-east1"}},
	})
}

func (s *HandlersSuite) TestDockerLogsInfoHandler(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/docker/logs", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var conf map[string]container.DockerLogConfig
	err = json.Unmarshal(recorder.Body.Bytes(), &conf)
	c.Assert(err, check.IsNil)
	c.Assert(conf, check.DeepEquals, map[string]container.DockerLogConfig{
		"": {},
	})
	newConf := container.DockerLogConfig{Driver: "syslog"}
	err = newConf.Save("p1")
	c.Assert(err, check.IsNil)
	request, err = http.NewRequest("GET", "/docker/logs", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var conf2 map[string]container.DockerLogConfig
	err = json.Unmarshal(recorder.Body.Bytes(), &conf2)
	c.Assert(err, check.IsNil)
	c.Assert(conf2, check.DeepEquals, map[string]container.DockerLogConfig{
		"":   {},
		"p1": {Driver: "syslog", LogOpts: map[string]string{}},
	})
}

func boolPtr(b bool) *bool {
	return &b
}

func intPtr(i int) *int {
	return &i
}

func (s *HandlersSuite) TestNodeHealingUpdateRead(c *check.C) {
	doRequest := func(str string) map[string]healer.NodeHealerConfig {
		body := bytes.NewBufferString(str)
		request, err := http.NewRequest("POST", "/docker/healing/node", body)
		c.Assert(err, check.IsNil)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		recorder := httptest.NewRecorder()
		server := api.RunServer(true)
		server.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusOK)
		request, err = http.NewRequest("GET", "/docker/healing/node", body)
		c.Assert(err, check.IsNil)
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		recorder = httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusOK)
		var configMap map[string]healer.NodeHealerConfig
		json.Unmarshal(recorder.Body.Bytes(), &configMap)
		return configMap
	}
	tests := []struct {
		A string
		B map[string]healer.NodeHealerConfig
	}{
		{"", map[string]healer.NodeHealerConfig{
			"": {},
		}},
		{"Enabled=true&MaxTimeSinceSuccess=60", map[string]healer.NodeHealerConfig{
			"": {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60)},
		}},
		{"MaxUnresponsiveTime=10", map[string]healer.NodeHealerConfig{
			"": {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(10)},
		}},
		{"Enabled=false", map[string]healer.NodeHealerConfig{
			"": {Enabled: boolPtr(false), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(10)},
		}},
		{"MaxUnresponsiveTime=20", map[string]healer.NodeHealerConfig{
			"": {Enabled: boolPtr(false), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(20)},
		}},
		{"Enabled=true", map[string]healer.NodeHealerConfig{
			"": {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(20)},
		}},
		{"pool=p1&Enabled=false", map[string]healer.NodeHealerConfig{
			"":   {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(20)},
			"p1": {Enabled: boolPtr(false), MaxTimeSinceSuccess: intPtr(60), MaxTimeSinceSuccessInherited: true, MaxUnresponsiveTime: intPtr(20), MaxUnresponsiveTimeInherited: true},
		}},
		{"pool=p1&Enabled=true", map[string]healer.NodeHealerConfig{
			"":   {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(20)},
			"p1": {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxTimeSinceSuccessInherited: true, MaxUnresponsiveTime: intPtr(20), MaxUnresponsiveTimeInherited: true},
		}},
		{"pool=p1", map[string]healer.NodeHealerConfig{
			"":   {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(20)},
			"p1": {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxTimeSinceSuccessInherited: true, MaxUnresponsiveTime: intPtr(20), MaxUnresponsiveTimeInherited: true},
		}},
		{"pool=p1&MaxUnresponsiveTime=30", map[string]healer.NodeHealerConfig{
			"":   {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(20)},
			"p1": {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxTimeSinceSuccessInherited: true, MaxUnresponsiveTime: intPtr(30), MaxUnresponsiveTimeInherited: false},
		}},
		{"pool=p1&MaxUnresponsiveTime=0", map[string]healer.NodeHealerConfig{
			"":   {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(20)},
			"p1": {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxTimeSinceSuccessInherited: true, MaxUnresponsiveTime: intPtr(0), MaxUnresponsiveTimeInherited: false},
		}},
		{"pool=p1&Enabled=false", map[string]healer.NodeHealerConfig{
			"":   {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(20)},
			"p1": {Enabled: boolPtr(false), MaxTimeSinceSuccess: intPtr(60), MaxTimeSinceSuccessInherited: true, MaxUnresponsiveTime: intPtr(0), MaxUnresponsiveTimeInherited: false},
		}},
	}
	for i, t := range tests {
		configMap := doRequest(t.A)
		c.Assert(configMap, check.DeepEquals, t.B, check.Commentf("test %d", i+1))
	}
	request, err := http.NewRequest("DELETE", "/docker/healing/node?pool=p1&name=MaxUnresponsiveTime", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	configMap := doRequest("")
	c.Assert(configMap, check.DeepEquals, map[string]healer.NodeHealerConfig{
		"":   {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(20)},
		"p1": {Enabled: boolPtr(false), MaxTimeSinceSuccess: intPtr(60), MaxTimeSinceSuccessInherited: true, MaxUnresponsiveTime: intPtr(20), MaxUnresponsiveTimeInherited: true},
	})
	request, err = http.NewRequest("DELETE", "/docker/healing/node", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	configMap = doRequest("")
	c.Assert(configMap, check.DeepEquals, map[string]healer.NodeHealerConfig{
		"":   {},
		"p1": {Enabled: boolPtr(false), MaxTimeSinceSuccessInherited: true, MaxUnresponsiveTimeInherited: true},
	})
	request, err = http.NewRequest("DELETE", "/docker/healing/node?pool=p1&name=Enabled", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	configMap = doRequest("")
	c.Assert(configMap, check.DeepEquals, map[string]healer.NodeHealerConfig{
		"":   {},
		"p1": {EnabledInherited: true, MaxTimeSinceSuccessInherited: true, MaxUnresponsiveTimeInherited: true},
	})
}

func (s *HandlersSuite) TestNodeHealingConfigUpdateReadLimited(c *check.C) {
	doRequest := func(t auth.Token, code int, str string) map[string]healer.NodeHealerConfig {
		body := bytes.NewBufferString(str)
		request, err := http.NewRequest("POST", "/docker/healing/node", body)
		c.Assert(err, check.IsNil)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Authorization", "bearer "+t.GetValue())
		recorder := httptest.NewRecorder()
		server := api.RunServer(true)
		server.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, code)
		request, err = http.NewRequest("GET", "/docker/healing/node", body)
		c.Assert(err, check.IsNil)
		request.Header.Set("Authorization", "bearer "+t.GetValue())
		recorder = httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusOK)
		var configMap map[string]healer.NodeHealerConfig
		json.Unmarshal(recorder.Body.Bytes(), &configMap)
		return configMap
	}
	limitedUser := &auth.User{Email: "mylimited@groundcontrol.com", Password: "123456"}
	_, err := nativeScheme.Create(limitedUser)
	c.Assert(err, check.IsNil)
	defer nativeScheme.Remove(limitedUser)
	createTokenForUser(limitedUser, "healing.update", string(permission.CtxPool), "p2", c)
	t := createTokenForUser(limitedUser, "healing.read", string(permission.CtxPool), "p2", c)
	data := doRequest(t, http.StatusForbidden, "Enabled=true&MaxTimeSinceSuccess=60")
	c.Assert(data, check.DeepEquals, map[string]healer.NodeHealerConfig{
		"": {},
	})
	data = doRequest(s.token, http.StatusOK, "Enabled=true&MaxTimeSinceSuccess=60")
	c.Assert(data, check.DeepEquals, map[string]healer.NodeHealerConfig{
		"": {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60)},
	})
	data = doRequest(t, http.StatusForbidden, "pool=p1&Enabled=true&MaxTimeSinceSuccess=20")
	c.Assert(data, check.DeepEquals, map[string]healer.NodeHealerConfig{
		"": {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60)},
	})
	data = doRequest(t, http.StatusOK, "pool=p2&Enabled=true&MaxTimeSinceSuccess=20")
	c.Assert(data, check.DeepEquals, map[string]healer.NodeHealerConfig{
		"":   {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60)},
		"p2": {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(20), MaxUnresponsiveTimeInherited: true},
	})
}

func (s *HandlersSuite) TestNodeContainerList(c *check.C) {
	err := nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"A=1"},
		},
	})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Env: []string{"A=2"},
		},
	})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name: "c2",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"B=1"},
		},
	})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/docker/nodecontainers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var configEntries []nodecontainer.NodeContainerConfigGroup
	json.Unmarshal(recorder.Body.Bytes(), &configEntries)
	sort.Sort(nodecontainer.NodeContainerConfigGroupSlice(configEntries))
	c.Assert(configEntries, check.DeepEquals, []nodecontainer.NodeContainerConfigGroup{
		{Name: "c1", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"":   {Name: "c1", Config: docker.Config{Image: "img1", Env: []string{"A=1"}}},
			"p1": {Name: "c1", Config: docker.Config{Env: []string{"A=2"}}},
		}},
		{Name: "c2", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"": {Name: "c2", Config: docker.Config{Image: "img1", Env: []string{"B=1"}}},
		}},
	})
}

func (s *HandlersSuite) TestNodeContainerListLimited(c *check.C) {
	err := nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"A=1"},
		},
	})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Env: []string{"A=2"},
		},
	})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("p3", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Env: []string{"A=3"},
		},
	})
	c.Assert(err, check.IsNil)
	limitedUser := &auth.User{Email: "mylimited@groundcontrol.com", Password: "123456"}
	_, err = nativeScheme.Create(limitedUser)
	c.Assert(err, check.IsNil)
	defer nativeScheme.Remove(limitedUser)
	t := createTokenForUser(limitedUser, "nodecontainer.read", string(permission.CtxPool), "p3", c)
	request, err := http.NewRequest("GET", "/docker/nodecontainers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+t.GetValue())
	recorder := httptest.NewRecorder()
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var configEntries []nodecontainer.NodeContainerConfigGroup
	json.Unmarshal(recorder.Body.Bytes(), &configEntries)
	sort.Sort(nodecontainer.NodeContainerConfigGroupSlice(configEntries))
	c.Assert(configEntries, check.DeepEquals, []nodecontainer.NodeContainerConfigGroup{
		{Name: "c1", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"":   {Name: "c1", Config: docker.Config{Image: "img1", Env: []string{"A=1"}}},
			"p3": {Name: "c1", Config: docker.Config{Env: []string{"A=3"}}},
		}},
	})
}

func (s *HandlersSuite) TestNodeContainerInfoNotFound(c *check.C) {
	request, err := http.NewRequest("GET", "/docker/nodecontainers/c1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *HandlersSuite) TestNodeContainerInfo(c *check.C) {
	err := nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"A=1"},
		},
	})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Env: []string{"A=2"},
		},
	})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name: "c2",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"B=1"},
		},
	})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/docker/nodecontainers/c1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var configEntries map[string]nodecontainer.NodeContainerConfig
	json.Unmarshal(recorder.Body.Bytes(), &configEntries)
	c.Assert(configEntries, check.DeepEquals, map[string]nodecontainer.NodeContainerConfig{
		"":   {Name: "c1", Config: docker.Config{Image: "img1", Env: []string{"A=1"}}},
		"p1": {Name: "c1", Config: docker.Config{Env: []string{"A=2"}}},
	})
}

func (s *HandlersSuite) TestNodeContainerInfoLimited(c *check.C) {
	err := nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"A=1"},
		},
	})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Env: []string{"A=2"},
		},
	})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name: "c2",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"B=1"},
		},
	})
	c.Assert(err, check.IsNil)
	limitedUser := &auth.User{Email: "mylimited@groundcontrol.com", Password: "123456"}
	_, err = nativeScheme.Create(limitedUser)
	c.Assert(err, check.IsNil)
	defer nativeScheme.Remove(limitedUser)
	t := createTokenForUser(limitedUser, "nodecontainer.read", string(permission.CtxPool), "p-none", c)
	request, err := http.NewRequest("GET", "/docker/nodecontainers/c1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+t.GetValue())
	recorder := httptest.NewRecorder()
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var configEntries map[string]nodecontainer.NodeContainerConfig
	json.Unmarshal(recorder.Body.Bytes(), &configEntries)
	c.Assert(configEntries, check.DeepEquals, map[string]nodecontainer.NodeContainerConfig{
		"": {Name: "c1", Config: docker.Config{Image: "img1", Env: []string{"A=1"}}},
	})
}

func (s *HandlersSuite) TestNodeContainerCreate(c *check.C) {
	doReq := func(cont nodecontainer.NodeContainerConfig, expected []nodecontainer.NodeContainerConfigGroup, pool ...string) {
		values, err := form.EncodeToValues(cont)
		c.Assert(err, check.IsNil)
		if len(pool) > 0 {
			values.Set("pool", pool[0])
		}
		reader := strings.NewReader(values.Encode())
		request, err := http.NewRequest("POST", "/docker/nodecontainers", reader)
		c.Assert(err, check.IsNil)
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()
		server := api.RunServer(true)
		server.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusOK)
		request, err = http.NewRequest("GET", "/docker/nodecontainers", nil)
		c.Assert(err, check.IsNil)
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		recorder = httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusOK)
		var configEntries []nodecontainer.NodeContainerConfigGroup
		json.Unmarshal(recorder.Body.Bytes(), &configEntries)
		sort.Sort(nodecontainer.NodeContainerConfigGroupSlice(configEntries))
		c.Assert(configEntries, check.DeepEquals, expected)
	}
	doReq(nodecontainer.NodeContainerConfig{Name: "c1", Config: docker.Config{Image: "img1"}}, []nodecontainer.NodeContainerConfigGroup{
		{Name: "c1", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"": {Name: "c1", Config: docker.Config{Image: "img1"}},
		}},
	})
	doReq(nodecontainer.NodeContainerConfig{
		Name:       "c2",
		Config:     docker.Config{Env: []string{"A=1"}, Image: "img2"},
		HostConfig: docker.HostConfig{Memory: 256, Privileged: true},
	}, []nodecontainer.NodeContainerConfigGroup{
		{Name: "c1", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"": {Name: "c1", Config: docker.Config{Image: "img1"}},
		}},
		{Name: "c2", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"": {Name: "c2", Config: docker.Config{Env: []string{"A=1"}, Image: "img2"}, HostConfig: docker.HostConfig{Memory: 256, Privileged: true}},
		}},
	})
	doReq(nodecontainer.NodeContainerConfig{
		Name:       "c2",
		Config:     docker.Config{Env: []string{"Z=9"}, Image: "img2"},
		HostConfig: docker.HostConfig{Memory: 256},
	}, []nodecontainer.NodeContainerConfigGroup{
		{Name: "c1", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"": {Name: "c1", Config: docker.Config{Image: "img1"}},
		}},
		{Name: "c2", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"": {Name: "c2", Config: docker.Config{Env: []string{"Z=9"}, Image: "img2"}, HostConfig: docker.HostConfig{Memory: 256}},
		}},
	})
	doReq(nodecontainer.NodeContainerConfig{
		Name:   "c2",
		Config: docker.Config{Env: []string{"X=1"}},
	}, []nodecontainer.NodeContainerConfigGroup{
		{Name: "c1", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"": {Name: "c1", Config: docker.Config{Image: "img1"}},
		}},
		{Name: "c2", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"":   {Name: "c2", Config: docker.Config{Env: []string{"Z=9"}, Image: "img2"}, HostConfig: docker.HostConfig{Memory: 256}},
			"p1": {Name: "c2", Config: docker.Config{Env: []string{"X=1"}}},
		}},
	}, "p1")
}

func (s *HandlersSuite) TestNodeContainerCreateInvalid(c *check.C) {
	reader := strings.NewReader("")
	request, err := http.NewRequest("POST", "/docker/nodecontainers", reader)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Matches, "node container config name cannot be empty\n")
	values, err := form.EncodeToValues(nodecontainer.NodeContainerConfig{Name: ""})
	c.Assert(err, check.IsNil)
	reader = strings.NewReader(values.Encode())
	request, err = http.NewRequest("POST", "/docker/nodecontainers", reader)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Matches, "node container config name cannot be empty\n")
	values, err = form.EncodeToValues(nodecontainer.NodeContainerConfig{Name: "x1"})
	c.Assert(err, check.IsNil)
	reader = strings.NewReader(values.Encode())
	request, err = http.NewRequest("POST", "/docker/nodecontainers", reader)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Matches, "node container config image cannot be empty\n")
}

func (s *HandlersSuite) TestNodeContainerCreateLimited(c *check.C) {
	limitedUser := &auth.User{Email: "mylimited@groundcontrol.com", Password: "123456"}
	_, err := nativeScheme.Create(limitedUser)
	c.Assert(err, check.IsNil)
	defer nativeScheme.Remove(limitedUser)
	t := createTokenForUser(limitedUser, "nodecontainer.create", string(permission.CtxPool), "p1", c)
	values, err := form.EncodeToValues(nodecontainer.NodeContainerConfig{Name: "c1", Config: docker.Config{Image: "img1"}})
	c.Assert(err, check.IsNil)
	reader := strings.NewReader(values.Encode())
	request, err := http.NewRequest("POST", "/docker/nodecontainers", reader)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+t.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	values.Set("pool", "p1")
	reader = strings.NewReader(values.Encode())
	request, err = http.NewRequest("POST", "/docker/nodecontainers", reader)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+t.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *HandlersSuite) TestNodeContainerUpdate(c *check.C) {
	doReq := func(cont nodecontainer.NodeContainerConfig, expected map[string]nodecontainer.NodeContainerConfig, pool ...string) {
		values, err := form.EncodeToValues(cont)
		c.Assert(err, check.IsNil)
		if len(pool) > 0 {
			values.Set("pool", pool[0])
		}
		reader := strings.NewReader(values.Encode())
		request, err := http.NewRequest("POST", "/docker/nodecontainers/"+cont.Name, reader)
		c.Assert(err, check.IsNil)
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()
		server := api.RunServer(true)
		server.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusOK)
		request, err = http.NewRequest("GET", "/docker/nodecontainers/"+cont.Name, nil)
		c.Assert(err, check.IsNil)
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		recorder = httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusOK)
		var configEntries map[string]nodecontainer.NodeContainerConfig
		json.Unmarshal(recorder.Body.Bytes(), &configEntries)
		if len(pool) > 0 {
			for _, p := range pool {
				sort.Strings(configEntries[p].Config.Env)
				sort.Strings(expected[p].Config.Env)
			}
		}
		sort.Strings(configEntries[""].Config.Env)
		sort.Strings(expected[""].Config.Env)
		c.Assert(configEntries, check.DeepEquals, expected)
	}
	err := nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{Name: "c1", Config: docker.Config{Image: "img1"}})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{Name: "c2", Config: docker.Config{Image: "img2"}})
	c.Assert(err, check.IsNil)
	doReq(nodecontainer.NodeContainerConfig{Name: "c1"}, map[string]nodecontainer.NodeContainerConfig{
		"": {Name: "c1", Config: docker.Config{Image: "img1"}},
	})
	doReq(nodecontainer.NodeContainerConfig{
		Name:       "c2",
		Config:     docker.Config{Env: []string{"A=1"}},
		HostConfig: docker.HostConfig{Memory: 256, Privileged: true},
	}, map[string]nodecontainer.NodeContainerConfig{
		"": {Name: "c2", Config: docker.Config{Env: []string{"A=1"}, Image: "img2"}, HostConfig: docker.HostConfig{Memory: 256, Privileged: true}},
	})
	doReq(nodecontainer.NodeContainerConfig{
		Name:       "c2",
		Config:     docker.Config{Env: []string{"Z=9"}},
		HostConfig: docker.HostConfig{Memory: 256},
	}, map[string]nodecontainer.NodeContainerConfig{
		"": {Name: "c2", Config: docker.Config{Env: []string{"A=1", "Z=9"}, Image: "img2"}, HostConfig: docker.HostConfig{Memory: 256, Privileged: true}},
	})
	err = nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{Name: "c2"})
	c.Assert(err, check.IsNil)
	doReq(nodecontainer.NodeContainerConfig{
		Name:   "c2",
		Config: docker.Config{Env: []string{"X=1"}},
	}, map[string]nodecontainer.NodeContainerConfig{
		"":   {Name: "c2", Config: docker.Config{Env: []string{"A=1", "Z=9"}, Image: "img2"}, HostConfig: docker.HostConfig{Memory: 256, Privileged: true}},
		"p1": {Name: "c2", Config: docker.Config{Env: []string{"X=1"}}},
	}, "p1")
}

func (s *HandlersSuite) TestNodeContainerUpdateInvalid(c *check.C) {
	cont := nodecontainer.NodeContainerConfig{Name: "c1", Config: docker.Config{Image: "img1"}}
	val, err := form.EncodeToValues(cont)
	c.Assert(err, check.IsNil)
	reader := strings.NewReader(val.Encode())
	request, err := http.NewRequest("POST", "/docker/nodecontainers/c1", reader)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Matches, "node container not found\n")
}

func (s *HandlersSuite) TestNodeContainerUpdateLimited(c *check.C) {
	err := nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{Name: "c1", Config: docker.Config{Image: "img1"}})
	c.Assert(err, check.IsNil)
	limitedUser := &auth.User{Email: "mylimited@groundcontrol.com", Password: "123456"}
	_, err = nativeScheme.Create(limitedUser)
	c.Assert(err, check.IsNil)
	defer nativeScheme.Remove(limitedUser)
	t := createTokenForUser(limitedUser, "nodecontainer.update", string(permission.CtxPool), "p1", c)
	values, err := form.EncodeToValues(nodecontainer.NodeContainerConfig{Name: "c1"})
	c.Assert(err, check.IsNil)
	reader := strings.NewReader(values.Encode())
	request, err := http.NewRequest("POST", "/docker/nodecontainers/c1", reader)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+t.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	values.Set("pool", "p1")
	reader = strings.NewReader(values.Encode())
	request, err = http.NewRequest("POST", "/docker/nodecontainers/c1", reader)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+t.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *HandlersSuite) TestNodeContainerDelete(c *check.C) {
	err := nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"A=1"},
		},
	})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Env: []string{"A=2"},
		},
	})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/docker/nodecontainers/c1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	all, err := nodecontainer.AllNodeContainers()
	c.Assert(err, check.IsNil)
	c.Assert(all, check.DeepEquals, []nodecontainer.NodeContainerConfigGroup{
		{Name: "c1", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"p1": {Name: "c1", Config: docker.Config{Env: []string{"A=2"}}},
		}},
	})
	request, err = http.NewRequest("DELETE", "/docker/nodecontainers/c1?pool=p1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	all, err = nodecontainer.AllNodeContainers()
	c.Assert(err, check.IsNil)
	c.Assert(all, check.DeepEquals, []nodecontainer.NodeContainerConfigGroup{})
}

func (s *HandlersSuite) TestNodeContainerDeleteNotFounc(c *check.C) {
	err := nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"A=1"},
		},
	})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/docker/nodecontainers/c1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "node container \"c1\" not found for pool \"\"\n")
	request, err = http.NewRequest("DELETE", "/docker/nodecontainers/c1?pool=p1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *HandlersSuite) TestNodeContainerUpgrade(c *check.C) {
	err := nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name:        "c1",
		PinnedImage: "tsuru/c1@sha256:abcef384829283eff",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"A=1"},
		},
	})
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/docker/nodecontainers/c1/upgrade", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	all, err := nodecontainer.AllNodeContainers()
	c.Assert(err, check.IsNil)
	c.Assert(all, check.DeepEquals, []nodecontainer.NodeContainerConfigGroup{
		{Name: "c1", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"": {Name: "c1", Config: docker.Config{Env: []string{"A=1"}, Image: "img1"}},
		}},
	})
}

func (s *HandlersSuite) TestNodeContainerUpgradeNotFound(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/docker/nodecontainers/c1/upgrade", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}
