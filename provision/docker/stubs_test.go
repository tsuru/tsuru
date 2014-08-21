// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/safe"
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
	dCluster, err = cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: server.URL},
	)
	if err != nil {
		panic(err)
	}
	return func() {
		server.Close()
		dCluster = oldCluster
	}, server
}

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

type hijacker struct {
	http.ResponseWriter
	conn net.Conn
	err  error
}

func (h *hijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h.err != nil {
		return nil, nil, h.err
	}
	return h.conn, nil, nil
}

type fakeConn struct {
	buf *safe.Buffer
}

func (c *fakeConn) Read(b []byte) (int, error) {
	if c.buf != nil {
		return c.buf.Read(b)
	}
	return 0, io.EOF
}

func (c *fakeConn) Write(b []byte) (int, error) {
	if c.buf != nil {
		return c.buf.Write(b)
	}
	return 0, io.ErrClosedPipe
}

func (c *fakeConn) Close() error {
	c.buf = nil
	return nil
}

func (c *fakeConn) LocalAddr() net.Addr {
	return nil
}

func (c *fakeConn) RemoteAddr() net.Addr {
	return nil
}

func (c *fakeConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *fakeConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *fakeConn) SetWriteDeadline(t time.Time) error {
	return nil
}
