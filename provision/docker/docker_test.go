// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/docker-cluster/cluster"
	etesting "github.com/globocom/tsuru/exec/testing"
	ftesting "github.com/globocom/tsuru/fs/testing"
	"github.com/globocom/tsuru/log"
	rtesting "github.com/globocom/tsuru/router/testing"
	"github.com/globocom/tsuru/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	stdlog "log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
)

func (s *S) TestContainerGetAddress(c *gocheck.C) {
	container := container{ID: "id123", Port: "8888", HostPort: "49153"}
	address := container.getAddress()
	expected := fmt.Sprintf("http://%s:49153", s.hostAddr)
	c.Assert(address, gocheck.Equals, expected)
}

func (s *S) TestNewContainer(c *gocheck.C) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if strings.Contains(r.URL.Path, "/containers/") {
			w.Write([]byte(inspectOut))
		}
	}))
	defer server.Close()
	oldCluster := dockerCluster
	var err error
	dockerCluster, err = cluster.New(
		cluster.Node{ID: "server", Address: server.URL},
	)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		dockerCluster = oldCluster
	}()
	id := "945132e7b4c9"
	out := map[string][][]byte{
		"run": {[]byte(id)},
	}
	fexec := &etesting.FakeExecutor{Output: out}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("app-name", "python", 1)
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	_, err = newContainer(app, []string{"docker", "run"})
	defer s.conn.Collection(s.collName).RemoveId(id)
	var cont container
	err = s.conn.Collection(s.collName).FindId(id).One(&cont)
	c.Assert(err, gocheck.IsNil)
	c.Assert(cont.ID, gocheck.Equals, id)
	c.Assert(cont.AppName, gocheck.Equals, "app-name")
	c.Assert(cont.IP, gocheck.Equals, "10.10.10.10")
	c.Assert(cont.HostPort, gocheck.Equals, "34233")
	c.Assert(cont.Port, gocheck.Equals, s.port)
	c.Assert(called, gocheck.Equals, true)
}

func (s *S) TestNewContainerCallsDockerCreate(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("app-name", "python", 1)
	cmds := []string{"docker", "run"}
	newContainer(app, cmds)
	defer s.conn.Collection(s.collName).Remove(bson.M{"appname": app.GetName()})
	c.Assert(fexec.ExecutedCmd("docker", cmds[1:]), gocheck.Equals, true)
}

func (s *S) TestNewContainerReturnsNilAndLogsOnError(c *gocheck.C) {
	w := new(bytes.Buffer)
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	fexec := &etesting.ErrorExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("myapp", "python", 1)
	cmds, err := deployCmds(app, "version")
	c.Assert(err, gocheck.IsNil)
	container, err := newContainer(app, cmds)
	c.Assert(err, gocheck.NotNil)
	defer s.conn.Collection(s.collName).Remove(bson.M{"appname": app.GetName()})
	c.Assert(container, gocheck.IsNil)
	c.Assert(w.String(), gocheck.Matches, `(?s).*Error creating container for the app "myapp".*`)
}

func (s *S) TestNewContainerAddsRoute(c *gocheck.C) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if strings.Contains(r.URL.Path, "/containers/") {
			w.Write([]byte(inspectOut))
		}
	}))
	defer server.Close()
	oldCluster := dockerCluster
	var err error
	dockerCluster, err = cluster.New(
		cluster.Node{ID: "server", Address: server.URL},
	)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		dockerCluster = oldCluster
	}()
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
	cmds, err := deployCmds(app, "version")
	c.Assert(err, gocheck.IsNil)
	container, err := newContainer(app, cmds)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(s.collName).RemoveId(out)
	c.Assert(rtesting.FakeRouter.HasRoute(app.GetName(), container.getAddress()), gocheck.Equals, true)
	c.Assert(called, gocheck.Equals, true)
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
	commands, err := sshCmds()
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
	commands, err := sshCmds()
	c.Assert(err, gocheck.IsNil)
	c.Assert(commands[0], gocheck.Equals, "/var/lib/tsuru/add-key ssh-rsa ohwait! me@machine")
}

func (s *S) TestGetSSHCommandsMissingAddKeyCommand(c *gocheck.C) {
	old, _ := config.Get("docker:ssh:add-key-cmd")
	defer config.Set("docker:ssh:add-key-cmd", old)
	config.Unset("docker:ssh:add-key-cmd")
	commands, err := sshCmds()
	c.Assert(commands, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestGetSSHCommandsKeyFileNotFound(c *gocheck.C) {
	old := fsystem
	fsystem = &ftesting.RecordingFs{}
	defer func() {
		fsystem = old
	}()
	commands, err := sshCmds()
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
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if strings.Contains(r.URL.Path, "/containers/") {
			w.Write([]byte(inspectOut))
		}
	}))
	defer server.Close()
	oldCluster := dockerCluster
	var err error
	dockerCluster, err = cluster.New(
		cluster.Node{ID: "server", Address: server.URL},
	)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		dockerCluster = oldCluster
	}()
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	fexec.Output = map[string][][]byte{
		"*": {[]byte("c-01")},
	}
	container := container{AppName: "app-name", Type: "python"}
	app := testing.NewFakeApp("app-name", "python", 1)
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	commands := []string{"docker", "run"}
	err = container.create(commands)
	defer container.remove()
	c.Assert(err, gocheck.IsNil)
	c.Assert(fexec.ExecutedCmd("docker", commands[1:]), gocheck.Equals, true)
	c.Assert(container.Status, gocheck.Equals, "created")
	c.Assert(container.HostPort, gocheck.Equals, "34233")
	c.Assert(called, gocheck.Equals, true)
}

