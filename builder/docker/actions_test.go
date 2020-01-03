// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"net/url"

	docker "github.com/fsouza/go-dockerclient"
	mgo "github.com/globalsign/mgo"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/types"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/safe"
	check "gopkg.in/check.v1"
)

func (s *S) TestCreateContainerName(c *check.C) {
	c.Assert(createContainer.Name, check.Equals, "create-container")
}

func builderClient(c *docker.Client) provision.BuilderDockerClient {
	return &dockercommon.PullAndCreateClient{Client: c}
}

func (s *S) TestCreateContainerForward(c *check.C) {
	config.Set("docker:user", "ubuntu")
	defer config.Unset("docker:user")
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(client, "tsuru/python", nil)
	c.Assert(err, check.IsNil)
	images, err := client.ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	cmds := []string{"ps", "-ef"}
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	app.SetEnv(bind.EnvVar{
		Name:  "env1",
		Value: "val1",
	})
	c.Assert(err, check.IsNil)
	cont := container.Container{Container: types.Container{Name: "myName", AppName: app.GetName(), Type: app.GetPlatform(), Status: "created"}}
	args := runContainerActionsArgs{
		app:           app,
		imageID:       images[0].ID,
		commands:      cmds,
		client:        builderClient(client),
		provisioner:   s.provisioner,
		buildingImage: builder.MockImageInfo{FakeBuildImageName: images[0].ID, FakeIsBuild: true},
		isDeploy:      true,
	}
	context := action.FWContext{Previous: cont, Params: []interface{}{args}}
	r, err := createContainer.Forward(context)
	c.Assert(err, check.IsNil)
	cont = r.(container.Container)
	defer cont.Remove(builderClient(client), limiter())
	c.Assert(cont, check.FitsTypeOf, container.Container{})
	c.Assert(cont.ID, check.Not(check.Equals), "")
	cc, err := client.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(cc.State.Running, check.Equals, false)
	c.Assert(cc.Config.User, check.Equals, "ubuntu")
	c.Assert(cc.Config.Env, check.DeepEquals, []string{"TSURU_HOST=tsuru.io"})
}

func (s *S) TestCreateContainerBackward(c *check.C) {
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(client, "tsuru/python", nil)
	c.Assert(err, check.IsNil)
	defer client.RemoveImage("tsuru/python")
	conta, err := s.newContainer(client)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(conta, client)
	cont := *conta
	args := runContainerActionsArgs{
		provisioner: s.provisioner,
		client:      builderClient(client),
	}
	context := action.BWContext{FWResult: cont, Params: []interface{}{args}}
	createContainer.Backward(context)
	_, err = client.InspectContainer(cont.ID)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, &docker.NoSuchContainer{})
}

func (s *S) TestUploadToContainerName(c *check.C) {
	c.Assert(uploadToContainer.Name, check.Equals, "upload-to-container")
}

func (s *S) TestUploadToContainerForward(c *check.C) {
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	conta, err := s.newContainer(client)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(conta, client)
	cont := *conta
	imgFile := bytes.NewBufferString("file data")
	context := action.FWContext{Previous: cont, Params: []interface{}{runContainerActionsArgs{
		provisioner: s.provisioner,
		app:         provisiontest.NewFakeApp("myapp", "python", 1),
		client:      builderClient(client),
		tarFile:     imgFile,
	}}}
	r, err := uploadToContainer.Forward(context)
	c.Assert(err, check.IsNil)
	cont = r.(container.Container)
	c.Assert(cont, check.FitsTypeOf, container.Container{})
}

func (s *S) TestStartContainerName(c *check.C) {
	c.Assert(startContainer.Name, check.Equals, "start-container")
}

func (s *S) TestStartContainerForward(c *check.C) {
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	conta, err := s.newContainer(client)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(conta, client)
	cont := *conta
	context := action.FWContext{Previous: cont, Params: []interface{}{runContainerActionsArgs{
		provisioner: s.provisioner,
		app:         provisiontest.NewFakeApp("myapp", "python", 1),
		client:      builderClient(client),
	}}}
	r, err := startContainer.Forward(context)
	c.Assert(err, check.IsNil)
	cont = r.(container.Container)
	c.Assert(cont, check.FitsTypeOf, container.Container{})
}

