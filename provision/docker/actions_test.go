// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	dockerClient "github.com/fsouza/go-dockerclient"
	"github.com/globocom/tsuru/action"
	"launchpad.net/gocheck"
)

func (s *S) TestCreateContainerName(c *gocheck.C) {
	c.Assert(createContainer.Name, gocheck.Equals, "create-container")
}

func (s *S) TestCreateContainerForward(c *gocheck.C) {
	err := s.newImage()
	c.Assert(err, gocheck.IsNil)
	client, err := dockerClient.NewClient(s.server.URL())
	c.Assert(err, gocheck.IsNil)
	images, err := client.ListImages(true)
	c.Assert(err, gocheck.IsNil)
	cmds := []string{"ps", "-ef"}
	context := action.FWContext{Params: []interface{}{images[0].ID, cmds}}
	r, err := createContainer.Forward(context)
	c.Assert(err, gocheck.IsNil)
	cont := r.(container)
	defer cont.remove()
	c.Assert(cont, gocheck.FitsTypeOf, container{})
}

func (s *S) TestCreateContainerBackward(c *gocheck.C) {
	cont := container{ID: "ble"}
	context := action.BWContext{Params: []interface{}{cont}}
	createContainer.Backward(context)
}

func (s *S) TestInsertContainerName(c *gocheck.C) {
	c.Assert(insertContainer.Name, gocheck.Equals, "insert-container")
}

func (s *S) TestInsertContainerForward(c *gocheck.C) {
	cont := container{ID: "someid"}
	context := action.FWContext{Params: []interface{}{cont}}
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
	context := action.BWContext{Params: []interface{}{cont}}
	insertContainer.Backward(context)
	err = coll.FindId(cont.ID).One(&cont)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "not found")
}

func (s *S) TestAddRouteName(c *gocheck.C) {
	c.Assert(addRoute.Name, gocheck.Equals, "add-route")
}
