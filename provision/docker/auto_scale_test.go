// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"strings"
	"sync"
	"time"

	dtesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func newHealerIaaSConstructor(addr string, err error) func(string) iaas.IaaS {
	return func(name string) iaas.IaaS {
		return &TestHealerIaaS{addr: addr, err: err}
	}
}

func newHealerIaaSConstructorWithInst(addr string) (func(string) iaas.IaaS, *TestHealerIaaS) {
	inst := &TestHealerIaaS{addr: addr}
	return func(name string) iaas.IaaS {
		return inst
	}, inst
}

func (s *S) TestAutoScaleConfigRun(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	defer func() {
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
	}()
	node1, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	config.Set("iaas:node-port", urlPort(node2.URL()))
	defer config.Unset("iaas:node-port")

	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.storage = &cluster.MapStorage{}
	clusterInstance, err := cluster.New(nil, p.storage,
		cluster.Node{Address: node1.URL(), Metadata: map[string]string{
			"pool": "pool1",
			"iaas": "my-scale-iaas",
		}},
	)
	c.Assert(err, check.IsNil)
	p.cluster = clusterInstance
	iaas.RegisterIaasProvider("my-scale-iaas", newHealerIaaSConstructor("localhost", nil))
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)

	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
		Pool: "pool1",
	}
	err = provision.AddPool("pool1")
	c.Assert(err, check.IsNil)
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})

	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		unitsToAdd:  4,
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:              make(chan bool),
		provisioner:       &p,
		groupByMetadata:   "pool",
		maxContainerCount: 2,
	}
	go a.stop()
	err = a.run()
	c.Assert(err, check.IsNil)
	nodes, err := p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address, check.Not(check.Equals), nodes[1].Address)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "add")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	c.Assert(evts[0].Node.Address, check.Equals, fmt.Sprintf("http://localhost:%d", urlPort(node2.URL())))
	c.Assert(evts[0].Node.Metadata, check.DeepEquals, map[string]string{
		"pool":    "pool1",
		"iaas":    "my-scale-iaas",
		"iaas-id": "m-localhost",
	})
	logParts := strings.Split(evts[0].Log, "\n")
	c.Assert(logParts, check.HasLen, 15)
	c.Assert(logParts[0], check.Matches, `\[node autoscale\].*running scaler.*pool1.*`)
	c.Assert(logParts[2], check.Matches, `\[node autoscale\].*new machine created.*`)
	c.Assert(logParts[5], check.Matches, `.*Rebalancing 4 units.*`)

	// Also should have rebalanced
	containers1, err := p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 2)
	c.Assert(containers2, check.HasLen, 2)

	// Should do nothing if calling on already scaled
	go a.stop()
	err = a.run()
	c.Assert(err, check.IsNil)
	nodes, err = p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	evts, err = listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)

	containers1Again, err := p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2Again, err := p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.DeepEquals, containers1Again)
	c.Assert(containers2, check.DeepEquals, containers2Again)
	locked, err := app.AcquireApplicationLock(appStruct.Name, "x", "y")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
}

func (s *S) TestAutoScaleConfigRunNoRebalance(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	defer func() {
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
	}()
	node1, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	config.Set("iaas:node-port", urlPort(node2.URL()))
	defer config.Unset("iaas:node-port")
	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.storage = &cluster.MapStorage{}
	clusterInstance, err := cluster.New(nil, p.storage,
		cluster.Node{Address: node1.URL(), Metadata: map[string]string{
			"pool": "pool1",
			"iaas": "my-scale-iaas",
		}},
	)
	c.Assert(err, check.IsNil)
	p.cluster = clusterInstance
	iaas.RegisterIaasProvider("my-scale-iaas", newHealerIaaSConstructor("localhost", nil))
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)

	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
		Pool: "pool1",
	}
	err = provision.AddPool("pool1")
	c.Assert(err, check.IsNil)
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})

	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		unitsToAdd:  4,
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		preventRebalance:  true,
		done:              make(chan bool),
		provisioner:       &p,
		groupByMetadata:   "pool",
		maxContainerCount: 2,
	}
	go a.stop()
	err = a.run()
	c.Assert(err, check.IsNil)
	nodes, err := p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address, check.Not(check.Equals), nodes[1].Address)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "add")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")

	containers1, err := p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 4)
	c.Assert(containers2, check.HasLen, 0)

	go a.stop()
	err = a.run()
	c.Assert(err, check.IsNil)
	nodes, err = p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	evts, err = listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)

	containers1Again, err := p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2Again, err := p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.DeepEquals, containers1Again)
	c.Assert(containers2, check.DeepEquals, containers2Again)
}

