// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/testing"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"strings"
)

type HandlersSuite struct {
	conn   *db.Storage
	server *httptest.Server
}

var _ = gocheck.Suite(&HandlersSuite{})

func (s *HandlersSuite) SetUpSuite(c *gocheck.C) {
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
	config.Set("docker:collection", "docker_handler_suite")
	config.Set("docker:run-cmd:port", 8888)
	config.Set("docker:router", "fake")
	s.conn.Collection(schedulerCollection).RemoveAll(nil)
	s.server = httptest.NewServer(nil)
}

func (s *HandlersSuite) TearDownSuite(c *gocheck.C) {
	coll := collection()
	defer coll.Close()
	err := coll.Database.DropDatabase()
	c.Assert(err, gocheck.IsNil)
	s.conn.Close()
}

func (s *HandlersSuite) TestAddNodeHandler(c *gocheck.C) {
	dCluster, _ = cluster.New(&segScheduler, nil)
	p := Pool{Name: "pool1"}
	s.conn.Collection(schedulerCollection).Insert(p)
	json := fmt.Sprintf(`{"address": "%s", "pool": "pool1"}`, s.server.URL)
	b := bytes.NewBufferString(json)
	req, err := http.NewRequest("POST", "/docker/node?register=true", b)
	c.Assert(err, gocheck.IsNil)
	rec := httptest.NewRecorder()
	err = addNodeHandler(rec, req, nil)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(schedulerCollection).RemoveId("pool1")
	n, err := s.conn.Collection(schedulerCollection).FindId("pool1").Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 1)
}

func (s *HandlersSuite) TestAddNodeHandlerWithoutdCluster(c *gocheck.C) {
	p := Pool{Name: "pool1"}
	s.conn.Collection(schedulerCollection).Insert(p)
	config.Set("docker:segregate", true)
	defer config.Unset("docker:segregate")
	config.Set("docker:scheduler:redis-server", "127.0.0.1:6379")
	defer config.Unset("docker:scheduler:redis-server")
	dCluster = nil
	b := bytes.NewBufferString(fmt.Sprintf(`{"address": "%s", "pool": "pool1"}`, s.server.URL))
	req, err := http.NewRequest("POST", "/docker/node?register=true", b)
	c.Assert(err, gocheck.IsNil)
	rec := httptest.NewRecorder()
	err = addNodeHandler(rec, req, nil)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(schedulerCollection).RemoveId("pool1")
	n, err := s.conn.Collection(schedulerCollection).FindId("pool1").Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 1)
}

func (s *HandlersSuite) TestAddNodeHandlerWithoutdAddress(c *gocheck.C) {
	b := bytes.NewBufferString(`{"pool": "pool1"}`)
	req, err := http.NewRequest("POST", "/docker/node?register=true", b)
	c.Assert(err, gocheck.IsNil)
	rec := httptest.NewRecorder()
	err = addNodeHandler(rec, req, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Node address is required.")
}

func (s *HandlersSuite) TestAddNodeHandlerWithInvalidURLAddress(c *gocheck.C) {
	b := bytes.NewBufferString(`{"address": "url:1234", "pool": "pool1"}`)
	req, err := http.NewRequest("POST", "/docker/node?register=true", b)
	c.Assert(err, gocheck.IsNil)
	rec := httptest.NewRecorder()
	err = addNodeHandler(rec, req, nil)
	c.Assert(err, gocheck.NotNil)
}

func (s *HandlersSuite) TestAddNodeHandlerWithInaccessibleAddress(c *gocheck.C) {
	b := bytes.NewBufferString(`{"address": "cant-access-url:1234", "pool": "pool1"}`)
	req, err := http.NewRequest("POST", "/docker/node?register=true", b)
	c.Assert(err, gocheck.IsNil)
	rec := httptest.NewRecorder()
	err = addNodeHandler(rec, req, nil)
	c.Assert(err, gocheck.NotNil)
}

func (s *HandlersSuite) TestRemoveNodeHandler(c *gocheck.C) {
	p := Pool{Name: "pool1", Nodes: []string{"host.com:4243"}}
	err := s.conn.Collection(schedulerCollection).Insert(p)
	c.Assert(err, gocheck.IsNil)
	dCluster, _ = cluster.New(&segScheduler, nil)
	var pool Pool
	err = s.conn.Collection(schedulerCollection).FindId("pool1").One(&pool)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(pool.Nodes), gocheck.Equals, 1)
	b := bytes.NewBufferString(`{"address": "host.com:4243", "pool": "pool1"}`)
	req, err := http.NewRequest("POST", "/node/remove", b)
	c.Assert(err, gocheck.IsNil)
	rec := httptest.NewRecorder()
	err = removeNodeHandler(rec, req, nil)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(schedulerCollection).RemoveId("pool1")
	err = s.conn.Collection(schedulerCollection).FindId("pool1").One(&pool)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(pool.Nodes), gocheck.Equals, 0)
}

