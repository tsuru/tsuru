// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	"github.com/dotcloud/docker"
	dockerClient "github.com/fsouza/go-dockerclient"
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
	err := s.newImage()
	c.Assert(err, gocheck.IsNil)
	app := testing.NewFakeApp("app-name", "python", 1)
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	cont, err := newContainer(app, []string{"docker", "run"})
	c.Assert(err, gocheck.IsNil)
	defer cont.remove()
	var retrieved container
	err = s.conn.Collection(s.collName).FindId(cont.ID).One(&retrieved)
	c.Assert(retrieved.ID, gocheck.Not(gocheck.Equals), "")
	c.Assert(retrieved.AppName, gocheck.Equals, "app-name")
	c.Assert(retrieved.IP, gocheck.Not(gocheck.Equals), "")
	c.Assert(retrieved.HostPort, gocheck.Not(gocheck.Equals), "")
	c.Assert(retrieved.Port, gocheck.Equals, s.port)
}

func (s *S) TestNewContainerReturnsNilAndLogsOnError(c *gocheck.C) {
	w := new(bytes.Buffer)
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	fexec := &etesting.ErrorExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("myapp", "python", 1)
	cmds := []string{"ls"}
	container, err := newContainer(app, cmds)
	c.Assert(err, gocheck.NotNil)
	defer s.conn.Collection(s.collName).Remove(bson.M{"appname": app.GetName()})
	c.Assert(container, gocheck.IsNil)
	c.Assert(w.String(), gocheck.Matches, `(?s).*Error creating container for the app "myapp".*`)
}

func (s *S) TestNewContainerAddsRoute(c *gocheck.C) {
	err := s.newImage()
	c.Assert(err, gocheck.IsNil)
	app := testing.NewFakeApp("myapp", "python", 1)
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	cmds, err := deployCmds(app, "version")
	c.Assert(err, gocheck.IsNil)
	container, err := newContainer(app, cmds)
	c.Assert(err, gocheck.IsNil)
	defer container.remove()
	c.Assert(rtesting.FakeRouter.HasRoute(app.GetName(), container.getAddress()), gocheck.Equals, true)
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

func (s *S) TestContainerCreate(c *gocheck.C) {
	err := s.newImage()
	c.Assert(err, gocheck.IsNil)
	container := container{AppName: "app-name", Type: "python"}
	app := testing.NewFakeApp("app-name", "python", 1)
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	commands := []string{"docker", "run"}
	err = container.create(commands)
	defer container.remove()
	c.Assert(err, gocheck.IsNil)
	c.Assert(container.Status, gocheck.Equals, "created")
	c.Assert(container.HostPort, gocheck.Not(gocheck.Equals), "")
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

func (s *S) newImage() error {
	opts := dockerClient.PullImageOptions{Repository: "tsuru/python"}
	client, err := dockerClient.NewClient(s.server.URL())
	if err != nil {
		return err
	}
	var buffer bytes.Buffer
	return client.PullImage(opts, &buffer)
}

func (s *S) newContainer() (*container, error) {
	container := container{
		AppName:  "container",
		ID:       "id",
		IP:       "10.10.10.10",
		HostPort: "3333",
		Port:     "8888",
	}
	rtesting.FakeRouter.AddBackend(container.AppName)
	rtesting.FakeRouter.AddRoute(container.AppName, container.getAddress())
	client, err := dockerClient.NewClient(s.server.URL())
	if err != nil {
		return nil, err
	}
	config := docker.Config{
		Image:     "tsuru/python",
		Cmd:       []string{"ps"},
		PortSpecs: []string{"8888"},
	}
	c, err := client.CreateContainer(&config)
	if err != nil {
		return nil, err
	}
	container.ID = c.ID
	err = s.conn.Collection(s.collName).Insert(&container)
	if err != nil {
		return nil, err
	}
	return &container, err
}

func (s *S) TestContainerRemove(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	err := s.newImage()
	c.Assert(err, gocheck.IsNil)
	container, err := s.newContainer()
	c.Assert(err, gocheck.IsNil)
	defer rtesting.FakeRouter.RemoveBackend(container.AppName)
	err = container.remove()
	c.Assert(err, gocheck.IsNil)
	args := []string{"-R", container.IP}
	c.Assert(fexec.ExecutedCmd("ssh-keygen", args), gocheck.Equals, true)
	coll := s.conn.Collection(s.collName)
	err = coll.FindId(container.ID).One(&container)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "not found")
	c.Assert(rtesting.FakeRouter.HasRoute(container.AppName, container.getAddress()), gocheck.Equals, false)
}

func (s *S) TestContainerIP(c *gocheck.C) {
	err := s.newImage()
	c.Assert(err, gocheck.IsNil)
	cont, err := s.newContainer()
	c.Assert(err, gocheck.IsNil)
	defer cont.remove()
	ip, err := cont.ip()
	c.Assert(err, gocheck.IsNil)
	c.Assert(ip, gocheck.Not(gocheck.Equals), "")
}

func (s *S) TestContainerHostPort(c *gocheck.C) {
	err := s.newImage()
	c.Assert(err, gocheck.IsNil)
	cont, err := s.newContainer()
	c.Assert(err, gocheck.IsNil)
	defer rtesting.FakeRouter.RemoveBackend(cont.AppName)
	defer cont.remove()
	port, err := cont.hostPort()
	c.Assert(err, gocheck.IsNil)
	c.Assert(port, gocheck.Not(gocheck.Equals), "")
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
	err := s.newImage()
	c.Assert(err, gocheck.IsNil)
	cont, err := s.newContainer()
	c.Assert(err, gocheck.IsNil)
	defer cont.remove()
	defer rtesting.FakeRouter.RemoveBackend(cont.AppName)
	imageId, err := cont.commit()
	c.Assert(err, gocheck.IsNil)
	c.Assert(imageId, gocheck.Not(gocheck.Equals), "")
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
	err := s.newImage()
	c.Assert(err, gocheck.IsNil)
	app := testing.NewFakeApp("myapp", "python", 1)
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	var buf bytes.Buffer
	_, err = deploy(app, "ff13e", &buf)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestStart(c *gocheck.C) {
	err := s.newImage()
	c.Assert(err, gocheck.IsNil)
	app := testing.NewFakeApp("myapp", "python", 1)
	imageId := getImage(app)
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	var buf bytes.Buffer
	cont, err := start(app, imageId, &buf)
	c.Assert(err, gocheck.IsNil)
	defer cont.remove()
	c.Assert(cont.ID, gocheck.Not(gocheck.Equals), "")
	cont2, err := getContainer(cont.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(cont2.Image, gocheck.Equals, imageId)
	c.Assert(cont2.Status, gocheck.Equals, "running")
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
	err := s.newImage()
	c.Assert(err, gocheck.IsNil)
	cont, err := s.newContainer()
	c.Assert(err, gocheck.IsNil)
	defer cont.remove()
	defer rtesting.FakeRouter.RemoveBackend(cont.AppName)
	result, err := cont.stopped()
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.Equals, true)
}

func (s *S) TestContainerLogs(c *gocheck.C) {
	err := s.newImage()
	c.Assert(err, gocheck.IsNil)
	cont, err := s.newContainer()
	c.Assert(err, gocheck.IsNil)
	defer cont.remove()
	defer rtesting.FakeRouter.RemoveBackend(cont.AppName)
	var buff bytes.Buffer
	err = cont.logs(&buff)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buff.String(), gocheck.Not(gocheck.Equals), "")
}