func (s *S) TestAutoScaleConfigRunOnce(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	defer func() {
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
	}()
	node1, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	config.Set("iaas:node-port", urlPort(node2.URL()))
	defer config.Unset("iaas:node-port")
	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.storage = &cluster.MapStorage{}
	clusterInstance, err := cluster.New(nil, p.storage,
		cluster.Node{Address: node1.URL(), Metadata: map[string]string{
			"pool": "pool1",
			"iaas": "my-scale-iaas",
		}},
	)
	c.Assert(err, check.IsNil)
	p.cluster = clusterInstance
	iaas.RegisterIaasProvider("my-scale-iaas", newHealerIaaSConstructor("localhost", nil))
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)

	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
		Pool: "pool1",
	}
	err = provision.AddPool("pool1")
	c.Assert(err, check.IsNil)
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})

	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		unitsToAdd:  4,
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:              make(chan bool),
		provisioner:       &p,
		groupByMetadata:   "pool",
		maxContainerCount: 2,
	}
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address, check.Not(check.Equals), nodes[1].Address)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "add")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")

	containers1, err := p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 2)
	c.Assert(containers2, check.HasLen, 2)
}

func (s *S) TestAutoScaleConfigRunRebalanceOnly(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	defer func() {
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
	}()
	node1, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	config.Set("iaas:node-port", urlPort(node2.URL()))
	defer config.Unset("iaas:node-port")

	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.storage = &cluster.MapStorage{}
	otherUrl := strings.Replace(node2.URL(), "127.0.0.1", "localhost", 1)
	clusterInstance, err := cluster.New(nil, p.storage,
		cluster.Node{Address: node1.URL(), Metadata: map[string]string{
			"pool": "pool1",
			"iaas": "my-scale-iaas",
		}},
		cluster.Node{Address: otherUrl, Metadata: map[string]string{
			"pool": "pool1",
			"iaas": "my-scale-iaas",
		}},
	)
	c.Assert(err, check.IsNil)
	p.cluster = clusterInstance
	iaas.RegisterIaasProvider("my-scale-iaas", newHealerIaaSConstructor("localhost", nil))
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)

	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
		Pool: "pool1",
	}
	err = provision.AddPool("pool1")
	c.Assert(err, check.IsNil)
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})

	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		unitsToAdd:  4,
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
		toHost:      "127.0.0.1",
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:              make(chan bool),
		provisioner:       &p,
		groupByMetadata:   "pool",
		maxContainerCount: 2,
	}
	go a.stop()
	err = a.run()
	c.Assert(err, check.IsNil)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "rebalance")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	nodes, err := p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	containers1, err := p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 2)
	c.Assert(containers2, check.HasLen, 2)
}

func (s *S) TestAutoScaleConfigRunNoGroup(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	defer func() {
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
	}()
	node1, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	config.Set("iaas:node-port", urlPort(node2.URL()))
	defer config.Unset("iaas:node-port")

	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.storage = &cluster.MapStorage{}
	clusterInstance, err := cluster.New(nil, p.storage,
		cluster.Node{Address: node1.URL(), Metadata: map[string]string{
			"iaas": "my-scale-iaas",
			"pool": "test-fallback",
		}},
	)
	c.Assert(err, check.IsNil)
	p.cluster = clusterInstance
	iaas.RegisterIaasProvider("my-scale-iaas", newHealerIaaSConstructor("localhost", nil))
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)

	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
	}
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})

	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		unitsToAdd:  4,
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:              make(chan bool),
		provisioner:       &p,
		maxContainerCount: 2,
	}
	go a.stop()
	err = a.run()
	c.Assert(err, check.IsNil)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "")
	c.Assert(evts[0].Action, check.Equals, "add")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	nodes, err := p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address, check.Not(check.Equals), nodes[1].Address)
	containers1, err := p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 2)
	c.Assert(containers2, check.HasLen, 2)
}

