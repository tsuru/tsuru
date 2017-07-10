// Copyright 2017 docker-cluster authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cluster

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsouza/go-dockerclient"
	dtesting "github.com/fsouza/go-dockerclient/testing"
	cstorage "github.com/tsuru/docker-cluster/storage"
	"github.com/tsuru/tsuru/safe"
)

func TestCreateContainer(t *testing.T) {
	body := `{"Id":"e90302"}`
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server2.Close()
	cluster, err := New(nil, &MapStorage{}, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	config := docker.Config{Memory: 67108864, Image: "myhost/somwhere/myimg"}
	nodeAddr, container, err := cluster.CreateContainer(docker.CreateContainerOptions{Config: &config}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if nodeAddr != server1.URL {
		t.Errorf("CreateContainer: wrong node  ID. Want %q. Got %q.", server1.URL, nodeAddr)
	}
	if container.ID != "e90302" {
		t.Errorf("CreateContainer: wrong container ID. Want %q. Got %q.", "e90302", container.ID)
	}
	img, err := cluster.storage().RetrieveImage("myhost/somwhere/myimg")
	if err != nil {
		t.Fatal(err)
	}
	if img.LastNode != server1.URL {
		t.Fatalf("CreateContainer: should store image in host, found %s", img.LastNode)
	}
	if len(img.History) != 1 {
		t.Fatal("CreateContainer: should store image id in host, none found")
	}
}

func TestCreateContainerOptions(t *testing.T) {
	body := `{"Id":"e90302"}`
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server2.Close()
	cluster, err := New(nil, &MapStorage{}, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	config := docker.Config{Memory: 67108864, Image: "myimg"}
	opts := docker.CreateContainerOptions{Name: "name", Config: &config}
	nodeAddr, container, err := cluster.CreateContainer(opts, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if nodeAddr != server1.URL {
		t.Errorf("CreateContainer: wrong node  ID. Want %q. Got %q.", server1.URL, nodeAddr)
	}
	if container.ID != "e90302" {
		t.Errorf("CreateContainer: wrong container ID. Want %q. Got %q.", "e90302", container.ID)
	}
}

func TestCreateContainerErrorImageInRepo(t *testing.T) {
	server1, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server1.Stop()
	server1.PrepareFailure("createImgErr", "/images/create")
	cluster, err := New(nil, &MapStorage{}, "",
		Node{Address: server1.URL()},
	)
	if err != nil {
		t.Fatal(err)
	}
	config := docker.Config{Memory: 67108864, Image: "myserver/user/myimg"}
	wait := registerErrorWait()
	addr, _, err := cluster.CreateContainer(docker.CreateContainerOptions{Config: &config}, time.Minute)
	if err == nil || strings.Index(err.Error(), "createImgErr") == -1 {
		t.Fatalf("Expected pull image error, got: %s", err)
	}
	if addr != server1.URL() {
		t.Errorf("CreateContainer: wrong node addr. Want %q. Got %q.", server1.URL(), addr)
	}
	wait()
	nodes, err := cluster.UnfilteredNodes()
	if err != nil {
		t.Fatal(err)
	}
	if nodes[0].FailureCount() != 0 {
		t.Fatalf("Expected failure count to be 0, got: %d", nodes[0].FailureCount())
	}
}

func registerErrorWait() func() {
	wg := sync.WaitGroup{}
	wg.Add(1)
	nodeUpdatedOnError.Store(func() {
		wg.Done()
	})
	return func() {
		wg.Wait()
		nodeUpdatedOnError.Store(func() {})
	}
}

func TestCreateContainerErrorInCreateContainer(t *testing.T) {
	server1, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server1.Stop()
	server1.PrepareFailure("createContErr", "/containers/create")
	cluster, err := New(nil, &MapStorage{}, "",
		Node{Address: server1.URL()},
	)
	if err != nil {
		t.Fatal(err)
	}
	wait := registerErrorWait()
	config := docker.Config{Memory: 67108864, Image: "myserver/user/myimg"}
	addr, _, err := cluster.CreateContainer(docker.CreateContainerOptions{Config: &config}, time.Minute)
	if err == nil || strings.Index(err.Error(), "createContErr") == -1 {
		t.Fatalf("Expected create container error, got: %s", err)
	}
	if addr != server1.URL() {
		t.Errorf("CreateContainer: wrong node addr. Want %q. Got %q.", server1.URL(), addr)
	}
	wait()
	nodes, err := cluster.UnfilteredNodes()
	if err != nil {
		t.Fatal(err)
	}
	if nodes[0].FailureCount() != 1 {
		t.Fatalf("Expected failure count to be 1, got: %d", nodes[0].FailureCount())
	}
}

func TestCreateContainerErrorNetError(t *testing.T) {
	server1, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &MapStorage{}, "",
		Node{Address: server1.URL()},
	)
	if err != nil {
		t.Fatal(err)
	}
	server1.Stop()
	config := docker.Config{Memory: 67108864, Image: "myserver/user/myimg"}
	wait := registerErrorWait()
	addr, _, err := cluster.CreateContainer(docker.CreateContainerOptions{Config: &config}, time.Minute)
	if err == nil || strings.Index(err.Error(), "cannot connect to Docker endpoint") == -1 {
		t.Fatalf("Expected create container error, got: %s", err)
	}
	if addr != server1.URL() {
		t.Errorf("CreateContainer: wrong node addr. Want %q. Got %q.", server1.URL(), addr)
	}
	wait()
	nodes, err := cluster.UnfilteredNodes()
	if err != nil {
		t.Fatal(err)
	}
	if nodes[0].FailureCount() != 1 {
		t.Fatalf("Expected failure count to be 1, got: %d", nodes[0].FailureCount())
	}
}

func TestCreateContainerErrorDialError(t *testing.T) {
	serverAddr := "http://192.0.2.10:1234"
	cluster, err := New(nil, &MapStorage{}, "",
		Node{Address: serverAddr},
	)
	if err != nil {
		t.Fatal(err)
	}
	config := docker.Config{Memory: 67108864, Image: "myserver/user/myimg"}
	wait := registerErrorWait()
	addr, _, err := cluster.CreateContainer(docker.CreateContainerOptions{Config: &config}, time.Minute)
	if err == nil || strings.Index(err.Error(), "i/o timeout") == -1 {
		t.Fatalf("Expected create container error, got: %s", err)
	}
	if addr != serverAddr {
		t.Errorf("CreateContainer: wrong node addr. Want %q. Got %q.", serverAddr, addr)
	}
	wait()
	nodes, err := cluster.UnfilteredNodes()
	if err != nil {
		t.Fatal(err)
	}
	if nodes[0].FailureCount() != 1 {
		t.Fatalf("Expected failure count to be 1, got: %d", nodes[0].FailureCount())
	}
}

func TestCreateContainerWithoutRepo(t *testing.T) {
	server1, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server1.Stop()
	cluster, err := New(nil, &MapStorage{}, "",
		Node{Address: server1.URL()},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.PullImage(docker.PullImageOptions{
		Repository: "user/myimg",
	}, docker.AuthConfiguration{}, server1.URL())
	if err != nil {
		t.Fatal(err)
	}
	server1.PrepareFailure("createErr", "/images/create")
	config := docker.Config{Memory: 67108864, Image: "user/myimg"}
	nodeAddr, container, err := cluster.CreateContainer(docker.CreateContainerOptions{Config: &config}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if nodeAddr != server1.URL() {
		t.Errorf("CreateContainer: wrong node  ID. Want %q. Got %q.", server1.URL(), nodeAddr)
	}
	if container.ID == "" {
		t.Errorf("CreateContainer: wrong container ID. Expected not empty.")
	}
}

func TestCreateContainerSchedulerOpts(t *testing.T) {
	body := `{"Id":"e90302"}`
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server2.Close()
	scheduler := optsScheduler{roundRobin{lastUsed: -1}}
	cluster, err := New(&scheduler, &MapStorage{}, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	config := docker.Config{Memory: 67108864, Image: "myimg"}
	opts := docker.CreateContainerOptions{Name: "name", Config: &config}
	schedulerOpts := "myOpt"
	nodeAddr, container, err := cluster.CreateContainerSchedulerOpts(opts, schedulerOpts, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if nodeAddr != server1.URL {
		t.Errorf("CreateContainer: wrong node  ID. Want %q. Got %q.", server1.URL, nodeAddr)
	}
	if container.ID != "e90302" {
		t.Errorf("CreateContainer: wrong container ID. Want %q. Got %q.", "e90302", container.ID)
	}
	schedulerOpts = "myOptX"
	nodeAddr, container, err = cluster.CreateContainerSchedulerOpts(opts, schedulerOpts, time.Minute)
	if err == nil || err.Error() != "Invalid option myOptX" {
		t.Fatal("Expected error but none returned.")
	}
}

func TestCreateContainerFailure(t *testing.T) {
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "NoSuchImage", http.StatusNotFound)
	}))
	defer server1.Close()
	cluster, err := New(nil, &MapStorage{}, "", Node{Address: server1.URL})
	if err != nil {
		t.Fatal(err)
	}
	config := docker.Config{Memory: 67108864}
	addr, _, err := cluster.CreateContainer(docker.CreateContainerOptions{Config: &config}, time.Minute)
	expected := "no such image"
	if err == nil || strings.Index(err.Error(), expected) == -1 {
		t.Errorf("Expected error %q, got: %#v", expected, err)
	}
	if addr != server1.URL {
		t.Errorf("CreateContainer: wrong node addr. Want %q. Got %q.", server1.URL, addr)
	}
}

func TestCreateContainerSpecifyNode(t *testing.T) {
	var requests []string
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `{"Id":"e90302"}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.RequestURI)
		body := `{"Id":"e90303"}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server2.Close()
	var storage MapStorage
	cluster, err := New(nil, &storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	opts := docker.CreateContainerOptions{Config: &docker.Config{
		Memory: 67108864,
		Image:  "some.host/user/myImage",
	}}
	nodeAddr, container, err := cluster.CreateContainer(opts, time.Minute, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	if nodeAddr != server2.URL {
		t.Errorf("CreateContainer: wrong node ID. Want %q. Got %q.", server2.URL, nodeAddr)
	}
	if container.ID != "e90303" {
		t.Errorf("CreateContainer: wrong container ID. Want %q. Got %q.", "e90303", container.ID)
	}
	host, _ := storage.RetrieveContainer("e90303")
	if host != server2.URL {
		t.Errorf("Cluster.CreateContainer() with storage: wrong data. Want %#v. Got %#v.", server2.URL, host)
	}
	if len(requests) != 3 {
		t.Fatalf("Expected 3 api calls, got %d.", len(requests))
	}
	expectedReq := "/images/create?fromImage=some.host%2Fuser%2FmyImage"
	if requests[0] != expectedReq {
		t.Errorf("Incorrect request 0. Want %#v. Got %#v", expectedReq, requests[0])
	}
	expectedReq = "/images/some.host/user/myImage/json"
	if requests[1] != expectedReq {
		t.Errorf("Incorrect request 1. Want %#v. Got %#v", expectedReq, requests[1])
	}
	expectedReq = `^/containers/create\??$`
	if !regexp.MustCompile(expectedReq).MatchString(requests[2]) {
		t.Errorf("Incorrect request 2. Want %#v. Got %#v", expectedReq, requests[2])
	}
}

func TestCreateContainerSpecifyUnknownNode(t *testing.T) {
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `{"Id":"e90302"}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server1.Close()
	cluster, err := New(nil, &MapStorage{}, "",
		Node{Address: server1.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	opts := docker.CreateContainerOptions{Config: &docker.Config{Memory: 67108864}}
	nodeAddr, container, err := cluster.CreateContainer(opts, time.Minute, "invalid.addr")
	if nodeAddr != "invalid.addr" {
		t.Errorf("Got wrong node ID. Want %q. Got %q.", "invalid.addr", nodeAddr)
	}
	if container != nil {
		t.Errorf("Got unexpected value for container. Want <nil>. Got %#v", container)
	}
}

func TestCreateContainerRandonNodeFromSlice(t *testing.T) {
	reqsServer1 := 0
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqsServer1++
		body := `{"Id":"e90302"}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server1.Close()
	reqsServer2 := 0
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqsServer2++
		body := `{"Id":"e90303"}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server2.Close()
	var storage MapStorage
	cluster, err := New(nil, &storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	opts := docker.CreateContainerOptions{Config: &docker.Config{Memory: 67108864}}
	for i := 0; i < 100; i++ {
		_, _, err := cluster.CreateContainer(opts, time.Minute, server1.URL, server2.URL)
		if err != nil {
			t.Fatal(err)
		}
	}
	if reqsServer1 == 0 {
		t.Fatalf("Expected some reqs to server 1, got 0")
	}
	if reqsServer2 == 0 {
		t.Fatalf("Expected some reqs to server 2, got 0")
	}
	if reqsServer1+reqsServer2 != 100 {
		t.Fatalf("Expected 100 reqs to servers, got: %d", reqsServer1+reqsServer2)
	}
}

func TestCreateContainerWithStorage(t *testing.T) {
	body := `{"Id":"e90302"}`
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server2.Close()
	var storage MapStorage
	cluster, err := New(nil, &storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	config := docker.Config{Memory: 67108864, Image: "myimg"}
	_, _, err = cluster.CreateContainer(docker.CreateContainerOptions{Config: &config}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	host, _ := storage.RetrieveContainer("e90302")
	if host != server1.URL {
		t.Errorf("Cluster.CreateContainer() with storage: wrong data. Want %#v. Got %#v.", server1.URL, host)
	}
}

type firstNodeScheduler struct{}

func (firstNodeScheduler) Schedule(c *Cluster, opts docker.CreateContainerOptions, schedulerOpts SchedulerOptions) (Node, error) {
	var node Node
	nodes, err := c.Nodes()
	if err != nil {
		return node, err
	}
	if len(nodes) == 0 {
		return node, fmt.Errorf("no nodes in scheduler")
	}
	return nodes[0], nil
}

func TestCreateContainerTryAnotherNodeInFailure(t *testing.T) {
	body := `{"Id":"e90302"}`
	called1 := false
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called1 = true
		http.Error(w, "NoSuchImage", http.StatusNotFound)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server2.Close()
	cluster, err := New(firstNodeScheduler{}, &MapStorage{}, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	config := docker.Config{Memory: 67108864, Image: "myimg"}
	nodeAddr, container, err := cluster.CreateContainer(docker.CreateContainerOptions{Config: &config}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if called1 != true {
		t.Error("CreateContainer: server1 should've been called.")
	}
	if nodeAddr != server2.URL {
		t.Errorf("CreateContainer: wrong node  ID. Want %q. Got %q.", server2.URL, nodeAddr)
	}
	if container.ID != "e90302" {
		t.Errorf("CreateContainer: wrong container ID. Want %q. Got %q.", "e90302", container.ID)
	}
}

type myHook func(evt HookEvent, node *Node) error

func (fn myHook) RunClusterHook(evt HookEvent, node *Node) error {
	return fn(evt, node)
}

func TestCreateContainerTryAnotherNodeAfterFailureInHook(t *testing.T) {
	called1 := false
	called2 := false
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called1 = true
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"Id":"e90301"}`))
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called2 = true
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"Id":"e90302"}`))
	}))
	defer server2.Close()
	cluster, err := New(firstNodeScheduler{}, &MapStorage{}, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	hookCalled := false
	cluster.AddHook(HookEventBeforeContainerCreate, myHook(func(evt HookEvent, node *Node) error {
		hookCalled = true
		if node.Address == server1.URL {
			return fmt.Errorf("my hook err")
		}
		return nil
	}))
	config := docker.Config{Memory: 67108864, Image: "myimg"}
	wait := registerErrorWait()
	nodeAddr, container, err := cluster.CreateContainer(docker.CreateContainerOptions{Config: &config}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	wait()
	if !hookCalled {
		t.Error("CreateContainer: hook should've have been called.")
	}
	if called1 {
		t.Error("CreateContainer: server1 should NOT have been called.")
	}
	if !called2 {
		t.Error("CreateContainer: server2 should've been called.")
	}
	if nodeAddr != server2.URL {
		t.Errorf("CreateContainer: wrong node  ID. Want %q. Got %q.", server2.URL, nodeAddr)
	}
	if container.ID != "e90302" {
		t.Errorf("CreateContainer: wrong container ID. Want %q. Got %q.", "e90302", container.ID)
	}
	nodes, err := cluster.UnfilteredNodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Fatalf("Expected node len to be 2, got %d", len(nodes))
	}
	if nodes[0].FailureCount() != 0 || nodes[1].FailureCount() != 0 {
		t.Fatalf("Expected failure count to be 0, got: %d - %d", nodes[0].FailureCount(), nodes[1].FailureCount())
	}
}

func TestCreateContainerNetworkFailureInHook(t *testing.T) {
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"Id":"e90301"}`))
	}))
	defer server1.Close()
	cluster, err := New(firstNodeScheduler{}, &MapStorage{}, "",
		Node{Address: server1.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	hookCalled := false
	cluster.AddHook(HookEventBeforeContainerCreate, myHook(func(evt HookEvent, node *Node) error {
		hookCalled = true
		if node.Address == server1.URL {
			return &net.OpError{
				Op:   "read",
				Net:  "tcp",
				Addr: &net.TCPAddr{IP: net.IP{}, Port: 0, Zone: ""},
				Err:  fmt.Errorf("my hook net err"),
			}
		}
		return nil
	}))
	config := docker.Config{Memory: 67108864, Image: "myimg"}
	wait := registerErrorWait()
	_, _, err = cluster.CreateContainer(docker.CreateContainerOptions{Config: &config}, time.Minute)
	if err == nil || strings.Index(err.Error(), "my hook net err") == -1 {
		t.Fatalf("Expected hook error, got: %s", err)
	}
	wait()
	if !hookCalled {
		t.Error("CreateContainer: hook should've have been called.")
	}
	nodes, err := cluster.UnfilteredNodes()
	if err != nil {
		t.Fatal(err)
	}
	if nodes[0].FailureCount() != 1 {
		t.Fatalf("Expected failure count to be 1, got: %d", nodes[0].FailureCount())
	}
}

func TestInspectContainerWithStorage(t *testing.T) {
	body := `{"Id":"e90302","Path":"date","Args":[]}`
	var count int
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server2.Close()
	id := "e90302"
	storage := MapStorage{}
	err := storage.StoreContainer(id, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	container, err := cluster.InspectContainer(id)
	if err != nil {
		t.Fatal(err)
	}
	if container.ID != id {
		t.Errorf("InspectContainer(%q): Wrong ID. Want %q. Got %q.", id, id, container.ID)
	}
	if container.Path != "date" {
		t.Errorf("InspectContainer(%q): Wrong Path. Want %q. Got %q.", id, "date", container.Path)
	}
	if count > 0 {
		t.Errorf("InspectContainer(%q) with storage: should not send request to all servers, but did.", "e90302")
	}
}

func TestInspectContainerNoSuchContainer(t *testing.T) {
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "No such container", http.StatusNotFound)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "No such container", http.StatusNotFound)
	}))
	defer server2.Close()
	cluster, err := New(nil, &MapStorage{}, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	id := "e90302"
	container, err := cluster.InspectContainer(id)
	if container != nil {
		t.Errorf("InspectContainer(%q): Expected <nil> container, got %#v.", id, container)
	}
	expected := cstorage.ErrNoSuchContainer
	if !reflect.DeepEqual(err, expected) {
		t.Errorf("InspectContainer(%q): Wrong error. Want %#v. Got %#v.", id, expected, err)
	}
}

