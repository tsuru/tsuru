// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

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
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/quota"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type TestIaaS struct{}

func (TestIaaS) DeleteMachine(m *iaas.Machine) error {
	return nil
}

func (TestIaaS) CreateMachine(params map[string]string) (*iaas.Machine, error) {
	m := iaas.Machine{
		Id:      params["id"],
		Status:  "running",
		Address: params["id"] + ".fake.host",
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
	conn   *db.Storage
	server *httptest.Server
	user   *auth.User
	token  auth.Token
	team   *auth.Team
}

var _ = check.Suite(&HandlersSuite{})

func (s *HandlersSuite) SetUpSuite(c *check.C) {
	config.Set("database:name", "docker_provision_handlers_tests_s")
	config.Set("docker:collection", "docker_handler_suite")
	config.Set("docker:run-cmd:port", 8888)
	config.Set("docker:router", "fake")
	config.Set("docker:cluster:mongo-url", "127.0.0.1:27017")
	config.Set("docker:cluster:mongo-database", "docker_provision_handlers_tests_cluster_stor")
	config.Set("docker:repository-namespace", "tsuru")
	config.Set("queue:mongo-url", "127.0.0.1:27017")
	config.Set("queue:mongo-database", "queue_provision_docker_tests")
	config.Set("iaas:default", "test-iaas")
	config.Set("iaas:node-protocol", "http")
	config.Set("iaas:node-port", 1234)
	config.Set("admin-team", "admin")
	config.Set("routers:fake:type", "fake")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	pools, err := provision.ListPools(nil)
	c.Assert(err, check.IsNil)
	for _, pool := range pools {
		err = provision.RemovePool(pool.Name)
		c.Assert(err, check.IsNil)
	}
	s.server = httptest.NewServer(nil)
	s.user = &auth.User{Email: "myadmin@arrakis.com", Password: "123456", Quota: quota.Unlimited}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	_, err = nativeScheme.Create(s.user)
	c.Assert(err, check.IsNil)
	s.team = &auth.Team{Name: "admin", Users: []string{s.user.Email}}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
}

func (s *HandlersSuite) SetUpTest(c *check.C) {
	queue.ResetQueue()
	err := clearClusterStorage()
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = &dockerProvisioner{}
	err = mainDockerProvisioner.Initialize()
	c.Assert(err, check.IsNil)
	coll := mainDockerProvisioner.collection()
	defer coll.Close()
	err = dbtest.ClearAllCollectionsExcept(coll.Database, []string{"users", "tokens", "teams"})
	c.Assert(err, check.IsNil)
}

func (s *HandlersSuite) TearDownSuite(c *check.C) {
	coll := mainDockerProvisioner.collection()
	defer coll.Close()
	err := dbtest.ClearAllCollections(coll.Database)
	c.Assert(err, check.IsNil)
	s.conn.Close()
}

func (s *HandlersSuite) TestAddNodeHandler(c *check.C) {
	mainDockerProvisioner.cluster, _ = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{})
	err := provision.AddPool("pool1")
	defer provision.RemovePool("pool1")
	json := fmt.Sprintf(`{"address": "%s", "pool": "pool1"}`, s.server.URL)
	b := bytes.NewBufferString(json)
	req, err := http.NewRequest("POST", "/docker/node?register=true", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = addNodeHandler(rec, req, nil)
	c.Assert(err, check.IsNil)
	nodes, err := mainDockerProvisioner.getCluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address, check.Equals, s.server.URL)
	c.Assert(nodes[0].Metadata, check.DeepEquals, map[string]string{
		"pool": "pool1",
	})
}

func (s *HandlersSuite) TestAddNodeHandlerCreatingAnIaasMachine(c *check.C) {
	iaas.RegisterIaasProvider("test-iaas", newTestIaaS)
	mainDockerProvisioner.cluster, _ = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{})
	err := provision.AddPool("pool1")
	defer provision.RemovePool("pool1")
	b := bytes.NewBufferString(`{"pool": "pool1", "id": "test1"}`)
	req, err := http.NewRequest("POST", "/docker/node?register=false", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = addNodeHandler(rec, req, nil)
	c.Assert(err, check.IsNil)
	var result map[string]string
	err = json.NewDecoder(rec.Body).Decode(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, map[string]string{"description": "my iaas description"})
	nodes, err := mainDockerProvisioner.getCluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address, check.Equals, "http://test1.fake.host:1234")
	c.Assert(nodes[0].Metadata, check.DeepEquals, map[string]string{
		"id":      "test1",
		"pool":    "pool1",
		"iaas":    "test-iaas",
		"iaas-id": "test1",
	})
}

