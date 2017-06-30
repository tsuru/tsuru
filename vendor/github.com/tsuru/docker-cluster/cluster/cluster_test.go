// Copyright 2014 docker-cluster authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cluster

import (
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/docker-cluster/storage"
)

func TestNewCluster(t *testing.T) {
	var tests = []struct {
		input []Node
		fail  bool
	}{
		{
			[]Node{{Address: "http://localhost:8083"}},
			false,
		},
		{
			[]Node{{Address: ""}, {Address: "http://localhost:8083"}},
			true,
		},
		{
			[]Node{{Address: "http://localhost:8083"}},
			false,
		},
	}
	for _, tt := range tests {
		_, err := New(nil, &MapStorage{}, "", tt.input...)
		if tt.fail && err == nil || !tt.fail && err != nil {
			t.Errorf("cluster.New() for input %#v. Expect failure: %v. Got: %v.", tt.input, tt.fail, err)
		}
	}
}

func TestClusterSetTLSConfigInNodes(t *testing.T) {
	c, err := New(nil, &MapStorage{}, "./testdata", []Node{{Address: "http://localhost:8888"}}...)
	if err != nil {
		t.Fatalf("error setting up cluster: %s", err)
	}
	nodes, err := c.Nodes()
	if err != nil {
		t.Fatalf("Error getting nodes: %s", err)
	}
	if c.tlsConfig == nil {
		t.Fatalf("tlsConfig not setted in cluster.")
	}
	if nodes[0].defTLSConfig == nil {
		t.Fatal("tlsConfig not setted in node.")
	}
}

func TestNewFailure(t *testing.T) {
	_, err := New(&roundRobin{}, nil, "")
	if err != errStorageMandatory {
		t.Fatalf("expected errStorageMandatory error, got: %#v", err)
	}
}