func TestInspectContainerNoSuchContainerWithStorage(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "", Node{Address: "http://localhost:4243"})
	if err != nil {
		t.Fatal(err)
	}
	id := "e90302"
	container, err := cluster.InspectContainer(id)
	if container != nil {
		t.Errorf("InspectContainer(%q): Expected <nil> container, got %#v.", id, container)
	}
	expected := cstorage.ErrNoSuchContainer
	if !reflect.DeepEqual(err, expected) {
		t.Errorf("InspectContainer(%q): Wrong error. Want %#v. Got %#v.", id, expected, err)
	}
}

func TestInspectContainerFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "No such container", http.StatusInternalServerError)
	}))
	defer server.Close()
	cluster, err := New(nil, &MapStorage{}, "", Node{Address: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	id := "a2033"
	container, err := cluster.InspectContainer(id)
	if container != nil {
		t.Errorf("InspectContainer(%q): Expected <nil> container, got %#v.", id, container)
	}
	if err == nil {
		t.Errorf("InspectContainer(%q): Expected non-nil error, got <nil>", id)
	}
}

func TestKillContainer(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "No such container", http.StatusNotFound)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte("ok"))
	}))
	defer server2.Close()
	id := "abc123"
	storage := &MapStorage{}
	err := storage.StoreContainer(id, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.KillContainer(docker.KillContainerOptions{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Errorf("KillContainer(%q): Did not call node http server", id)
	}
}

func TestKillContainerWithStorage(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "No such container", http.StatusNotFound)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer server2.Close()
	id := "abc123"
	storage := MapStorage{}
	err := storage.StoreContainer(id, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.KillContainer(docker.KillContainerOptions{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Errorf("KillContainer(%q): should not call the node server", id)
	}
}

func TestKillContainerNotFoundWithStorage(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "", Node{Address: "http://localhost:8282"})
	if err != nil {
		t.Fatal(err)
	}
	id := "abc123"
	err = cluster.KillContainer(docker.KillContainerOptions{ID: id})
	expected := cstorage.ErrNoSuchContainer
	if !reflect.DeepEqual(err, expected) {
		t.Errorf("KillContainer(%q): Wrong error. Want %#v. Got %#v.", id, expected, err)
	}
}

func TestListContainers(t *testing.T) {
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `[
     {
             "Id": "8dfafdbc3a40",
             "Image": "base:latest",
             "Command": "echo 1",
             "Created": 1367854155,
             "Status": "Exit 0"
     }
]`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `[
     {
             "Id": "3176a2479c92",
             "Image": "base:latest",
             "Command": "echo 3333333333333333",
             "Created": 1367854154,
             "Status": "Exit 0"
     },
     {
             "Id": "9cd87474be90",
             "Image": "base:latest",
             "Command": "echo 222222",
             "Created": 1367854155,
             "Status": "Exit 0"
     }
]`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server2.Close()
	server3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `[]`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server3.Close()
	cluster, err := New(nil, &MapStorage{}, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
		Node{Address: server3.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	expected := containerList([]docker.APIContainers{
		{ID: "3176a2479c92", Image: "base:latest", Command: "echo 3333333333333333", Created: 1367854154, Status: "Exit 0"},
		{ID: "9cd87474be90", Image: "base:latest", Command: "echo 222222", Created: 1367854155, Status: "Exit 0"},
		{ID: "8dfafdbc3a40", Image: "base:latest", Command: "echo 1", Created: 1367854155, Status: "Exit 0"},
	})
	sort.Sort(expected)
	containers, err := cluster.ListContainers(docker.ListContainersOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := containerList(containers)
	sort.Sort(got)
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("ListContainers: Wrong containers. Want %#v. Got %#v.", expected, got)
	}
}

func TestListContainersFailure(t *testing.T) {
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `[
     {
             "Id": "8dfafdbc3a40",
             "Image": "base:latest",
             "Command": "echo 1",
             "Created": 1367854155,
             "Status": "Exit 0"
     }
]`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal failure", http.StatusInternalServerError)
	}))
	defer server2.Close()
	cluster, err := New(nil, &MapStorage{}, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	expected := []docker.APIContainers{
		{ID: "8dfafdbc3a40", Image: "base:latest", Command: "echo 1", Created: 1367854155, Status: "Exit 0"},
	}
	containers, err := cluster.ListContainers(docker.ListContainersOptions{})
	if err == nil {
		t.Error("ListContainers: Expected non-nil error, got <nil>")
	}
	if !reflect.DeepEqual(containers, expected) {
		t.Errorf("ListContainers: Want %#v. Got %#v.", expected, containers)
	}
}

func TestListContainersSchedulerFailure(t *testing.T) {
	cluster, err := New(nil, &failingStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	containers, err := cluster.ListContainers(docker.ListContainersOptions{})
	expected := "storage error"
	if err.Error() != expected {
		t.Errorf("ListContainers(): wrong error. Want %q. Got %q", expected, err.Error())
	}
	if containers != nil {
		t.Errorf("ListContainers(): wrong result. Want <nil>. Got %#v.", containers)
	}
}

func TestRemoveContainer(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "No such container", http.StatusNotFound)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte("ok"))
	}))
	defer server2.Close()
	id := "abc123"
	storage := &MapStorage{}
	err := storage.StoreContainer(id, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.RemoveContainer(docker.RemoveContainerOptions{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Errorf("RemoveContainer(%q): Did not call node HTTP server", id)
	}
}

func TestRemoveContainerWithStorage(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "No such container", http.StatusNotFound)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer server2.Close()
	id := "abc123"
	storage := MapStorage{}
	err := storage.StoreContainer(id, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.RemoveContainer(docker.RemoveContainerOptions{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Errorf("RemoveContainer(%q): should not call the node server", id)
	}
	_, err = storage.RetrieveContainer(id)
	if err == nil {
		t.Errorf("RemoveContainer(%q): should remove the container from the storage", id)
	}
}

func TestRemoveContainerNotFoundWithStorage(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "", Node{Address: "http://localhost:8282"})
	if err != nil {
		t.Fatal(err)
	}
	id := "abc123"
	err = cluster.RemoveContainer(docker.RemoveContainerOptions{ID: id})
	expected := cstorage.ErrNoSuchContainer
	if !reflect.DeepEqual(err, expected) {
		t.Errorf("RemoveContainer(%q): Wrong error. Want %#v. Got %#v.", id, expected, err)
	}
}

func TestRemoveContainerNotFoundInServer(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "No such container", http.StatusNotFound)
	}))
	defer server1.Close()
	id := "abc123"
	storage := MapStorage{}
	err := storage.StoreContainer(id, server1.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &storage, "",
		Node{Address: server1.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.RemoveContainer(docker.RemoveContainerOptions{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Errorf("RemoveContainer(%q): Did not call node HTTP server", id)
	}
	_, err = storage.RetrieveContainer(id)
	if err == nil {
		t.Errorf("RemoveContainer(%q): should remove the container from the storage", id)
	}
}

func TestRemoveContainerServerError(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "Random server error", http.StatusInternalServerError)
	}))
	defer server1.Close()
	id := "abc123"
	storage := MapStorage{}
	err := storage.StoreContainer(id, server1.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &storage, "",
		Node{Address: server1.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.RemoveContainer(docker.RemoveContainerOptions{ID: id})
	if err == nil {
		t.Errorf("RemoveContainer(%q): should not remove the container from the storage", id)
	}
	if !called {
		t.Errorf("RemoveContainer(%q): Did not call node HTTP server", id)
	}
	addr, err := storage.RetrieveContainer(id)
	if err != nil || addr != server1.URL {
		t.Errorf("RemoveContainer(%q): should not remove the container from the storage", id)
	}
}

func TestStartContainer(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "No such container", http.StatusNotFound)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte("ok"))
	}))
	defer server2.Close()
	id := "abc123"
	storage := &MapStorage{}
	err := storage.StoreContainer(id, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.StartContainer(id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Errorf("StartContainer(%q): Did not call node HTTP server", id)
	}
}