func (s *HandlersSuite) TestRemoveNodeHandlerWithoutCluster(c *gocheck.C) {
	p := Pool{Name: "pool1", Nodes: []string{"host.com:4243"}}
	err := s.conn.Collection(schedulerCollection).Insert(p)
	c.Assert(err, gocheck.IsNil)
	config.Set("docker:segregate", true)
	defer config.Unset("docker:segregate")
	config.Set("docker:scheduler:redis-server", "127.0.0.1:6379")
	defer config.Unset("docker:scheduler:redis-server")
	dCluster = nil
	var pool Pool
	err = s.conn.Collection(schedulerCollection).FindId("pool1").One(&pool)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(pool.Nodes), gocheck.Equals, 1)
	b := bytes.NewBufferString(`{"address": "host.com:4243", "pool": "pool1"}`)
	req, err := http.NewRequest("POST", "/node/remove", b)
	c.Assert(err, gocheck.IsNil)
	rec := httptest.NewRecorder()
	err = removeNodeHandler(rec, req, nil)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(schedulerCollection).RemoveId("pool1")
	err = s.conn.Collection(schedulerCollection).FindId("pool1").One(&pool)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(pool.Nodes), gocheck.Equals, 0)
}

func (s *HandlersSuite) TestListNodeHandler(c *gocheck.C) {
	var result []map[string]string
	dCluster, _ = cluster.New(segScheduler, nil)
	p1 := Pool{Name: "pool1", Nodes: []string{"host.com:4243"}}
	p2 := Pool{Name: "pool2", Nodes: []string{"host.com:4243"}}
	err := s.conn.Collection(schedulerCollection).Insert(p1, p2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(schedulerCollection).RemoveId(p1.Name)
	defer s.conn.Collection(schedulerCollection).RemoveId(p2.Name)
	req, err := http.NewRequest("GET", "/node/", nil)
	rec := httptest.NewRecorder()
	err = listNodeHandler(rec, req, nil)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	err = json.Unmarshal(body, &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result[0]["ID"], gocheck.Equals, "host.com:4243")
	c.Assert(result[0]["Address"], gocheck.DeepEquals, "host.com:4243")
	c.Assert(result[1]["ID"], gocheck.Equals, "host.com:4243")
	c.Assert(result[1]["Address"], gocheck.DeepEquals, "host.com:4243")
}

func (s *HandlersSuite) TestListNodeHandlerWithoutCluster(c *gocheck.C) {
	var result []map[string]string
	p1 := Pool{Name: "pool1", Nodes: []string{"host.com:4243"}}
	p2 := Pool{Name: "pool2", Nodes: []string{"host.com:4243"}}
	err := s.conn.Collection(schedulerCollection).Insert(p1, p2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(schedulerCollection).RemoveId(p1.Name)
	defer s.conn.Collection(schedulerCollection).RemoveId(p2.Name)
	config.Set("docker:segregate", true)
	defer config.Unset("docker:segregate")
	config.Set("docker:scheduler:redis-server", "127.0.0.1:6379")
	defer config.Unset("docker:scheduler:redis-server")
	dCluster = nil
	req, err := http.NewRequest("GET", "/node/", nil)
	rec := httptest.NewRecorder()
	err = listNodeHandler(rec, req, nil)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	err = json.Unmarshal(body, &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result[0]["ID"], gocheck.DeepEquals, "host.com:4243")
	c.Assert(result[0]["Address"], gocheck.DeepEquals, "host.com:4243")
	c.Assert(result[1]["ID"], gocheck.DeepEquals, "host.com:4243")
	c.Assert(result[1]["Address"], gocheck.DeepEquals, "host.com:4243")
}

func (s *HandlersSuite) TestFixContainerHandler(c *gocheck.C) {
	coll := collection()
	defer coll.Close()
	err := coll.Insert(
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
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"appname": "makea"})
	cleanup, server := startDocker()
	defer cleanup()
	var storage mapStorage
	storage.StoreContainer("9930c24f1c4x", "server0")
	cmutex.Lock()
	dCluster, err = cluster.New(nil, &storage,
		cluster.Node{ID: "server0", Address: server.URL},
	)
	cmutex.Unlock()
	request, err := http.NewRequest("POST", "/fix-containers", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = fixContainersHandler(recorder, request, nil)
	c.Assert(err, gocheck.IsNil)
	cont, err := getContainer("9930c24f1c4x")
	c.Assert(err, gocheck.IsNil)
	c.Assert(cont.IP, gocheck.Equals, "127.0.0.9")
	c.Assert(cont.HostPort, gocheck.Equals, "9999")
}

func (s *HandlersSuite) TestListContainersByHostHandler(c *gocheck.C) {
	var result []container
	coll := collection()
	dCluster, _ = cluster.New(segScheduler, nil)
	err := coll.Insert(container{ID: "blabla", Type: "python", HostAddr: "http://cittavld1182.globoi.com"})
	c.Assert(err, gocheck.IsNil)
	defer coll.Remove(bson.M{"id": "blabla"})
	err = coll.Insert(container{ID: "bleble", Type: "java", HostAddr: "http://cittavld1182.globoi.com"})
	c.Assert(err, gocheck.IsNil)
	defer coll.Remove(bson.M{"id": "bleble"})
	req, err := http.NewRequest("GET", "/node/cittavld1182.globoi.com/containers?:address=http://cittavld1182.globoi.com", nil)
	rec := httptest.NewRecorder()
	err = listContainersHandler(rec, req, nil)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	err = json.Unmarshal(body, &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result[0].ID, gocheck.DeepEquals, "blabla")
	c.Assert(result[0].Type, gocheck.DeepEquals, "python")
	c.Assert(result[0].HostAddr, gocheck.DeepEquals, "http://cittavld1182.globoi.com")
	c.Assert(result[1].ID, gocheck.DeepEquals, "bleble")
	c.Assert(result[1].Type, gocheck.DeepEquals, "java")
	c.Assert(result[1].HostAddr, gocheck.DeepEquals, "http://cittavld1182.globoi.com")
}

func (s *HandlersSuite) TestListContainersByAppHandler(c *gocheck.C) {
	var result []container
	coll := collection()
	dCluster, _ = cluster.New(segScheduler, nil)
	err := coll.Insert(container{ID: "blabla", AppName: "appbla", HostAddr: "http://cittavld1182.globoi.com"})
	c.Assert(err, gocheck.IsNil)
	defer coll.Remove(bson.M{"id": "blabla"})
	err = coll.Insert(container{ID: "bleble", AppName: "appbla", HostAddr: "http://cittavld1180.globoi.com"})
	c.Assert(err, gocheck.IsNil)
	defer coll.Remove(bson.M{"id": "bleble"})
	req, err := http.NewRequest("GET", "/node/appbla/containers?:appname=appbla", nil)
	rec := httptest.NewRecorder()
	err = listContainersHandler(rec, req, nil)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	err = json.Unmarshal(body, &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result[0].ID, gocheck.DeepEquals, "blabla")
	c.Assert(result[0].AppName, gocheck.DeepEquals, "appbla")
	c.Assert(result[0].HostAddr, gocheck.DeepEquals, "http://cittavld1182.globoi.com")
	c.Assert(result[1].ID, gocheck.DeepEquals, "bleble")
	c.Assert(result[1].AppName, gocheck.DeepEquals, "appbla")
	c.Assert(result[1].HostAddr, gocheck.DeepEquals, "http://cittavld1180.globoi.com")
}

func (s *HandlersSuite) TestMoveContainersEmptyBodyHandler(c *gocheck.C) {
	b := bytes.NewBufferString("")
	req, err := http.NewRequest("POST", "/containers/move", b)
	rec := httptest.NewRecorder()
	err = moveContainersHandler(rec, req, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "unexpected end of JSON input")
}

func (s *HandlersSuite) TestMoveContainersEmptyToHandler(c *gocheck.C) {
	b := bytes.NewBufferString(`{"from": "fromhost", "to": ""}`)
	req, err := http.NewRequest("POST", "/containers/move", b)
	rec := httptest.NewRecorder()
	err = moveContainersHandler(rec, req, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Invalid params: from: fromhost - to: ")
}

func (s *HandlersSuite) TestMoveContainersHandler(c *gocheck.C) {
	b := bytes.NewBufferString(`{"from": "localhost", "to": "127.0.0.1"}`)
	req, err := http.NewRequest("POST", "/containers/move", b)
	rec := httptest.NewRecorder()
	err = moveContainersHandler(rec, req, nil)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	validJson := fmt.Sprintf("[%s]", strings.Replace(strings.Trim(string(body), "\n "), "\n", ",", -1))
	var result []progressLog
	err = json.Unmarshal([]byte(validJson), &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.DeepEquals, []progressLog{
		{Message: "No units to move in localhost."},
		{Message: "Containers moved successfully!"},
	})
}

func (s *HandlersSuite) TestMoveContainerHandler(c *gocheck.C) {
	b := bytes.NewBufferString(`{"to": "127.0.0.1"}`)
	req, err := http.NewRequest("POST", "/container/myid/move?:id=myid", b)
	rec := httptest.NewRecorder()
	err = moveContainerHandler(rec, req, nil)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	var result progressLog
	err = json.Unmarshal(body, &result)
	c.Assert(err, gocheck.IsNil)
	expected := progressLog{Message: "Error trying to move container: not found"}
	c.Assert(result, gocheck.DeepEquals, expected)
}

func (s *S) TestRebalanceContainersEmptyBodyHandler(c *gocheck.C) {
	cluster, err := s.startMultipleServersCluster()
	c.Assert(err, gocheck.IsNil)
	defer s.stopMultipleServersCluster(cluster)
	err = newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	appInstance := testing.NewFakeApp("myapp", "python", 0)
	var p dockerProvisioner
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	coll := collection()
	defer coll.Close()
	coll.Insert(container{ID: "container-id", AppName: appInstance.GetName(), Version: "container-version", Image: "tsuru/python"})
	defer coll.RemoveAll(bson.M{"appname": appInstance.GetName()})
	units, err := addUnitsWithHost(appInstance, 5, "localhost")
	c.Assert(err, gocheck.IsNil)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name:     appInstance.GetName(),
		Platform: appInstance.GetPlatform(),
	}
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})
	err = conn.Apps().Update(
		bson.M{"name": appStruct.Name},
		bson.M{"$set": bson.M{"units": units}},
	)
	c.Assert(err, gocheck.IsNil)
	b := bytes.NewBufferString("")
	req, err := http.NewRequest("POST", "/containers/move", b)
	rec := httptest.NewRecorder()
	err = rebalanceContainersHandler(rec, req, nil)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	validJson := fmt.Sprintf("[%s]", strings.Replace(strings.Trim(string(body), "\n "), "\n", ",", -1))
	var result []progressLog
	err = json.Unmarshal([]byte(validJson), &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(result), gocheck.Equals, 10)
	c.Assert(result[0].Message, gocheck.Equals, "Rebalancing app \"myapp\" (6 units)...")
	c.Assert(result[1].Message, gocheck.Equals, "Trying to move 2 units for \"myapp\" from localhost...")
	c.Assert(result[2].Message, gocheck.Matches, "Moving unit .*")
	c.Assert(result[3].Message, gocheck.Matches, "Moving unit .*")
	c.Assert(result[8].Message, gocheck.Equals, "Rebalance finished for \"myapp\"")
	c.Assert(result[9].Message, gocheck.Equals, "Containers rebalanced successfully!")
	testing.CleanQ("tsuru-app")
}