func (s *HandlersSuite) TestAddNodeHandlerCreatingAnIaasMachineExplicit(c *check.C) {
	iaas.RegisterIaasProvider("test-iaas", newTestIaaS)
	iaas.RegisterIaasProvider("another-test-iaas", newTestIaaS)
	mainDockerProvisioner.cluster, _ = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{})
	err := provision.AddPool("pool1")
	defer provision.RemovePool("pool1")
	b := bytes.NewBufferString(`{"pool": "pool1", "id": "test1", "iaas": "another-test-iaas"}`)
	req, err := http.NewRequest("POST", "/docker/node?register=false", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = addNodeHandler(rec, req, nil)
	c.Assert(err, check.IsNil)
	nodes, err := mainDockerProvisioner.getCluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address, check.Equals, "http://test1.fake.host:1234")
	c.Assert(nodes[0].Metadata, check.DeepEquals, map[string]string{
		"id":      "test1",
		"pool":    "pool1",
		"iaas":    "another-test-iaas",
		"iaas-id": "test1",
	})
}

func (s *HandlersSuite) TestAddNodeHandlerWithoutdCluster(c *check.C) {
	err := provision.AddPool("pool1")
	defer provision.RemovePool("pool1")
	config.Set("docker:cluster:redis-server", "127.0.0.1:6379")
	defer config.Unset("docker:cluster:redis-server")
	b := bytes.NewBufferString(fmt.Sprintf(`{"address": "%s", "pool": "pool1"}`, s.server.URL))
	req, err := http.NewRequest("POST", "/docker/node?register=true", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = addNodeHandler(rec, req, nil)
	c.Assert(err, check.IsNil)
	nodes, err := mainDockerProvisioner.getCluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address, check.Equals, s.server.URL)
	c.Assert(nodes[0].Metadata, check.DeepEquals, map[string]string{
		"pool": "pool1",
	})
}