func TestStartContainerWithStorage(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "No such container", http.StatusNotFound)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer server2.Close()
	id := "abc123"
	storage := MapStorage{}
	err := storage.StoreContainer(id, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.StartContainer(id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Errorf("StartContainer(%q): should not call the node server", id)
	}
}

func TestStartContainerNotFoundWithStorage(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "", Node{Address: "http://localhost:8282"})
	if err != nil {
		t.Fatal(err)
	}
	id := "abc123"
	err = cluster.StartContainer(id, nil)
	expected := cstorage.ErrNoSuchContainer
	if !reflect.DeepEqual(err, expected) {
		t.Errorf("StartContainer(%q): Wrong error. Want %#v. Got %#v.", id, expected, err)
	}
}

func TestStopContainer(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "No such container", http.StatusNotFound)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte("ok"))
	}))
	defer server2.Close()
	id := "abc123"
	storage := &MapStorage{}
	err := storage.StoreContainer(id, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.StopContainer(id, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Errorf("StopContainer(%q, 10): Did not call node HTTP server", id)
	}
}

func TestStopContainerWithStorage(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "No such container", http.StatusNotFound)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer server2.Close()
	id := "abc123"
	storage := MapStorage{}
	err := storage.StoreContainer(id, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.StopContainer(id, 10)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Errorf("StopContainer(%q): should not call the node server", id)
	}
}

