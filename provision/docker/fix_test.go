// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/types"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
)

func startDocker(hostPort string) (func(), *httptest.Server, *dockerProvisioner) {
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
					"HostPort": "%s"
				}
			]
		}
	}
}`
	output = fmt.Sprintf(output, hostPort)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/containers/9930c24f1c4x") {
			w.Write([]byte(output))
		}
	}))
	var err error
	var p dockerProvisioner
	err = p.Initialize()
	if err != nil {
		panic(err)
	}
	p.cluster, err = cluster.New(nil, &cluster.MapStorage{}, "",
		cluster.Node{Address: server.URL},
	)
	if err != nil {
		panic(err)
	}
	return func() {
		server.Close()
	}, server, &p
}

func (s *S) TestFixContainer(c *check.C) {
	cleanup, server, p := startDocker("9999")
	defer cleanup()
	coll := p.Collection()
	defer coll.Close()
	cont := container.Container{
		Container: types.Container{
			ID:          "9930c24f1c4x",
			AppName:     "makea",
			Type:        "python",
			Status:      provision.StatusStarted.String(),
			IP:          "127.0.0.4",
			HostPort:    "9025",
			HostAddr:    "127.0.0.1",
			ExposedPort: "8888/tcp",
		},
	}
	err := coll.Insert(cont)
	c.Assert(err, check.IsNil)
	defer coll.RemoveAll(bson.M{"appname": cont.AppName})
	err = s.conn.Apps().Insert(&app.App{Name: cont.AppName})
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp(cont.AppName, cont.Type, "test-default", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	var storage cluster.MapStorage
	storage.StoreContainer(cont.ID, server.URL)
	p.cluster, err = cluster.New(nil, &storage, "",
		cluster.Node{Address: server.URL},
	)
	c.Assert(err, check.IsNil)
	info, err := cont.NetworkInfo(p.ClusterClient())
	c.Assert(err, check.IsNil)
	err = p.fixContainer(&cont, info)
	c.Assert(err, check.IsNil)
	conta, err := p.GetContainer("9930c24f1c4x")
	c.Assert(err, check.IsNil)
	c.Assert(conta.IP, check.Equals, "127.0.0.9")
	c.Assert(conta.HostPort, check.Equals, "9999")
}

func (s *S) TestCheckContainer(c *check.C) {
	cleanup, server, p := startDocker("9999")
	defer cleanup()
	coll := p.Collection()
	defer coll.Close()
	cont := container.Container{
		Container: types.Container{
			ID:       "9930c24f1c4x",
			AppName:  "makea",
			Type:     "python",
			Status:   provision.StatusStarted.String(),
			IP:       "127.0.0.9",
			HostPort: "9999",
			HostAddr: "127.0.0.1",
		},
	}
	err := coll.Insert(cont)
	c.Assert(err, check.IsNil)
	defer coll.RemoveAll(bson.M{"appname": cont.AppName})
	var storage cluster.MapStorage
	storage.StoreContainer(cont.ID, server.URL)
	p.cluster, err = cluster.New(nil, &storage, "",
		cluster.Node{Address: server.URL},
	)
	c.Assert(err, check.IsNil)
	err = p.checkContainer(&cont)
	c.Assert(err, check.IsNil)
}
