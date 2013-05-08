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
	"github.com/globocom/tsuru/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	stdlog "log"
)

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
	runCmd := fmt.Sprintf("run -d -t -p %s tsuru/python /bin/bash -c /var/lib/tsuru/deploy git://%s/app-name.git && %s %s", s.port, s.gitHost, s.runBin, s.runArgs)
	inspectCmd := fmt.Sprintf("inspect %s", id)
	out := map[string][]byte{runCmd: []byte(id), inspectCmd: []byte(inspectOut)}
	fexec := &etesting.FakeExecutor{Output: out}
	execut = fexec
	defer func() {
		execut = nil
	}()
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
	execut = fexec
	defer func() {
		execut = nil
	}()
	app := testing.NewFakeApp("app-name", "python", 1)
	newContainer(app)
	appRepo := fmt.Sprintf("git://%s/app-name.git", s.gitHost)
	containerCmd := fmt.Sprintf("/var/lib/tsuru/deploy %s && %s %s", appRepo, s.runBin, s.runArgs)
	args := []string{
		"run", "-d", "-t", "-p", s.port, "tsuru/python",
		"/bin/bash", "-c",
		containerCmd,
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
	c.Assert(container, gocheck.IsNil)
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
	fexec := &etesting.FakeExecutor{Output: map[string][]byte{"*": []byte(out)}}
	execut = fexec
	defer func() {
		execut = nil
	}()
	app := testing.NewFakeApp("myapp", "python", 1)
	cont, err := newContainer(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(s.collName).RemoveId(cont.Id)
	r, err := getRouter()
	c.Assert(err, gocheck.IsNil)
	addr, err := r.Addr(app.GetName())
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, "10.10.10.10")
}

func (s *S) TestRunContainerCmdReturnsCommandToRunContainer(c *gocheck.C) {
	app := testing.NewFakeApp("myapp", "python", 1)
	cmd, port, err := runContainerCmd(app)
	c.Assert(err, gocheck.IsNil)
	appRepo := fmt.Sprintf("git://%s/myapp.git", s.gitHost)
	containerCmd := fmt.Sprintf("%s %s && %s %s", s.deployCmd, appRepo, s.runBin, s.runArgs)
	expected := []string{"docker", "run", "-d", "-t", "-p", s.port, fmt.Sprintf("%s/python", s.repoNamespace), "/bin/bash", "-c", containerCmd}
	c.Assert(cmd, gocheck.DeepEquals, expected)
	c.Assert(port, gocheck.Equals, s.port)
}

func (s *S) TestDockerCreate(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	container := container{AppName: "app-name", Type: "python"}
	app := testing.NewFakeApp("app-name", "python", 1)
	err := container.create(app)
	c.Assert(err, gocheck.IsNil)
	appRepo := fmt.Sprintf("git://%s/app-name.git", s.gitHost)
	containerCmd := fmt.Sprintf("/var/lib/tsuru/deploy %s && %s %s", appRepo, s.runBin, s.runArgs)
	args := []string{
		"run", "-d", "-t", "-p", s.port, "tsuru/python",
		"/bin/bash", "-c",
		containerCmd,
	}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestDockerStart(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	container := container{AppName: "container", Id: "123"}
	err := container.start()
	c.Assert(err, gocheck.IsNil)
	args := []string{"start", "123"}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestDockerStop(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	container := container{AppName: "container", Id: "id"}
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
	container := container{AppName: "container", Id: "id"}
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
	containers, err := getContainers("something")
	c.Assert(err, gocheck.IsNil)
	c.Assert(containers, gocheck.HasLen, 2)
	c.Assert(containers[0].Id, gocheck.Equals, "abcdef")
	c.Assert(containers[1].Id, gocheck.Equals, "fedajs")
	containers, err = getContainers("otherthing")
	c.Assert(err, gocheck.IsNil)
	c.Assert(containers, gocheck.HasLen, 1)
	c.Assert(containers[0].Id, gocheck.Equals, "wat")
	containers, err = getContainers("unknown")
	c.Assert(err, gocheck.IsNil)
	c.Assert(containers, gocheck.HasLen, 0)
}