func (s *S) TestAutoScaleConfigRunNoMatch(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	defer func() {
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
	}()
	node1, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	config.Set("iaas:node-port", urlPort(node2.URL()))
	defer config.Unset("iaas:node-port")

	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster, err = cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL(), Metadata: map[string]string{
			"iaas": "my-scale-iaas",
		}},
	)
	c.Assert(err, check.IsNil)
	iaas.RegisterIaasProvider("my-scale-iaas", newHealerIaaSConstructor("localhost", nil))
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)

	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
		Pool: "pool1",
	}
	err = provision.AddPool("pool1")
	c.Assert(err, check.IsNil)
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})

	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		unitsToAdd:  4,
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:              make(chan bool),
		provisioner:       &p,
		maxContainerCount: 2,
		groupByMetadata:   "pool",
	}
	go a.stop()
	err = a.run()
	c.Assert(err, check.IsNil)
	nodes, err := p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address, check.Equals, node1.URL())
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)

	p.cluster, err = cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL(), Metadata: map[string]string{
			"iaas": "my-scale-iaas",
			"pool": "pool1",
		}},
	)
	c.Assert(err, check.IsNil)
	a.matadataFilter = "pool2"
	go a.stop()
	err = a.run()
	c.Assert(err, check.IsNil)
	nodes, err = p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	evts, err = listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)

	a.matadataFilter = "pool1"
	go a.stop()
	err = a.run()
	c.Assert(err, check.IsNil)
	nodes, err = p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	evts, err = listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
}

func (s *S) TestAutoScaleConfigRunStress(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	defer func() {
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
	}()
	node1, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	config.Set("iaas:node-port", urlPort(node2.URL()))
	defer config.Unset("iaas:node-port")

	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.storage = &cluster.MapStorage{}
	clusterInstance, err := cluster.New(nil, p.storage,
		cluster.Node{Address: node1.URL(), Metadata: map[string]string{
			"pool": "pool1",
			"iaas": "my-scale-iaas",
		}},
	)
	c.Assert(err, check.IsNil)
	p.cluster = clusterInstance
	iaas.RegisterIaasProvider("my-scale-iaas", newHealerIaaSConstructor("localhost", nil))
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)

	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
		Pool: "pool1",
	}
	err = provision.AddPool("pool1")
	c.Assert(err, check.IsNil)
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})

	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		unitsToAdd:  4,
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
	})
	c.Assert(err, check.IsNil)
	wg := sync.WaitGroup{}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			a := autoScaleConfig{
				done:              make(chan bool),
				provisioner:       &p,
				groupByMetadata:   "pool",
				maxContainerCount: 2,
			}
			defer wg.Done()
			go a.stop()
			err := a.run()
			c.Assert(err, check.IsNil)
		}()
	}
	wg.Wait()
	nodes, err := p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address, check.Not(check.Equals), nodes[1].Address)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "add")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")

	containers1, err := p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 2)
	c.Assert(containers2, check.HasLen, 2)
}

