// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	etesting "github.com/globocom/tsuru/exec/testing"
	"github.com/globocom/tsuru/provision"
	rtesting "github.com/globocom/tsuru/router/testing"
	"launchpad.net/gocheck"
	"strings"
	"sync/atomic"
)

func (s *S) TestCollectStatus(c *gocheck.C) {
	defer createTestRoutes("ashamed", "make-up")()
	listener := startTestListener("127.0.0.1:0")
	defer listener.Close()
	listenPort := strings.Split(listener.Addr().String(), ":")[1]
	var calls int64
	var err error
	cleanup, _ := startDockerTestServer(listenPort, &calls)
	defer cleanup()
	sshHandler, cleanup := startSSHAgentServer("")
	defer cleanup()
	defer insertContainers(listenPort, c)()
	expected := []provision.Unit{
		{
			Name:    "9930c24f1c5f",
			AppName: "ashamed",
			Type:    "python",
			Machine: 0,
			Ip:      "127.0.0.1",
			Status:  provision.StatusStarted,
		},
		{
			Name:    "9930c24f1c4f",
			AppName: "make-up",
			Type:    "python",
			Machine: 0,
			Ip:      "127.0.0.1",
			Status:  provision.StatusUnreachable,
		},
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
	cont, err := getContainer("9930c24f1c4f")
	c.Assert(err, gocheck.IsNil)
	c.Assert(cont.IP, gocheck.Equals, "127.0.0.1")
	c.Assert(cont.HostPort, gocheck.Equals, "9024")
	if sshHandler.requests[0].URL.Path == "/container/127.0.0.4" {
		sshHandler.requests[0], sshHandler.requests[1] = sshHandler.requests[1], sshHandler.requests[0]
	}
	c.Assert(sshHandler.requests[0].URL.Path, gocheck.Equals, "/container/127.0.0.3")
	c.Assert(sshHandler.requests[0].Method, gocheck.Equals, "DELETE")
	c.Assert(sshHandler.requests[1].URL.Path, gocheck.Equals, "/container/127.0.0.4")
	c.Assert(sshHandler.requests[1].Method, gocheck.Equals, "DELETE")
	c.Assert(rtesting.FakeRouter.HasRoute("make-up", "http://127.0.0.1:9025"), gocheck.Equals, false)
	c.Assert(rtesting.FakeRouter.HasRoute("make-up", "http://127.0.0.1:9024"), gocheck.Equals, true)
	c.Assert(atomic.LoadInt64(&calls), gocheck.Equals, int64(2))
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