func (s *HandlersSuite) TestAddNodeHandlerWithoutdAddress(c *check.C) {
	config.Set("docker:cluster:redis-server", "127.0.0.1:6379")
	defer config.Unset("docker:cluster:redis-server")
	b := bytes.NewBufferString(`{"pool": "pool1"}`)
	req, err := http.NewRequest("POST", "/docker/node?register=true", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = addNodeHandler(rec, req, nil)
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
	err = addNodeHandler(rec, req, nil)
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
	err = addNodeHandler(rec, req, nil)
	c.Assert(err, check.IsNil)
	err = json.NewDecoder(rec.Body).Decode(&result)
	c.Assert(err, check.IsNil)
	c.Assert(rec.Code, check.Equals, http.StatusBadRequest)
	c.Assert(result["error"], check.Matches, `Invalid address url: scheme must be http\[s\]`)
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
	_, err = mainDockerProvisioner.getCluster().Register("host.com:2375", nil)
	c.Assert(err, check.IsNil)
	b := bytes.NewBufferString(`{"address": "host.com:2375"}`)
	req, err := http.NewRequest("POST", "/node/remove", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = removeNodeHandler(rec, req, nil)
	c.Assert(err, check.IsNil)
	nodes, err := mainDockerProvisioner.getCluster().Nodes()
	c.Assert(len(nodes), check.Equals, 0)
}

func (s *HandlersSuite) TestRemoveNodeHandlerWithoutRemoveIaaS(c *check.C) {
	iaas.RegisterIaasProvider("some-iaas", newTestIaaS)
	machine, err := iaas.CreateMachineForIaaS("some-iaas", map[string]string{})
	c.Assert(err, check.IsNil)
	mainDockerProvisioner.cluster, err = cluster.New(nil, &cluster.MapStorage{})
	c.Assert(err, check.IsNil)
	_, err = mainDockerProvisioner.getCluster().Register(fmt.Sprintf("http://%s:2375", machine.Address), nil)
	c.Assert(err, check.IsNil)
	b := bytes.NewBufferString(fmt.Sprintf(`{"address": "http://%s:2375", "remove_iaas": "false"}`, machine.Address))
	req, err := http.NewRequest("POST", "/node/remove", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = removeNodeHandler(rec, req, nil)
	c.Assert(err, check.IsNil)
	nodes, err := mainDockerProvisioner.getCluster().Nodes()
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
	_, err = mainDockerProvisioner.getCluster().Register(fmt.Sprintf("http://%s:2375", machine.Address), nil)
	c.Assert(err, check.IsNil)
	b := bytes.NewBufferString(fmt.Sprintf(`{"address": "http://%s:2375", "remove_iaas": "true"}`, machine.Address))
	req, err := http.NewRequest("POST", "/node/remove", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = removeNodeHandler(rec, req, nil)
	c.Assert(err, check.IsNil)
	nodes, err := mainDockerProvisioner.getCluster().Nodes()
	c.Assert(len(nodes), check.Equals, 0)
	_, err = iaas.FindMachineById(machine.Id)
	c.Assert(err, check.Equals, mgo.ErrNotFound)
}

func (s *HandlersSuite) TestListNodeHandler(c *check.C) {
	var result struct {
		Nodes    []cluster.Node `json:"nodes"`
		Machines []iaas.Machine `json:"machines"`
	}
	var err error
	mainDockerProvisioner.cluster, err = cluster.New(nil, &cluster.MapStorage{})
	c.Assert(err, check.IsNil)
	_, err = mainDockerProvisioner.getCluster().Register("host1.com:2375", map[string]string{"pool": "pool1"})
	c.Assert(err, check.IsNil)
	_, err = mainDockerProvisioner.getCluster().Register("host2.com:2375", map[string]string{"pool": "pool2", "foo": "bar"})
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest("GET", "/node/", nil)
	rec := httptest.NewRecorder()
	err = listNodeHandler(rec, req, nil)
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

func (s *HandlersSuite) TestFixContainerHandler(c *check.C) {
	queue.ResetQueue()
	cleanup, server, p := startDocker("9999")
	defer cleanup()
	coll := p.collection()
	defer coll.Close()
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Apps().Insert(&app.App{Name: "makea"})
	c.Assert(err, check.IsNil)
	defer conn.Apps().RemoveAll(bson.M{"name": "makea"})
	err = coll.Insert(
		container{
			ID:       "9930c24f1c4x",
			AppName:  "makea",
			Type:     "python",
			Status:   provision.StatusStarted.String(),
			IP:       "127.0.0.4",
			HostPort: "9025",
			HostAddr: "127.0.0.1",
		},
	)
	c.Assert(err, check.IsNil)
	defer coll.RemoveAll(bson.M{"appname": "makea"})
	var storage cluster.MapStorage
	storage.StoreContainer("9930c24f1c4x", server.URL)
	mainDockerProvisioner = p
	mainDockerProvisioner.cluster, err = cluster.New(nil, &storage,
		cluster.Node{Address: server.URL},
	)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/fix-containers", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = fixContainersHandler(recorder, request, nil)
	c.Assert(err, check.IsNil)
	cont, err := p.getContainer("9930c24f1c4x")
	c.Assert(err, check.IsNil)
	c.Assert(cont.IP, check.Equals, "127.0.0.9")
	c.Assert(cont.HostPort, check.Equals, "9999")
}

func (s *HandlersSuite) TestListContainersByHostHandler(c *check.C) {
	var result []container
	var err error
	mainDockerProvisioner.cluster, err = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{})
	coll := mainDockerProvisioner.collection()
	c.Assert(err, check.IsNil)
	err = coll.Insert(container{ID: "blabla", Type: "python", HostAddr: "http://cittavld1182.globoi.com"})
	c.Assert(err, check.IsNil)
	defer coll.Remove(bson.M{"id": "blabla"})
	err = coll.Insert(container{ID: "bleble", Type: "java", HostAddr: "http://cittavld1182.globoi.com"})
	c.Assert(err, check.IsNil)
	defer coll.Remove(bson.M{"id": "bleble"})
	req, err := http.NewRequest("GET", "/node/cittavld1182.globoi.com/containers?:address=http://cittavld1182.globoi.com", nil)
	rec := httptest.NewRecorder()
	err = listContainersHandler(rec, req, nil)
	c.Assert(err, check.IsNil)
	body, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, check.IsNil)
	err = json.Unmarshal(body, &result)
	c.Assert(err, check.IsNil)
	c.Assert(result[0].ID, check.DeepEquals, "blabla")
	c.Assert(result[0].Type, check.DeepEquals, "python")
	c.Assert(result[0].HostAddr, check.DeepEquals, "http://cittavld1182.globoi.com")
	c.Assert(result[1].ID, check.DeepEquals, "bleble")
	c.Assert(result[1].Type, check.DeepEquals, "java")
	c.Assert(result[1].HostAddr, check.DeepEquals, "http://cittavld1182.globoi.com")
}

func (s *HandlersSuite) TestListContainersByAppHandler(c *check.C) {
	var result []container
	var err error
	mainDockerProvisioner.cluster, err = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{})
	coll := mainDockerProvisioner.collection()
	err = coll.Insert(container{ID: "blabla", AppName: "appbla", HostAddr: "http://cittavld1182.globoi.com"})
	c.Assert(err, check.IsNil)
	defer coll.Remove(bson.M{"id": "blabla"})
	err = coll.Insert(container{ID: "bleble", AppName: "appbla", HostAddr: "http://cittavld1180.globoi.com"})
	c.Assert(err, check.IsNil)
	defer coll.Remove(bson.M{"id": "bleble"})
	req, err := http.NewRequest("GET", "/node/appbla/containers?:appname=appbla", nil)
	rec := httptest.NewRecorder()
	err = listContainersHandler(rec, req, nil)
	c.Assert(err, check.IsNil)
	body, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, check.IsNil)
	err = json.Unmarshal(body, &result)
	c.Assert(err, check.IsNil)
	c.Assert(result[0].ID, check.DeepEquals, "blabla")
	c.Assert(result[0].AppName, check.DeepEquals, "appbla")
	c.Assert(result[0].HostAddr, check.DeepEquals, "http://cittavld1182.globoi.com")
	c.Assert(result[1].ID, check.DeepEquals, "bleble")
	c.Assert(result[1].AppName, check.DeepEquals, "appbla")
	c.Assert(result[1].HostAddr, check.DeepEquals, "http://cittavld1180.globoi.com")
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

func (s *HandlersSuite) TestMoveContainerHandler(c *check.C) {
	recorder := httptest.NewRecorder()
	b := bytes.NewBufferString(`{"to": "127.0.0.1"}`)
	request, err := http.NewRequest("POST", "/docker/container/myid/move", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	var result tsuruIo.SimpleJsonMessage
	err = json.Unmarshal(body, &result)
	c.Assert(err, check.IsNil)
	expected := tsuruIo.SimpleJsonMessage{Message: "Error trying to move container: unit not found\n"}
	c.Assert(result, check.DeepEquals, expected)
}

func (s *S) TestRebalanceContainersEmptyBodyHandler(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = p
	defer s.stopMultipleServersCluster(p)
	err = s.newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	coll := p.collection()
	defer coll.Close()
	coll.Insert(container{ID: "container-id", AppName: appInstance.GetName(), Version: "container-version", Image: "tsuru/python", ProcessName: "web"})
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
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name:     appInstance.GetName(),
		Platform: appInstance.GetPlatform(),
	}
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})
	err = conn.Apps().Update(
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
	c.Assert(result[13].Message, check.Equals, "Containers rebalanced successfully!\n")
}

func (s *S) TestRebalanceContainersFilters(c *check.C) {
	p, err := s.startMultipleServersClusterSeggregated()
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = p
	defer s.stopMultipleServersCluster(p)
	err = s.newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	coll := p.collection()
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
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name:     appInstance.GetName(),
		Platform: appInstance.GetPlatform(),
	}
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})
	err = conn.Apps().Update(
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
	c.Assert(result[1].Message, check.Equals, "Containers rebalanced successfully!\n")
}

