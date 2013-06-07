// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	"github.com/globocom/commandmocker"
	"github.com/globocom/config"
	etesting "github.com/globocom/tsuru/exec/testing"
	ftesting "github.com/globocom/tsuru/fs/testing"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/repository"
	rtesting "github.com/globocom/tsuru/router/testing"
	"github.com/globocom/tsuru/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	stdlog "log"
	"os"
)

func (s *S) TestContainerGetAddress(c *gocheck.C) {
	container := container{ID: "id123", Port: "8888", HostPort: "49153"}
	address := container.getAddress()
	expected := fmt.Sprintf("http://%s:49153", s.hostAddr)
	c.Assert(address, gocheck.Equals, expected)
}

func (s *S) TestNewContainer(c *gocheck.C) {
	inspectOut := `
    {
            "NetworkSettings": {
            "IpAddress": "10.10.10.10",
            "IpPrefixLen": 8,
            "Gateway": "10.65.41.1",
	    "PortMapping": {"8888": "34233"}
    }
}`
	id := "945132e7b4c9"
	sshCmd := "/var/lib/tsuru/add-key key-content && /usr/sbin/sshd -D"
	runCmd := fmt.Sprintf("run -d -t -p %s tsuru/python /bin/bash -c %s",
		s.port, sshCmd)
	inspectCmd := fmt.Sprintf("inspect %s", id)
	out := map[string][][]byte{runCmd: {[]byte(id)}, inspectCmd: {[]byte(inspectOut)}}
	fexec := &etesting.FakeExecutor{Output: out}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("app-name", "python", 1)
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	_, err := newContainer(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(s.collName).RemoveId(id)
	var cont container
	err = s.conn.Collection(s.collName).FindId(id).One(&cont)
	c.Assert(err, gocheck.IsNil)
	c.Assert(cont.ID, gocheck.Equals, id)
	c.Assert(cont.AppName, gocheck.Equals, "app-name")
	c.Assert(cont.IP, gocheck.Equals, "10.10.10.10")
	c.Assert(cont.HostPort, gocheck.Equals, "34233")
	c.Assert(cont.Port, gocheck.Equals, s.port)
}

func (s *S) TestNewContainerCallsDockerCreate(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("app-name", "python", 1)
	newContainer(app)
	defer s.conn.Collection(s.collName).Remove(bson.M{"appname": app.GetName()})
	sshCmd := "/var/lib/tsuru/add-key key-content && /usr/sbin/sshd -D"
	args := []string{
		"run", "-d", "-t", "-p", s.port, "tsuru/python",
		"/bin/bash", "-c", sshCmd,
	}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestNewContainerReturnsNilAndLogsOnError(c *gocheck.C) {
	w := new(bytes.Buffer)
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	tmpdir, err := commandmocker.Error("docker", "cool error", 1)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	app := testing.NewFakeApp("myapp", "python", 1)
	container, err := newContainer(app)
	c.Assert(err, gocheck.NotNil)
	defer s.conn.Collection(s.collName).Remove(bson.M{"appname": app.GetName()})
	c.Assert(container, gocheck.IsNil)
	c.Assert(w.String(), gocheck.Matches, `(?s).*Error creating container for the app "myapp".*`)
}

func (s *S) TestNewContainerAddsRoute(c *gocheck.C) {
	out := fmt.Sprintf(`{
	"NetworkSettings": {
		"IpAddress": "10.10.10.10",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"PortMapping": {
			"%s": "30000"
		}
	}
}`, s.port)
	fexec := &etesting.FakeExecutor{Output: map[string][][]byte{"*": {[]byte(out)}}}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("myapp", "python", 1)
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	container, err := newContainer(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(s.collName).RemoveId(out)
	c.Assert(rtesting.FakeRouter.HasRoute(app.GetName(), container.getAddress()), gocheck.Equals, true)
}

func (s *S) TestCommandsToRun(c *gocheck.C) {
	rfs := ftesting.RecordingFs{}
	f, err := rfs.Create("/opt/me/id_dsa.pub")
	c.Assert(err, gocheck.IsNil)
	f.Write([]byte("ssh-rsa ohwait! me@machine\n"))
	f.Close()
	old := fsystem
	fsystem = &rfs
	defer func() {
		fsystem = old
	}()
	config.Set("docker:ssh:sshd-path", "/opt/bin/sshd")
	config.Set("docker:ssh:public-key", "/opt/me/id_dsa.pub")
	config.Set("docker:ssh:private-key", "/opt/me/id_dsa")
	defer config.Unset("docker:ssh:sshd-path")
	defer config.Unset("docker:ssh:public-key")
	defer config.Unset("docker:ssh:private-key")
	app := testing.NewFakeApp("myapp", "python", 1)
	cmd, err := commandToRun(app)
	c.Assert(err, gocheck.IsNil)
	sshCmd := "/var/lib/tsuru/add-key ssh-rsa ohwait! me@machine && /opt/bin/sshd -D"
	expected := []string{
		"docker", "run", "-d", "-t", "-p", s.port, fmt.Sprintf("%s/python", s.repoNamespace),
		"/bin/bash", "-c", sshCmd,
	}
	c.Assert(cmd, gocheck.DeepEquals, expected)
}

func (s *S) TestGetSSHCommandsDefaultSSHDPath(c *gocheck.C) {
	rfs := ftesting.RecordingFs{}
	f, err := rfs.Create("/opt/me/id_dsa.pub")
	c.Assert(err, gocheck.IsNil)
	f.Write([]byte("ssh-rsa ohwait! me@machine"))
	f.Close()
	old := fsystem
	fsystem = &rfs
	defer func() {
		fsystem = old
	}()
	config.Set("docker:ssh:public-key", "/opt/me/id_dsa.pub")
	defer config.Unset("docker:ssh:public-key")
	commands, err := getSSHCommands()
	c.Assert(err, gocheck.IsNil)
	c.Assert(commands[1], gocheck.Equals, "/usr/sbin/sshd -D")
}

func (s *S) TestGetSSHCommandsDefaultKeyFile(c *gocheck.C) {
	rfs := ftesting.RecordingFs{}
	f, err := rfs.Create(os.ExpandEnv("${HOME}/.ssh/id_rsa.pub"))
	c.Assert(err, gocheck.IsNil)
	f.Write([]byte("ssh-rsa ohwait! me@machine"))
	f.Close()
	old := fsystem
	fsystem = &rfs
	defer func() {
		fsystem = old
	}()
	commands, err := getSSHCommands()
	c.Assert(err, gocheck.IsNil)
	c.Assert(commands[0], gocheck.Equals, "/var/lib/tsuru/add-key ssh-rsa ohwait! me@machine")
}

func (s *S) TestGetSSHCommandsMissingAddKeyCommand(c *gocheck.C) {
	old, _ := config.Get("docker:ssh:add-key-cmd")
	defer config.Set("docker:ssh:add-key-cmd", old)
	config.Unset("docker:ssh:add-key-cmd")
	commands, err := getSSHCommands()
	c.Assert(commands, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestGetSSHCommandsKeyFileNotFound(c *gocheck.C) {
	old := fsystem
	fsystem = &ftesting.RecordingFs{}
	defer func() {
		fsystem = old
	}()
	commands, err := getSSHCommands()
	c.Assert(commands, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(os.IsNotExist(err), gocheck.Equals, true)
}

func (s *S) TestGetPort(c *gocheck.C) {
	port, err := getPort()
	c.Assert(err, gocheck.IsNil)
	c.Assert(port, gocheck.Equals, s.port)
}

func (s *S) TestGetPortUndefined(c *gocheck.C) {
	old, _ := config.Get("docker:run-cmd:port")
	defer config.Set("docker:run-cmd:port", old)
	config.Unset("docker:run-cmd:port")
	port, err := getPort()
	c.Assert(port, gocheck.Equals, "")
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestDockerCreate(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	output := `{
	"NetworkSettings": {
		"IpAddress": "10.10.10.10",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"PortMapping": {"8888": "37457"}
	}
}`
	fexec.Output = map[string][][]byte{
		"inspect c-01": {[]byte(output)},
		"*":            {[]byte("c-01")},
	}
	container := container{AppName: "app-name", Type: "python"}
	app := testing.NewFakeApp("app-name", "python", 1)
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	err := container.create(app)
	defer container.remove()
	c.Assert(err, gocheck.IsNil)
	sshCmd := "/var/lib/tsuru/add-key key-content && /usr/sbin/sshd -D"
	args := []string{
		"run", "-d", "-t", "-p", s.port, "tsuru/python",
		"/bin/bash", "-c", sshCmd,
	}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
	c.Assert(container.Status, gocheck.Equals, "created")
	c.Assert(container.HostPort, gocheck.Equals, "37457")
}

func (s *S) TestContainerCreateWithoutHostAddr(c *gocheck.C) {
	old, _ := config.Get("docker:host-address")
	defer config.Set("docker:host-address", old)
	config.Unset("docker:host-address")
	container := container{AppName: "myapp", Type: "python"}
	err := container.create(testing.NewFakeApp("myapp", "python", 1))
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestContainerSetStatus(c *gocheck.C) {
	container := container{ID: "something-300"}
	s.conn.Collection(s.collName).Insert(container)
	defer s.conn.Collection(s.collName).RemoveId(container.ID)
	container.setStatus("what?!")
	c2, err := getContainer(container.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(c2.Status, gocheck.Equals, "what?!")
}

func (s *S) TestDockerDeploy(c *gocheck.C) {
	var buf bytes.Buffer
	fexec := &etesting.FakeExecutor{
		Output: map[string][][]byte{"*": {[]byte("success\n")}},
	}
	setExecut(fexec)
	defer setExecut(nil)
	container := container{ID: "c-01", IP: "10.10.10.10", AppName: "myapp"}
	err := s.conn.Collection(s.collName).Insert(container)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(s.collName).RemoveId(container.ID)
	err = container.deploy("ff13e", &buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "success\n")
	appRepo := repository.ReadOnlyURL(container.AppName)
	deployArgs := []string{
		"10.10.10.10", "-l", s.sshUser, "-o", "StrictHostKeyChecking no",
		"--", s.deployCmd, appRepo, "ff13e",
	}
	runArgs := []string{
		"10.10.10.10", "-l", s.sshUser, "-o", "StrictHostKeyChecking no",
		"--", s.runBin, s.runArgs,
	}
	c.Assert(fexec.ExecutedCmd("ssh", deployArgs), gocheck.Equals, true)
	c.Assert(fexec.ExecutedCmd("ssh", runArgs), gocheck.Equals, true)
	cont, err := getContainer(container.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(cont.Status, gocheck.Equals, "running")
	c.Assert(cont.Version, gocheck.Equals, "ff13e")
}

func (s *S) TestDockerDeployRetries(c *gocheck.C) {
	var buf bytes.Buffer
	fexec := etesting.RetryExecutor{
		Failures: 3,
		FakeExecutor: etesting.FakeExecutor{
			Output: map[string][][]byte{"*": {[]byte("connection refused")}},
		},
	}
	setExecut(&fexec)
	defer setExecut(nil)
	container := container{ID: "c-01", IP: "10.10.10.10", AppName: "myapp"}
	s.conn.Collection(s.collName).Insert(container)
	defer s.conn.Collection(s.collName).RemoveId(container.ID)
	err := container.deploy("origin/master", &buf)
	c.Assert(err, gocheck.IsNil)
	commands := fexec.GetCommands("ssh")
	c.Assert(commands, gocheck.HasLen, 5)
	deployArgs := []string{
		"10.10.10.10", "-l", s.sshUser, "-o", "StrictHostKeyChecking no",
		"--", s.deployCmd, repository.ReadOnlyURL(container.AppName), "origin/master",
	}
	for _, cmd := range commands[:4] {
		c.Check(cmd.GetArgs(), gocheck.DeepEquals, deployArgs)
	}
	runArgs := []string{
		"10.10.10.10", "-l", s.sshUser, "-o", "StrictHostKeyChecking no",
		"--", s.runBin, s.runArgs,
	}
	c.Assert(commands[4].GetArgs(), gocheck.DeepEquals, runArgs)
}

func (s *S) TestDockerDeployNoDeployCommand(c *gocheck.C) {
	old, _ := config.Get("docker:deploy-cmd")
	defer config.Set("docker:deploy-cmd", old)
	config.Unset("docker:deploy-cmd")
	var container container
	err := container.deploy("origin/master", nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `key "docker:deploy-cmd" not found`)
}

func (s *S) TestDockerDeployNoBinaryToRun(c *gocheck.C) {
	old, _ := config.Get("docker:run-cmd:bin")
	defer config.Set("docker:run-cmd:bin", old)
	config.Unset("docker:run-cmd:bin")
	var container container
	err := container.deploy("origin/master", nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `key "docker:run-cmd:bin" not found`)
}

func (s *S) TestDockerDeployFailure(c *gocheck.C) {
	var buf bytes.Buffer
	fexec := etesting.ErrorExecutor{
		FakeExecutor: etesting.FakeExecutor{
			Output: map[string][][]byte{"*": {[]byte("deploy failed")}},
		},
	}
	setExecut(&fexec)
	defer setExecut(nil)
	container := container{ID: "c-01", IP: "10.10.10.10", AppName: "myapp"}
	err := s.conn.Collection(s.collName).Insert(container)
	defer s.conn.Collection(s.collName).RemoveId(container.ID)
	err = container.deploy("origin/master", &buf)
	c.Assert(err, gocheck.NotNil)
	c.Assert(buf.String(), gocheck.Equals, "deploy failed")
	c2, err := getContainer(container.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(c2.Status, gocheck.Equals, "error")
}

func (s *S) TestDockerRemove(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	container := container{AppName: "container", ID: "id", IP: "10.10.10.10", HostPort: "3333"}
	err := s.conn.Collection(s.collName).Insert(&container)
	c.Assert(err, gocheck.IsNil)
	rtesting.FakeRouter.AddBackend(container.AppName)
	defer rtesting.FakeRouter.RemoveBackend(container.AppName)
	rtesting.FakeRouter.AddRoute(container.AppName, container.getAddress())
	err = container.remove()
	c.Assert(err, gocheck.IsNil)
	args := []string{"rm", container.ID}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
	args = []string{"-R", container.IP}
	c.Assert(fexec.ExecutedCmd("ssh-keygen", args), gocheck.Equals, true)
}

func (s *S) TestDockerRemoveRemovesContainerFromDatabase(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	cntnr := container{AppName: "container", ID: "id", HostPort: "3456"}
	rtesting.FakeRouter.AddBackend(cntnr.AppName)
	defer rtesting.FakeRouter.RemoveBackend(cntnr.AppName)
	rtesting.FakeRouter.AddRoute(cntnr.AppName, cntnr.getAddress())
	err := s.conn.Collection(s.collName).Insert(&cntnr)
	c.Assert(err, gocheck.IsNil)
	err = cntnr.remove()
	c.Assert(err, gocheck.IsNil)
	coll := s.conn.Collection(s.collName)
	coll.FindId("id")
	err = coll.FindId(cntnr.ID).One(&cntnr)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "not found")
}

func (s *S) TestDockerRemoveRemovesRoute(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("myapp", "python", 1)
	cntnr := container{AppName: app.GetName(), ID: "id", IP: "10.10.10.10", HostPort: "3456"}
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	rtesting.FakeRouter.AddRoute(app.GetName(), cntnr.getAddress())
	err := s.conn.Collection(s.collName).Insert(&cntnr)
	err = cntnr.remove()
	c.Assert(err, gocheck.IsNil)
	c.Assert(rtesting.FakeRouter.HasRoute(app.GetName(), cntnr.getAddress()), gocheck.Equals, false)
}

func (s *S) TestContainerIPRunsDockerInspectCommand(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	cont := container{AppName: "vm1", ID: "id"}
	cont.ip()
	args := []string{"inspect", "id"}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestContainerIPReturnsIPFromDockerInspect(c *gocheck.C) {
	cmdReturn := `
    {
            \"NetworkSettings\": {
            \"IpAddress\": \"10.10.10.10\",
            \"IpPrefixLen\": 8,
            \"Gateway\": \"10.65.41.1\",
            \"PortMapping\": {}
    }
}`
	tmpdir, err := commandmocker.Add("docker", cmdReturn)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	cont := container{AppName: "vm1", ID: "id"}
	ip, err := cont.ip()
	c.Assert(err, gocheck.IsNil)
	c.Assert(ip, gocheck.Equals, "10.10.10.10")
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
}

func (s *S) TestContainerHostPortReturnsPortFromDockerInspect(c *gocheck.C) {
	container := container{ID: "c-01", Port: "8888"}
	output := `{
	"NetworkSettings": {
		"IpAddress": "10.10.10.10",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"PortMapping": {
			"8888": "59322"
		}
	}
}`
	out := map[string][][]byte{
		"inspect c-01": {[]byte(output)},
	}
	fexec := &etesting.FakeExecutor{Output: out}
	setExecut(fexec)
	defer setExecut(nil)
	port, err := container.hostPort()
	c.Assert(err, gocheck.IsNil)
	c.Assert(port, gocheck.Equals, "59322")
}

func (s *S) TestContainerHostPortNoPort(c *gocheck.C) {
	container := container{ID: "c-01"}
	port, err := container.hostPort()
	c.Assert(port, gocheck.Equals, "")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Container does not contain any mapped port")
}

func (s *S) TestContainerHostPortNotFound(c *gocheck.C) {
	container := container{ID: "c-01", Port: "8888"}
	output := `{
	"NetworkSettings": {
		"IpAddress": "10.10.10.10",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"PortMapping": {
			"8889": "59322"
		}
	}
}`
	out := map[string][][]byte{
		"inspect c-01": {[]byte(output)},
	}
	fexec := &etesting.FakeExecutor{Output: out}
	setExecut(fexec)
	defer setExecut(nil)
	port, err := container.hostPort()
	c.Assert(port, gocheck.Equals, "")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Container port 8888 is not mapped to any host port")
}

func (s *S) TestContainerInspect(c *gocheck.C) {
	container := container{ID: "c-01", Port: "8888"}
	output := `{"NetworkSettings": null}`
	out := map[string][][]byte{
		"inspect c-01": {[]byte(output)},
	}
	fexec := &etesting.FakeExecutor{Output: out}
	setExecut(fexec)
	defer setExecut(nil)
	expected := map[string]interface{}{"NetworkSettings": nil}
	got, err := container.inspect()
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.DeepEquals, expected)
}

func (s *S) TestContainerInspectNoBinary(c *gocheck.C) {
	old, _ := config.Get("docker:binary")
	defer config.Set("docker:binary", old)
	config.Unset("docker:binary")
	container := container{ID: "something"}
	got, err := container.inspect()
	c.Assert(got, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestContainerInspectInvalidJSON(c *gocheck.C) {
	container := container{ID: "c-01", Port: "8888"}
	out := map[string][][]byte{
		"inspect c-01": {[]byte("somethinginvalid}")},
	}
	fexec := &etesting.FakeExecutor{Output: out}
	setExecut(fexec)
	defer setExecut(nil)
	got, err := container.inspect()
	c.Assert(got, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestContainerSSH(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	output := []byte(". ..")
	out := map[string][][]byte{"*": {output}}
	fexec := &etesting.FakeExecutor{Output: out}
	setExecut(fexec)
	defer setExecut(nil)
	container := container{ID: "c-01", IP: "10.10.10.10"}
	err := container.ssh(&stdout, &stderr, "ls", "-a")
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, string(output))
	args := []string{
		"10.10.10.10", "-l", s.sshUser,
		"-o", "StrictHostKeyChecking no",
		"--", "ls", "-a",
	}
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
}

func (s *S) TestContainerSSHWithPrivateKey(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	config.Set("docker:ssh:private-key", "/opt/me/id_dsa")
	defer config.Unset("docker:ssh:private-key")
	output := []byte(". ..")
	out := map[string][][]byte{"*": {output}}
	fexec := &etesting.FakeExecutor{Output: out}
	setExecut(fexec)
	defer setExecut(nil)
	container := container{ID: "c-01", IP: "10.10.10.13"}
	err := container.ssh(&stdout, &stderr, "ls", "-a")
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, string(output))
	args := []string{
		"10.10.10.13", "-l", s.sshUser,
		"-o", "StrictHostKeyChecking no",
		"-i", "/opt/me/id_dsa",
		"--", "ls", "-a",
	}
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
}

func (s *S) TestContainerSSHWithoutUserConfigured(c *gocheck.C) {
	old, _ := config.Get("docker:ssh:user")
	defer config.Set("docker:ssh:user", old)
	config.Unset("docker:ssh:user")
	container := container{ID: "c-01", IP: "127.0.0.1"}
	err := container.ssh(nil, nil, "ls", "-a")
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestContainerSSHCommandFailure(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	fexec := &etesting.ErrorExecutor{
		FakeExecutor: etesting.FakeExecutor{
			Output: map[string][][]byte{"*": {[]byte("failed")}},
		},
	}
	setExecut(fexec)
	defer setExecut(nil)
	container := container{ID: "c-01", IP: "10.10.10.10"}
	err := container.ssh(&stdout, &stderr, "ls", "-a")
	c.Assert(err, gocheck.NotNil)
	c.Assert(stdout.Bytes(), gocheck.IsNil)
	c.Assert(stderr.String(), gocheck.Equals, "failed")
}

func (s *S) TestContainerSSHFiltersStderr(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	fexec := &etesting.ErrorExecutor{
		FakeExecutor: etesting.FakeExecutor{
			Output: map[string][][]byte{"*": {[]byte("failed\nunable to resolve host abcdef")}},
		},
	}
	setExecut(fexec)
	defer setExecut(nil)
	container := container{ID: "c-01", IP: "10.10.10.10"}
	err := container.ssh(&stdout, &stderr, "ls", "-a")
	c.Assert(err, gocheck.NotNil)
	c.Assert(stdout.Bytes(), gocheck.IsNil)
	c.Assert(stderr.String(), gocheck.Equals, "failed\n")
}

func (s *S) TestImageCommit(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	img := image{Name: "app-name", ID: "image-id"}
	_, err := img.commit("container-id")
	defer img.remove()
	c.Assert(err, gocheck.IsNil)
	repoNamespace, err := config.GetString("docker:repository-namespace")
	c.Assert(err, gocheck.IsNil)
	imageName := fmt.Sprintf("%s/app-name", repoNamespace)
	args := []string{"commit", "container-id", imageName}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestImageCommitReturnsImageID(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("docker", "945132e7b4c9\n")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	img := image{Name: "app-name", ID: "image-id"}
	id, err := img.commit("container-id")
	defer img.remove()
	c.Assert(err, gocheck.IsNil)
	c.Assert(id, gocheck.Equals, "945132e7b4c9")
}

func (s *S) TestImageCommitInsertImageInformationToMongo(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("docker", "945132e7b4c9\n")
	c.Assert(err, gocheck.IsNil)
	img := image{Name: "app-name", ID: "image-id"}
	_, err = img.commit("cid")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	var imgMgo image
	defer imgMgo.remove()
	err = s.conn.Collection(s.imageCollName).Find(bson.M{"name": img.Name}).One(&imgMgo)
	c.Assert(err, gocheck.IsNil)
	c.Assert(imgMgo.ID, gocheck.Equals, img.ID)
}

func (s *S) TestImageRemove(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	img := image{Name: "app-name", ID: "image-id"}
	err := s.conn.Collection(s.imageCollName).Insert(&img)
	c.Assert(err, gocheck.IsNil)
	err = img.remove()
	c.Assert(err, gocheck.IsNil)
	args := []string{"rmi", img.ID}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
	var imgMgo image
	err = s.conn.Collection(s.imageCollName).Find(bson.M{"name": img.Name}).One(&imgMgo)
	c.Assert(err.Error(), gocheck.Equals, "not found")
}

func (s *S) TestGetContainer(c *gocheck.C) {
	collection().Insert(
		container{ID: "abcdef", Type: "python"},
		container{ID: "fedajs", Type: "ruby"},
		container{ID: "wat", Type: "java"},
	)
	defer collection().RemoveAll(bson.M{"_id": bson.M{"$in": []string{"abcdef", "fedajs", "wat"}}})
	container, err := getContainer("abcdef")
	c.Assert(err, gocheck.IsNil)
	c.Assert(container.ID, gocheck.Equals, "abcdef")
	c.Assert(container.Type, gocheck.Equals, "python")
	container, err = getContainer("wut")
	c.Assert(container, gocheck.IsNil)
	c.Assert(err.Error(), gocheck.Equals, "not found")
}

func (s *S) TestGetContainers(c *gocheck.C) {
	collection().Insert(
		container{ID: "abcdef", Type: "python", AppName: "something"},
		container{ID: "fedajs", Type: "python", AppName: "something"},
		container{ID: "wat", Type: "java", AppName: "otherthing"},
	)
	defer collection().RemoveAll(bson.M{"_id": bson.M{"$in": []string{"abcdef", "fedajs", "wat"}}})
	containers, err := listAppContainers("something")
	c.Assert(err, gocheck.IsNil)
	c.Assert(containers, gocheck.HasLen, 2)
	c.Assert(containers[0].ID, gocheck.Equals, "abcdef")
	c.Assert(containers[1].ID, gocheck.Equals, "fedajs")
	containers, err = listAppContainers("otherthing")
	c.Assert(err, gocheck.IsNil)
	c.Assert(containers, gocheck.HasLen, 1)
	c.Assert(containers[0].ID, gocheck.Equals, "wat")
	containers, err = listAppContainers("unknown")
	c.Assert(err, gocheck.IsNil)
	c.Assert(containers, gocheck.HasLen, 0)
}

func (s *S) TestGetImageFromAppPlatform(c *gocheck.C) {
	app := testing.NewFakeApp("myapp", "python", 1)
	img := getImage(app)
	repoNamespace, err := config.GetString("docker:repository-namespace")
	c.Assert(err, gocheck.IsNil)
	c.Assert(img, gocheck.Equals, fmt.Sprintf("%s/python", repoNamespace))
}

func (s *S) TestGetImageFromDatabase(c *gocheck.C) {
	cont := container{ID: "bleble", Type: "python", AppName: "myapp", Image: "someimageid"}
	err := collection().Insert(cont)
	c.Assert(err, gocheck.IsNil)
	defer collection().RemoveAll(bson.M{"_id": "bleble"})
	app := testing.NewFakeApp("myapp", "python", 1)
	img := getImage(app)
	c.Assert(img, gocheck.Equals, "someimageid")
}
