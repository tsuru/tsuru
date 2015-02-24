// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"time"

	dtesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

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

	clusterInstance, err := cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL(), Metadata: map[string]string{
			"pool": "pool1",
			"iaas": "my-scale-iaas",
		}},
	)
	c.Assert(err, check.IsNil)
	var p dockerProvisioner
	p.cluster = clusterInstance
	iaasInstance := &TestHealerIaaS{addr: "localhost"}
	iaas.RegisterIaasProvider("my-scale-iaas", iaasInstance)
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
		provisioner:        &p,
		groupByMetadata:    "pool",
		maxContainerCount:  2,
		waitTimeNewMachine: 1 * time.Second,
	}
	err = a.runOnce(false)
	c.Assert(err, check.IsNil)
	nodes, err := p.cluster.Nodes()
	c.Assert(err, check.IsNil)

	// Should create new nodes
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address, check.Not(check.Equals), nodes[1].Address)

	// Should rebalance
	containers1, err := p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 2)
	c.Assert(containers2, check.HasLen, 2)

	// Should do nothing if calling on already scaled
	err = a.runOnce(false)
	c.Assert(err, check.IsNil)
	nodes, err = p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)

	containers1Again, err := p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2Again, err := p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.DeepEquals, containers1Again)
	c.Assert(containers2, check.DeepEquals, containers2Again)
}

func (s *S) TestAutoScaleConfigRunOnceNoGroup(c *check.C) {
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

	clusterInstance, err := cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL(), Metadata: map[string]string{
			"iaas": "my-scale-iaas",
		}},
	)
	c.Assert(err, check.IsNil)
	var p dockerProvisioner
	p.cluster = clusterInstance
	iaasInstance := &TestHealerIaaS{addr: "localhost"}
	iaas.RegisterIaasProvider("my-scale-iaas", iaasInstance)
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
		provisioner:        &p,
		maxContainerCount:  2,
		waitTimeNewMachine: 1 * time.Second,
	}
	err = a.runOnce(false)
	c.Assert(err, check.IsNil)
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
		&cluster.Node{Address: "", Metadata: map[string]string{
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
		&cluster.Node{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
		&cluster.Node{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
		&cluster.Node{Address: "", Metadata: map[string]string{
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
		&cluster.Node{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
		&cluster.Node{Address: "", Metadata: map[string]string{
			"pool": "pool2",
			"zone": "zone2",
		}},
		&cluster.Node{Address: "", Metadata: map[string]string{
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
		&cluster.Node{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
		&cluster.Node{Address: "", Metadata: map[string]string{
			"pool": "pool2",
			"zone": "zone2",
		}},
		&cluster.Node{Address: "", Metadata: map[string]string{
			"pool": "pool2",
			"zone": "zone3",
		}},
	}
	_, err = chooseMetadataFromNodes(nodes)
	c.Assert(err, check.ErrorMatches, "unbalanced metadata for node group:.*")
}