func (s *S) TestContainerCreateWithoutHostAddr(c *gocheck.C) {
	old, _ := config.Get("docker:host-address")
	defer config.Set("docker:host-address", old)
	config.Unset("docker:host-address")
	container := container{AppName: "myapp", Type: "python"}
	app := testing.NewFakeApp("myapp", "python", 1)
	commands, err := deployCmds(app, "version")
	c.Assert(err, gocheck.IsNil)
	err = container.create(commands)
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

func (s *S) TestContainerSetImage(c *gocheck.C) {
	container := container{ID: "something-300"}
	s.conn.Collection(s.collName).Insert(container)
	defer s.conn.Collection(s.collName).RemoveId(container.ID)
	container.setImage("newimage")
	c2, err := getContainer(container.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(c2.Image, gocheck.Equals, "newimage")
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
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if strings.Contains(r.URL.Path, "/containers/") {
			w.Write([]byte(inspectOut))
		}
	}))
	defer server.Close()
	oldCluster := dockerCluster
	var err error
	dockerCluster, err = cluster.New(
		cluster.Node{ID: "server", Address: server.URL},
	)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		dockerCluster = oldCluster
	}()
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	cont := container{AppName: "vm1", ID: "id"}
	ip, err := cont.ip()
	c.Assert(err, gocheck.IsNil)
	c.Assert(ip, gocheck.Equals, "10.10.10.10")
	c.Assert(called, gocheck.Equals, true)
}

func (s *S) TestContainerHostPortReturnsPortFromDockerInspect(c *gocheck.C) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if strings.Contains(r.URL.Path, "/containers/") {
			w.Write([]byte(inspectOut))
		}
	}))
	defer server.Close()
	oldCluster := dockerCluster
	var err error
	dockerCluster, err = cluster.New(
		cluster.Node{ID: "server", Address: server.URL},
	)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		dockerCluster = oldCluster
	}()
	container := container{ID: "c-01", Port: "8888"}
	port, err := container.hostPort()
	c.Assert(err, gocheck.IsNil)
	c.Assert(port, gocheck.Equals, "34233")
	c.Assert(called, gocheck.Equals, true)
}

func (s *S) TestContainerHostPortNoPort(c *gocheck.C) {
	container := container{ID: "c-01"}
	port, err := container.hostPort()
	c.Assert(port, gocheck.Equals, "")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Container does not contain any mapped port")
}

