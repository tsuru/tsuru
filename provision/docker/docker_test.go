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
	"github.com/globocom/tsuru/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	stdlog "log"
	"os"
)

func (s *S) TestContainerGetAddress(c *gocheck.C) {
	cmdOut := `
{
    "NetworkSettings": {
        "IpAddress": "10.10.10.10",
        "IpPrefixLen": 8,
        "Gateway": "10.65.41.1",
        "PortMapping": {
            "8888": "49153"
        }
    }
}`
	out := map[string][][]byte{"*": {[]byte(cmdOut)}}
	fexec := &etesting.FakeExecutor{Output: out}
	setExecut(fexec)
	defer setExecut(nil)
	container := container{Id: "id123", Port: "8888"}
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
            "PortMapping": {}
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
	_, err := newContainer(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(s.collName).RemoveId(id)
	var cont container
	err = s.conn.Collection(s.collName).FindId(id).One(&cont)
	c.Assert(err, gocheck.IsNil)
	c.Assert(cont.Id, gocheck.Equals, id)
	c.Assert(cont.AppName, gocheck.Equals, "app-name")
	c.Assert(cont.Ip, gocheck.Equals, "10.10.10.10")
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
	c.Assert(w.String(), gocheck.Matches, "(?s).*Error creating container myapp.*")
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
	_, err := newContainer(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(s.collName).RemoveId(out)
	r, err := getRouter()
	c.Assert(err, gocheck.IsNil)
	addr, err := r.Addr(app.GetName())
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, "http://"+s.hostAddr+":30000")
}

func (s *S) TestNewContainerRouteNoMappedPort(c *gocheck.C) {
	out := `{
	"NetworkSettings": {
		"IpAddress": "10.10.10.1",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
        "PortMapping": {
            "8888": "49153"
        }
	}
}`
	fexec := &etesting.FakeExecutor{Output: map[string][][]byte{"*": {[]byte(out)}}}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("myapp", "python", 1)
	_, err := newContainer(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(s.collName).RemoveId(out)
	r, err := getRouter()
	c.Assert(err, gocheck.IsNil)
	addr, err := r.Addr(app.GetName())
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, "http://"+s.hostAddr+":49153")
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
		"PortMapping": {}
	}
}`
	fexec.Output = map[string][][]byte{
		"inspect c-01": {[]byte(output)},
		"*":            {[]byte("c-01")},
	}
	container := container{AppName: "app-name", Type: "python"}
	app := testing.NewFakeApp("app-name", "python", 1)
	err := container.create(app)
	c.Assert(err, gocheck.IsNil)
	sshCmd := "/var/lib/tsuru/add-key key-content && /usr/sbin/sshd -D"
	args := []string{
		"run", "-d", "-t", "-p", s.port, "tsuru/python",
		"/bin/bash", "-c", sshCmd,
	}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestContainerCreateWithoutHostAddr(c *gocheck.C) {
	old, _ := config.Get("docker:host-address")
	defer config.Set("docker:host-address", old)
	config.Unset("docker:host-address")
	container := container{AppName: "myapp", Type: "python"}
	err := container.create(testing.NewFakeApp("myapp", "python", 1))
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestDockerDeploy(c *gocheck.C) {
	var buf bytes.Buffer
	fexec := &etesting.FakeExecutor{
		Output: map[string][][]byte{"*": {[]byte("success\n")}},
	}
	setExecut(fexec)
	defer setExecut(nil)
	container := container{Id: "c-01", Ip: "10.10.10.10", AppName: "myapp"}
	err := container.deploy(&buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "success\nsuccess\n")
	appRepo := repository.GetReadOnlyUrl(container.AppName)
	deployArgs := []string{
		"10.10.10.10", "-l", s.sshUser, "-o", "StrictHostKeyChecking no",
		"--", s.deployCmd, appRepo,
	}
	runArgs := []string{
		"10.10.10.10", "-l", s.sshUser, "-o", "StrictHostKeyChecking no",
		"--", s.runBin, s.runArgs,
	}
	c.Assert(fexec.ExecutedCmd("ssh", deployArgs), gocheck.Equals, true)
	c.Assert(fexec.ExecutedCmd("ssh", runArgs), gocheck.Equals, true)
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
	container := container{Id: "c-01", Ip: "10.10.10.10", AppName: "myapp"}
	err := container.deploy(&buf)
	c.Assert(err, gocheck.IsNil)
	commands := fexec.GetCommands("ssh")
	c.Assert(commands, gocheck.HasLen, 5)
	deployArgs := []string{
		"10.10.10.10", "-l", s.sshUser, "-o", "StrictHostKeyChecking no",
		"--", s.deployCmd, repository.GetReadOnlyUrl(container.AppName),
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
	err := container.deploy(nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `key "docker:deploy-cmd" not found`)
}

func (s *S) TestDockerDeployNoBinaryToRun(c *gocheck.C) {
	old, _ := config.Get("docker:run-cmd:bin")
	defer config.Set("docker:run-cmd:bin", old)
	config.Unset("docker:run-cmd:bin")
	var container container
	err := container.deploy(nil)
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
	container := container{Id: "c-01", Ip: "10.10.10.10", AppName: "myapp"}
	err := container.deploy(&buf)
	c.Assert(err, gocheck.NotNil)
	c.Assert(buf.String(), gocheck.Equals, "deploy failed")
}

func (s *S) TestDockerRemove(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	container := container{AppName: "container", Id: "id"}
	err := s.conn.Collection(s.collName).Insert(&container)
	c.Assert(err, gocheck.IsNil)
	err = container.remove()
	c.Assert(err, gocheck.IsNil)
	args := []string{"rm", "id"}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestDockerRemoveRemovesContainerFromDatabase(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	cntnr := container{AppName: "container", Id: "id"}
	err := s.conn.Collection(s.collName).Insert(&cntnr)
	c.Assert(err, gocheck.IsNil)
	err = cntnr.remove()
	c.Assert(err, gocheck.IsNil)
	coll := s.conn.Collection(s.collName)
	coll.FindId("id")
	err = coll.FindId(cntnr.Id).One(&cntnr)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "not found")
}

func (s *S) TestDockerRemoveRemovesRoute(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("myapp", "python", 1)
	cntnr := container{AppName: "myapp", Id: "id", Ip: "10.10.10.10"}
	err := s.conn.Collection(s.collName).Insert(&cntnr)
	r, err := getRouter()
	c.Assert(err, gocheck.IsNil)
	r.AddRoute(app.GetName(), "10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	err = cntnr.remove()
	c.Assert(err, gocheck.IsNil)
	addr, err := r.Addr(app.GetName())
	c.Assert(err, gocheck.NotNil)
	c.Assert(addr, gocheck.Equals, "")
}

func (s *S) TestContainerIPRunsDockerInspectCommand(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	cont := container{AppName: "vm1", Id: "id"}
	cont.ip()
	args := []string{"inspect", "id"}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestContainerIPReturnsIpFromDockerInspect(c *gocheck.C) {
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
	cont := container{AppName: "vm1", Id: "id"}
	ip, err := cont.ip()
	c.Assert(err, gocheck.IsNil)
	c.Assert(ip, gocheck.Equals, "10.10.10.10")
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
}

func (s *S) TestContainerHostPortReturnsPortFromDockerInspect(c *gocheck.C) {
	container := container{Id: "c-01", Port: "8888"}
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
	container := container{Id: "c-01"}
	port, err := container.hostPort()
	c.Assert(port, gocheck.Equals, "")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Container does not contain any mapped port")
}

func (s *S) TestContainerHostPortNotFound(c *gocheck.C) {
	container := container{Id: "c-01", Port: "8888"}
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
	container := container{Id: "c-01", Port: "8888"}
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
	container := container{Id: "something"}
	got, err := container.inspect()
	c.Assert(got, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestContainerInspectInvalidJSON(c *gocheck.C) {
	container := container{Id: "c-01", Port: "8888"}
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
	container := container{Id: "c-01", Ip: "10.10.10.10"}
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
	container := container{Id: "c-01", Ip: "10.10.10.13"}
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
	container := container{Id: "c-01", Ip: "127.0.0.1"}
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
	container := container{Id: "c-01", Ip: "10.10.10.10"}
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
	container := container{Id: "c-01", Ip: "10.10.10.10"}
	err := container.ssh(&stdout, &stderr, "ls", "-a")
	c.Assert(err, gocheck.NotNil)
	c.Assert(stdout.Bytes(), gocheck.IsNil)
	c.Assert(stderr.String(), gocheck.Equals, "failed\n")
}

func (s *S) TestImageCommit(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	img := image{Name: "app-name", Id: "image-id"}
	_, err := img.commit("container-id")
	defer img.remove()
	c.Assert(err, gocheck.IsNil)
	repoNamespace, err := config.GetString("docker:repository-namespace")
	c.Assert(err, gocheck.IsNil)
	imageName := fmt.Sprintf("%s/app-name", repoNamespace)
	args := []string{"commit", "container-id", imageName}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestImageCommitReturnsImageId(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("docker", "945132e7b4c9\n")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	img := image{Name: "app-name", Id: "image-id"}
	id, err := img.commit("container-id")
	defer img.remove()
	c.Assert(err, gocheck.IsNil)
	c.Assert(id, gocheck.Equals, "945132e7b4c9")
}

func (s *S) TestImageCommitInsertImageInformationToMongo(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("docker", "945132e7b4c9\n")
	c.Assert(err, gocheck.IsNil)
	img := image{Name: "app-name", Id: "image-id"}
	_, err = img.commit("cid")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	var imgMgo image
	defer imgMgo.remove()
	err = s.conn.Collection(s.imageCollName).Find(bson.M{"name": img.Name}).One(&imgMgo)
	c.Assert(err, gocheck.IsNil)
	c.Assert(imgMgo.Id, gocheck.Equals, img.Id)
}

func (s *S) TestImageRemove(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	img := image{Name: "app-name", Id: "image-id"}
	err := s.conn.Collection(s.imageCollName).Insert(&img)
	c.Assert(err, gocheck.IsNil)
	err = img.remove()
	c.Assert(err, gocheck.IsNil)
	args := []string{"rmi", img.Id}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
	var imgMgo image
	err = s.conn.Collection(s.imageCollName).Find(bson.M{"name": img.Name}).One(&imgMgo)
	c.Assert(err.Error(), gocheck.Equals, "not found")
}

func (s *S) TestGetContainer(c *gocheck.C) {
	collection().Insert(
		container{Id: "abcdef", Type: "python"},
		container{Id: "fedajs", Type: "ruby"},
		container{Id: "wat", Type: "java"},
	)
	defer collection().RemoveAll(bson.M{"_id": bson.M{"$in": []string{"abcdef", "fedajs", "wat"}}})
	container, err := getContainer("abcdef")
	c.Assert(err, gocheck.IsNil)
	c.Assert(container.Id, gocheck.Equals, "abcdef")
	c.Assert(container.Type, gocheck.Equals, "python")
	container, err = getContainer("wut")
	c.Assert(container, gocheck.IsNil)
	c.Assert(err.Error(), gocheck.Equals, "not found")
}

func (s *S) TestGetContainers(c *gocheck.C) {
	collection().Insert(
		container{Id: "abcdef", Type: "python", AppName: "something"},
		container{Id: "fedajs", Type: "python", AppName: "something"},
		container{Id: "wat", Type: "java", AppName: "otherthing"},
	)
	defer collection().RemoveAll(bson.M{"_id": bson.M{"$in": []string{"abcdef", "fedajs", "wat"}}})
	containers, err := listAppContainers("something")
	c.Assert(err, gocheck.IsNil)
	c.Assert(containers, gocheck.HasLen, 2)
	c.Assert(containers[0].Id, gocheck.Equals, "abcdef")
	c.Assert(containers[1].Id, gocheck.Equals, "fedajs")
	containers, err = listAppContainers("otherthing")
	c.Assert(err, gocheck.IsNil)
	c.Assert(containers, gocheck.HasLen, 1)
	c.Assert(containers[0].Id, gocheck.Equals, "wat")
	containers, err = listAppContainers("unknown")
	c.Assert(err, gocheck.IsNil)
	c.Assert(containers, gocheck.HasLen, 0)
}