func TestStopContainerNotFoundWithStorage(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "", Node{Address: "http://localhost:8282"})
	if err != nil {
		t.Fatal(err)
	}
	id := "abc123"
	err = cluster.StopContainer(id, 10)
	expected := cstorage.ErrNoSuchContainer
	if !reflect.DeepEqual(err, expected) {
		t.Errorf("StopContainer(%q): Wrong error. Want %#v. Got %#v.", id, expected, err)
	}
}

func TestRestartContainer(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "No such container", http.StatusNotFound)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte("ok"))
	}))
	defer server2.Close()
	id := "abc123"
	storage := &MapStorage{}
	err := storage.StoreContainer(id, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.RestartContainer(id, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Errorf("RestartContainer(%q, 10): Did not call node HTTP server", id)
	}
}

func TestRestartContainerWithStorage(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "No such container", http.StatusNotFound)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer server2.Close()
	id := "abc123"
	storage := MapStorage{}
	err := storage.StoreContainer(id, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.RestartContainer(id, 10)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Errorf("RestartContainer(%q): should not call the node server", id)
	}
}

func TestRestartContainerNotFoundWithStorage(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "", Node{Address: "http://localhost:8282"})
	if err != nil {
		t.Fatal(err)
	}
	id := "abc123"
	err = cluster.RestartContainer(id, 10)
	expected := cstorage.ErrNoSuchContainer
	if !reflect.DeepEqual(err, expected) {
		t.Errorf("RestartContainer(%q): Wrong error. Want %#v. Got %#v.", id, expected, err)
	}
}