func (s *S) TestAutoScaleConfigRunMemoryBased(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	defer func() {
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
	}()
	plan := app.Plan{Memory: 21000, Name: "default", CpuShare: 10}
	err := plan.Save()
	c.Assert(err, check.IsNil)
	node1, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	config.Set("iaas:node-port", urlPort(node2.URL()))
	defer config.Unset("iaas:node-port")

	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.storage = &cluster.MapStorage{}
	clusterInstance, err := cluster.New(nil, p.storage,
		cluster.Node{Address: node1.URL(), Metadata: map[string]string{
			"pool":     "pool1",
			"iaas":     "my-scale-iaas",
			"totalMem": "125000",
		}},
	)
	c.Assert(err, check.IsNil)
	p.cluster = clusterInstance
	iaas.RegisterIaasProvider("my-scale-iaas", newHealerIaaSConstructor("localhost", nil))
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)

	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
		Plan: app.Plan{Memory: 21000},
		Pool: "pool1",
	}
	err = provision.AddPool("pool1")
	c.Assert(err, check.IsNil)
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})

	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		unitsToAdd:  4,
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:                make(chan bool),
		provisioner:         &p,
		groupByMetadata:     "pool",
		totalMemoryMetadata: "totalMem",
		maxMemoryRatio:      0.8,
	}
	go a.stop()
	err = a.run()
	c.Assert(err, check.IsNil)
	nodes, err := p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address, check.Not(check.Equals), nodes[1].Address)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "add")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")

	// Also should have rebalanced
	containers1, err := p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 2)
	c.Assert(containers2, check.HasLen, 2)

	// Should do nothing if calling on already scaled
	go a.stop()
	err = a.run()
	c.Assert(err, check.IsNil)
	nodes, err = p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	evts, err = listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)

	containers1Again, err := p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2Again, err := p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.DeepEquals, containers1Again)
	c.Assert(containers2, check.DeepEquals, containers2Again)
	locked, err := app.AcquireApplicationLock(appStruct.Name, "x", "y")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
}

func (s *S) TestAutoScaleConfigRunPriorityToCountBased(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	defer func() {
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
	}()
	node1, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	config.Set("iaas:node-port", urlPort(node2.URL()))
	defer config.Unset("iaas:node-port")

	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.storage = &cluster.MapStorage{}
	clusterInstance, err := cluster.New(nil, p.storage,
		cluster.Node{Address: node1.URL(), Metadata: map[string]string{
			"pool": "pool1",
			"iaas": "my-scale-iaas",
		}},
	)
	c.Assert(err, check.IsNil)
	p.cluster = clusterInstance
	iaas.RegisterIaasProvider("my-scale-iaas", newHealerIaaSConstructor("localhost", nil))
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)

	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
		Pool: "pool1",
	}
	err = provision.AddPool("pool1")
	c.Assert(err, check.IsNil)
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})

	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		unitsToAdd:  4,
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:                make(chan bool),
		provisioner:         &p,
		groupByMetadata:     "pool",
		maxContainerCount:   2,
		totalMemoryMetadata: "totalMem",
		maxMemoryRatio:      0.8,
	}
	go a.stop()
	err = a.run()
	c.Assert(err, check.IsNil)
	nodes, err := p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address, check.Not(check.Equals), nodes[1].Address)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "add")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")

	containers1, err := p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 2)
	c.Assert(containers2, check.HasLen, 2)
}

func (s *S) TestAutoScaleConfigRunMemoryBasedPlanTooBig(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	defer func() {
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
	}()
	plan := app.Plan{Memory: 126000, Name: "default", CpuShare: 10}
	err := plan.Save()
	c.Assert(err, check.IsNil)
	node1, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)

	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.storage = &cluster.MapStorage{}
	clusterInstance, err := cluster.New(nil, p.storage,
		cluster.Node{Address: node1.URL(), Metadata: map[string]string{
			"pool":     "pool1",
			"iaas":     "my-scale-iaas",
			"totalMem": "125000",
		}},
	)
	c.Assert(err, check.IsNil)
	p.cluster = clusterInstance
	iaas.RegisterIaasProvider("my-scale-iaas", newHealerIaaSConstructor("localhost", nil))
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)

	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
		Plan: app.Plan{Memory: 21000},
		Pool: "pool1",
	}
	err = provision.AddPool("pool1")
	c.Assert(err, check.IsNil)
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})

	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		unitsToAdd:  1,
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:                make(chan bool),
		provisioner:         &p,
		groupByMetadata:     "pool",
		totalMemoryMetadata: "totalMem",
		maxMemoryRatio:      0.8,
	}
	go a.stop()
	err = a.run()
	c.Assert(err, check.ErrorMatches, `\[node autoscale\] error scaling group pool1: aborting, impossible to fit max plan memory of 126000 bytes, node max available memory is 100000`)
	nodes, err := p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address, check.Equals, node1.URL())
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
}

