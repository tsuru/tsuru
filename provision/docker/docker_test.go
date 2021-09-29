// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"context"
	"net/url"
	"sort"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/clusterclient"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/types"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

type newContainerOpts struct {
	AppName         string
	Status          string
	Image           string
	ProcessName     string
	ImageCustomData map[string]interface{}
}

func (s *S) newContainer(opts *newContainerOpts, p *dockerProvisioner) (*container.Container, error) {
	container := container.Container{
		Container: types.Container{
			ID:          "id",
			IP:          "10.10.10.10",
			HostPort:    "3333",
			HostAddr:    "127.0.0.1",
			ProcessName: "web",
			ExposedPort: "8888/tcp",
		},
	}
	if p == nil {
		p = s.p
	}
	var imageID string
	var customData map[string]interface{}
	if opts != nil {
		if opts.ProcessName != "" {
			container.ProcessName = opts.ProcessName
		}
		customData = opts.ImageCustomData
		container.AppName = opts.AppName
		imageID = opts.Image
		container.SetStatus(p.ClusterClient(), provision.Status(opts.Status), false)
	}
	if container.AppName == "" {
		container.AppName = "container"
	}
	fakeApp := provisiontest.NewFakeApp(container.AppName, "python", 0)
	if imageID == "" {
		version, err := servicemanager.AppVersion.LatestSuccessfulVersion(context.TODO(), fakeApp)
		if err == appTypes.ErrNoVersionsAvailable {
			version, err = newSuccessfulVersionForApp(p, fakeApp, customData)
		}
		if err != nil {
			return nil, err
		}
		if opts != nil && opts.Status == provision.StatusBuilding.String() {
			testBuildImage, err := version.BuildImageName()
			if err != nil {
				return nil, err
			}
			testBaseImage, err := version.BaseImageName()
			if err != nil {
				return nil, err
			}
			imageID = testBuildImage
			container.BuildingImage = testBaseImage
		} else {
			testBaseImage, err := version.BaseImageName()
			if err != nil {
				return nil, err
			}
			imageID = testBaseImage
		}
	} else {
		err := p.Cluster().PullImage(docker.PullImageOptions{
			Repository: imageID,
		}, docker.AuthConfiguration{})
		if err != nil {
			return nil, err
		}
	}

	app := routertest.FakeApp{Name: container.AppName}
	routertest.FakeRouter.AddBackend(context.TODO(), app)
	routertest.FakeRouter.AddRoutes(context.TODO(), app, []*url.URL{container.Address()})
	ports := map[docker.Port]struct{}{
		docker.Port(s.port + "/tcp"): {},
	}
	config := docker.Config{
		Image:        imageID,
		Cmd:          []string{"ps"},
		ExposedPorts: ports,
	}
	createOptions := docker.CreateContainerOptions{Config: &config}
	createOptions.Name = randomString()
	_, c, err := p.Cluster().CreateContainer(createOptions, net.StreamInactivityTimeout)
	if err != nil {
		return nil, err
	}
	container.ID = c.ID
	container.Image = imageID
	container.Name = createOptions.Name
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Collection(s.collName).Insert(&container)
	if err != nil {
		return nil, err
	}
	return &container, nil
}

func (s *S) removeTestContainer(c *container.Container) error {
	routertest.FakeRouter.RemoveBackend(context.TODO(), routertest.FakeApp{Name: c.AppName})
	return c.Remove(s.p.ClusterClient(), s.p.ActionLimiter())
}

func newSuccessfulVersionForApp(p *dockerProvisioner, a provision.App, customData map[string]interface{}) (appTypes.AppVersion, error) {
	version, err := newVersionForApp(p, a, customData)
	if err != nil {
		return nil, err
	}
	err = version.CommitBaseImage()
	if err != nil {
		return nil, err
	}
	err = version.CommitSuccessful()
	if err != nil {
		return nil, err
	}
	return version, nil
}

func newVersionForApp(p *dockerProvisioner, a provision.App, customData map[string]interface{}) (appTypes.AppVersion, error) {
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
	if err != nil {
		return nil, err
	}
	err = version.AddData(appTypes.AddVersionDataArgs{
		CustomData: customData,
	})
	if err != nil {
		return nil, err
	}
	err = version.CommitBuildImage()
	if err != nil {
		return nil, err
	}
	testBuildImage, err := version.BuildImageName()
	if err != nil {
		return nil, err
	}
	p.Cluster().PullImage(docker.PullImageOptions{
		Repository: testBuildImage,
	}, docker.AuthConfiguration{})
	return version, nil
}

func (s *S) TestGetContainer(c *check.C) {
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(
		container.Container{Container: types.Container{ID: "abcdef", Type: "python"}},
		container.Container{Container: types.Container{ID: "fedajs", Type: "ruby"}},
		container.Container{Container: types.Container{ID: "wat", Type: "java"}},
	)
	container, err := s.p.GetContainer("abcdef")
	c.Assert(err, check.IsNil)
	c.Assert(container.ID, check.Equals, "abcdef")
	c.Assert(container.Type, check.Equals, "python")
	container, err = s.p.GetContainer("wut")
	c.Assert(container, check.IsNil)
	c.Assert(err, check.NotNil)
	e, ok := err.(*provision.UnitNotFoundError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.ID, check.Equals, "wut")
}

