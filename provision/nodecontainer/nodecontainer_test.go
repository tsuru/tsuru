// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodecontainer

import (
	"sort"

	docker "github.com/fsouza/go-dockerclient"
	check "gopkg.in/check.v1"
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
				Config: map[string]string{"a": "b", "c": "d"},
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
	config2 := NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Env: []string{"C=3"},
		},
		HostConfig: docker.HostConfig{
			LogConfig: docker.LogConfig{
				Config: map[string]string{"a": "", "e": "f"},
			},
		},
	}
	err = AddNewContainer("p1", &config2)
	c.Assert(err, check.IsNil)
	var result2 NodeContainerConfig
	err = conf.Load("", &result2)
	c.Assert(err, check.IsNil)
	c.Assert(result2, check.DeepEquals, config)
	var result3 NodeContainerConfig
	err = conf.Load("p1", &result3)
	c.Assert(err, check.IsNil)
	expected2 := config
	expected2.Config.Env = []string{"A=1", "B=2", "C=3"}
	expected2.HostConfig.LogConfig.Config = map[string]string{"a": "", "c": "d", "e": "f"}
	c.Assert(result3, check.DeepEquals, expected2)
}

func (s *S) TestAddUpdateContainerDisabled(c *check.C) {
	boolPtr := func(a bool) *bool { return &a }
	err := AddNewContainer("", &NodeContainerConfig{Name: "x", Disabled: boolPtr(true), Config: docker.Config{Image: "img1"}})
	c.Assert(err, check.IsNil)
	err = AddNewContainer("p1", &NodeContainerConfig{Name: "x", Disabled: boolPtr(false)})
	c.Assert(err, check.IsNil)
	err = AddNewContainer("p2", &NodeContainerConfig{Name: "x"})
	c.Assert(err, check.IsNil)
	err = AddNewContainer("", &NodeContainerConfig{Name: "y", Config: docker.Config{Image: "img1"}})
	c.Assert(err, check.IsNil)
	err = AddNewContainer("p1", &NodeContainerConfig{Name: "y", Disabled: boolPtr(true), Config: docker.Config{Image: "img1"}})
	c.Assert(err, check.IsNil)
	err = UpdateContainer("p1", &NodeContainerConfig{Name: "y", Config: docker.Config{Image: "img1"}})
	c.Assert(err, check.IsNil)
	tests := []struct {
		name, pool    string
		expectedValid bool
	}{
		{"x", "", false},
		{"x", "p1", true},
		{"x", "p2", false},
		{"x", "px", false},
		{"y", "", true},
		{"y", "px", true},
		{"y", "p1", false},
	}
	for _, tt := range tests {
		conf := configFor(tt.name)
		var result NodeContainerConfig
		err = conf.Load(tt.pool, &result)
		c.Assert(err, check.IsNil)
		c.Assert(result.Valid(), check.Equals, tt.expectedValid)
	}
	err = UpdateContainer("p1", &NodeContainerConfig{Name: "y", Disabled: boolPtr(false), Config: docker.Config{Image: "img1"}})
	c.Assert(err, check.IsNil)
	conf := configFor("y")
	result := NodeContainerConfig{}
	err = conf.Load("p1", &result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Valid(), check.Equals, true)
}

func (s *S) TestAddNewContainerInvalid(c *check.C) {
	err := AddNewContainer("", &NodeContainerConfig{})
	c.Assert(err, check.ErrorMatches, "node container config name cannot be empty")
	err = AddNewContainer("", &NodeContainerConfig{Name: "x", Config: docker.Config{Image: ""}})
	c.Assert(err, check.ErrorMatches, "node container config image cannot be empty")
	err = AddNewContainer("", &NodeContainerConfig{Name: "x", Config: docker.Config{Image: "img1"}})
	c.Assert(err, check.IsNil)
	err = AddNewContainer("p1", &NodeContainerConfig{Name: "y", Config: docker.Config{Image: ""}})
	c.Assert(err, check.ErrorMatches, "node container config image cannot be empty")
	err = AddNewContainer("p1", &NodeContainerConfig{Name: "x", Config: docker.Config{Image: ""}})
	c.Assert(err, check.IsNil)
	err = AddNewContainer("p1", &NodeContainerConfig{Name: "y", Config: docker.Config{Image: "img2"}})
	c.Assert(err, check.IsNil)
	err = AddNewContainer("p1", &NodeContainerConfig{Name: "y", Config: docker.Config{Image: "img3"}})
	c.Assert(err, check.IsNil)
	err = AddNewContainer("", &NodeContainerConfig{Name: "x", Config: docker.Config{Image: ""}})
	c.Assert(err, check.ErrorMatches, "node container config image cannot be empty")
}

func (s *S) TestUpdateContainer(c *check.C) {
	err := AddNewContainer("", &NodeContainerConfig{Name: "x", Config: docker.Config{Image: "img1"}})
	c.Assert(err, check.IsNil)
	err = UpdateContainer("", &NodeContainerConfig{Name: "x", HostConfig: docker.HostConfig{Privileged: true}})
	c.Assert(err, check.IsNil)
	entry, err := LoadNodeContainer("", "x")
	c.Assert(err, check.IsNil)
	c.Assert(entry, check.DeepEquals, &NodeContainerConfig{
		Name:       "x",
		Config:     docker.Config{Image: "img1"},
		HostConfig: docker.HostConfig{Privileged: true}},
	)
}