func TestPauseContainer(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/containers/abc123/pause" {
			http.Error(w, "No such container", http.StatusNotFound)
		}
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/containers/abc123/pause" {
			called = true
			w.Write([]byte("ok"))
		}
	}))
	defer server2.Close()
	id := "abc123"
	storage := &MapStorage{}
	err := storage.StoreContainer(id, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.PauseContainer(id)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Errorf("PauseContainer(%q): Did not call node HTTP server", id)
	}
}

func TestPauseContainerWithStorage(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/containers/abc123/pause" {
			called = true
			http.Error(w, "No such container", http.StatusNotFound)
		}
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/containers/abc123/pause" {
			w.Write([]byte("ok"))
		}
	}))
	defer server2.Close()
	id := "abc123"
	storage := MapStorage{}
	err := storage.StoreContainer(id, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.PauseContainer(id)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Errorf("PauseContainer(%q): should not call the node server", id)
	}
}

func TestPauseContainerNotFoundWithStorage(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "", Node{Address: "http://localhost:8282"})
	if err != nil {
		t.Fatal(err)
	}
	id := "abc123"
	err = cluster.PauseContainer(id)
	expected := cstorage.ErrNoSuchContainer
	if !reflect.DeepEqual(err, expected) {
		t.Errorf("PauseContainer(%q): Wrong error. Want %#v. Got %#v.", id, expected, err)
	}
}

func TestUnpauseContainer(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/containers/abc123/unpause" {
			http.Error(w, "No such container", http.StatusNotFound)
		}
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/containers/abc123/unpause" {
			called = true
			w.Write([]byte("ok"))
		}
	}))
	defer server2.Close()
	id := "abc123"
	storage := &MapStorage{}
	err := storage.StoreContainer(id, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.UnpauseContainer(id)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Errorf("UnpauseContainer(%q): Did not call node HTTP server", id)
	}
}

func TestUnpauseContainerWithStorage(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/containers/abc123/unpause" {
			called = true
			http.Error(w, "No such container", http.StatusNotFound)
		}
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/containers/abc123/unpause" {
			w.Write([]byte("ok"))
		}
	}))
	defer server2.Close()
	id := "abc123"
	storage := MapStorage{}
	err := storage.StoreContainer(id, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.UnpauseContainer(id)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Errorf("UnpauseContainer(%q): should not call the node server", id)
	}
}

func TestUnpauseContainerNotFoundWithStorage(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "", Node{Address: "http://localhost:8282"})
	if err != nil {
		t.Fatal(err)
	}
	id := "abc123"
	err = cluster.UnpauseContainer(id)
	expected := cstorage.ErrNoSuchContainer
	if !reflect.DeepEqual(err, expected) {
		t.Errorf("UnpauseContainer(%q): Wrong error. Want %#v. Got %#v.", id, expected, err)
	}
}