func (s *S) TestStartContainerBackward(c *check.C) {
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(client, "tsuru/python", nil)
	c.Assert(err, check.IsNil)
	defer client.RemoveImage("tsuru/python")
	conta, err := s.newContainer(client)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(conta, client)
	cont := *conta
	err = client.StartContainer(cont.ID, nil)
	c.Assert(err, check.IsNil)
	context := action.BWContext{FWResult: cont, Params: []interface{}{runContainerActionsArgs{
		provisioner: s.provisioner,
		client:      builderClient(client),
	}}}
	startContainer.Backward(context)
	cc, err := client.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(cc.State.Running, check.Equals, false)
}

func (s *S) TestFollowLogsAndCommitName(c *check.C) {
	c.Assert(followLogsAndCommit.Name, check.Equals, "follow-logs-and-commit")
}

func (s *S) TestFollowLogsAndCommitForward(c *check.C) {
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(client, "tsuru/python", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("mightyapp", "python", 1)
	nextImgName, err := image.AppNewImageName(app.GetName())
	c.Assert(err, check.IsNil)
	cont := container.Container{Container: types.Container{AppName: "mightyapp", ID: "myid123", BuildingImage: nextImgName.BaseImageName()}}
	err = cont.Create(&container.CreateArgs{
		App:      app,
		ImageID:  "tsuru/python",
		Commands: []string{"foo"},
		Client:   builderClient(client),
	})
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	args := runContainerActionsArgs{writer: buf, provisioner: s.provisioner, client: builderClient(client)}
	context := action.FWContext{Params: []interface{}{args}, Previous: cont}
	imageID, err := followLogsAndCommit.Forward(context)
	c.Assert(err, check.IsNil)
	c.Assert(imageID, check.Equals, "tsuru/app-mightyapp:v1")
	c.Assert(buf.String(), check.Not(check.Equals), "")
	_, err = client.InspectContainer(cont.ID)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Matches, "No such container.*")
	err = client.RemoveImage("tsuru/app-mightyapp:v1")
	c.Assert(err, check.IsNil)
}