func (s *S) TestUpdateContainerMergeEnvs(c *check.C) {
	err := AddNewContainer("", &NodeContainerConfig{Name: "x", Config: docker.Config{
		Image: "img1",
		Env:   []string{"A=1", "B=2"},
	}})
	c.Assert(err, check.IsNil)
	err = UpdateContainer("", &NodeContainerConfig{Name: "x", Config: docker.Config{
		Env: []string{"B=3", "C=4"},
	}})
	c.Assert(err, check.IsNil)
	entry, err := LoadNodeContainer("", "x")
	c.Assert(err, check.IsNil)
	sort.Strings(entry.Config.Env)
	c.Assert(entry.Config.Env, check.DeepEquals, []string{"A=1", "B=3", "C=4"})
}

func (s *S) TestUpdateContainerInvalid(c *check.C) {
	err := UpdateContainer("", &NodeContainerConfig{})
	c.Assert(err, check.ErrorMatches, "node container config name cannot be empty")
	err = UpdateContainer("", &NodeContainerConfig{Name: "x"})
	c.Assert(err, check.Equals, ErrNodeContainerNotFound)
	err = UpdateContainer("", &NodeContainerConfig{Name: "x"})
	c.Assert(err, check.Equals, ErrNodeContainerNotFound)
	err = AddNewContainer("", &NodeContainerConfig{Name: "x", Config: docker.Config{Image: "img1"}})
	c.Assert(err, check.IsNil)
	err = UpdateContainer("p1", &NodeContainerConfig{Name: "x"})
	c.Assert(err, check.Equals, ErrNodeContainerNotFound)
	err = UpdateContainer("", &NodeContainerConfig{Name: "x"})
	c.Assert(err, check.IsNil)
	err = UpdateContainer("p1", &NodeContainerConfig{Name: "x"})
	c.Assert(err, check.Equals, ErrNodeContainerNotFound)
	err = AddNewContainer("p1", &NodeContainerConfig{Name: "x"})
	c.Assert(err, check.IsNil)
	err = UpdateContainer("p1", &NodeContainerConfig{Name: "x"})
	c.Assert(err, check.IsNil)
}

func (s *S) TestUpgradeContainerCreatesEmptyConfigEntry(c *check.C) {
	err := AddNewContainer("", &NodeContainerConfig{Name: "x", Config: docker.Config{
		Image: "img1",
	}})
	c.Assert(err, check.IsNil)
	err = UpgradeContainer("p1", "x")
	c.Assert(err, check.IsNil)
	pools, err := LoadNodeContainersForPools("x")
	c.Assert(err, check.IsNil)
	c.Assert(pools, check.DeepEquals, map[string]NodeContainerConfig{
		"": {Name: "x", Config: docker.Config{
			Image: "img1",
		}},
		"p1": {Name: "x"},
	})
}

func (s *S) TestUpgradeContainerNotFound(c *check.C) {
	err := AddNewContainer("pool", &NodeContainerConfig{Name: "x", Config: docker.Config{
		Image: "img1",
	}})
	c.Assert(err, check.IsNil)
	err = UpgradeContainer("", "bs")
	c.Assert(err, check.DeepEquals, ErrNodeContainerNotFound)
	err = UpgradeContainer("otherpool", "x")
	c.Assert(err, check.DeepEquals, ErrNodeContainerNotFound)
}

func (s *S) TestLoadNodeContainersForPools(c *check.C) {
	err := AddNewContainer("p1", &NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Image: "myregistry/tsuru/bs",
		},
	})
	c.Assert(err, check.IsNil)
	result, err := LoadNodeContainersForPools("c1")
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, map[string]NodeContainerConfig{
		"p1": {
			Name: "c1",
			Config: docker.Config{
				Image: "myregistry/tsuru/bs",
			},
		},
	})
}

func (s *S) TestLoadNodeContainersForPoolsNotFound(c *check.C) {
	_, err := LoadNodeContainersForPools("notfound")
	c.Assert(err, check.Equals, ErrNodeContainerNotFound)
}

func (s *S) TestResetImage(c *check.C) {
	err := AddNewContainer("", &NodeContainerConfig{
		Name:        "c1",
		PinnedImage: "img1@1",
		Config: docker.Config{
			Image: "img1",
		},
	})
	c.Assert(err, check.IsNil)
	err = AddNewContainer("p1", &NodeContainerConfig{
		Name:        "c1",
		PinnedImage: "img1@2",
	})
	c.Assert(err, check.IsNil)
	err = AddNewContainer("p2", &NodeContainerConfig{
		Name: "c1",
	})
	c.Assert(err, check.IsNil)
	err = resetImage("p1", "c1")
	c.Assert(err, check.IsNil)
	result, err := LoadNodeContainersForPools("c1")
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, map[string]NodeContainerConfig{
		"": {
			Name:        "c1",
			PinnedImage: "img1@1",
			Config: docker.Config{
				Image: "img1",
			},
		},
		"p1": {Name: "c1"},
		"p2": {Name: "c1"},
	})
	err = resetImage("p2", "c1")
	c.Assert(err, check.IsNil)
	result, err = LoadNodeContainersForPools("c1")
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, map[string]NodeContainerConfig{
		"": {
			Name: "c1",
			Config: docker.Config{
				Image: "img1",
			},
		},
		"p1": {Name: "c1"},
		"p2": {Name: "c1"},
	})
	err = UpdateContainer("p1", &NodeContainerConfig{Name: "c1", PinnedImage: "img1@1"})
	c.Assert(err, check.IsNil)
	err = resetImage("", "c1")
	c.Assert(err, check.IsNil)
	result, err = LoadNodeContainersForPools("c1")
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, map[string]NodeContainerConfig{
		"": {
			Name: "c1",
			Config: docker.Config{
				Image: "img1",
			},
		},
		"p1": {Name: "c1"},
		"p2": {Name: "c1"},
	})

}
