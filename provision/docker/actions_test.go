// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	dockerClient "github.com/fsouza/go-dockerclient"
	"github.com/globocom/tsuru/action"
	rtesting "github.com/globocom/tsuru/router/testing"
	"github.com/globocom/tsuru/testing"
	"launchpad.net/gocheck"
)

func (s *S) TestCreateContainerName(c *gocheck.C) {
	c.Assert(createContainer.Name, gocheck.Equals, "create-container")
}

func (s *S) TestCreateContainerForward(c *gocheck.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	client, err := dockerClient.NewClient(s.server.URL())
	c.Assert(err, gocheck.IsNil)
	images, err := client.ListImages(true)
	c.Assert(err, gocheck.IsNil)
	cmds := []string{"ps", "-ef"}
	app := testing.NewFakeApp("myapp", "python", 1)
	context := action.FWContext{Params: []interface{}{app, images[0].ID, cmds}}
	r, err := createContainer.Forward(context)
	c.Assert(err, gocheck.IsNil)
	cont := r.(container)
	defer cont.remove()
	c.Assert(cont, gocheck.FitsTypeOf, container{})
	c.Assert(cont.AppName, gocheck.Equals, app.GetName())
	c.Assert(cont.Type, gocheck.Equals, app.GetPlatform())
	port, err := getPort()
	c.Assert(err, gocheck.IsNil)
	c.Assert(cont.Port, gocheck.Equals, port)
}

func (s *S) TestCreateContainerBackward(c *gocheck.C) {
	cont := container{ID: "ble"}
	context := action.BWContext{FWResult: cont}
	createContainer.Backward(context)
}

func (s *S) TestInsertContainerName(c *gocheck.C) {
	c.Assert(insertContainer.Name, gocheck.Equals, "insert-container")
}

func (s *S) TestInsertContainerForward(c *gocheck.C) {
	cont := container{ID: "someid"}
	context := action.FWContext{Previous: cont}
	r, err := insertContainer.Forward(context)
	c.Assert(err, gocheck.IsNil)
	coll := s.conn.Collection(s.collName)
	defer coll.RemoveId(cont.ID)
	cont = r.(container)
	var retrieved container
	err = coll.FindId(cont.ID).One(&retrieved)
	c.Assert(retrieved.ID, gocheck.Equals, cont.ID)
	c.Assert(retrieved.Status, gocheck.Equals, "created")
	c.Assert(cont, gocheck.FitsTypeOf, container{})
}

func (s *S) TestInsertContainerBackward(c *gocheck.C) {
	cont := container{ID: "someid"}
	coll := s.conn.Collection(s.collName)
	err := coll.Insert(&cont)
	c.Assert(err, gocheck.IsNil)
	context := action.BWContext{FWResult: cont}
	insertContainer.Backward(context)
	err = coll.FindId(cont.ID).One(&cont)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "not found")
}

func (s *S) TestAddRouteName(c *gocheck.C) {
	c.Assert(addRoute.Name, gocheck.Equals, "add-route")
}

func (s *S) TestAddRouteForward(c *gocheck.C) {
	app := testing.NewFakeApp("myapp", "python", 1)
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	cont := container{ID: "ble", AppName: app.GetName()}
	context := action.FWContext{Previous: cont}
	r, err := addRoute.Forward(context)
	c.Assert(err, gocheck.IsNil)
	cont = r.(container)
	hasRoute := rtesting.FakeRouter.HasRoute(app.GetName(), cont.getAddress())
	c.Assert(hasRoute, gocheck.Equals, true)
	c.Assert(cont, gocheck.FitsTypeOf, container{})
}

func (s *S) TestSetNetworkInfoName(c *gocheck.C) {
	c.Assert(setNetworkInfo.Name, gocheck.Equals, "set-network-info")
}

func (s *S) TestSetNetworkInfoForward(c *gocheck.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	conta, err := s.newContainer()
	c.Assert(err, gocheck.IsNil)
	defer rtesting.FakeRouter.RemoveBackend(conta.AppName)
	cont := *conta
	context := action.FWContext{Previous: cont}
	r, err := setNetworkInfo.Forward(context)
	c.Assert(err, gocheck.IsNil)
	cont = r.(container)
	c.Assert(cont, gocheck.FitsTypeOf, container{})
	c.Assert(cont.IP, gocheck.Not(gocheck.Equals), "")
	c.Assert(cont.HostPort, gocheck.Not(gocheck.Equals), "")
}

func (s *S) TestSetImage(c *gocheck.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	conta, err := s.newContainer()
	c.Assert(err, gocheck.IsNil)
	defer rtesting.FakeRouter.RemoveBackend(conta.AppName)
	cont := *conta
	context := action.FWContext{Previous: cont}
	r, err := setNetworkInfo.Forward(context)
	c.Assert(err, gocheck.IsNil)
	cont = r.(container)
	c.Assert(cont, gocheck.FitsTypeOf, container{})
	c.Assert(cont.HostPort, gocheck.Not(gocheck.Equals), "")
}

func (s *S) TestStartContainer(c *gocheck.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	conta, err := s.newContainer()
	c.Assert(err, gocheck.IsNil)
	defer conta.remove()
	defer rtesting.FakeRouter.RemoveBackend(conta.AppName)
	cont := *conta
	context := action.FWContext{Previous: cont}
	r, err := startContainer.Forward(context)
	c.Assert(err, gocheck.IsNil)
	cont = r.(container)
	c.Assert(cont, gocheck.FitsTypeOf, container{})
}