func (s *S) TestFollowLogsAndCommitForwardNonZeroStatus(c *check.C) {
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(client, "tsuru/python", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	cont := container.Container{Container: types.Container{AppName: "mightyapp"}}
	err = cont.Create(&container.CreateArgs{
		App:      app,
		ImageID:  "tsuru/python",
		Commands: []string{"foo"},
		Client:   builderClient(client),
	})
	c.Assert(err, check.IsNil)
	err = s.server.MutateContainer(cont.ID, docker.State{ExitCode: 1})
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	args := runContainerActionsArgs{writer: buf, provisioner: s.provisioner, client: builderClient(client)}
	context := action.FWContext{Params: []interface{}{args}, Previous: cont}
	imageID, err := followLogsAndCommit.Forward(context)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Exit status 1")
	c.Assert(imageID, check.IsNil)
}

func (s *S) TestFollowLogsAndCommitForwardWaitFailure(c *check.C) {
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	s.server.PrepareFailure("failed to wait for the container", "/containers/.*/wait")
	defer s.server.ResetFailure("failed to wait for the container")
	err = s.newFakeImage(client, "tsuru/python", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	cont := container.Container{Container: types.Container{AppName: "mightyapp"}}
	err = cont.Create(&container.CreateArgs{
		App:      app,
		ImageID:  "tsuru/python",
		Commands: []string{"foo"},
		Client:   builderClient(client),
	})
	c.Assert(err, check.IsNil)
	err = cont.Start(&container.StartArgs{
		Client:  builderClient(client),
		Limiter: limiter(),
	})
	c.Assert(err, check.IsNil)
	err = cont.Stop(builderClient(client), limiter())
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	args := runContainerActionsArgs{writer: buf, provisioner: s.provisioner, client: builderClient(client)}
	context := action.FWContext{Params: []interface{}{args}, Previous: cont}
	imageID, err := followLogsAndCommit.Forward(context)
	c.Assert(err, check.ErrorMatches, `.*failed to wait for the container\n$`)
	c.Assert(imageID, check.IsNil)
}

func (s *S) TestUpdateAppBuilderImageName(c *check.C) {
	c.Assert(updateAppBuilderImage.Name, check.Equals, "update-app-builder-image")
}

func (s *S) TestUpdateAppBuilderImageForward(c *check.C) {
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(client, "tsuru/python", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("mightyapp", "python", 1)
	nextImgName, err := image.AppNewBuildImageName(app.GetName(), "", "")
	c.Assert(err, check.IsNil)
	cont := container.Container{Container: types.Container{AppName: "mightyapp", ID: "myid123", BuildingImage: nextImgName.BuildImageName()}}
	err = cont.Create(&container.CreateArgs{
		App:      app,
		ImageID:  "tsuru/python",
		Commands: []string{"foo"},
		Client:   builderClient(client),
	})
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	imgID, err := cont.Commit(builderClient(client), limiter(), buf, false)
	c.Assert(err, check.IsNil)
	c.Assert(imgID, check.Equals, "tsuru/app-mightyapp:v1-builder")
	c.Assert(buf.String(), check.Not(check.Equals), "")
	args := runContainerActionsArgs{app: app, writer: buf, provisioner: s.provisioner, client: builderClient(client), buildingImage: nextImgName}
	context := action.FWContext{Params: []interface{}{args}, Previous: cont}
	_, err = updateAppBuilderImage.Forward(context)
	c.Assert(err, check.IsNil)
	allImages, err := image.ListAppBuilderImages(args.app.GetName())
	c.Assert(err, check.IsNil)
	c.Assert(len(allImages), check.Equals, 1)
	c.Assert(allImages[0], check.Equals, "tsuru/app-mightyapp:v1-builder")
}

func (s *S) newContainer(client *docker.Client) (*container.Container, error) {
	container := container.Container{Container: types.Container{
		ID:          "id",
		IP:          "10.10.10.10",
		HostPort:    "3333",
		HostAddr:    "127.0.0.1",
		ProcessName: "web",
		ExposedPort: "8888/tcp",
	}}
	imageName := "tsuru/python:latest"
	var customData map[string]interface{}
	err := s.newFakeImage(client, imageName, customData)
	if err != nil {
		return nil, err
	}
	if container.AppName == "" {
		container.AppName = "container"
	}
	routertest.FakeRouter.AddBackend(routertest.FakeApp{Name: container.AppName})
	routertest.FakeRouter.AddRoutes(container.AppName, []*url.URL{container.Address()})
	ports := map[docker.Port]struct{}{
		docker.Port(s.port + "/tcp"): {},
	}
	config := docker.Config{
		Image:        imageName,
		Cmd:          []string{"ps"},
		ExposedPorts: ports,
	}
	createOptions := docker.CreateContainerOptions{Config: &config}
	createOptions.Name = randomString()
	c, err := client.CreateContainer(createOptions)
	if err != nil {
		return nil, err
	}
	container.ID = c.ID
	container.Image = imageName
	container.Name = createOptions.Name
	imageID, err := image.AppCurrentImageName(container.AppName)
	if err != nil {
		return nil, err
	}
	err = s.newFakeImage(client, imageID, nil)
	if err != nil {
		return nil, err
	}
	return &container, nil
}

func (s *S) removeTestContainer(c *container.Container, client *docker.Client) error {
	routertest.FakeRouter.RemoveBackend(c.AppName)
	return c.Remove(builderClient(client), limiter())
}

func (s *S) newFakeImage(client *docker.Client, repo string, customData map[string]interface{}) error {
	if customData == nil {
		customData = map[string]interface{}{
			"processes": map[string]interface{}{
				"web": "python myapp.py",
			},
		}
	}
	var buf safe.Buffer
	opts := docker.PullImageOptions{Repository: repo, OutputStream: &buf}
	err := image.SaveImageCustomData(repo, customData)
	if err != nil && !mgo.IsDup(err) {
		return err
	}
	return client.PullImage(opts, docker.AuthConfiguration{})
}