func (s *S) TestGetContainers(c *check.C) {
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(
		container.Container{Container: types.Container{ID: "abcdef", Type: "python", AppName: "something"}},
		container.Container{Container: types.Container{ID: "fedajs", Type: "python", AppName: "something"}},
		container.Container{Container: types.Container{ID: "wat", Type: "java", AppName: "otherthing"}},
	)
	containers, err := s.p.listContainersByApp("something")
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	ids := []string{containers[0].ID, containers[1].ID}
	sort.Strings(ids)
	c.Assert(ids[0], check.Equals, "abcdef")
	c.Assert(ids[1], check.Equals, "fedajs")
	containers, err = s.p.listContainersByApp("otherthing")
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	c.Assert(containers[0].ID, check.Equals, "wat")
	containers, err = s.p.listContainersByApp("unknown")
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 0)
}

func (s *S) TestStart(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	version, err := newVersionForApp(s.p, app, nil)
	c.Assert(err, check.IsNil)
	err = version.CommitBaseImage()
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.AddBackend(context.TODO(), app)
	var buf bytes.Buffer
	cmdData, err := dockercommon.ContainerCmdsDataFromVersion(version)
	c.Assert(err, check.IsNil)
	cont, err := s.p.start(context.TODO(), &container.Container{Container: types.Container{ProcessName: "web"}}, app, cmdData, version, &buf, "")
	c.Assert(err, check.IsNil)
	c.Assert(cont.ID, check.Not(check.Equals), "")
	cont2, err := s.p.GetContainer(cont.ID)
	c.Assert(err, check.IsNil)
	testBaseImage, err := version.BaseImageName()
	c.Assert(err, check.IsNil)
	c.Assert(cont2.Image, check.Equals, testBaseImage)
	c.Assert(cont2.Status, check.Equals, provision.StatusStarting.String())
}

func (s *S) TestStartStoppedContainer(c *check.C) {
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	cont.Status = provision.StatusStopped.String()
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	version, err := newVersionForApp(s.p, app, nil)
	c.Assert(err, check.IsNil)
	err = version.CommitBaseImage()
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.AddBackend(context.TODO(), app)
	cmdData, err := dockercommon.ContainerCmdsDataFromVersion(version)
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	cont, err = s.p.start(context.TODO(), cont, app, cmdData, version, &buf, "")
	c.Assert(err, check.IsNil)
	c.Assert(cont.ID, check.Not(check.Equals), "")
	cont2, err := s.p.GetContainer(cont.ID)
	c.Assert(err, check.IsNil)
	testBaseImage, err := version.BaseImageName()
	c.Assert(err, check.IsNil)
	c.Assert(cont2.Image, check.Equals, testBaseImage)
	c.Assert(cont2.Status, check.Equals, provision.StatusStopped.String())
}

func (s *S) TestProvisionerGetCluster(c *check.C) {
	config.Set("docker:cluster:redis-server", "127.0.0.1:6379")
	defer config.Unset("docker:cluster:redis-server")
	var p dockerProvisioner
	err := p.Initialize()
	c.Assert(err, check.IsNil)
	clus := p.Cluster()
	c.Assert(clus, check.NotNil)
	currentNodes, err := clus.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(currentNodes, check.HasLen, 0)
	c.Assert(p.scheduler, check.NotNil)
}

func (s *S) TestBuildClusterStorage(c *check.C) {
	defer config.Set("docker:cluster:mongo-url", "127.0.0.1:27017?maxPoolSize=100")
	defer config.Set("docker:cluster:mongo-database", "docker_provision_tests_cluster_stor")
	config.Unset("docker:cluster:mongo-url")
	_, err := buildClusterStorage()
	c.Assert(err, check.ErrorMatches, ".*docker:cluster:{mongo-url,mongo-database} must be set.")
	config.Set("docker:cluster:mongo-url", "127.0.0.1:27017?maxPoolSize=100")
	config.Unset("docker:cluster:mongo-database")
	_, err = buildClusterStorage()
	c.Assert(err, check.ErrorMatches, ".*docker:cluster:{mongo-url,mongo-database} must be set.")
	config.Set("docker:cluster:storage", "xxxx")
}

func (s *S) TestGetClient(c *check.C) {
	p := &dockerProvisioner{storage: &cluster.MapStorage{}}
	err := p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster, err = cluster.New(nil, p.storage, "")
	c.Assert(err, check.IsNil)
	client, err := p.GetClient(nil)
	c.Assert(err, check.IsNil)
	clusterClient := client.(*clusterclient.ClusterClient)
	c.Assert(clusterClient.Cluster, check.Equals, p.Cluster())
	c.Assert(clusterClient.Limiter, check.Equals, p.ActionLimiter())
	c.Assert(clusterClient.Collection, check.NotNil)
}
