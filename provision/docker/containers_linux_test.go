// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"

	dtesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/safe"
	"gopkg.in/check.v1"
)

func (s *S) TestRebalanceContainersManyAppsSegStress(c *check.C) {
	var nodes []cluster.Node
	var nodeHosts []string
	for i := 0; i < 6; i++ {
		newIp := fmt.Sprintf("127.0.0.%d", i+1)
		otherServer, err := dtesting.NewServer(newIp+":0", nil, nil)
		c.Assert(err, check.IsNil)
		defer otherServer.Stop()
		nodes = append(nodes, cluster.Node{Address: otherServer.URL(), Metadata: map[string]string{"pool": "pool1"}})
		nodeHosts = append(nodeHosts, net.URLToHost(otherServer.URL()))
	}
	var err error
	p := &dockerProvisioner{}
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.storage, err = buildClusterStorage()
	c.Assert(err, check.IsNil)
	p.scheduler = &segregatedScheduler{provisioner: p}
	p.cluster, err = cluster.New(p.scheduler, p.storage, "", nodes...)
	c.Assert(err, check.IsNil)
	opts := pool.AddPoolOptions{Name: "pool1"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("pool1", []string{"team1"})
	c.Assert(err, check.IsNil)
	variation := []int{10, 20, 30, 40, 50, 100}
	maxContainers := 40
	for i := 0; i < maxContainers; i++ {
		appName := fmt.Sprintf("myapp-%d", i)
		err = newFakeImage(p, "tsuru/app-"+appName, nil)
		c.Assert(err, check.IsNil)
		appInstance := provisiontest.NewFakeApp(appName, "python", 0)
		defer p.Destroy(appInstance)
		p.Provision(appInstance)
		imageID, aErr := image.AppCurrentImageName(appInstance.GetName())
		c.Assert(aErr, check.IsNil)
		var chosenNode string
		for j := range variation {
			if i < (maxContainers*variation[j])/100 {
				chosenNode = nodeHosts[j]
				break
			}
		}
		args := changeUnitsPipelineArgs{
			app:         appInstance,
			toAdd:       map[string]*containersToAdd{"web": {Quantity: 6}},
			imageID:     imageID,
			provisioner: p,
			toHost:      chosenNode,
		}
		pipeline := action.NewPipeline(
			&provisionAddUnitsToHost,
			&bindAndHealthcheck,
			&addNewRoutes,
			&setRouterHealthcheck,
			&updateAppImage,
		)
		err = pipeline.Execute(args)
		c.Assert(err, check.IsNil)
		appStruct := s.newAppFromFake(appInstance)
		appStruct.TeamOwner = "team1"
		appStruct.Pool = "pool1"
		err = s.conn.Apps().Insert(appStruct)
		c.Assert(err, check.IsNil)
	}
	buf := safe.NewBuffer(nil)
	cloneProv, err := p.rebalanceContainersByFilter(buf, []string{}, map[string]string{"pool": "pool1"}, false)
	c.Assert(err, check.IsNil)
	c.Assert(cloneProv.cluster.Healer, check.Equals, p.cluster.Healer)
	for i := range nodeHosts {
		conts, err := p.listContainersByHost(nodeHosts[i])
		c.Assert(err, check.IsNil)
		c.Logf("containers in %q: %d", nodeHosts[i], len(conts))
		c.Check(len(conts) >= 39 || len(conts) <= 41, check.Equals, true)
	}
}
