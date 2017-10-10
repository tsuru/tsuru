// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/fsouza/go-dockerclient/testing"
	dtesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/api"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/provision/docker/container"
	dockerTypes "github.com/tsuru/tsuru/provision/docker/types"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/types"
	serviceTypes "github.com/tsuru/tsuru/types/service"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
)

type HandlersSuite struct {
	conn        *db.Storage
	user        *auth.User
	token       auth.Token
	team        *types.Team
	clusterSess *mgo.Session
	p           *dockerProvisioner
	server      *dtesting.DockerServer
}

var _ = check.Suite(&HandlersSuite{})
var nativeScheme = auth.ManagedScheme(native.NativeScheme{})

func (s *HandlersSuite) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
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
	config.Set("auth:hash-cost", bcrypt.MinCost)
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	clusterDbURL, _ := config.GetString("docker:cluster:mongo-url")
	s.clusterSess, err = mgo.Dial(clusterDbURL)
	c.Assert(err, check.IsNil)
	app.AuthScheme = nativeScheme
	s.team = &types.Team{Name: "admin"}
	err = serviceTypes.Team().Insert(*s.team)
	c.Assert(err, check.IsNil)
}

func (s *HandlersSuite) SetUpTest(c *check.C) {
	config.Set("docker:api-timeout", 2)
	queue.ResetQueue()
	err := clearClusterStorage(s.clusterSess)
	c.Assert(err, check.IsNil)
	s.server, err = dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	s.p = &dockerProvisioner{storage: &cluster.MapStorage{}}
	mainDockerProvisioner = s.p
	err = mainDockerProvisioner.Initialize()
	c.Assert(err, check.IsNil)
	s.p.cluster, err = cluster.New(nil, s.p.storage, "",
		cluster.Node{Address: s.server.URL(), Metadata: map[string]string{"pool": "test-default"}},
	)
	c.Assert(err, check.IsNil)
	coll := mainDockerProvisioner.Collection()
	defer coll.Close()
	err = dbtest.ClearAllCollections(coll.Database)
	c.Assert(err, check.IsNil)
	s.user, s.token = permissiontest.CustomUserWithPermission(c, nativeScheme, "provisioner-docker", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.PermissionContext{CtxType: permission.CtxGlobal},
	})
}

func (s *HandlersSuite) TearDownTest(c *check.C) {
	s.server.Stop()
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

func startFakeDockerNode(c *check.C) (*testing.DockerServer, func()) {
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
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeNode, Value: "localhost"},
		Owner:  s.token.GetUserName(),
		Kind:   "node.update.move.containers",
		StartCustomData: []map[string]interface{}{
			{"name": "from", "value": "localhost"},
			{"name": "to", "value": "127.0.0.1"},
		},
	}, eventtest.HasEvent)
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
		var pool string
		var customData []map[string]interface{}
		for k, v := range val {
			if k == "pool" {
				pool = v[0]
				continue
			}
			customData = append(customData, map[string]interface{}{"name": k, "value": v[0]})
		}
		c.Assert(eventtest.EventDesc{
			Target:          event.Target{Type: event.TargetTypePool, Value: pool},
			Owner:           s.token.GetUserName(),
			Kind:            "pool.update.logs",
			StartCustomData: customData,
		}, eventtest.HasEvent)
	}
	doReq(values1)
	entries, err := container.LogLoadAll()
	c.Assert(err, check.IsNil)
	c.Assert(entries, check.DeepEquals, map[string]container.DockerLogConfig{
		"": {DockerLogConfig: dockerTypes.DockerLogConfig{Driver: "awslogs", LogOpts: map[string]string{"awslogs-region": "sa-east1"}}},
	})
	doReq(values2)
	entries, err = container.LogLoadAll()
	c.Assert(err, check.IsNil)
	c.Assert(entries, check.DeepEquals, map[string]container.DockerLogConfig{
		"":      {DockerLogConfig: dockerTypes.DockerLogConfig{Driver: "awslogs", LogOpts: map[string]string{"awslogs-region": "sa-east1"}}},
		"POOL1": {DockerLogConfig: dockerTypes.DockerLogConfig{Driver: "bs", LogOpts: map[string]string{}}},
	})
	doReq(values3)
	entries, err = container.LogLoadAll()
	c.Assert(err, check.IsNil)
	c.Assert(entries, check.DeepEquals, map[string]container.DockerLogConfig{
		"":      {DockerLogConfig: dockerTypes.DockerLogConfig{Driver: "awslogs", LogOpts: map[string]string{"awslogs-region": "sa-east1"}}},
		"POOL1": {DockerLogConfig: dockerTypes.DockerLogConfig{Driver: "bs", LogOpts: map[string]string{}}},
		"POOL2": {DockerLogConfig: dockerTypes.DockerLogConfig{Driver: "fluentd", LogOpts: map[string]string{"fluentd-address": "localhost:2222"}}},
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
		"": {DockerLogConfig: dockerTypes.DockerLogConfig{Driver: "awslogs", LogOpts: map[string]string{"awslogs-region": "sa-east1"}}},
	})
}

func (s *HandlersSuite) TestDockerLogsUpdateHandlerWithRestartSomeApps(c *check.C) {
	appPools := [][]string{{"app1", "pool1"}, {"app2", "pool2"}, {"app3", "pool2"}}
	storage, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer storage.Close()
	for _, appPool := range appPools {
		opts := pool.AddPoolOptions{Name: appPool[1]}
		pool.AddPool(opts)
		err = newFakeImage(s.p, "tsuru/app-"+appPool[0], nil)
		c.Assert(err, check.IsNil)
		appInstance := provisiontest.NewFakeApp(appPool[0], "python", 0)
		appStruct := &app.App{
			Name:     appInstance.GetName(),
			Platform: appInstance.GetPlatform(),
			Pool:     opts.Name,
			Router:   "fake",
		}

		err = storage.Apps().Insert(appStruct)
		c.Assert(err, check.IsNil)
		err = s.p.Provision(appStruct)
		c.Assert(err, check.IsNil)
	}
	values := url.Values{
		"pool":                   []string{"pool2"},
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
		"pool2": {DockerLogConfig: dockerTypes.DockerLogConfig{Driver: "awslogs", LogOpts: map[string]string{"awslogs-region": "sa-east1"}}},
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
	newConf := container.DockerLogConfig{DockerLogConfig: dockerTypes.DockerLogConfig{Driver: "syslog"}}
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
		"p1": {DockerLogConfig: dockerTypes.DockerLogConfig{Driver: "syslog", LogOpts: map[string]string{}}},
	})
}