func (s *S) TestAutoScaleConfigRunScaleDown(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	defer func() {
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
	}()
	node1, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	config.Set("iaas:node-port", urlPort(node2.URL()))
	defer config.Unset("iaas:node-port")

	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.storage = &cluster.MapStorage{}
	otherUrl := strings.Replace(node2.URL(), "127.0.0.1", "localhost", 1)
	clusterInstance, err := cluster.New(nil, p.storage,
		cluster.Node{Address: node1.URL(), Metadata: map[string]string{
			"pool": "pool1",
			"iaas": "my-scale-iaas",
		}},
		cluster.Node{Address: otherUrl, Metadata: map[string]string{
			"pool": "pool1",
			"iaas": "my-scale-iaas",
		}},
	)
	c.Assert(err, check.IsNil)
	p.cluster = clusterInstance
	iaas.RegisterIaasProvider("my-scale-iaas", newHealerIaaSConstructor("localhost", nil))
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)

	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
	}
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})

	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		unitsToAdd:  1,
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
		toHost:      "127.0.0.1",
	})
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		unitsToAdd:  1,
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
		toHost:      "localhost",
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:              make(chan bool),
		provisioner:       &p,
		groupByMetadata:   "pool",
		maxContainerCount: 4,
	}
	go a.stop()
	err = a.run()
	c.Assert(err, check.IsNil)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "remove")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	nodes, err := p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	containers, err := p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
}

func (s *S) TestAutoScaleConfigRunScaleDownMemoryScaler(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	defer func() {
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
	}()
	plan := app.Plan{Memory: 21000, Name: "default", CpuShare: 10}
	err := plan.Save()
	c.Assert(err, check.IsNil)
	node1, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	config.Set("iaas:node-port", urlPort(node2.URL()))
	defer config.Unset("iaas:node-port")

	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.storage = &cluster.MapStorage{}
	otherUrl := strings.Replace(node2.URL(), "127.0.0.1", "localhost", 1)
	clusterInstance, err := cluster.New(nil, p.storage,
		cluster.Node{Address: node1.URL(), Metadata: map[string]string{
			"pool":     "pool1",
			"iaas":     "my-scale-iaas",
			"totalMem": "125000",
		}},
		cluster.Node{Address: otherUrl, Metadata: map[string]string{
			"pool":     "pool1",
			"iaas":     "my-scale-iaas",
			"totalMem": "125000",
		}},
	)
	c.Assert(err, check.IsNil)
	p.cluster = clusterInstance
	iaas.RegisterIaasProvider("my-scale-iaas", newHealerIaaSConstructor("localhost", nil))
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)

	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
		Pool: "pool1",
	}
	err = provision.AddPool("pool1")
	c.Assert(err, check.IsNil)
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})

	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		unitsToAdd:  1,
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
		toHost:      "127.0.0.1",
	})
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		unitsToAdd:  1,
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
		toHost:      "localhost",
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:                make(chan bool),
		provisioner:         &p,
		groupByMetadata:     "pool",
		totalMemoryMetadata: "totalMem",
		maxMemoryRatio:      0.8,
	}
	go a.stop()
	err = a.run()
	c.Assert(err, check.IsNil)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "remove")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	nodes, err := p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	containers, err := p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
}

