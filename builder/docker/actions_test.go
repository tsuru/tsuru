// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"context"
	"net/url"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/types"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/safe"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
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
	cmds := []string{"ps", "-ef"}
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	version := newVersionForApp(c, client, app, nil)
	app.SetEnv(bind.EnvVar{
		Name:  "env1",
		Value: "val1",
	})
	c.Assert(err, check.IsNil)
	buildImg, err := image.GetBuildImage(context.TODO(), app)
	c.Assert(err, check.IsNil)
	cont := container.Container{Container: types.Container{Name: "myName", AppName: app.GetName(), Type: app.GetPlatform(), Status: "created"}}
	args := runContainerActionsArgs{
		app:         app,
		imageID:     buildImg,
		commands:    cmds,
		client:      builderClient(client),
		provisioner: s.provisioner,
		version:     version,
		isDeploy:    true,
	}
	context := action.FWContext{Previous: cont, Params: []interface{}{args}}
	r, err := createContainer.Forward(context)
	c.Assert(err, check.IsNil)
	cont = r.(container.Container)
	defer cont.Remove(builderClient(client), limiter())
	c.Assert(cont, check.FitsTypeOf, container.Container{})
	c.Assert(cont.ID, check.Not(check.Equals), "")
	cc, err := client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: cont.ID})
	c.Assert(err, check.IsNil)
	c.Assert(cc.State.Running, check.Equals, false)
	c.Assert(cc.Config.User, check.Equals, "ubuntu")
	c.Assert(cc.Config.Env, check.DeepEquals, []string{"TSURU_HOST=tsuru.io"})
}

func (s *S) TestCreateContainerBackward(c *check.C) {
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	conta := s.newContainer(c, client)
	cont := *conta
	args := runContainerActionsArgs{
		provisioner: s.provisioner,
		client:      builderClient(client),
	}
	context := action.BWContext{FWResult: cont, Params: []interface{}{args}}
	createContainer.Backward(context)
	_, err = client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: cont.ID})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, &docker.NoSuchContainer{})
}

func (s *S) TestUploadToContainerName(c *check.C) {
	c.Assert(uploadToContainer.Name, check.Equals, "upload-to-container")
}

func (s *S) TestUploadToContainerForward(c *check.C) {
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	conta := s.newContainer(c, client)
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
	conta := s.newContainer(c, client)
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
	conta := s.newContainer(c, client)
	cont := *conta
	err = client.StartContainer(cont.ID, nil)
	c.Assert(err, check.IsNil)
	context := action.BWContext{FWResult: cont, Params: []interface{}{runContainerActionsArgs{
		provisioner: s.provisioner,
		client:      builderClient(client),
	}}}
	startContainer.Backward(context)
	cc, err := client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: cont.ID})
	c.Assert(err, check.IsNil)
	c.Assert(cc.State.Running, check.Equals, false)
}

func (s *S) TestFollowLogsAndCommitName(c *check.C) {
	c.Assert(followLogsAndCommit.Name, check.Equals, "follow-logs-and-commit")
}

func (s *S) TestFollowLogsAndCommitForward(c *check.C) {
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("mightyapp", "python", 1)
	version := newVersionForApp(c, client, app, nil)
	cont := container.Container{Container: types.Container{AppName: "mightyapp", ID: "myid123", BuildingImage: version.BaseImageName()}}
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
	_, err = client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: cont.ID})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Matches, "No such container.*")
	err = client.RemoveImage("tsuru/app-mightyapp:v1")
	c.Assert(err, check.IsNil)
}

func (s *S) TestFollowLogsAndCommitForwardNonZeroStatus(c *check.C) {
	client, err := docker.NewClient(s.server.URL())
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
	app := provisiontest.NewFakeApp("mightyapp", "python", 1)
	version := newVersionForApp(c, client, app, nil)
	cont := container.Container{Container: types.Container{AppName: "mightyapp", ID: "myid123", BuildingImage: version.BuildImageName()}}
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
	args := runContainerActionsArgs{app: app, writer: buf, provisioner: s.provisioner, client: builderClient(client), version: version}
	context := action.FWContext{Params: []interface{}{args}, Previous: cont}
	_, err = updateAppBuilderImage.Forward(context)
	c.Assert(err, check.IsNil)
	c.Assert(version.VersionInfo().BuildImage, check.Equals, version.BuildImageName())
}

func (s *S) newContainer(c *check.C, client *docker.Client) *container.Container {
	container := container.Container{Container: types.Container{
		ID:          "id",
		IP:          "10.10.10.10",
		HostPort:    "3333",
		HostAddr:    "127.0.0.1",
		ProcessName: "web",
		ExposedPort: "8888/tcp",
		AppName:     "container",
	}}
	fakeApp := provisiontest.NewFakeApp(container.AppName, "python", 0)
	version := newVersionForApp(c, client, fakeApp, nil)
	routertest.FakeRouter.AddBackend(context.TODO(), routertest.FakeApp{Name: container.AppName})
	routertest.FakeRouter.AddRoutes(context.TODO(), fakeApp, []*url.URL{container.Address()})
	ports := map[docker.Port]struct{}{
		docker.Port(s.port + "/tcp"): {},
	}
	config := docker.Config{
		Image:        version.BuildImageName(),
		Cmd:          []string{"ps"},
		ExposedPorts: ports,
	}
	createOptions := docker.CreateContainerOptions{Config: &config}
	createOptions.Name = randomString()
	cont, err := client.CreateContainer(createOptions)
	c.Assert(err, check.IsNil)
	container.ID = cont.ID
	container.Image = version.BuildImageName()
	container.Name = createOptions.Name
	return &container
}

func newVersionForApp(c *check.C, client *docker.Client, a provision.App, customData map[string]interface{}) appTypes.AppVersion {
	if customData == nil {
		customData = map[string]interface{}{
			"processes": map[string]interface{}{
				"web": "python myapp.py",
			},
		}
	}
	version, err := servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
		App: a,
	})
	c.Assert(err, check.IsNil)
	err = version.AddData(appTypes.AddVersionDataArgs{
		CustomData: customData,
	})
	c.Assert(err, check.IsNil)
	client.PullImage(docker.PullImageOptions{
		Repository: version.BuildImageName(),
	}, docker.AuthConfiguration{})
	return version
}