func (s *S) TestRebalanceContainersDryBodyHandler(c *gocheck.C) {
	cluster, err := s.startMultipleServersCluster()
	c.Assert(err, gocheck.IsNil)
	defer s.stopMultipleServersCluster(cluster)
	err = newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	appInstance := testing.NewFakeApp("myapp", "python", 0)
	var p dockerProvisioner
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	coll := collection()
	defer coll.Close()
	coll.Insert(container{ID: "container-id", AppName: appInstance.GetName(), Version: "container-version", Image: "tsuru/python"})
	defer coll.RemoveAll(bson.M{"appname": appInstance.GetName()})
	units, err := addUnitsWithHost(appInstance, 5, "localhost")
	c.Assert(err, gocheck.IsNil)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name:     appInstance.GetName(),
		Platform: appInstance.GetPlatform(),
	}
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})
	err = conn.Apps().Update(
		bson.M{"name": appStruct.Name},
		bson.M{"$set": bson.M{"units": units}},
	)
	c.Assert(err, gocheck.IsNil)
	b := bytes.NewBufferString(`{"dry": "true"}`)
	req, err := http.NewRequest("POST", "/containers/move", b)
	rec := httptest.NewRecorder()
	err = rebalanceContainersHandler(rec, req, nil)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	validJson := fmt.Sprintf("[%s]", strings.Replace(strings.Trim(string(body), "\n "), "\n", ",", -1))
	var result []progressLog
	err = json.Unmarshal([]byte(validJson), &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(result), gocheck.Equals, 6)
	c.Assert(result[0].Message, gocheck.Equals, "Rebalancing app \"myapp\" (6 units)...")
	c.Assert(result[1].Message, gocheck.Equals, "Trying to move 2 units for \"myapp\" from localhost...")
	c.Assert(result[2].Message, gocheck.Matches, "Would move unit .*")
	c.Assert(result[3].Message, gocheck.Matches, "Would move unit .*")
	c.Assert(result[4].Message, gocheck.Equals, "Rebalance finished for \"myapp\"")
	c.Assert(result[5].Message, gocheck.Equals, "Containers rebalanced successfully!")
	testing.CleanQ("tsuru-app")
}

