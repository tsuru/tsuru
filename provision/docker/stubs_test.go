// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"github.com/fsouza/go-dockerclient"
	"github.com/globocom/config"
	"github.com/globocom/docker-cluster/cluster"
	"github.com/globocom/docker-cluster/storage"
	etesting "github.com/globocom/tsuru/exec/testing"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
)

func startTestListener(addr string) net.Listener {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	return listener
}

func startDockerTestServer(containerPort string, calls *int64) (func(), *httptest.Server) {
	listAllOutput := `[
    {
        "Id": "8dfafdbc3a40",
        "Image": "base:latest",
        "Command": "echo 1",
        "Created": 1367854155,
        "Status": "Ghost",
        "Ports": null,
        "SizeRw":12288,
        "SizeRootFs": 0
    },
    {
        "Id": "dca19cd9bb9e",
        "Image": "tsuru/python:latest",
        "Command": "echo 1",
        "Created": 1376319760,
        "Status": "Exit 0",
        "Ports": null,
        "SizeRw": 0,
        "SizeRootFs": 0
    },
    {
        "Id": "3fd99cd9bb84",
        "Image": "tsuru/python:latest",
        "Command": "echo 1",
        "Created": 1376319760,
        "Status": "Up 7 seconds",
        "Ports": null,
        "SizeRw": 0,
        "SizeRootFs": 0
    }
]`
	c1Output := fmt.Sprintf(`{
    "State": {
        "Running": true,
        "Pid": 2785,
        "ExitCode": 0,
        "StartedAt": "2013-08-15T03:38:45.709874216-03:00",
        "Ghost": false
    },
	"NetworkSettings": {
		"IpAddress": "127.0.0.4",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"Ports": {
			"8888/tcp": [
				{
					"HostIp": "0.0.0.0",
					"HostPort": "%s"
				}
			]
		}
	}
}`, containerPort)
	c2Output := `{
    "State": {
        "Running": true,
        "Pid": 2785,
        "ExitCode": 0,
        "StartedAt": "2013-08-15T03:38:45.709874216-03:00",
        "Ghost": false
    },
    "Image": "b750fe79269d2ec9a3c593ef05b4332b1d1a02a62b4accb2c21d589ff2f5f2dc",
	"NetworkSettings": {
		"IpAddress": "127.0.0.1",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"Ports": {
			"8888/tcp": [
				{
					"HostIp": "0.0.0.0",
					"HostPort": "9024"
				}
			]
		}
	}
}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(calls, 1)
		if strings.Contains(r.URL.Path, "/containers/") {
			if strings.Contains(r.URL.Path, "/containers/9930c24f1c4f") {
				w.Write([]byte(c2Output))
			}
			if strings.Contains(r.URL.Path, "/containers/9930c24f1c5f") {
				w.Write([]byte(c1Output))
			}
			if strings.Contains(r.URL.Path, "/containers/json") {
				w.Write([]byte(listAllOutput))
			}
			if strings.Contains(r.URL.Path, "/export") {
				w.Write([]byte("tar stream data"))
			}
		}
		if strings.Contains(r.URL.Path, "/commit") {
			w.Write([]byte(`{"Id":"i-1"}`))
		}
	}))
	var err error
	oldCluster := dockerCluster()
	dCluster, err = cluster.New(nil, storage.Redis("localhost:6379", "tests"),
		cluster.Node{ID: "server", Address: server.URL},
	)
	if err != nil {
		panic(err)
	}
	return func() {
		server.Close()
		dCluster = oldCluster
	}, server
}

func startSSHAgentServer(output string) (*FakeSSHServer, func()) {
	var handler FakeSSHServer
	handler.output = output
	server := httptest.NewServer(&handler)
	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	portNumber, _ := strconv.Atoi(port)
	config.Set("docker:ssh-agent-port", portNumber)
	return &handler, func() {
		server.Close()
		config.Unset("docker:ssh-agent-port")
	}
}

func mockExecutor() (*etesting.FakeExecutor, func()) {
	fexec := &etesting.FakeExecutor{Output: map[string][][]byte{}}
	setExecut(fexec)
	return fexec, func() {
		setExecut(nil)
	}
}

type mapStorage struct {
	containers map[string]string
}

func (m *mapStorage) StoreContainer(containerID, hostID string) error {
	if m.containers == nil {
		m.containers = make(map[string]string)
	}
	m.containers[containerID] = hostID
	return nil
}

func (m *mapStorage) RetrieveContainer(containerID string) (string, error) {
	return m.containers[containerID], nil
}

func (m *mapStorage) RemoveContainer(containerID string) error {
	delete(m.containers, containerID)
	return nil
}

func (m *mapStorage) StoreImage(imageID, hostID string) error      { return nil }
func (m *mapStorage) RetrieveImage(imageID string) (string, error) { return "", nil }
func (m *mapStorage) RemoveImage(imageID string) error             { return nil }

type fakeScheduler struct {
	nodes     []cluster.Node
	container *docker.Container
}

func (s *fakeScheduler) Nodes() ([]cluster.Node, error) {
	return s.nodes, nil
}

func (s *fakeScheduler) Schedule(config *docker.Config) (string, *docker.Container, error) {
	return "server", s.container, nil
}

func (s *fakeScheduler) Register(nodes ...cluster.Node) error {
	s.nodes = append(s.nodes, nodes...)
	return nil
}
