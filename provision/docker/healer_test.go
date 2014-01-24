// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/globocom/tsuru/heal"
	"launchpad.net/gocheck"
)

type HealerSuite struct {
	healer  *ContainerHealer
	calls   int
	cleanup func()
}

var _ = gocheck.Suite(&HealerSuite{})

func (s *HealerSuite) SetUpSuite(c *gocheck.C) {
	s.healer = &ContainerHealer{}
	createFakeContainers([]string{"8dfafdbc3a40", "dca19cd9bb9e", "3fd99cd9bb84"}, c)
	s.cleanup, _ = startDockerTestServer("4567", &s.calls)
}

func (s *HealerSuite) TearDownTest(c *gocheck.C) {
	s.calls = 0
}

func (s *HealerSuite) TearDownSuite(c *gocheck.C) {
	defer s.cleanup()
	clearSchedStorage(c)
}

func (s *HealerSuite) TestContainerHealerShouldBeRegistered(c *gocheck.C) {
	h, err := heal.Get("docker", "container")
	c.Assert(err, gocheck.IsNil)
	c.Assert(h, gocheck.FitsTypeOf, ContainerHealer{})
}

func (s *HealerSuite) TestContainerHealerImplementsHealInterface(c *gocheck.C) {
	var h interface{}
	h = &ContainerHealer{}
	_, ok := h.(heal.Healer)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *HealerSuite) TestContainerHealPerformListContainersKillAndStartOnUnhealthyContainers(c *gocheck.C) {
	err := s.healer.Heal()
	c.Assert(err, gocheck.IsNil)
	c.Assert(s.calls, gocheck.Equals, 3)
}

func (s *HealerSuite) TestCollectContainersCallsDockerApi(c *gocheck.C) {
	_, err := s.healer.collectContainers()
	c.Assert(err, gocheck.IsNil)
	c.Assert(s.calls, gocheck.Equals, 1)
}

func (s *HealerSuite) TestCollectContainerReturnsCollectedContainers(c *gocheck.C) {
	containers, err := s.healer.collectContainers()
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(containers), gocheck.Equals, 3)
	expected := []container{
		{ID: "8dfafdbc3a40", Image: "base:latest", Status: "Ghost"},
		{ID: "dca19cd9bb9e", Image: "tsuru/python:latest", Status: "Exit 0"},
		{ID: "3fd99cd9bb84", Image: "tsuru/python:latest", Status: "Up 7 seconds"},
	}
	c.Assert(containers, gocheck.DeepEquals, expected)
}

func (s *HealerSuite) TestIsHealthyReturnsTrueWhenContainerIsUp(c *gocheck.C) {
	container := container{ID: "3fd99cd9bb84", Image: "tsuru/python:latest", Status: "Up 7 seconds"}
	isHealthy := s.healer.isHealthy(&container)
	c.Assert(isHealthy, gocheck.Equals, true)
}

func (s *HealerSuite) TestIsHealthyReturnsFalseWhenContainerIsGhost(c *gocheck.C) {
	container := container{ID: "8dfafdbc3a40", Image: "base:latest", Status: "Ghost"}
	isHealthy := s.healer.isHealthy(&container)
	c.Assert(isHealthy, gocheck.Equals, false)
}

func (s *HealerSuite) TestIsHealthyReturnsFalseWhenContainerHasExitedWithStatusNotZero(c *gocheck.C) {
	container := container{ID: "dca19cd9bb9e", Image: "tsuru/python:latest", Status: "Exit 127"}
	isHealthy := s.healer.isHealthy(&container)
	c.Assert(isHealthy, gocheck.Equals, false)
}

func (s *HealerSuite) TestIsHealthyReturnsTrueWhenContainerHasExitedWithStatusZero(c *gocheck.C) {
	container := container{ID: "dca19cd9bb9e", Image: "tsuru/python:latest", Status: "Exit 0"}
	isHealthy := s.healer.isHealthy(&container)
	c.Assert(isHealthy, gocheck.Equals, true)
}

func (s *HealerSuite) TestIsRunningReturnsFalseForExitedContainers(c *gocheck.C) {
	container := container{ID: "dca19cd9bb9e", Image: "tsuru/python:latest", Status: "Exit 0"}
	running := s.healer.isRunning(&container)
	c.Assert(running, gocheck.Equals, false)
}

func (s *HealerSuite) TestIsRunningReturnsTrueForGhostContainers(c *gocheck.C) {
	container := container{ID: "8dfafdbc3a40", Image: "base:latest", Status: "Ghost"}
	running := s.healer.isRunning(&container)
	c.Assert(running, gocheck.Equals, true)
}

func (s *HealerSuite) TestUnhealthyRunningContainers(c *gocheck.C) {
	containers := []container{
		{ID: "8dfafdbc3a40", Image: "base:latest", Status: "Ghost"},
		{ID: "dca19cd9bb9e", Image: "tsuru/python:latest", Status: "Exit 0"},
		{ID: "3fd99cd9bb84", Image: "tsuru/python:latest", Status: "Exit 127"},
		{ID: "3fd99cd9bb84", Image: "tsuru/python:latest", Status: "Up 7 seconds"},
	}
	expected := []container{
		{ID: "8dfafdbc3a40", Image: "base:latest", Status: "Ghost"},
	}
	unhealthy := s.healer.unhealthyRunningContainers(containers)
	c.Assert(unhealthy, gocheck.DeepEquals, expected)
}