func (s *S) TestAutoScaleConfigRunScaleDownRespectsMinNodes(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	defer func() {
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
	}()
	node1, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	config.Set("iaas:node-port", urlPort(node2.URL()))
	defer config.Unset("iaas:node-port")

	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.storage = &cluster.MapStorage{}
	otherUrl := strings.Replace(node2.URL(), "127.0.0.1", "localhost", 1)
	clusterInstance, err := cluster.New(nil, p.storage,
		cluster.Node{Address: node1.URL(), Metadata: map[string]string{
			"pool":    "pool1",
			"iaas":    "my-scale-iaas",
			"network": "net1",
		}},
		cluster.Node{Address: otherUrl, Metadata: map[string]string{
			"pool":    "pool1",
			"iaas":    "my-scale-iaas",
			"network": "net2",
		}},
	)
	c.Assert(err, check.IsNil)
	p.cluster = clusterInstance
	iaas.RegisterIaasProvider("my-scale-iaas", newHealerIaaSConstructor("localhost", nil))
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)

	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
		Pool: "pool1",
	}
	err = provision.AddPool("pool1")
	c.Assert(err, check.IsNil)
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})

	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		unitsToAdd:  1,
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
		toHost:      "127.0.0.1",
	})
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		unitsToAdd:  1,
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
		toHost:      "localhost",
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:              make(chan bool),
		provisioner:       &p,
		groupByMetadata:   "pool",
		maxContainerCount: 4,
	}
	go a.stop()
	err = a.run()
	c.Assert(err, check.IsNil)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
	nodes, err := p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
}

func (s *S) TestAutoScaleConfigRunLockedApp(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	defer func() {
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
	}()
	node1, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	config.Set("iaas:node-port", urlPort(node2.URL()))
	defer config.Unset("iaas:node-port")
	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.storage = &cluster.MapStorage{}
	clusterInstance, err := cluster.New(nil, p.storage,
		cluster.Node{Address: node1.URL(), Metadata: map[string]string{
			"pool": "pool1",
			"iaas": "my-scale-iaas",
		}},
	)
	c.Assert(err, check.IsNil)
	p.cluster = clusterInstance
	iaas.RegisterIaasProvider("my-scale-iaas", newHealerIaaSConstructor("localhost", nil))
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
		Pool: "pool1",
	}
	err = provision.AddPool("pool1")
	c.Assert(err, check.IsNil)
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		unitsToAdd:  4,
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
	})
	c.Assert(err, check.IsNil)
	locked, err := app.AcquireApplicationLock(appStruct.Name, "tsr", "something")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
	a := autoScaleConfig{
		done:              make(chan bool),
		provisioner:       &p,
		groupByMetadata:   "pool",
		maxContainerCount: 2,
	}
	go a.stop()
	err = a.run()
	c.Assert(err, check.ErrorMatches, `.*unable to lock app myapp, aborting.*`)
	nodes, err := p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
}

func (s *S) TestAutoScaleConfigRunMemoryBasedLockedApp(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	defer func() {
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
	}()
	plan := app.Plan{Memory: 21000, Name: "default", CpuShare: 10}
	err := plan.Save()
	c.Assert(err, check.IsNil)
	node1, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	config.Set("iaas:node-port", urlPort(node2.URL()))
	defer config.Unset("iaas:node-port")
	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.storage = &cluster.MapStorage{}
	clusterInstance, err := cluster.New(nil, p.storage,
		cluster.Node{Address: node1.URL(), Metadata: map[string]string{
			"pool":     "pool1",
			"iaas":     "my-scale-iaas",
			"totalMem": "125000",
		}},
	)
	c.Assert(err, check.IsNil)
	p.cluster = clusterInstance
	iaas.RegisterIaasProvider("my-scale-iaas", newHealerIaaSConstructor("localhost", nil))
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
		Plan: app.Plan{Memory: 21000},
		Pool: "pool1",
	}
	err = provision.AddPool("pool1")
	c.Assert(err, check.IsNil)
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})

	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		unitsToAdd:  4,
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
	})
	c.Assert(err, check.IsNil)
	locked, err := app.AcquireApplicationLock(appStruct.Name, "tsr", "something")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
	a := autoScaleConfig{
		done:                make(chan bool),
		provisioner:         &p,
		groupByMetadata:     "pool",
		totalMemoryMetadata: "totalMem",
		maxMemoryRatio:      0.8,
	}
	go a.stop()
	err = a.run()
	c.Assert(err, check.ErrorMatches, `.*unable to lock app myapp, aborting.*`)
	nodes, err := p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
}

