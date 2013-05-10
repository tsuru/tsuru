// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	etesting "github.com/globocom/tsuru/exec/testing"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	stdlog "log"
	"net"
	"runtime"
	"strings"
	"time"
)

func (s *S) TestShouldBeRegistered(c *gocheck.C) {
	p, err := provision.Get("docker")
	c.Assert(err, gocheck.IsNil)
	c.Assert(p, gocheck.FitsTypeOf, &DockerProvisioner{})
}

func (s *S) TestProvisionerProvision(c *gocheck.C) {
	app := testing.NewFakeApp("myapp", "python", 1)
	var p DockerProvisioner
	err := p.Provision(app)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestProvisionerRestartCallsDockerStopAndDockerStart(c *gocheck.C) {
	id := "caad7bbd5411"
	fexec := &etesting.FakeExecutor{Output: map[string][]byte{"*": []byte(id)}}
	execut = fexec
	defer func() {
		execut = nil
	}()
	var p DockerProvisioner
	app := testing.NewFakeApp("almah", "static", 1)
	cont := container{
		Id:      id,
		AppName: app.GetName(),
		Type:    app.GetPlatform(),
	}
	err := collection().Insert(cont)
	c.Assert(err, gocheck.IsNil)
	defer collection().RemoveId(cont.Id)
	err = p.Restart(app)
	c.Assert(err, gocheck.IsNil)
	args := []string{"stop", id}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
	args = []string{"start", id}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestDeployShouldCallDockerCreate(c *gocheck.C) {
	out := `{
	"NetworkSettings": {
		"IpAddress": "10.10.10.10",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"PortMapping": {}
	}
}`
	fexec := &etesting.FakeExecutor{Output: map[string][]byte{"*": []byte(out)}}
	execut = fexec
	defer func() {
		execut = nil
	}()
	p := DockerProvisioner{}
	app := testing.NewFakeApp("cribcaged", "python", 1)
	w := &bytes.Buffer{}
	err := p.Deploy(app, w)
	defer p.Destroy(app)
	defer s.conn.Collection(s.collName).RemoveId(out)
	c.Assert(err, gocheck.IsNil)
	image := fmt.Sprintf("%s/python", s.repoNamespace)
	appRepo := fmt.Sprintf("git://%s/cribcaged.git", s.gitHost)
	sshCmd := "/var/lib/tsuru/add-key key-content && /usr/sbin/sshd"
	containerCmd := fmt.Sprintf("/var/lib/tsuru/deploy %s && %s %s", appRepo, s.runBin, s.runArgs)
	args := []string{
		"run", "-d", "-t", "-p", s.port, image,
		"/bin/bash", "-c", fmt.Sprintf("%s && %s", sshCmd, containerCmd),
	}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestProvisionerDestroy(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	w := new(bytes.Buffer)
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	app := testing.NewFakeApp("myapp", "python", 1)
	cont := container{
		Id:      app.ProvisionUnits()[0].GetName(),
		AppName: app.GetName(),
	}
	err := s.conn.Collection(s.collName).Insert(cont)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(s.collName).RemoveId(cont.Id)
	img := image{Name: app.GetName()}
	err = s.conn.Collection(s.imageCollName).Insert(&img)
	c.Assert(err, gocheck.IsNil)
	var p DockerProvisioner
	c.Assert(p.Destroy(app), gocheck.IsNil)
	ok := make(chan bool, 1)
	go func() {
		coll := s.conn.Collection(s.collName)
		for {
			ct, err := coll.Find(bson.M{"_id": cont.Id}).Count()
			if err != nil {
				c.Fatal(err)
			}
			if ct == 0 {
				ok <- true
				return
			}
			runtime.Gosched()
		}
	}()
	select {
	case <-ok:
	case <-time.After(10e9):
		c.Fatal("Timed out waiting for the container to be destroyed (10 seconds)")
	}
	args := []string{"rm", "myapp/0"}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestProvisionerAddr(c *gocheck.C) {
	var p DockerProvisioner
	app := testing.NewFakeApp("myapp", "python", 1)
	addr, err := p.Addr(app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, app.ProvisionUnits()[0].GetIp())
}

func (s *S) TestProvisionerAddUnits(c *gocheck.C) {
	var p DockerProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	units, err := p.AddUnits(app, 2)
	c.Assert(err, gocheck.IsNil)
	c.Assert(units, gocheck.DeepEquals, []provision.Unit{})
}

func (s *S) TestProvisionerRemoveUnit(c *gocheck.C) {
	var p DockerProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	err := p.RemoveUnit(app, "")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestProvisionerExecuteCommand(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{
		Output: map[string][]byte{"*": []byte(". ..")},
	}
	execut = fexec
	defer func() {
		execut = nil
	}()
	app := testing.NewFakeApp("starbreaker", "python", 1)
	container := container{Id: "c-036", AppName: "starbreaker", Type: "python", Ip: "10.10.10.1"}
	err := s.conn.Collection(s.collName).Insert(container)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(s.collName).Remove(bson.M{"_id": container.Id})
	var stdout, stderr bytes.Buffer
	var p DockerProvisioner
	err = p.ExecuteCommand(&stdout, &stderr, app, "ls", "-ar")
	c.Assert(err, gocheck.IsNil)
	c.Assert(stderr.Bytes(), gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, ". ..")
	args := []string{
		"10.10.10.1", "-l", s.sshUser,
		"-o", "StrictHostKeyChecking no",
		"--", "ls", "-ar",
	}
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
}

func (s *S) TestProvisionerExecuteCommandMultipleContainers(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{
		Output: map[string][]byte{"*": []byte(". ..")},
	}
	execut = fexec
	defer func() {
		execut = nil
	}()
	app := testing.NewFakeApp("starbreaker", "python", 1)
	err := s.conn.Collection(s.collName).Insert(
		container{Id: "c-036", AppName: "starbreaker", Type: "python", Ip: "10.10.10.1"},
		container{Id: "c-037", AppName: "starbreaker", Type: "python", Ip: "10.10.10.2"},
	)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(s.collName).RemoveAll(bson.M{"_id": bson.M{"$in": []string{"c-036", "c-037"}}})
	var stdout, stderr bytes.Buffer
	var p DockerProvisioner
	err = p.ExecuteCommand(&stdout, &stderr, app, "ls", "-ar")
	c.Assert(err, gocheck.IsNil)
	c.Assert(stderr.Bytes(), gocheck.IsNil)
	args1 := []string{
		"10.10.10.1", "-l", s.sshUser,
		"-o", "StrictHostKeyChecking no",
		"--", "ls", "-ar",
	}
	args2 := []string{
		"10.10.10.1", "-l", s.sshUser,
		"-o", "StrictHostKeyChecking no",
		"--", "ls", "-ar",
	}
	c.Assert(fexec.ExecutedCmd("ssh", args1), gocheck.Equals, true)
	c.Assert(fexec.ExecutedCmd("ssh", args2), gocheck.Equals, true)
}

func (s *S) TestProvisionerExecuteCommandFailure(c *gocheck.C) {
	fexec := &etesting.ErrorExecutor{
		Output: map[string][]byte{"*": []byte("permission denied")},
	}
	execut = fexec
	defer func() {
		execut = nil
	}()
	app := testing.NewFakeApp("starbreaker", "python", 1)
	container := container{Id: "c-036", AppName: "starbreaker", Type: "python", Ip: "10.10.10.1"}
	err := s.conn.Collection(s.collName).Insert(container)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(s.collName).Remove(bson.M{"_id": container.Id})
	var stdout, stderr bytes.Buffer
	var p DockerProvisioner
	err = p.ExecuteCommand(&stdout, &stderr, app, "ls", "-ar")
	c.Assert(err, gocheck.NotNil)
	c.Assert(stdout.Bytes(), gocheck.IsNil)
	c.Assert(stderr.String(), gocheck.Equals, "permission denied")
}

func (s *S) TestProvisionerExecuteCommandNoContainers(c *gocheck.C) {
	var p DockerProvisioner
	app := testing.NewFakeApp("almah", "static", 2)
	var buf bytes.Buffer
	err := p.ExecuteCommand(&buf, &buf, app, "ls", "-lh")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "No containers for this app")
}

func (s *S) TestCollectStatus(c *gocheck.C) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, gocheck.IsNil)
	defer listener.Close()
	err = collection().Insert(
		container{Id: "9930c24f1c5f", AppName: "ashamed", Type: "python", Port: strings.Split(listener.Addr().String(), ":")[1]},
		container{Id: "9930c24f1c4f", AppName: "make-up", Type: "python", Port: "8889"},
	)
	c.Assert(err, gocheck.IsNil)
	defer collection().RemoveAll(bson.M{"_id": bson.M{"$in": []string{"9930c24f1c5f", "9930c24f1c4f"}}})
	psOutput := `9930c24f1c5f
9930c24f1c4f
9930c24f1c3f
`
	c1Output := `{
	"NetworkSettings": {
		"IpAddress": "127.0.0.1",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"PortMapping": {
			"8888": "90293"
		}
	}
}`
	c2Output := `{
	"NetworkSettings": {
		"IpAddress": "127.0.0.1",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"PortMapping": {
			"8889": "90294"
		}
	}
}`
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
			Status:  provision.StatusInstalling,
		},
	}
	output := map[string][]byte{
		"ps -q":                []byte(psOutput),
		"inspect 9930c24f1c5f": []byte(c1Output),
		"inspect 9930c24f1c4f": []byte(c2Output),
	}
	fexec := &etesting.FakeExecutor{Output: output}
	execut = fexec
	defer func() {
		execut = nil
	}()
	var p DockerProvisioner
	units, err := p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	if units[0].Name != expected[0].Name {
		units[0], units[1] = units[1], units[0]
	}
	c.Assert(units, gocheck.DeepEquals, expected)
}

func (s *S) TestProvisionCollection(c *gocheck.C) {
	collection := collection()
	c.Assert(collection.Name, gocheck.Equals, s.collName)
}