func (s *HandlersSuite) TestAddPoolHandler(c *gocheck.C) {
	b := bytes.NewBufferString(`{"pool": "pool1"}`)
	req, err := http.NewRequest("POST", "/pool", b)
	c.Assert(err, gocheck.IsNil)
	rec := httptest.NewRecorder()
	err = addPoolHandler(rec, req, nil)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(schedulerCollection).RemoveId("pool1")
	n, err := s.conn.Collection(schedulerCollection).FindId("pool1").Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 1)
}

func (s *HandlersSuite) TestRemovePoolHandler(c *gocheck.C) {
	pool := Pool{Name: "pool1"}
	err := s.conn.Collection(schedulerCollection).Insert(pool)
	c.Assert(err, gocheck.IsNil)
	b := bytes.NewBufferString(`{"pool": "pool1"}`)
	req, err := http.NewRequest("DELETE", "/pool", b)
	c.Assert(err, gocheck.IsNil)
	rec := httptest.NewRecorder()
	err = removePoolHandler(rec, req, nil)
	c.Assert(err, gocheck.IsNil)
	p, err := s.conn.Collection(schedulerCollection).FindId("pool1").Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(p, gocheck.Equals, 0)
}

func (s *HandlersSuite) TestListPoolsHandler(c *gocheck.C) {
	pool := Pool{Name: "pool1", Nodes: []string{"url:1234", "url:2345"}, Teams: []string{"tsuruteam", "ateam"}}
	err := s.conn.Collection(schedulerCollection).Insert(pool)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(schedulerCollection).RemoveId(pool.Name)
	poolsExpected := []Pool{pool}
	req, err := http.NewRequest("GET", "/pool", nil)
	c.Assert(err, gocheck.IsNil)
	rec := httptest.NewRecorder()
	err = listPoolHandler(rec, req, nil)
	c.Assert(err, gocheck.IsNil)
	var pools []Pool
	err = json.NewDecoder(rec.Body).Decode(&pools)
	c.Assert(err, gocheck.IsNil)
	c.Assert(pools, gocheck.DeepEquals, poolsExpected)
}

