// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bs

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/tsuru/provision/docker/dockertest"
	"github.com/tsuru/tsuru/safe"
	"gopkg.in/check.v1"
)

func (s *S) TestAddNewContainer(c *check.C) {
	config := NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image:        "myimg",
			Memory:       100,
			ExposedPorts: map[docker.Port]struct{}{docker.Port("80/tcp"): {}},
			Env: []string{
				"A=1",
				"B=2",
			},
		},
		HostConfig: docker.HostConfig{
			Privileged: true,
			Binds:      []string{"/xyz:/abc:rw"},
			PortBindings: map[docker.Port][]docker.PortBinding{
				docker.Port("80/tcp"): {{HostIP: "", HostPort: ""}},
			},
			LogConfig: docker.LogConfig{
				Type:   "syslog",
				Config: map[string]string{"a": "b"},
			},
		},
	}
	err := AddNewContainer("", &config)
	c.Assert(err, check.IsNil)
	conf := configFor(config.Name)
	var result1 NodeContainerConfig
	err = conf.Load("", &result1)
	c.Assert(err, check.IsNil)
	c.Assert(result1, check.DeepEquals, config)
	config2 := config
	config2.Config.Env = nil
	err = AddNewContainer("p1", &config2)
	c.Assert(err, check.IsNil)
	var result2 NodeContainerConfig
	err = conf.Load("", &result2)
	c.Assert(err, check.IsNil)
	c.Assert(result2, check.DeepEquals, config)
	var result3 NodeContainerConfig
	err = conf.Load("p1", &result3)
	c.Assert(err, check.IsNil)
	c.Assert(result3, check.DeepEquals, config2)
}

func (s *S) TestEnsureContainersStarted(c *check.C) {
	c1 := NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image: "bsimg",
			Env: []string{
				"A=1",
				"B=2",
			},
		},
		HostConfig: docker.HostConfig{
			RestartPolicy: docker.AlwaysRestart(),
			Privileged:    true,
			Binds:         []string{"/xyz:/abc:rw"},
		},
	}
	err := AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	c2 := c1
	c2.Name = "sysdig"
	c2.Config.Image = "sysdigimg"
	c2.Config.Env = []string{"X=Z"}
	err = AddNewContainer("", &c2)
	c.Assert(err, check.IsNil)
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	var createBodies []string
	var names []string
	var mut sync.Mutex
	server := p.Servers()[0]
	server.CustomHandler("/containers/create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mut.Lock()
		defer mut.Unlock()
		data, _ := ioutil.ReadAll(r.Body)
		createBodies = append(createBodies, string(data))
		names = append(names, r.URL.Query().Get("name"))
		r.Body = ioutil.NopCloser(bytes.NewBuffer(data))
		server.DefaultHandler().ServeHTTP(w, r)
	}))
	defer p.Destroy()
	buf := safe.NewBuffer(nil)
	err = EnsureContainersStarted(p, buf)
	c.Assert(err, check.IsNil)
	parts := strings.Split(buf.String(), "\n")
	c.Assert(parts, check.HasLen, 5)
	sort.Strings(parts)
	c.Assert(parts[1], check.Matches, `relaunching node container "bs" in the node http://127.0.0.1:\d+/ \[\]`)
	c.Assert(parts[2], check.Matches, `relaunching node container "bs" in the node http://localhost:\d+/ \[\]`)
	c.Assert(parts[3], check.Matches, `relaunching node container "sysdig" in the node http://127.0.0.1:\d+/ \[\]`)
	c.Assert(parts[4], check.Matches, `relaunching node container "sysdig" in the node http://localhost:\d+/ \[\]`)
	c.Assert(createBodies, check.HasLen, 2)
	c.Assert(names, check.HasLen, 2)
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{"bs", "sysdig"})
	sort.Strings(createBodies)
	result := make([]struct {
		docker.Config
		HostConfig docker.HostConfig
	}, 2)
	err = json.Unmarshal([]byte(createBodies[0]), &result[0])
	c.Assert(err, check.IsNil)
	err = json.Unmarshal([]byte(createBodies[1]), &result[1])
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, []struct {
		docker.Config
		HostConfig docker.HostConfig
	}{
		{
			Config: docker.Config{Env: []string{"DOCKER_ENDPOINT=" + server.URL(), "A=1", "B=2"}, Image: "bsimg"},
			HostConfig: docker.HostConfig{
				Binds:         []string{"/xyz:/abc:rw"},
				Privileged:    true,
				RestartPolicy: docker.RestartPolicy{Name: "always"},
				LogConfig:     docker.LogConfig{},
			},
		},
		{
			Config: docker.Config{Env: []string{"DOCKER_ENDPOINT=" + server.URL(), "X=Z"}, Image: "sysdigimg"},
			HostConfig: docker.HostConfig{
				Binds:         []string{"/xyz:/abc:rw"},
				Privileged:    true,
				RestartPolicy: docker.RestartPolicy{Name: "always"},
				LogConfig:     docker.LogConfig{},
			},
		},
	})
	conf := configFor("bs")
	var result1 NodeContainerConfig
	err = conf.Load("", &result1)
	c.Assert(err, check.IsNil)
	c.Assert(result1.PinnedImage, check.Equals, "bsimg")
}
