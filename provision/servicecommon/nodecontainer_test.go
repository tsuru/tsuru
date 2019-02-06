// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicecommon

import (
	"bytes"
	"errors"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	check "gopkg.in/check.v1"
)

type ncCall struct {
	conf          *nodecontainer.NodeContainerConfig
	pool          string
	filter        PoolFilter
	placementOnly bool
}

type ncManager struct {
	calls []ncCall
	err   map[string]error
}

func (m *ncManager) reset() {
	m.calls = nil
	m.err = nil
}

func (m *ncManager) DeployNodeContainer(conf *nodecontainer.NodeContainerConfig, pool string, filter PoolFilter, placementOnly bool) error {
	m.calls = append(m.calls, ncCall{
		conf:          conf,
		pool:          pool,
		filter:        filter,
		placementOnly: placementOnly,
	})
	if m.err != nil {
		return m.err[pool]
	}
	return nil
}

func (s *S) TestUpgradeNodeContainerSingleConfig(c *check.C) {
	m := ncManager{}
	buf := &bytes.Buffer{}
	c1 := nodecontainer.NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image: "bsimg",
		},
	}
	err := nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	err = UpgradeNodeContainer(&m, "bs", "", buf)
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []ncCall{
		{conf: &c1, pool: "", filter: PoolFilter{}, placementOnly: false},
	})
	c.Assert(buf.String(), check.Matches, `(?s).*upserting node container "bs" \[""\].*`)
}

func (s *S) TestUpgradeNodeContainerMultiple(c *check.C) {
	m := ncManager{}
	buf := &bytes.Buffer{}
	c1 := nodecontainer.NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image: "bsimg",
		},
	}
	err := nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	c2 := c1
	c2.Config.Env = []string{"e1=v1"}
	err = nodecontainer.AddNewContainer("p1", &c2)
	c.Assert(err, check.IsNil)
	c3 := c1
	err = nodecontainer.AddNewContainer("p2", &c3)
	c.Assert(err, check.IsNil)
	err = UpgradeNodeContainer(&m, "bs", "", buf)
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []ncCall{
		{conf: &c1, pool: "", filter: PoolFilter{Exclude: []string{"p1", "p2"}}, placementOnly: false},
		{conf: &c2, pool: "p1", filter: PoolFilter{Include: []string{"p1"}}, placementOnly: false},
		{conf: &c3, pool: "p2", filter: PoolFilter{Include: []string{"p2"}}, placementOnly: false},
	})
	c.Assert(buf.String(), check.Matches, `(?s).*upserting node container "bs" \[""\].*upserting node container "bs" \["p1"\].*upserting node container "bs" \["p2"\].*`)
	buf.Reset()
	m.reset()
	err = UpgradeNodeContainer(&m, "bs", "p2", buf)
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []ncCall{
		{conf: &c1, pool: "", filter: PoolFilter{Exclude: []string{"p1", "p2"}}, placementOnly: true},
		{conf: &c3, pool: "p2", filter: PoolFilter{Include: []string{"p2"}}, placementOnly: false},
	})
	c.Assert(buf.String(), check.Matches, `(?s).*upserting node container "bs" \[""\].*upserting node container "bs" \["p2"\].*`)
}

func (s *S) TestUpgradeNodeContainerMultipleWithoutDefault(c *check.C) {
	m := ncManager{}
	buf := &bytes.Buffer{}
	c1 := nodecontainer.NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image: "bsimg",
		},
	}
	err := nodecontainer.AddNewContainer("p1", &c1)
	c.Assert(err, check.IsNil)
	c2 := c1
	c2.Config.Env = []string{"e1=v1"}
	err = nodecontainer.AddNewContainer("p2", &c2)
	c.Assert(err, check.IsNil)
	err = UpgradeNodeContainer(&m, "bs", "", buf)
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []ncCall{
		{conf: &c1, pool: "p1", filter: PoolFilter{Include: []string{"p1"}}, placementOnly: false},
		{conf: &c2, pool: "p2", filter: PoolFilter{Include: []string{"p2"}}, placementOnly: false},
	})
	c.Assert(buf.String(), check.Matches, `(?s).*upserting node container "bs" \["p1"\].*upserting node container "bs" \["p2"\].*`)
	buf.Reset()
	m.reset()
	err = UpgradeNodeContainer(&m, "bs", "p2", buf)
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []ncCall{
		{conf: &c2, pool: "p2", filter: PoolFilter{Include: []string{"p2"}}, placementOnly: false},
	})
	c.Assert(buf.String(), check.Matches, `(?s).*skipping node container "bs" \[""\], invalid config.*upserting node container "bs" \["p2"\].*`)
}

func (s *S) TestUpgradeNodeContainerErrorMiddle(c *check.C) {
	m := ncManager{
		err: map[string]error{
			"p1": errors.New("myerr"),
		},
	}
	buf := &bytes.Buffer{}
	c1 := nodecontainer.NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image: "bsimg",
		},
	}
	err := nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	c2 := c1
	c2.Config.Env = []string{"e1=v1"}
	err = nodecontainer.AddNewContainer("p1", &c2)
	c.Assert(err, check.IsNil)
	c3 := c1
	err = nodecontainer.AddNewContainer("p2", &c3)
	c.Assert(err, check.IsNil)
	err = UpgradeNodeContainer(&m, "bs", "", buf)
	c.Assert(err, check.ErrorMatches, `.*myerr.*`)
	c.Assert(m.calls, check.DeepEquals, []ncCall{
		{conf: &c1, pool: "", filter: PoolFilter{Exclude: []string{"p1", "p2"}}, placementOnly: false},
		{conf: &c2, pool: "p1", filter: PoolFilter{Include: []string{"p1"}}, placementOnly: false},
		{conf: &c3, pool: "p2", filter: PoolFilter{Include: []string{"p2"}}, placementOnly: false},
	})
	c.Assert(buf.String(), check.Matches, `(?s).*upserting node container "bs" \[""\].*upserting node container "bs" \["p1"\].*upserting node container "bs" \["p2"\].*`)
}

func (s *S) TestEnsureNodeContainersCreated(c *check.C) {
	m := ncManager{}
	buf := &bytes.Buffer{}
	c1 := nodecontainer.NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image: "bsimg",
		},
	}
	err := nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	c2 := c1
	c2.Config.Env = []string{"e1=v1"}
	err = nodecontainer.AddNewContainer("p1", &c2)
	c.Assert(err, check.IsNil)
	c3 := nodecontainer.NodeContainerConfig{
		Name: "other",
		Config: docker.Config{
			Image: "otherimg",
		},
	}
	err = nodecontainer.AddNewContainer("", &c3)
	c.Assert(err, check.IsNil)
	err = EnsureNodeContainersCreated(&m, buf)
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []ncCall{
		{conf: &c1, pool: "", filter: PoolFilter{Exclude: []string{"p1"}}, placementOnly: true},
		{conf: &c2, pool: "p1", filter: PoolFilter{Include: []string{"p1"}}, placementOnly: true},
		{conf: &c3, pool: "", filter: PoolFilter{}, placementOnly: true},
	})
	c.Assert(buf.String(), check.Matches, `(?s).*upserting node container "bs" \[""\].*upserting node container "bs" \["p1"\].*upserting node container "other" \[""\].*`)
}