func TestWaitContainer(t *testing.T) {
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "No such container", http.StatusNotFound)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `{"StatusCode":34}`
		w.Write([]byte(body))
	}))
	defer server2.Close()
	id := "abc123"
	storage := &MapStorage{}
	err := storage.StoreContainer(id, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	expected := 34
	status, err := cluster.WaitContainer(id)
	if err != nil {
		t.Fatal(err)
	}
	if status != expected {
		t.Errorf("WaitContainer(%q): Wrong status. Want %d. Got %d.", id, expected, status)
	}
}

func TestWaitContainerNotFound(t *testing.T) {
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "No such container", http.StatusNotFound)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "No such container", http.StatusNotFound)
	}))
	defer server2.Close()
	cluster, err := New(nil, &MapStorage{}, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	id := "abc123"
	expected := -1
	status, err := cluster.WaitContainer(id)
	if err == nil {
		t.Errorf("WaitContainer(%q): unexpected <nil> error", id)
	}
	if status != expected {
		t.Errorf("WaitContainer(%q): Wrong status. Want %d. Got %d.", id, expected, status)
	}
}

func TestWaitContainerWithStorage(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "No such container", http.StatusNotFound)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `{"StatusCode":34}`
		w.Write([]byte(body))
	}))
	defer server2.Close()
	id := "abc123"
	storage := MapStorage{}
	err := storage.StoreContainer(id, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	expected := 34
	status, err := cluster.WaitContainer(id)
	if err != nil {
		t.Fatal(err)
	}
	if status != expected {
		t.Errorf("WaitContainer(%q): Wrong status. Want %d. Got %d.", id, expected, status)
	}
	if called {
		t.Errorf("WaitContainer(%q): should not call the all node servers.", id)
	}
}

func TestWaitContainerNotFoundWithStorage(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "", Node{Address: "http://localhost:4243"})
	if err != nil {
		t.Fatal(err)
	}
	id := "abc123"
	expectedStatus := -1
	status, err := cluster.WaitContainer(id)
	if status != expectedStatus {
		t.Errorf("WaitContainer(%q): wrong status. Want %d. Got %d.", id, expectedStatus, status)
	}
	expected := cstorage.ErrNoSuchContainer
	if !reflect.DeepEqual(err, expected) {
		t.Errorf("WaitContainer(%q): wrong error. Want %#v. Got %#v.", id, expected, err)
	}
}

func TestAttachToContainer(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "container not found", http.StatusNotFound)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte{1, 0, 0, 0, 0, 0, 0, 18})
		w.Write([]byte("something happened"))
	}))
	defer server2.Close()
	storage := &MapStorage{}
	err := storage.StoreContainer("abcdef", server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	opts := docker.AttachToContainerOptions{
		Container:    "abcdef",
		OutputStream: &safe.Buffer{},
		Logs:         true,
		Stdout:       true,
	}
	err = cluster.AttachToContainer(opts)
	if err != nil {
		t.Errorf("AttachToContainer: unexpected error. Want <nil>. Got %#v.", err)
	}
	if !called {
		t.Error("AttachToContainer: Did not call the remote HTTP API")
	}
}

func TestAttachToContainerWithStorage(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "No such container", http.StatusNotFound)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer server2.Close()
	id := "abcdef"
	storage := MapStorage{}
	err := storage.StoreContainer(id, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	opts := docker.AttachToContainerOptions{
		Container:    id,
		OutputStream: &safe.Buffer{},
		Logs:         true,
		Stdout:       true,
	}
	err = cluster.AttachToContainer(opts)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("AttachToContainer(): should not call the node server")
	}
}

func TestAttachToContainerNotFoundWithStorage(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "", Node{Address: "http://localhost:8282"})
	if err != nil {
		t.Fatal(err)
	}
	id := "abcdef"
	opts := docker.AttachToContainerOptions{
		Container:    "abcdef",
		OutputStream: &safe.Buffer{},
		Logs:         true,
		Stdout:       true,
	}
	err = cluster.AttachToContainer(opts)
	expected := cstorage.ErrNoSuchContainer
	if !reflect.DeepEqual(err, expected) {
		t.Errorf("AttachToContainer(%q): Wrong error. Want %#v. Got %#v.", id, expected, err)
	}
}

func TestAttachToContainerNonBlocking(t *testing.T) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte{1, 0, 0, 0, 0, 0, 0, 18})
		w.Write([]byte("something happened"))
	}))
	defer server.Close()
	storage := MapStorage{}
	err := storage.StoreContainer("abc", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &storage, "",
		Node{Address: server.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	opts := docker.AttachToContainerOptions{
		Container:    "abc",
		OutputStream: &safe.Buffer{},
		Stdout:       true,
	}
	waiter, err := cluster.AttachToContainerNonBlocking(opts)
	if err != nil {
		t.Errorf("AttachToContainerNonBlocking: unexpected error. Want <nil>. Got %#v.", err)
	}
	waiter.Wait()
	if !called {
		t.Error("AttachToContainerNonBlocking: Did not call the remote HTTP API")
	}
}

func TestLogs(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "container not found", http.StatusNotFound)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte{1, 0, 0, 0, 0, 0, 0, 18})
		w.Write([]byte("something happened"))
	}))
	defer server2.Close()
	storage := &MapStorage{}
	err := storage.StoreContainer("abcdef", server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	opts := docker.LogsOptions{
		Container:    "abcdef",
		OutputStream: &safe.Buffer{},
		Stdout:       true,
		Stderr:       true,
	}
	err = cluster.Logs(opts)
	if err != nil {
		t.Errorf("Logs: unexpected error. Want <nil>. Got %#v.", err)
	}
	if !called {
		t.Error("Logs: Did not call the remote HTTP API")
	}
}

func TestLogsWithStorage(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "No such container", http.StatusNotFound)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer server2.Close()
	id := "abcdef"
	storage := MapStorage{}
	err := storage.StoreContainer(id, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	opts := docker.LogsOptions{
		Container:    id,
		OutputStream: &safe.Buffer{},
		Stdout:       true,
		Stderr:       true,
	}
	err = cluster.Logs(opts)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("Logs(): should not call the node server")
	}
}

func TestLogsContainerNotFoundWithStorage(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "", Node{Address: "http://localhost:8282"})
	if err != nil {
		t.Fatal(err)
	}
	id := "abcdef"
	opts := docker.LogsOptions{
		Container:    "abcdef",
		OutputStream: &safe.Buffer{},
		Stdout:       true,
		Stderr:       true,
	}
	err = cluster.Logs(opts)
	expected := cstorage.ErrNoSuchContainer
	if !reflect.DeepEqual(err, expected) {
		t.Errorf("Logs(%q): Wrong error. Want %#v. Got %#v.", id, expected, err)
	}
}