func TestRegister(t *testing.T) {
	scheduler := &roundRobin{}
	cluster, err := New(scheduler, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	node := Node{
		Address:  "http://localhost1:4243",
		Metadata: map[string]string{"x": "y", "a": "b"},
	}
	err = cluster.Register(node)
	if err != nil {
		t.Fatal(err)
	}
	opts := docker.CreateContainerOptions{}
	node, err = scheduler.Schedule(cluster, opts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if node.Address != "http://localhost1:4243" {
		t.Errorf("Register failed. Got wrong Address. Want %q. Got %q.", "http://localhost1:4243", node.Address)
	}
	err = cluster.Register(Node{Address: "http://localhost2:4243"})
	if err != nil {
		t.Fatal(err)
	}
	node, err = scheduler.Schedule(cluster, opts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if node.Address != "http://localhost2:4243" {
		t.Errorf("Register failed. Got wrong ID. Want %q. Got %q.", "http://localhost2:4243", node.Address)
	}
	node, err = scheduler.Schedule(cluster, opts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if node.Address != "http://localhost1:4243" {
		t.Errorf("Register failed. Got wrong ID. Want %q. Got %q.", "http://localhost1:4243", node.Address)
	}
}

func TestRegisterDoesNotAllowRepeatedAddresses(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.Register(Node{Address: "http://localhost1:4243"})
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.Register(Node{Address: "http://localhost1:4243"})
	if err != storage.ErrDuplicatedNodeAddress {
		t.Fatalf("Expected error ErrDuplicatedNodeAddress, got: %#v", err)
	}
}

func TestRegisterFailure(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.Register(Node{})
	if err == nil {
		t.Error("Expected non-nil error, got <nil>.")
	}
}

func TestUpdateNode(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	node := Node{Address: "http://localhost1:4243"}
	err = cluster.Register(node)
	if err != nil {
		t.Fatal(err)
	}
	node.Metadata = map[string]string{"k1": "v1", "k2": "v2"}
	node, err = cluster.UpdateNode(node)
	if err != nil {
		t.Fatal(err)
	}
	expected := Node{Address: "http://localhost1:4243", Metadata: map[string]string{
		"k1": "v1",
		"k2": "v2",
	}}
	node.Healing = HealingData{}
	if !reflect.DeepEqual(node, expected) {
		t.Errorf("Expected nodes to be equal %+v, got %+v", expected, node)
	}
	nodes, err := cluster.Nodes()
	if err != nil {
		t.Fatal(err)
	}
	nodes[0].Healing = HealingData{}
	if !reflect.DeepEqual(nodes, []Node{expected}) {
		t.Errorf("Expected nodes to be equal %+v, got %+v", []Node{expected}, nodes)
	}
}

func TestUpdateNodeCreationStatus(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	node := Node{Address: "http://localhost1:4243", CreationStatus: NodeCreationStatusPending}
	err = cluster.Register(node)
	if err != nil {
		t.Fatal(err)
	}
	node.CreationStatus = NodeCreationStatusError
	_, err = cluster.UpdateNode(node)
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := cluster.UnfilteredNodes()
	if err != nil {
		t.Fatal(err)
	}
	if nodes[0].CreationStatus != NodeCreationStatusError {
		t.Errorf("UpdateNode: wrong status. Want NodeCreationStatusError. Got %s", nodes[0].CreationStatus)
	}
}

func TestUpdateNodeCreationStatusDisabled(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	node := Node{
		Address:        "http://localhost1:4243",
		CreationStatus: NodeCreationStatusCreated,
		Metadata:       map[string]string{"k1": "v1", "k2": "v2"},
	}
	err = cluster.Register(node)
	if err != nil {
		t.Fatal(err)
	}
	node.Metadata["k1"] = "v3"
	node.CreationStatus = NodeCreationStatusDisabled
	_, err = cluster.UpdateNode(node)
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := cluster.UnfilteredNodes()
	if err != nil {
		t.Fatal(err)
	}
	if nodes[0].CreationStatus != NodeCreationStatusDisabled {
		t.Errorf("UpdateNode: wrong status. Want NodeCreationStatusDisabled. Got %s", nodes[0].CreationStatus)
	}
}

func TestUpdateNodeRemoveMetadata(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	node := Node{
		Address:  "http://localhost1:4243",
		Metadata: map[string]string{"k1": "v1", "k2": "v2"},
	}
	err = cluster.Register(node)
	if err != nil {
		t.Fatal(err)
	}
	node.Metadata = map[string]string{"k1": "", "k2": "v9", "k3": "v10"}
	node, err = cluster.UpdateNode(node)
	if err != nil {
		t.Fatal(err)
	}
	expected := Node{Address: "http://localhost1:4243", Metadata: map[string]string{
		"k2": "v9",
		"k3": "v10",
	}}
	node.Healing = HealingData{}
	if !reflect.DeepEqual(node, expected) {
		t.Errorf("Expected nodes to be equal %+v, got %+v", expected, node)
	}
	nodes, err := cluster.Nodes()
	if err != nil {
		t.Fatal(err)
	}
	nodes[0].Healing = HealingData{}
	if !reflect.DeepEqual(nodes, []Node{expected}) {
		t.Errorf("Expected nodes to be equal %+v, got %+v", []Node{expected}, nodes)
	}
}

func TestUpdateNodeStress(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(8))
	cluster, err := New(nil, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.Register(Node{Address: "http://localhost1:4243"})
	if err != nil {
		t.Fatal(err)
	}
	var errCount int32
	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			node := Node{
				Address:  "http://localhost1:4243",
				Metadata: map[string]string{fmt.Sprintf("k%d", i): fmt.Sprintf("v%d", i)},
			}
			_, err := cluster.UpdateNode(node)
			if err == errHealerInProgress {
				atomic.AddInt32(&errCount, 1)
			} else if err != nil {
				t.Fatal(err)
			}
		}(i)
	}
	wg.Wait()
	if errCount <= 0 {
		t.Error("Expected errCount to me greater than 0")
	}
	nodes, err := cluster.Nodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes[0].Metadata) == 0 {
		t.Error("Expected to have at least one metadata, got 0")
	}
}

func TestGetNode(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	addr := "http://localhost:4243"
	nd := Node{Address: addr}
	err = cluster.Register(nd)
	if err != nil {
		t.Fatal(err)
	}
	defer cluster.Unregister(addr)
	node, err := cluster.GetNode(addr)
	if err != nil {
		t.Fatal(err)
	}
	if node.Address != nd.Address {
		t.Errorf("GetNode(%q): wrong node. Want %s. Got %s", addr, nd.Address, node.Address)
	}
}

func TestUnregister(t *testing.T) {
	scheduler := &roundRobin{}
	cluster, err := New(scheduler, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.Register(Node{Address: "http://localhost1:4243"})
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.Unregister("http://localhost1:4243")
	if err != nil {
		t.Fatal(err)
	}
	opts := docker.CreateContainerOptions{}
	_, err = scheduler.Schedule(cluster, opts, nil)
	if err == nil || err.Error() != "No nodes available" {
		t.Fatal("Expected no nodes available error")
	}
}

func TestNodesShouldGetClusterNodes(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.Register(Node{Address: "http://localhost:4243"})
	if err != nil {
		t.Fatal(err)
	}
	defer cluster.Unregister("http://localhost:4243")
	nodes, err := cluster.Nodes()
	if err != nil {
		t.Fatal(err)
	}
	expected := []Node{{Address: "http://localhost:4243", Metadata: map[string]string{}}}
	if !reflect.DeepEqual(nodes, expected) {
		t.Errorf("Expected nodes to be equal %+v, got %+v", expected, nodes)
	}
}

func TestNodesShouldGetClusterNodesWithoutDisabledNodes(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "")
	stopChan := make(chan bool)
	healer := &blockingHealer{stop: stopChan}
	defer close(stopChan)
	cluster.Healer = healer
	if err != nil {
		t.Fatal(err)
	}
	defer cluster.Unregister("http://server1:4243")
	defer cluster.Unregister("http://server2:4243")
	err = cluster.Register(Node{Address: "http://server1:4243"})
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.Register(Node{Address: "http://server2:4243"})
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.handleNodeError("http://server1:4243", errors.New("some err"), true)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan bool)
	go func() {
		stopChan <- true
		for {
			node, err := cluster.storage().RetrieveNode("http://server1:4243")
			if err != nil {
				t.Fatal(err)
			}
			if !node.isHealing() {
				break
			}
		}
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for healer call being made and unlocked")
	}
	nodes, err := cluster.Nodes()
	if err != nil {
		t.Fatal(err)
	}
	expected := []Node{
		{Address: "http://server2:4243", Metadata: map[string]string{}},
	}
	if !reflect.DeepEqual(nodes, expected) {
		t.Errorf("Expected nodes to be equal %#v, got %#v", expected, nodes)
	}
}