func (s *S) TestAutoScaleConfigRunParamsError(c *check.C) {
	a := autoScaleConfig{
		provisioner:       s.p,
		groupByMetadata:   "pool",
		maxContainerCount: 0,
	}
	err := a.run()
	c.Assert(err, check.ErrorMatches, `\[node autoscale\] aborting node auto scale, either memory information or max container count must be informed in config`)
	a = autoScaleConfig{
		provisioner:       s.p,
		groupByMetadata:   "pool",
		maxContainerCount: 10,
		scaleDownRatio:    0.9,
	}
	err = a.run()
	c.Assert(err, check.ErrorMatches, `\[node autoscale\] scale down ratio needs to be greater than 1.0, got .+`)
}

func (s *S) TestAutoScaleConfigRunDefaultValues(c *check.C) {
	a := autoScaleConfig{
		done:              make(chan bool),
		provisioner:       s.p,
		maxContainerCount: 10,
	}
	go a.stop()
	err := a.run()
	c.Assert(err, check.IsNil)
	c.Assert(a.runInterval, check.Equals, 1*time.Hour)
	c.Assert(a.waitTimeNewMachine, check.Equals, 5*time.Minute)
	c.Assert(a.scaleDownRatio > 1.332 && a.scaleDownRatio < 1.334, check.Equals, true)
}

func (s *S) TestAutoScaleConfigRunConfigValues(c *check.C) {
	a := autoScaleConfig{
		done:               make(chan bool),
		provisioner:        s.p,
		maxContainerCount:  10,
		runInterval:        10 * time.Minute,
		waitTimeNewMachine: 7 * time.Minute,
		scaleDownRatio:     1.5,
	}
	go a.stop()
	err := a.run()
	c.Assert(err, check.IsNil)
	c.Assert(a.runInterval, check.Equals, 10*time.Minute)
	c.Assert(a.waitTimeNewMachine, check.Equals, 7*time.Minute)
	c.Assert(a.scaleDownRatio > 1.49 && a.scaleDownRatio < 1.51, check.Equals, true)
}