func TestCommitContainer(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "container not found", http.StatusNotFound)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte(`{"Id":"596069db4bf5"}`))
	}))
	defer server2.Close()
	storage := &MapStorage{}
	err := storage.StoreContainer("abcdef", server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	opts := docker.CommitContainerOptions{
		Container: "abcdef",
	}
	image, err := cluster.CommitContainer(opts)
	if err != nil {
		t.Fatal(err)
	}
	if image.ID != "596069db4bf5" {
		t.Errorf("CommitContainer: the image container is %s, expected: '596069db4bf5'", image.ID)
	}
	if !called {
		t.Error("CommitContainer: Did not call the remote HTTP API")
	}
}

func TestCommitContainerError(t *testing.T) {
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "container not found", http.StatusNotFound)
	}))
	defer server1.Close()
	cluster, err := New(nil, &MapStorage{}, "",
		Node{Address: server1.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	opts := docker.CommitContainerOptions{
		Container: "abcdef",
	}
	image, err := cluster.CommitContainer(opts)
	if err == nil {
		t.Fatal(err)
	}
	if image != nil {
		t.Errorf("CommitContainerError: the image should be nil but it is %s", image.ID)
	}
}

func TestCommitContainerWithStorage(t *testing.T) {
	var called bool
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "container not found", http.StatusNotFound)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"Id":"596069db4bf5"}`))
	}))
	defer server2.Close()
	id := "abc123"
	storage := MapStorage{}
	err := storage.StoreContainer(id, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	opts := docker.CommitContainerOptions{Container: id, Repository: "tsuru/python"}
	image, err := cluster.CommitContainer(opts)
	if err != nil {
		t.Fatal(err)
	}
	if image.ID != "596069db4bf5" {
		t.Errorf("CommitContainer: the image container is %s, expected: '596069db4bf5'", image.ID)
	}
	if called {
		t.Errorf("CommitContainer(%q): should not call the all node servers.", id)
	}
	img, _ := storage.RetrieveImage("tsuru/python")
	if img.LastNode != server2.URL {
		t.Errorf("CommitContainer(%q): wrong image last node in the storage. Want %q. Got %q", id, server2.URL, img.LastNode)
	}
}

func TestCommitContainerWithStorageAndImageID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"Id":"596069db4bf5"}`))
	}))
	defer server.Close()
	id := "abc123"
	stor := MapStorage{}
	err := stor.StoreContainer(id, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &stor, "", Node{Address: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	opts := docker.CommitContainerOptions{Container: id}
	image, err := cluster.CommitContainer(opts)
	if err != nil {
		t.Fatal(err)
	}
	_, err = stor.RetrieveImage(image.ID)
	if err != cstorage.ErrNoSuchImage {
		t.Errorf("CommitContainer(%q): Expected no such image error, got: %s", id, err)
	}
}

func TestCommitContainerNotFoundWithStorage(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "", Node{Address: "http://localhost:4243"})
	if err != nil {
		t.Fatal(err)
	}
	id := "abc123"
	opts := docker.CommitContainerOptions{Container: id}
	_, err = cluster.CommitContainer(opts)
	expected := cstorage.ErrNoSuchContainer
	if !reflect.DeepEqual(err, expected) {
		t.Errorf("CommitContainer(%q): wrong error. Want %#v. Got %#v.", id, expected, err)
	}
}

