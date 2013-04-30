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
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	stdlog "log"
)

func (s *S) TestNewContainer(c *gocheck.C) {
	id := "945132e7b4c9"
	tmpdir, err := commandmocker.Add("docker", id)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	app := testing.NewFakeApp("app-name", "python", 1)
	container, _ := newContainer(app, deployContainerCmd)
	c.Assert(container.name, gocheck.Equals, "app-name")
	c.Assert(container.id, gocheck.Equals, id)
}

func (s *S) TestNewContainerCallsDockerCreate(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	app := testing.NewFakeApp("app-name", "python", 1)
	newContainer(app, deployContainerCmd)
	args := []string{"run", "-d", fmt.Sprintf("%s/python", s.repoNamespace), fmt.Sprintf("/var/lib/tsuru/deploy git://%s/app-name.git", s.gitHost)}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestNewContainerInsertContainerOnDatabase(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	app := testing.NewFakeApp("app-name", "python", 1)
	newContainer(app, deployContainerCmd)
	u := provision.Unit{}
	err := s.conn.Collection(s.collName).Find(bson.M{"name": "app-name"}).One(&u)
	c.Assert(err, gocheck.IsNil)
	c.Assert(u.Type, gocheck.Equals, "python")
}

func (s *S) TestNewContainerReturnsContainerWithoutIdAndLogsOnError(c *gocheck.C) {
	w := new(bytes.Buffer)
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	tmpdir, err := commandmocker.Error("docker", "cool error", 1)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	app := testing.NewFakeApp("myapp", "python", 1)
	container, err := newContainer(app, deployContainerCmd)
	c.Assert(err, gocheck.NotNil)
	c.Assert(container.id, gocheck.Equals, "")
	c.Assert(w.String(), gocheck.Matches, "(?s).*Error creating container myapp.*")
}

func (s *S) TestNewContainerAddsRoute(c *gocheck.C) {
	out := `
    {
            "NetworkSettings": {
            "IpAddress": "10.10.10.10",
            "IpPrefixLen": 8,
            "Gateway": "10.65.41.1",
            "PortMapping": {}
    }
}`
	fexec := &etesting.FakeExecutor{Output: []byte(out)}
	execut = fexec
	defer func() {
		execut = nil
	}()
	app := testing.NewFakeApp("myapp", "python", 1)
	_, err := newContainer(app, runContainerCmd)
	c.Assert(err, gocheck.IsNil)
	r, err := getRouter()
	c.Assert(err, gocheck.IsNil)
	addr, err := r.Addr(app.GetName())
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, "10.10.10.10")
}

func (s *S) TestDeployContainerCmdReturnsCommandToDeployContainer(c *gocheck.C) {
	app := testing.NewFakeApp("myapp", "python", 1)
	cmd, err := deployContainerCmd(app)
	c.Assert(err, gocheck.IsNil)
	deployCmd, err := config.GetString("docker:deploy-cmd")
	c.Assert(err, gocheck.IsNil)
	deployCmd = fmt.Sprintf("%s git://%s/myapp.git", deployCmd, s.gitHost)
	expected := []string{"docker", "run", "-d", fmt.Sprintf("%s/python", s.repoNamespace), deployCmd}
	c.Assert(cmd, gocheck.DeepEquals, expected)
}

func (s *S) TestRunContainerCmdReturnsCommandToRunContainer(c *gocheck.C) {
	app := testing.NewFakeApp("myapp", "python", 1)
	cmd, err := runContainerCmd(app)
	c.Assert(err, gocheck.IsNil)
	runBin, err := config.GetString("docker:run-cmd:bin")
	c.Assert(err, gocheck.IsNil)
	runArgs, err := config.GetString("docker:run-cmd:args")
	c.Assert(err, gocheck.IsNil)
	port, err := config.GetString("docker:run-cmd:port")
	c.Assert(err, gocheck.IsNil)
	runCmd := fmt.Sprintf("%s %s", runBin, runArgs)
	expected := []string{"docker", "run", "-d", "-p", port, fmt.Sprintf("%s/myapp", s.repoNamespace), runCmd}
	c.Assert(cmd, gocheck.DeepEquals, expected)
}

func (s *S) TestDockerCreate(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	container := container{name: "container"}
	app := testing.NewFakeApp("app-name", "python", 1)
	_, err := container.create(app, deployContainerCmd)
	c.Assert(err, gocheck.IsNil)
	args := []string{"run", "-d", fmt.Sprintf("%s/python", s.repoNamespace), fmt.Sprintf("/var/lib/tsuru/deploy git://%s/app-name.git", s.gitHost)}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestDockerStart(c *gocheck.C) {
	container := container{name: "container"}
	err := container.start()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestDockerStop(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	container := container{name: "container", id: "id"}
	err := container.stop()
	c.Assert(err, gocheck.IsNil)
	args := []string{"stop", "id"}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestDockerDestroy(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	container := container{name: "container", id: "id"}
	err := container.remove()
	c.Assert(err, gocheck.IsNil)
	args := []string{"rm", "id"}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestContainerIPRunsDockerInspectCommand(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	cont := container{name: "vm1", id: "id"}
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
	cont := container{name: "vm1", id: "id"}
	ip, err := cont.ip()
	c.Assert(err, gocheck.IsNil)
	c.Assert(ip, gocheck.Equals, "10.10.10.10")
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
}

func (s *S) TestImageCommit(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
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
	execut = fexec
	defer func() {
		execut = nil
	}()
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