func (s *HandlersSuite) TestAddTeamsToPoolHandler(c *gocheck.C) {
	pool := Pool{Name: "pool1"}
	err := s.conn.Collection(schedulerCollection).Insert(pool)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(schedulerCollection).RemoveId(pool.Name)
	b := bytes.NewBufferString(`{"pool": "pool1", "teams": ["test"]}`)
	req, err := http.NewRequest("POST", "/pool/team", b)
	c.Assert(err, gocheck.IsNil)
	rec := httptest.NewRecorder()
	err = addTeamToPoolHandler(rec, req, nil)
	c.Assert(err, gocheck.IsNil)
	var p Pool
	err = s.conn.Collection(schedulerCollection).FindId("pool1").One(&p)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.Teams, gocheck.DeepEquals, []string{"test"})
}

func (s *HandlersSuite) TestRemoveTeamsToPoolHandler(c *gocheck.C) {
	pool := Pool{Name: "pool1", Teams: []string{"test"}}
	err := s.conn.Collection(schedulerCollection).Insert(pool)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(schedulerCollection).RemoveId(pool.Name)
	b := bytes.NewBufferString(`{"pool": "pool1", "teams": ["test"]}`)
	req, err := http.NewRequest("DELETE", "/pool/team", b)
	c.Assert(err, gocheck.IsNil)
	rec := httptest.NewRecorder()
	err = removeTeamToPoolHandler(rec, req, nil)
	c.Assert(err, gocheck.IsNil)
	var p Pool
	err = s.conn.Collection(schedulerCollection).FindId("pool1").One(&p)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.Teams, gocheck.DeepEquals, []string{})
}
