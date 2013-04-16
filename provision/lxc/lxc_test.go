// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lxc

import (
	"github.com/globocom/commandmocker"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/fs/testing"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net"
	"os"
)

func (s *S) TestLXCCreate(c *gocheck.C) {
	config.Set("lxc:authorized-key-path", "somepath")
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	container := container{name: "container"}
	err = container.create()
	c.Assert(err, gocheck.IsNil)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	expected := "lxc-create -t ubuntu-cloud -n container -- -S somepath"
	c.Assert(commandmocker.Output(tmpdir), gocheck.Equals, expected)
}

func (s *S) TestLXCStart(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	container := container{name: "container"}
	err = container.start()
	c.Assert(err, gocheck.IsNil)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	expected := "lxc-start --daemon -n container"
	c.Assert(commandmocker.Output(tmpdir), gocheck.Equals, expected)
}

func (s *S) TestLXCStop(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	container := container{name: "container"}
	err = container.stop()
	c.Assert(err, gocheck.IsNil)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	expected := "lxc-stop -n container"
	c.Assert(commandmocker.Output(tmpdir), gocheck.Equals, expected)
}

func (s *S) TestLXCDestroy(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	container := container{name: "container"}
	err = container.destroy()
	c.Assert(err, gocheck.IsNil)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	expected := "lxc-destroy -n container"
	c.Assert(commandmocker.Output(tmpdir), gocheck.Equals, expected)
}

func (s *S) TestContainerIP(c *gocheck.C) {
	config.Set("lxc:ip-timeout", 10)
	file, _ := os.Open("testdata/dnsmasq.leases")
	data, err := ioutil.ReadAll(file)
	c.Assert(err, gocheck.IsNil)
	rfs := &testing.RecordingFs{FileContent: string(data)}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	f, _ := rfs.Open("/var/lib/misc/dnsmasq.leases")
	f.Write(data)
	f.Close()
	cont := container{name: "vm1"}
	c.Assert(cont.Ip(), gocheck.Equals, "10.10.10.10")
	cont = container{name: "notfound"}
	c.Assert(cont.Ip(), gocheck.Equals, "")
}

func (s *S) TestWaitForNetwork(c *gocheck.C) {
	ln, err := net.Listen("tcp", "127.0.0.1:2222")
	c.Assert(err, gocheck.IsNil)
	defer ln.Close()
	config.Set("lxc:ip-timeout", 5)
	config.Set("lxc:ssh-port", 2222)
	cont := container{name: "vm", ip: "127.0.0.1"}
	err = cont.waitForNetwork()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestWaitForNetworkTimeout(c *gocheck.C) {
	config.Set("lxc:ip-timeout", 1)
	config.Set("lxc:ssh-port", 2222)
	cont := container{name: "vm", ip: "localhost"}
	err := cont.waitForNetwork()
	c.Assert(err, gocheck.NotNil)
}
