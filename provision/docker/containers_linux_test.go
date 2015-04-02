// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"

	dtesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/safe"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestRebalanceContainersManyAppsSegStress(c *check.C) {
	var nodes []cluster.Node
	var nodeHosts []string
	for i := 0; i < 6; i++ {
		newIp := fmt.Sprintf("127.0.0.%d", i+1)
		otherServer, err := dtesting.NewServer(newIp+":0", nil, nil)
		c.Assert(err, check.IsNil)
		nodes = append(nodes, cluster.Node{Address: otherServer.URL(), Metadata: map[string]string{"pool": "pool1"}})
		nodeHosts = append(nodeHosts, urlToHost(otherServer.URL()))
	}
	var err error
	p := &dockerProvisioner{}
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.storage, err = buildClusterStorage()
	c.Assert(err, check.IsNil)
	p.scheduler = &segregatedScheduler{provisioner: p}
	p.cluster, err = cluster.New(p.scheduler, p.storage, nodes...)
	c.Assert(err, check.IsNil)
	err = provision.AddPool("pool1")
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool("pool1", []string{"team1"})
	c.Assert(err, check.IsNil)
	variation := []int{10, 20, 30, 40, 50, 100}
	maxContainers := 40
	for i := 0; i < maxContainers; i++ {
		appName := fmt.Sprintf("myapp-%d", i)
		err := s.newFakeImage(p, "tsuru/app-"+appName)
		c.Assert(err, check.IsNil)
		appInstance := provisiontest.NewFakeApp(appName, "python", 0)
		defer p.Destroy(appInstance)
		p.Provision(appInstance)
		imageId, err := appCurrentImageName(appInstance.GetName())
		c.Assert(err, check.IsNil)
		var chosenNode string
		for j := range variation {
			if i < (maxContainers*variation[j])/100 {
				chosenNode = nodeHosts[j]
				break
			}
		}
		args := changeUnitsPipelineArgs{
			app:         appInstance,
			unitsToAdd:  6,
			imageId:     imageId,
			provisioner: p,
			toHost:      chosenNode,
		}
		pipeline := action.NewPipeline(
			&provisionAddUnitsToHost,
			&bindAndHealthcheck,
			&addNewRoutes,
			&updateAppImage,
		)
		err = pipeline.Execute(args)
		c.Assert(err, check.IsNil)
		appStruct := &app.App{
			Name:      appInstance.GetName(),
			TeamOwner: "team1",
			Pool:      "pool1",
		}
		conn, err := db.Conn()
		c.Assert(err, check.IsNil)
		err = conn.Apps().Insert(appStruct)
		c.Assert(err, check.IsNil)
		defer conn.Apps().Remove(bson.M{"name": appStruct.Name})
	}
	buf := safe.NewBuffer(nil)
	_, err = p.rebalanceContainersByFilter(buf, []string{}, map[string]string{"pool": "pool1"}, false)
	c.Assert(err, check.IsNil)
	for i := range nodeHosts {
		conts, err := p.listContainersByHost(nodeHosts[i])
		c.Assert(err, check.IsNil)
		c.Assert(len(conts), check.Equals, 240/len(nodeHosts))
	}
}