func TestUnfilteredNodesReturnAllNodes(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer cluster.Unregister("http://server1:4243")
	defer cluster.Unregister("http://server2:4243")
	err = cluster.Register(Node{Address: "http://server1:4243"})
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.Register(Node{Address: "http://server2:4243"})
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.handleNodeError("http://server1:4243", errors.New("some err"), true)
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := cluster.UnfilteredNodes()
	if err != nil {
		t.Fatal(err)
	}
	expected := []Node{
		{Address: "http://server1:4243", Metadata: map[string]string{}},
		{Address: "http://server2:4243", Metadata: map[string]string{}},
	}
	sort.Sort(NodeList(nodes))
	if len(nodes) != 2 {
		t.Errorf("Expected nodes length to be equal %d, got %d", 2, len(nodes))
	}
	nodes[0].Healing = HealingData{}
	if !reflect.DeepEqual(nodes, expected) {
		t.Errorf("Expected nodes to be equal %+v, got %+v", expected, nodes)
	}
}

func TestNodesForMetadataShouldGetClusterNodesWithMetadata(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.Register(Node{
		Address:  "http://server1:4243",
		Metadata: map[string]string{"key1": "val1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.Register(Node{
		Address:  "http://server2:4243",
		Metadata: map[string]string{"key1": "val2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cluster.Unregister("http://server1:4243")
	defer cluster.Unregister("http://server2:4243")
	nodes, err := cluster.NodesForMetadata(map[string]string{"key1": "val2"})
	if err != nil {
		t.Fatal(err)
	}
	expected := []Node{{Address: "http://server2:4243", Metadata: map[string]string{"key1": "val2"}}}
	if !reflect.DeepEqual(nodes, expected) {
		t.Errorf("Expected nodes to be equal %+v, got %+v", expected, nodes)
	}
}

func TestNodesShouldReturnEmptyListWhenNoNodeIsFound(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := cluster.Nodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Errorf("Expected nodes to be empty, got %+v", nodes)
	}
}

func TestUnfilteredNodesForMetadataShouldGetClusterNodesWithMetadata(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.Register(Node{
		Address:  "http://server1:4243",
		Metadata: map[string]string{"key1": "val1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = cluster.Register(Node{
		Address:        "http://server2:4243",
		Metadata:       map[string]string{"key1": "val2"},
		CreationStatus: NodeCreationStatusDisabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cluster.Unregister("http://server1:4243")
	defer cluster.Unregister("http://server2:4243")
	nodes, err := cluster.UnfilteredNodesForMetadata(map[string]string{"key1": "val2"})
	if err != nil {
		t.Fatal(err)
	}
	expected := []Node{{
		Address:        "http://server2:4243",
		Metadata:       map[string]string{"key1": "val2"},
		CreationStatus: NodeCreationStatusDisabled},
	}
	if !reflect.DeepEqual(nodes, expected) {
		t.Errorf("Expected nodes to be equal %+v, got %+v", expected, nodes)
	}
}

func TestRunOnNodesWhenReceiveingNodeShouldntLoadStorage(t *testing.T) {
	id := "e90302"
	body := fmt.Sprintf(`{"Id":"%s","Path":"date","Args":[]}`, id)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	cluster, err := New(nil, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := cluster.runOnNodes(func(n node) (interface{}, error) {
		return n.InspectContainer(id)
	}, &docker.NoSuchContainer{ID: id}, false, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	container := result.(*docker.Container)
	if container.ID != id {
		t.Errorf("InspectContainer(%q): Wrong ID. Want %q. Got %q.", id, id, container.ID)
	}
	if container.Path != "date" {
		t.Errorf("InspectContainer(%q): Wrong Path. Want %q. Got %q.", id, "date", container.Path)
	}
}

func TestRunOnNodesStress(t *testing.T) {
	n := 1000
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(16))
	body := `{"Id":"e90302","Path":"date","Args":[]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server.Close()
	id := "e90302"
	cluster, err := New(nil, &MapStorage{}, "", Node{Address: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < rand.Intn(10)+n; i++ {
		result, err := cluster.runOnNodes(func(n node) (interface{}, error) {
			return n.InspectContainer(id)
		}, &docker.NoSuchContainer{ID: id}, false)
		if err != nil {
			t.Fatal(err)
		}
		container := result.(*docker.Container)
		if container.ID != id {
			t.Errorf("InspectContainer(%q): Wrong ID. Want %q. Got %q.", id, id, container.ID)
		}
		if container.Path != "date" {
			t.Errorf("InspectContainer(%q): Wrong Path. Want %q. Got %q.", id, "date", container.Path)
		}
	}
}

func TestClusterNodes(t *testing.T) {
	c, err := New(&roundRobin{}, &MapStorage{}, "")
	if err != nil {
		t.Fatalf("unexpected error %s", err.Error())
	}
	nodes := []Node{
		{Address: "http://localhost:8080", Metadata: map[string]string{}},
		{Address: "http://localhost:8081", Metadata: map[string]string{}},
	}
	for _, n := range nodes {
		c.Register(n)
	}
	got, err := c.Nodes()
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(got, nodes) {
		t.Errorf("roundRobin.Nodes(): wrong result. Want %#v. Got %#v.", nodes, got)
	}
}

func TestClusterNodesUnregister(t *testing.T) {
	c, err := New(&roundRobin{}, &MapStorage{}, "")
	if err != nil {
		t.Fatalf("unexpected error %s", err.Error())
	}
	nodes := []Node{
		{Address: "http://localhost:8080"},
		{Address: "http://localhost:8081"},
	}
	for _, n := range nodes {
		c.Register(n)
	}
	c.Unregister(nodes[0].Address)
	got, err := c.Nodes()
	if err != nil {
		t.Error(err)
	}
	expected := []Node{{Address: "http://localhost:8081", Metadata: map[string]string{}}}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("roundRobin.Nodes(): wrong result. Want %#v. Got %#v.", nodes, got)
	}
}

type blockingHealer struct {
	calls         int
	disabledUntil string
	failureCount  int
	stop          <-chan bool
}

func (h *blockingHealer) HandleError(n *Node) time.Duration {
	h.calls++
	h.failureCount = n.FailureCount()
	h.disabledUntil = n.Metadata["DisabledUntil"]
	<-h.stop
	return 1 * time.Minute
}

func isDateSameMinute(dt1, dt2 string) bool {
	re := regexp.MustCompile(`(.*T\d{2}:\d{2}).*`)
	dt1Minute := re.ReplaceAllString(dt1, "$1")
	dt2Minute := re.ReplaceAllString(dt2, "$1")
	return dt1Minute == dt2Minute
}

func TestClusterHandleNodeErrorStress(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(100))
	c, err := New(&roundRobin{}, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	stopChan := make(chan bool)
	healer := &blockingHealer{stop: stopChan}
	c.Healer = healer
	err = c.Register(Node{Address: "stress-addr-1"})
	if err != nil {
		t.Fatal(err)
	}
	expectedErr := errors.New("some error")
	for i := 0; i < 200; i++ {
		c.handleNodeError("stress-addr-1", expectedErr, true)
	}
	done := make(chan bool)
	go func() {
		stopChan <- true
		for {
			node, err := c.storage().RetrieveNode("stress-addr-1")
			if err != nil {
				continue
			}
			if !node.isHealing() {
				break
			}
		}
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for node unlock")
	}
	if healer.failureCount != 1 {
		t.Errorf("Expected %d failures count, got: %d", 1, healer.failureCount)
	}
	if healer.calls != 1 {
		t.Errorf("Expected healer to have 1 call, got: %d", healer.calls)
	}
	err = c.handleNodeError("stress-addr-1", expectedErr, true)
	if err != nil {
		t.Fatal(err)
	}
	done = make(chan bool)
	go func() {
		stopChan <- true
		for {
			node, err := c.storage().RetrieveNode("stress-addr-1")
			if err != nil {
				continue
			}
			if !node.isHealing() {
				break
			}
		}
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for node unlock")
	}
	if healer.calls != 2 {
		t.Errorf("Expected healer to have 2 calls, got: %d", healer.calls)
	}
	if healer.failureCount != 2 {
		t.Errorf("Expected %d failures count, got: %d", 2, healer.failureCount)
	}
	disabledStr := healer.disabledUntil
	now := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	if !isDateSameMinute(disabledStr, now) {
		t.Errorf("Expected DisabledUntil to be like %s, got: %s", now, disabledStr)
	}
	nodes, err := c.storage().RetrieveNodes()
	node := nodes[0]
	if err != nil {
		t.Fatal(err)
	}
	if node.FailureCount() != 2 {
		t.Errorf("Expected FailureCount to be 2, got: %d", node.FailureCount())
	}
	if !isDateSameMinute(node.Metadata["DisabledUntil"], disabledStr) {
		t.Errorf("Expected DisabledUntil to be like %s, got: %s", disabledStr, node.Metadata["DisabledUntil"])
	}
}

func TestClusterHandleNodeSuccess(t *testing.T) {
	c, err := New(&roundRobin{}, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	err = c.Register(Node{
		Address:  "addr-1",
		Metadata: map[string]string{"Failures": "10"},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = c.handleNodeSuccess("addr-1")
	if err != nil {
		t.Fatal(err)
	}
	node, err := c.storage().RetrieveNode("addr-1")
	if err != nil {
		t.Fatal(err)
	}
	if node.FailureCount() != 0 {
		t.Errorf("Expected FailureCount to be 0, got: %d", node.FailureCount())
	}
	if !node.Healing.LockedUntil.IsZero() {
		t.Errorf("Expected node.Healing to be zero, got: %s", node.Healing.LockedUntil)
	}
}

func TestClusterHandleNodeSuccessStressShouldntBlockNodes(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(10))
	c, err := New(&roundRobin{}, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	err = c.Register(Node{Address: "addr-1"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		go func() {
			err := c.handleNodeSuccess("addr-1")
			if err != nil && err != errHealerInProgress {
				t.Fatal(err)
			}
		}()
		go func() {
			nodes, err := c.Nodes()
			if err != nil {
				t.Fatal(err)
			}
			if len(nodes) != 1 {
				t.Fatalf("Expected nodes len to be 1, got %d", len(nodes))
			}
		}()
	}
}

func TestClusterStartActiveMonitoring(t *testing.T) {
	c, err := New(&roundRobin{}, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	callCount1 := int32(0)
	callCount2 := int32(0)
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount1, 1)
		w.WriteHeader(http.StatusOK)
	}))
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount2, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	err = c.Register(Node{Address: server1.URL})
	if err != nil {
		t.Fatal(err)
	}
	err = c.Register(Node{Address: server2.URL})
	if err != nil {
		t.Fatal(err)
	}
	c.StartActiveMonitoring(100 * time.Millisecond)
	time.Sleep(200 * time.Millisecond)
	if atomic.LoadInt32(&callCount1) == 0 {
		t.Fatal("Expected server1 to be called")
	}
	if atomic.LoadInt32(&callCount2) == 0 {
		t.Fatal("Expected server2 to be called")
	}
	nodes, err := c.Nodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("Expected nodes to have len 1, got: %d", len(nodes))
	}
	if nodes[0].Address != server1.URL {
		t.Errorf("Expected node to have address %s, got: %s", server1.URL, nodes[0].Address)
	}
	nodes, err = c.UnfilteredNodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Errorf("Expected unfiltered nodes to have len 2, got: %d", len(nodes))
	}
	enabledMap := make(map[string]bool)
	for _, node := range nodes {
		enabledMap[node.Address] = node.isEnabled()
	}
	if !enabledMap[server1.URL] {
		t.Error("Expected server1 to be enabled")
	}
	if enabledMap[server2.URL] {
		t.Error("Expected server2 to be disabled")
	}
	c.StopActiveMonitoring()
	oldCallCount := atomic.LoadInt32(&callCount1)
	time.Sleep(200 * time.Millisecond)
	currentCallCount := atomic.LoadInt32(&callCount1)
	if currentCallCount != oldCallCount {
		t.Errorf("Expected stop monitoring to stop calls to server, previous: %d current: %d", oldCallCount, currentCallCount)
	}
}

func TestWrapError(t *testing.T) {
	err := errors.New("my error")
	node := node{addr: "199.222.111.10"}
	wrapped := wrapError(node, err)
	expected := "error in docker node \"199.222.111.10\": my error"
	if wrapped.Error() != expected {
		t.Fatalf("Expected to receive %s, got: %s", expected, wrapped.Error())
	}
	nodeErr, ok := wrapped.(DockerNodeError)
	if !ok {
		t.Fatalf("Expected wrapped to be DockerNodeError")
	}
	if nodeErr.BaseError() != err {
		t.Fatalf("Expected BaseError to be original error")
	}
}

func TestWrapErrorNil(t *testing.T) {
	node := node{addr: "199.222.111.10"}
	wrapped := wrapError(node, nil)
	if wrapped != nil {
		t.Fatalf("Expected to receive nil, got: %#v", wrapped)
	}
}

func TestClusterGetNodeByAddr(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	node, err := cluster.getNodeByAddr("http://199.222.111.10")
	if err != nil {
		t.Fatal(err)
	}
	if node.HTTPClient.Timeout != 5*time.Minute {
		t.Fatalf("Expected timeout %#v, got %#v", 5*time.Minute, node.HTTPClient.Timeout)
	}
	tlsConfig := node.HTTPClient.Transport.(*http.Transport).TLSClientConfig
	if tlsConfig != nil {
		t.Fatalf("Expected tls config to be nil, got %#v", tlsConfig)
	}
}

func TestClusterGetNodeByAddrWithTLS(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "./testdata")
	if err != nil {
		t.Fatal(err)
	}
	node, err := cluster.getNodeByAddr("https://199.222.111.10")
	if err != nil {
		t.Fatal(err)
	}
	if node.HTTPClient.Timeout != 5*time.Minute {
		t.Fatalf("Expected timeout %#v, got %#v", 5*time.Minute, node.HTTPClient.Timeout)
	}
	tlsConfig := node.HTTPClient.Transport.(*http.Transport).TLSClientConfig
	if tlsConfig == nil {
		t.Fatalf("Expected tls config to be set")
	}
}

func TestNodeSetPersistentClient(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "")
	if err != nil {
		t.Fatal(err)
	}
	node, err := cluster.getNodeByAddr("http://199.222.111.10")
	if err != nil {
		t.Fatal(err)
	}
	node.setPersistentClient()
	if node.HTTPClient.Timeout != 0 {
		t.Fatalf("Expected timeout %#v, got %#v", 0, node.HTTPClient.Timeout)
	}
	tlsConfig := node.HTTPClient.Transport.(*http.Transport).TLSClientConfig
	if tlsConfig != nil {
		t.Fatalf("Expected tls config to be nil, got %#v", tlsConfig)
	}
}

func TestNodeSetPersistentClientWithTLS(t *testing.T) {
	cluster, err := New(nil, &MapStorage{}, "./testdata")
	if err != nil {
		t.Fatal(err)
	}
	node, err := cluster.getNodeByAddr("https://199.222.111.10")
	if err != nil {
		t.Fatal(err)
	}
	node.setPersistentClient()
	if node.HTTPClient.Timeout != 0 {
		t.Fatalf("Expected timeout %#v, got %#v", 0, node.HTTPClient.Timeout)
	}
	tlsConfig := node.HTTPClient.Transport.(*http.Transport).TLSClientConfig
	if tlsConfig == nil {
		t.Fatalf("Expected tls config to be set")
	}
}