func (s *S) TestRebalanceContainersDryBodyHandler(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = p
	defer s.stopMultipleServersCluster(p)
	err = s.newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	coll := p.collection()
	defer coll.Close()
	coll.Insert(container{ID: "container-id", AppName: appInstance.GetName(), Version: "container-version", Image: "tsuru/python", ProcessName: "web"})
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
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name:     appInstance.GetName(),
		Platform: appInstance.GetPlatform(),
	}
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})
	err = conn.Apps().Update(
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
	c.Assert(result[7].Message, check.Equals, "Containers rebalanced successfully!\n")
}

func (s *HandlersSuite) TestHealingHistoryHandler(c *check.C) {
	evt1, err := newHealingEvent(cluster.Node{Address: "addr1"})
	c.Assert(err, check.IsNil)
	evt1.update(cluster.Node{Address: "addr2"}, nil)
	evt2, err := newHealingEvent(cluster.Node{Address: "addr3"})
	evt2.update(cluster.Node{}, errors.New("some error"))
	evt3, err := newHealingEvent(container{ID: "1234"})
	evt3.update(container{ID: "9876"}, nil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/docker/healing", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body := recorder.Body.Bytes()
	healings := []healingEvent{}
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
	evt1, err := newHealingEvent(cluster.Node{Address: "addr1"})
	c.Assert(err, check.IsNil)
	evt1.update(cluster.Node{Address: "addr2"}, nil)
	evt2, err := newHealingEvent(cluster.Node{Address: "addr3"})
	evt2.update(cluster.Node{}, errors.New("some error"))
	evt3, err := newHealingEvent(container{ID: "1234"})
	evt3.update(container{ID: "9876"}, nil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/docker/healing?filter=container", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body := recorder.Body.Bytes()
	healings := []healingEvent{}
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
	evt1, err := newHealingEvent(cluster.Node{Address: "addr1"})
	c.Assert(err, check.IsNil)
	evt1.update(cluster.Node{Address: "addr2"}, nil)
	evt2, err := newHealingEvent(cluster.Node{Address: "addr3"})
	evt2.update(cluster.Node{}, errors.New("some error"))
	evt3, err := newHealingEvent(container{ID: "1234"})
	evt3.update(container{ID: "9876"}, nil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/docker/healing?filter=node", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := api.RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body := recorder.Body.Bytes()
	healings := []healingEvent{}
	err = json.Unmarshal(body, &healings)
	c.Assert(err, check.IsNil)
	c.Assert(healings, check.HasLen, 2)
	c.Assert(healings[0].Action, check.Equals, "node-healing")
	c.Assert(healings[0].ID, check.Equals, evt2.ID)
	c.Assert(healings[1].Action, check.Equals, "node-healing")
	c.Assert(healings[1].ID, check.Equals, evt1.ID)
}

func (s *HandlersSuite) TestAutoScaleHistoryHandler(c *check.C) {
	evt1, err := newAutoScaleEvent("poolx")
	c.Assert(err, check.IsNil)
	err = evt1.update("add", "reason 1")
	c.Assert(err, check.IsNil)
	err = evt1.finish(nil, "my log")
	c.Assert(err, check.IsNil)
	evt2, err := newAutoScaleEvent("pooly")
	c.Assert(err, check.IsNil)
	err = evt2.update("rebalance", "reason 2")
	c.Assert(err, check.IsNil)
	err = evt2.finish(errors.New("my error"), "my log 2")
	c.Assert(err, check.IsNil)
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
	err := provision.AddPool("pool1")
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
	nodes, err := mainDockerProvisioner.getCluster().Nodes()
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
	err := provision.AddPool("pool1")
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

func (s *HandlersSuite) TestAutoScaleRunHandler(c *check.C) {
	mainDockerProvisioner.cluster, _ = cluster.New(&segregatedScheduler{}, &cluster.MapStorage{},
		cluster.Node{Address: "localhost:1999", Metadata: map[string]string{
			"pool": "pool1",
		}},
	)
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
		`{"Message":"[node autoscale] running scaler *docker.countScaler for \"pool\": \"pool1\"\n"}`,
		`{"Message":"[node autoscale] nothing to do for \"pool\": \"pool1\"\n"}`,
		``,
	})
}