func (s *S) TestAutoScaleCanRemoveNode(c *check.C) {
	nodes := []*cluster.Node{
		{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
		{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
		{Address: "", Metadata: map[string]string{
			"pool": "pool2",
			"zone": "zone2",
		}},
	}
	ok, err := canRemoveNode(nodes[0], nodes)
	c.Assert(err, check.IsNil)
	c.Assert(ok, check.Equals, true)
	ok, err = canRemoveNode(nodes[1], nodes)
	c.Assert(err, check.IsNil)
	c.Assert(ok, check.Equals, true)
	ok, err = canRemoveNode(nodes[2], nodes)
	c.Assert(err, check.IsNil)
	c.Assert(ok, check.Equals, false)
	nodes = []*cluster.Node{
		{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
		{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
	}
	ok, err = canRemoveNode(nodes[0], nodes)
	c.Assert(err, check.IsNil)
	c.Assert(ok, check.Equals, true)
	ok, err = canRemoveNode(nodes[1], nodes)
	c.Assert(err, check.IsNil)
	c.Assert(ok, check.Equals, true)
}

func (s *S) TestSplitMetadata(c *check.C) {
	var err error
	exclusive, common, err := splitMetadata([]map[string]string{
		{"1": "a", "2": "z1", "3": "n1"},
		{"1": "a", "2": "z2", "3": "n2"},
		{"1": "a", "2": "z3", "3": "n3"},
		{"1": "a", "2": "z3", "3": "n3"},
	})
	c.Assert(err, check.IsNil)
	c.Assert(exclusive, check.DeepEquals, metaWithFrequencyList{
		{metadata: map[string]string{"2": "z1", "3": "n1"}, freq: 1},
		{metadata: map[string]string{"2": "z2", "3": "n2"}, freq: 1},
		{metadata: map[string]string{"2": "z3", "3": "n3"}, freq: 2},
	})
	c.Assert(common, check.DeepEquals, map[string]string{
		"1": "a",
	})
	exclusive, common, err = splitMetadata([]map[string]string{
		{"1": "a", "2": "z1", "3": "n1", "4": "b"},
		{"1": "a", "2": "z2", "3": "n2", "4": "b"},
	})
	c.Assert(err, check.IsNil)
	c.Assert(exclusive, check.DeepEquals, metaWithFrequencyList{
		{metadata: map[string]string{"2": "z1", "3": "n1"}, freq: 1},
		{metadata: map[string]string{"2": "z2", "3": "n2"}, freq: 1},
	})
	c.Assert(common, check.DeepEquals, map[string]string{
		"1": "a",
		"4": "b",
	})
	exclusive, common, err = splitMetadata([]map[string]string{
		{"1": "a", "2": "b"},
		{"1": "a", "2": "b"},
	})
	c.Assert(err, check.IsNil)
	c.Assert(exclusive, check.IsNil)
	c.Assert(common, check.DeepEquals, map[string]string{
		"1": "a",
		"2": "b",
	})
	exclusive, common, err = splitMetadata([]map[string]string{})
	c.Assert(err, check.IsNil)
	c.Assert(exclusive, check.IsNil)
	c.Assert(common, check.DeepEquals, map[string]string{})
	_, _, err = splitMetadata([]map[string]string{
		{"1": "a", "2": "z1", "3": "n1", "4": "b"},
		{"1": "a", "2": "z2", "3": "n2", "4": "b"},
		{"1": "a", "2": "z3", "3": "n3", "4": "c"},
	})
	c.Assert(err, check.ErrorMatches, "unbalanced metadata for node group:.*")
	_, _, err = splitMetadata([]map[string]string{
		{"1": "a", "2": "z1", "3": "n1", "4": "b"},
		{"1": "a", "2": "z2", "3": "n2", "4": "b"},
		{"1": "a", "2": "z3", "3": "n1", "4": "b"},
	})
	c.Assert(err, check.ErrorMatches, "unbalanced metadata for node group:.*")
}

func (s *S) TestChooseMetadataFromNodes(c *check.C) {
	nodes := []*cluster.Node{
		{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
	}
	metadata, err := chooseMetadataFromNodes(nodes)
	c.Assert(err, check.IsNil)
	c.Assert(metadata, check.DeepEquals, map[string]string{
		"pool": "pool1",
		"zone": "zone1",
	})
	nodes = []*cluster.Node{
		{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
		{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
		{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone2",
		}},
	}
	metadata, err = chooseMetadataFromNodes(nodes)
	c.Assert(err, check.IsNil)
	c.Assert(metadata, check.DeepEquals, map[string]string{
		"pool": "pool1",
		"zone": "zone2",
	})
	nodes = []*cluster.Node{
		{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
		{Address: "", Metadata: map[string]string{
			"pool": "pool2",
			"zone": "zone2",
		}},
		{Address: "", Metadata: map[string]string{
			"pool": "pool2",
			"zone": "zone2",
		}},
	}
	metadata, err = chooseMetadataFromNodes(nodes)
	c.Assert(err, check.IsNil)
	c.Assert(metadata, check.DeepEquals, map[string]string{
		"pool": "pool1",
		"zone": "zone1",
	})
	nodes = []*cluster.Node{
		{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
		{Address: "", Metadata: map[string]string{
			"pool": "pool2",
			"zone": "zone2",
		}},
		{Address: "", Metadata: map[string]string{
			"pool": "pool2",
			"zone": "zone3",
		}},
	}
	_, err = chooseMetadataFromNodes(nodes)
	c.Assert(err, check.ErrorMatches, "unbalanced metadata for node group:.*")
}