func (s *S) TestContainerHostPortNotFound(c *gocheck.C) {
	inspectOut := `{
	"NetworkSettings": {
		"IpAddress": "10.10.10.10",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"PortMapping": {
			"8889": "59322"
		}
	}
}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/containers/") {
			w.Write([]byte(inspectOut))
		}
	}))
	defer server.Close()
	oldCluster := dockerCluster
	var err error
	dockerCluster, err = cluster.New(
		cluster.Node{ID: "server", Address: server.URL},
	)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		dockerCluster = oldCluster
	}()
	container := container{ID: "c-01", Port: "8888"}
	port, err := container.hostPort()
	c.Assert(port, gocheck.Equals, "")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Container port 8888 is not mapped to any host port")
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

func (s *S) TestBinary(c *gocheck.C) {
	bin, _ := config.Get("docker:binary")
	binary, err := binary()
	c.Assert(err, gocheck.IsNil)
	c.Assert(binary, gocheck.Equals, bin)
}

func (s *S) TestContainerCommit(c *gocheck.C) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte(`{"Id":"imageid"}`))
	}))
	defer server.Close()
	oldCluster := dockerCluster
	var err error
	dockerCluster, err = cluster.New(
		cluster.Node{ID: "server", Address: server.URL},
	)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		dockerCluster = oldCluster
	}()
	cont := container{ID: "someid", Type: "python", AppName: "myapp"}
	imageId, err := cont.commit()
	c.Assert(err, gocheck.IsNil)
	c.Assert(imageId, gocheck.Equals, "imageid")
	c.Assert(called, gocheck.Equals, true)
}

func (s *S) TestRemoveImage(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	imageId := "image-id"
	err := removeImage(imageId)
	c.Assert(err, gocheck.IsNil)
	args := []string{"rmi", imageId}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestContainerDeploy(c *gocheck.C) {
	var called int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		if strings.Contains(r.URL.Path, "/containers/") {
			w.Write([]byte(inspectOut))
		}
		if strings.Contains(r.URL.Path, "/commit") {
			w.Write([]byte(`{"Id":"someimageid"}`))
		}
	}))
	defer server.Close()
	oldCluster := dockerCluster
	var err error
	dockerCluster, err = cluster.New(
		cluster.Node{ID: "server", Address: server.URL},
	)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		dockerCluster = oldCluster
	}()
	id := "945132e7b4c9"
	sshCmd := "/var/lib/tsuru/add-key key-content && /usr/sbin/sshd -D"
	runCmd := fmt.Sprintf("run -d -t -p %s tsuru/python /bin/bash -c %s",
		s.port, sshCmd)
	app := testing.NewFakeApp("myapp", "python", 1)
	imageId := getImage(app)
	cmds, err := deployCmds(app, "ff13e")
	c.Assert(err, gocheck.IsNil)
	deployCmds := strings.Join(cmds[1:], " ")
	logCmd := fmt.Sprintf("logs %s", id)
	logOut := "log out"
	out := map[string][][]byte{
		runCmd:     {[]byte(id)},
		deployCmds: {[]byte(id)},
		logCmd:     {[]byte(logOut)},
	}
	fexec := &etesting.FakeExecutor{Output: out}
	setExecut(fexec)
	defer setExecut(nil)
	defer s.conn.Collection(s.collName).RemoveId(id)
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	var buf bytes.Buffer
	imageId, err = deploy(app, "ff13e", &buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(imageId, gocheck.Equals, "someimageid")
	c.Assert(buf.String(), gocheck.Equals, logOut)
	args := []string{"logs", id}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
	c.Assert(called, gocheck.Equals, 4)
}

func (s *S) TestStart(c *gocheck.C) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if strings.Contains(r.URL.Path, "/containers/") {
			w.Write([]byte(inspectOut))
		}
	}))
	defer server.Close()
	oldCluster := dockerCluster
	var err error
	dockerCluster, err = cluster.New(
		cluster.Node{ID: "server", Address: server.URL},
	)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		dockerCluster = oldCluster
	}()
	id := "945132e7b4c9"
	app := testing.NewFakeApp("myapp", "python", 1)
	imageId := getImage(app)
	cmds, err := runCmds(imageId)
	c.Assert(err, gocheck.IsNil)
	runCmds := strings.Join(cmds[1:], " ")
	inspectCmd := fmt.Sprintf("inspect %s", id)
	out := map[string][][]byte{runCmds: {[]byte(id)}, inspectCmd: {[]byte(inspectOut)}}
	fexec := &etesting.FakeExecutor{Output: out}
	setExecut(fexec)
	defer setExecut(nil)
	defer s.conn.Collection(s.collName).RemoveId(id)
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	var buf bytes.Buffer
	cont, err := start(app, imageId, &buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(cont.ID, gocheck.Equals, id)
	cont2, err := getContainer(cont.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(cont2.Image, gocheck.Equals, imageId)
	c.Assert(cont2.Status, gocheck.Equals, "running")
	c.Assert(called, gocheck.Equals, true)
}

func (s *S) TestContainerRunCmdError(c *gocheck.C) {
	fexec := etesting.ErrorExecutor{
		FakeExecutor: etesting.FakeExecutor{
			Output: map[string][][]byte{"*": {[]byte("f1 f2 f3")}},
		},
	}
	setExecut(&fexec)
	defer setExecut(nil)
	_, err := runCmd("ls", "-a")
	e, ok := err.(*cmdError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.err, gocheck.NotNil)
	c.Assert(e.out, gocheck.Equals, "f1 f2 f3")
	c.Assert(e.cmd, gocheck.Equals, "ls")
	c.Assert(e.args, gocheck.DeepEquals, []string{"-a"})
}

func (s *S) TestContainerStopped(c *gocheck.C) {
	inspectOut := `
    {
	"State": {
		"Running": false,
		"Pid": 0,
		"ExitCode": 0,
		"StartedAt": "2013-06-13T20:59:31.699407Z",
		"Ghost": false
	},
	"NetworkSettings": {
		"IpAddress": "10.10.10.10",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"PortMapping": {"8888": "34233"}
	}
}`
	var called int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		if strings.Contains(r.URL.Path, "/containers/") {
			w.Write([]byte(inspectOut))
		}
	}))
	defer server.Close()
	oldCluster := dockerCluster
	var err error
	dockerCluster, err = cluster.New(
		cluster.Node{ID: "server", Address: server.URL},
	)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		dockerCluster = oldCluster
	}()
	cont := container{ID: "someid", Type: "python", AppName: "myapp"}
	result, err := cont.stopped()
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.Equals, true)
	c.Assert(called, gocheck.Equals, 1)
}

func (s *S) TestContainerLogs(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{
		Output: map[string][][]byte{
			"*": {[]byte("some logs")},
		},
	}
	setExecut(fexec)
	defer setExecut(nil)
	cont := container{ID: "someid", Type: "python", AppName: "myapp"}
	result, err := cont.logs()
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.Equals, "some logs")
	args := []string{"logs", "someid"}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestDockerCluster(c *gocheck.C) {
	expected, err := cluster.New(
		cluster.Node{ID: "server", Address: "http://localhost:4243"},
	)
	c.Assert(err, gocheck.IsNil)
	c.Assert(dockerCluster, gocheck.DeepEquals, expected)
}
