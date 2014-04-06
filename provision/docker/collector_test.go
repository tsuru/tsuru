// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/tsuru/docker-cluster/cluster"
	etesting "github.com/tsuru/tsuru/exec/testing"
	"github.com/tsuru/tsuru/provision"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"strings"
)

func (s *S) TestCollectStatusForStartedUnit(c *gocheck.C) {
	listener := startTestListener("127.0.0.1:9024")
	defer listener.Close()
	coll := collection()
	defer coll.Close()
	err := coll.Insert(
		container{
			ID:       "9930c24f1c5f",
			AppName:  "ashamed",
			Type:     "python",
			Status:   "running",
			IP:       "127.0.0.3",
			HostPort: "9024",
			HostAddr: "127.0.0.1",
		},
	)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"appname": "ashamed"})
	expected := []provision.Unit{
		{
			Name:    "9930c24f1c5f",
			AppName: "ashamed",
			Type:    "python",
			Machine: 0,
			Ip:      "127.0.0.1",
			Status:  provision.StatusStarted,
		},
	}
	var p dockerProvisioner
	units, err := p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	sortUnits(units)
	sortUnits(expected)
	c.Assert(units, gocheck.DeepEquals, expected)
}

func (s *S) TestCollectStatusForUnreachableUnit(c *gocheck.C) {
	coll := collection()
	defer coll.Close()
	err := coll.Insert(
		container{
			ID:       "9930c24f1c4f",
			AppName:  "make-up",
			Type:     "python",
			Status:   "running",
			IP:       "127.0.0.4",
			HostPort: "9025",
			HostAddr: "127.0.0.1",
		},
	)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"appname": "make-up"})
	expected := []provision.Unit{
		{
			Name:    "9930c24f1c4f",
			AppName: "make-up",
			Type:    "python",
			Machine: 0,
			Ip:      "127.0.0.1",
			Status:  provision.StatusUnreachable,
		},
	}
	var p dockerProvisioner
	units, err := p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	sortUnits(units)
	sortUnits(expected)
	c.Assert(units, gocheck.DeepEquals, expected)
}

func startDocker() (func(), *httptest.Server) {
	output := `{
    "State": {
        "Running": true,
        "Pid": 2785,
        "ExitCode": 0,
        "StartedAt": "2013-08-15T03:38:45.709874216-03:00",
        "Ghost": false
    },
    "Image": "b750fe79269d2ec9a3c593ef05b4332b1d1a02a62b4accb2c21d589ff2f5f2dc",
	"NetworkSettings": {
		"IpAddress": "127.0.0.9",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"Ports": {
			"8888/tcp": [
				{
					"HostIp": "0.0.0.0",
					"HostPort": "9999"
				}
			]
		}
	}
}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/containers/9930c24f1c4x") {
			w.Write([]byte(output))
		}
	}))
	var err error
	oldCluster := dockerCluster()
	dCluster, err = cluster.New(nil, &mapStorage{},
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

func (s *S) TestCollectStatusFixContainer(c *gocheck.C) {
	coll := collection()
	defer coll.Close()
	err := coll.Insert(
		container{
			ID:       "9930c24f1c4x",
			AppName:  "makea",
			Type:     "python",
			Status:   "running",
			IP:       "127.0.0.4",
			HostPort: "9025",
			HostAddr: "127.0.0.1",
		},
	)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"appname": "makea"})
	expected := []provision.Unit{
		{
			Name:    "9930c24f1c4x",
			AppName: "makea",
			Type:    "python",
			Machine: 0,
			Ip:      "127.0.0.1",
			Status:  provision.StatusUnreachable,
		},
	}
	cleanup, server := startDocker()
	defer cleanup()
	var storage mapStorage
	storage.StoreContainer("9930c24f1c4x", "server0")
	cmutex.Lock()
	dCluster, err = cluster.New(nil, &storage,
		cluster.Node{ID: "server0", Address: server.URL},
	)
	cmutex.Unlock()
	c.Assert(err, gocheck.IsNil)
	var p dockerProvisioner
	units, err := p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	sortUnits(units)
	sortUnits(expected)
	c.Assert(units, gocheck.DeepEquals, expected)
	cont, err := getContainer("9930c24f1c4x")
	c.Assert(err, gocheck.IsNil)
	c.Assert(cont.IP, gocheck.Equals, "127.0.0.9")
	c.Assert(cont.HostPort, gocheck.Equals, "9999")
}

func (s *S) TestCollectStatusForDownUnit(c *gocheck.C) {
	coll := collection()
	defer coll.Close()
	err := coll.Insert(
		container{
			ID:       "9930c24f1c6f",
			AppName:  "make-up",
			Type:     "python",
			Status:   "error",
			HostAddr: "127.0.0.1",
		},
	)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"appname": "make-up"})
	expected := []provision.Unit{
		{
			Name:    "9930c24f1c6f",
			AppName: "make-up",
			Type:    "python",
			Status:  provision.StatusDown,
		},
	}
	var p dockerProvisioner
	units, err := p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	sortUnits(units)
	sortUnits(expected)
	c.Assert(units, gocheck.DeepEquals, expected)
}

func (s *S) TestProvisionCollectStatusEmpty(c *gocheck.C) {
	coll := collection()
	defer coll.Close()
	coll.RemoveAll(nil)
	output := map[string][][]byte{"ps -q": {[]byte("")}}
	fexec := &etesting.FakeExecutor{Output: output}
	setExecut(fexec)
	defer setExecut(nil)
	var p dockerProvisioner
	units, err := p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	c.Assert(units, gocheck.HasLen, 0)
}