func TestCommitContainerTagShouldIgnoreRemoveImageErrors(t *testing.T) {
	imgTag := "mytag/mytag"
	expectedImageId := "596069db4bf5"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fmt.Sprintf(`{"Id":"%s"}`, expectedImageId)))
	}))
	defer server.Close()
	containerId := "abc123"
	storage := MapStorage{}
	err := storage.StoreContainer(containerId, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = storage.StoreImage(imgTag, "id1", "http://invalid.invalid")
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &storage, "", Node{Address: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	opts := docker.CommitContainerOptions{Container: containerId, Repository: imgTag}
	image, err := cluster.CommitContainer(opts)
	if err != nil {
		t.Fatal(err)
	}
	if image.ID != expectedImageId {
		t.Fatalf("Expected image id to be %q, got: %q", expectedImageId, image.ID)
	}
	img, _ := storage.RetrieveImage(imgTag)
	if img.LastNode != server.URL {
		t.Errorf("CommitContainer(%q): wrong image last node in the storage. Want %q. Got %q", containerId, server.URL, img.LastNode)
	}
}

func TestCommitContainerWithRepositoryAndTag(t *testing.T) {
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"Id":"596069db4bf5"}`))
	}))
	defer server1.Close()
	id := "abc123"
	storage := MapStorage{}
	err := storage.StoreContainer(id, server1.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &storage, "",
		Node{Address: server1.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	opts := docker.CommitContainerOptions{Container: id, Repository: "tsuru/python", Tag: "v1"}
	image, err := cluster.CommitContainer(opts)
	if err != nil {
		t.Fatal(err)
	}
	if image.ID != "596069db4bf5" {
		t.Errorf("CommitContainer: the image container is %s, expected: '596069db4bf5'", image.ID)
	}
	img, err := storage.RetrieveImage("tsuru/python:v1")
	if err != nil {
		t.Fatal(err)
	}
	if img.LastNode != server1.URL {
		t.Errorf("CommitContainer(%q): wrong image last node in the storage. Want %q. Got %q", id, server1.URL, img.LastNode)
	}
}

func TestExportContainer(t *testing.T) {
	content := "tar content of container"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(content))
	}))
	defer server.Close()
	containerID := "3e2f21a89f"
	storage := &MapStorage{}
	err := storage.StoreContainer(containerID, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, storage, "", Node{Address: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	err = cluster.ExportContainer(docker.ExportContainerOptions{ID: containerID, OutputStream: out})
	if err != nil {
		t.Errorf("ExportContainer: unexpected error: %#v", err.Error())
	}
	if out.String() != content {
		t.Errorf("ExportContainer: wrong out. Want %#v. Got %#v.", content, out.String())
	}
}

func TestExportContainerNotFoundWithStorage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(""))
	}))
	defer server.Close()
	cluster, err := New(nil, &MapStorage{}, "", Node{Address: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	containerID := "3e2f21a89f"
	out := &bytes.Buffer{}
	err = cluster.ExportContainer(docker.ExportContainerOptions{ID: containerID, OutputStream: out})
	if err == nil {
		t.Error("ExportContainer: expected error not to be <nil>")
	}
}

func TestExportContainerNoStorage(t *testing.T) {
	content := "tar content of container"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(content))
	}))
	defer server.Close()
	cluster, err := New(nil, &MapStorage{}, "", Node{Address: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	containerID := "3e2f21a89f"
	out := &bytes.Buffer{}
	err = cluster.ExportContainer(docker.ExportContainerOptions{ID: containerID, OutputStream: out})
	if err == nil {
		t.Error("ExportContainer: expected error not to be <nil>")
	}
}

func TestGetNodeForContainer(t *testing.T) {
	var storage MapStorage
	storage.StoreContainer("e90301", "http://localhost:4242")
	storage.StoreContainer("e90304", "http://localhost:4242")
	storage.StoreContainer("e90303", "http://localhost:4241")
	storage.StoreContainer("e90302", "http://another")
	cluster, err := New(nil, &storage, "",
		Node{Address: "http://localhost:4243"},
		Node{Address: "http://localhost:4242"},
		Node{Address: "http://localhost:4241"},
	)
	if err != nil {
		t.Fatal(err)
	}
	node, err := cluster.getNodeForContainer("e90302")
	if err != nil {
		t.Error(err)
	}
	if node.addr != "http://another" {
		t.Errorf("cluster.getNode(%q): wrong node. Want %q. Got %q.", "e90302", "http://another", node.addr)
	}
	node, err = cluster.getNodeForContainer("e90301")
	if err != nil {
		t.Error(err)
	}
	if node.addr != "http://localhost:4242" {
		t.Errorf("cluster.getNode(%q): wrong node. Want %q. Got %q.", "e90301", "http://localhost:4242", node.addr)
	}
	_, err = cluster.getNodeForContainer("e90305")
	expected := cstorage.ErrNoSuchContainer
	if !reflect.DeepEqual(err, expected) {
		t.Errorf("cluster.getNode(%q): wrong error. Want %#v. Got %#v.", "e90305", expected, err)
	}
	cluster, err = New(nil, failingStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = cluster.getNodeForContainer("e90301")
	expectedMsg := "storage error"
	if err.Error() != expectedMsg {
		t.Errorf("cluster.getNode(%q): wrong error. Want %q. Got %q.", "e90301", expectedMsg, err.Error())
	}
}

func TestTopContainer(t *testing.T) {
	server1, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	server2, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &MapStorage{}, "",
		Node{Address: server1.URL()},
		Node{Address: server2.URL()},
	)
	if err != nil {
		t.Fatal(err)
	}
	config := docker.Config{Memory: 1000, Image: "myhost/somwhere/myimg"}
	config.Cmd = []string{"tail", "-f"}
	opts := docker.CreateContainerOptions{Config: &config}
	_, container, err := cluster.CreateContainer(opts, time.Minute, server1.URL())
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.StartContainer(container.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := cluster.TopContainer(container.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Processes) != 1 {
		t.Fatalf("TopContainer: Unexpected process len, got: %d", len(result.Processes))
	}
	if result.Processes[0][len(result.Processes[0])-1] != "tail -f" {
		t.Fatalf("TopContainer: Unexpected command name, got: %s", result.Processes[0][len(result.Processes[0])-1])
	}
}

func TestExecContainer(t *testing.T) {
	server, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &MapStorage{}, "", Node{Address: server.URL()})
	if err != nil {
		t.Fatal(err)
	}
	config := docker.Config{Memory: 1000, Image: "myhost/somwhere/myimg"}
	config.Cmd = []string{"tail", "-f"}
	opts := docker.CreateContainerOptions{Config: &config}
	_, container, err := cluster.CreateContainer(opts, time.Minute, server.URL())
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.StartContainer(container.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	createExecOpts := docker.CreateExecOptions{
		AttachStdin:  false,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
		Cmd:          []string{"ls"},
		Container:    container.ID,
	}
	exec, err := cluster.CreateExec(createExecOpts)
	if err != nil {
		t.Fatal(err)
	}
	if exec == nil {
		t.Fatal("CreateExec: Exec was not created!")
	}
	startExecOptions := docker.StartExecOptions{
		OutputStream: nil,
		ErrorStream:  nil,
		RawTerminal:  true,
	}
	err = cluster.StartExec(exec.ID, container.ID, startExecOptions)
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.ResizeExecTTY(exec.ID, container.ID, 10, 10)
	if err != nil {
		t.Fatal(err)
	}
}

func TestInspectExec(t *testing.T) {
	body := `{"ID":"d34db33f","Running":false,"ExitCode":1}`
	var count int
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server2.Close()
	contId := "e90302"
	storage := MapStorage{}
	err := storage.StoreContainer(contId, server2.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &storage, "",
		Node{Address: server1.URL},
		Node{Address: server2.URL},
	)
	if err != nil {
		t.Fatal(err)
	}
	execId := "d34db33f"
	exec, err := cluster.InspectExec(execId, contId)
	if err != nil {
		t.Fatal(err)
	}
	if exec.ID != execId {
		t.Errorf("InspectExec: Wrong ID. Want %q. Got %q.", execId, exec.ID)
	}
	if exec.Running {
		t.Errorf("InspectExec: Wrong Running. Want false. Got true.")
	}
	if exec.ExitCode != 1 {
		t.Errorf("InspectExec: Wrong Running. Want %d. Got %d.", 1, exec.ExitCode)
	}
	if count > 0 {
		t.Errorf("InspectExec: should not send request to all servers, but did.")
	}
}

func TestUploadToContainer(t *testing.T) {
	server, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &MapStorage{}, "", Node{Address: server.URL()})
	if err != nil {
		t.Fatal(err)
	}
	config := docker.Config{Memory: 1000, Image: "myhost/somwhere/myimg"}
	config.Cmd = []string{"tail", "-f"}
	opts := docker.CreateContainerOptions{Config: &config}
	_, container, err := cluster.CreateContainer(opts, time.Minute, server.URL())
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.StartContainer(container.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	input := bytes.NewBufferString("test")
	uploadOpts := docker.UploadToContainerOptions{
		InputStream: input,
		Path:        "test-test",
	}
	err = cluster.UploadToContainer(container.ID, uploadOpts)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDownloadFromContainer(t *testing.T) {
	server, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := New(nil, &MapStorage{}, "", Node{Address: server.URL()})
	if err != nil {
		t.Fatal(err)
	}
	config := docker.Config{Memory: 1000, Image: "myhost/somwhere/myimg"}
	config.Cmd = []string{"tail", "-f"}
	opts := docker.CreateContainerOptions{Config: &config}
	_, container, err := cluster.CreateContainer(opts, time.Minute, server.URL())
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.StartContainer(container.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	path := "test-test"
	input := bytes.NewBufferString("test")
	uploadOpts := docker.UploadToContainerOptions{
		InputStream: input,
		Path:        path,
	}
	err = cluster.UploadToContainer(container.ID, uploadOpts)
	if err != nil {
		t.Fatal(err)
	}
	output := bytes.Buffer{}
	downloadOpts := docker.DownloadFromContainerOptions{
		OutputStream: &output,
		Path:         path,
	}
	err = cluster.DownloadFromContainer(container.ID, downloadOpts)
	if err != nil {
		t.Fatal(err)
	}
}
