// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"math/rand"
	"sort"

	"github.com/pkg/errors"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"gopkg.in/check.v1"
	"k8s.io/client-go/rest"
)

func (s *S) TestClusterSave(c *check.C) {
	cluster := Cluster{
		Name:       "c1",
		Addresses:  []string{"addr1", "addr2"},
		CaCert:     []byte("cacert"),
		ClientCert: []byte("clientcert"),
		ClientKey:  []byte("clientkey"),
		Namespace:  "ns1",
		Default:    true,
	}
	err := cluster.Save()
	c.Assert(err, check.IsNil)
	coll, err := clusterCollection()
	c.Assert(err, check.IsNil)
	var dbCluster Cluster
	err = coll.FindId("c1").One(&dbCluster)
	c.Assert(err, check.IsNil)
	c.Assert(dbCluster, check.DeepEquals, cluster)
}

func (s *S) TestClusterSaveRemoveDefaults(c *check.C) {
	c1 := Cluster{
		Name:      "c1",
		Addresses: []string{"addr1"},
		Default:   true,
	}
	err := c1.Save()
	c.Assert(err, check.IsNil)
	c2 := Cluster{
		Name:      "c2",
		Addresses: []string{"addr2"},
		Default:   true,
	}
	err = c2.Save()
	c.Assert(err, check.IsNil)
	coll, err := clusterCollection()
	c.Assert(err, check.IsNil)
	var dbCluster1, dbCluster2 Cluster
	err = coll.FindId("c1").One(&dbCluster1)
	c.Assert(err, check.IsNil)
	c.Assert(dbCluster1.Default, check.Equals, false)
	err = coll.FindId("c2").One(&dbCluster2)
	c.Assert(err, check.IsNil)
	c.Assert(dbCluster2.Default, check.Equals, true)
}

func (s *S) TestClusterSaveRemovePools(c *check.C) {
	c1 := Cluster{
		Name:      "c1",
		Addresses: []string{"addr1"},
		Pools:     []string{"p1", "p2"},
	}
	err := c1.Save()
	c.Assert(err, check.IsNil)
	c2 := Cluster{
		Name:      "c2",
		Addresses: []string{"addr2"},
		Pools:     []string{"p2", "p3"},
	}
	err = c2.Save()
	c.Assert(err, check.IsNil)
	coll, err := clusterCollection()
	c.Assert(err, check.IsNil)
	var dbCluster1, dbCluster2 Cluster
	err = coll.FindId("c1").One(&dbCluster1)
	c.Assert(err, check.IsNil)
	c.Assert(dbCluster1.Pools, check.DeepEquals, []string{"p1"})
	err = coll.FindId("c2").One(&dbCluster2)
	c.Assert(err, check.IsNil)
	c.Assert(dbCluster2.Pools, check.DeepEquals, []string{"p2", "p3"})
}

func (s *S) TestClusterSaveValidation(c *check.C) {
	tests := []struct {
		c   Cluster
		err string
	}{
		{
			c: Cluster{
				Name:      "  ",
				Addresses: []string{"addr1", "addr2"},
				Namespace: "ns1",
				Default:   true,
			},
			err: "cluster name is mandatory",
		},
		{
			c: Cluster{
				Name:      "c1",
				Addresses: []string{},
				Namespace: "ns1",
				Default:   true,
			},
			err: "at least one address must be present",
		},
		{
			c: Cluster{
				Name:      "c1",
				Addresses: []string{"addr1"},
				Namespace: "ns1",
				Default:   false,
			},
			err: "either default or a list of pools must be set",
		},
		{
			c: Cluster{
				Name:      "c1",
				Addresses: []string{"addr1"},
				Namespace: "ns1",
				Default:   true,
				Pools:     []string{"p1"},
			},
			err: "cannot have both pools and default set",
		},
	}
	for _, tt := range tests {
		err := tt.c.Save()
		c.Assert(err, check.ErrorMatches, tt.err)
		c.Assert(errors.Cause(err), check.FitsTypeOf, &tsuruErrors.ValidationError{})
	}
}

func (s *S) TestAllClusters(c *check.C) {
	c1 := Cluster{
		Name:      "c1",
		Addresses: []string{"addr1"},
		Pools:     []string{"p1"},
	}
	err := c1.Save()
	c.Assert(err, check.IsNil)
	c2 := Cluster{
		Name:      "c2",
		Addresses: []string{"addr2"},
		Pools:     []string{"p2"},
	}
	err = c2.Save()
	c.Assert(err, check.IsNil)
	clusters, err := AllClusters()
	c.Assert(err, check.IsNil)
	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i].Name < clusters[j].Name
	})
	c.Assert(clusters, check.DeepEquals, []Cluster{c1, c2})
}

func (s *S) TestClusterForPool(c *check.C) {
	c1 := Cluster{
		Name:      "c1",
		Addresses: []string{"addr1"},
		Pools:     []string{"p1", "p2"},
	}
	err := c1.Save()
	c.Assert(err, check.IsNil)
	c2 := Cluster{
		Name:      "c2",
		Addresses: []string{"addr2"},
		Pools:     []string{"p3"},
	}
	err = c2.Save()
	c.Assert(err, check.IsNil)
	c3 := Cluster{
		Name:      "c3",
		Addresses: []string{"addr2"},
		Default:   true,
	}
	err = c3.Save()
	c.Assert(err, check.IsNil)
	cluster, err := ClusterForPool("p1")
	c.Assert(err, check.IsNil)
	c.Assert(cluster, check.DeepEquals, &c1)
	cluster, err = ClusterForPool("p2")
	c.Assert(err, check.IsNil)
	c.Assert(cluster, check.DeepEquals, &c1)
	cluster, err = ClusterForPool("p3")
	c.Assert(err, check.IsNil)
	c.Assert(cluster, check.DeepEquals, &c2)
	cluster, err = ClusterForPool("p4")
	c.Assert(err, check.IsNil)
	c.Assert(cluster, check.DeepEquals, &c3)
	cluster, err = ClusterForPool("")
	c.Assert(err, check.IsNil)
	c.Assert(cluster, check.DeepEquals, &c3)
	err = DeleteCluster("c3")
	c.Assert(err, check.IsNil)
	_, err = ClusterForPool("p4")
	c.Assert(err, check.Equals, errNoCluster)
}

func (s *S) TestClusterGetClientWithCfg(c *check.C) {
	c1 := Cluster{
		Name:       "c1",
		Addresses:  []string{"addr1"},
		CaCert:     []byte("cacert"),
		ClientCert: []byte("clientcert"),
		ClientKey:  []byte("clientkey"),
		Default:    true,
	}
	err := c1.Save()
	c.Assert(err, check.IsNil)
	cli, cfg, err := c1.getClientWithCfg()
	c.Assert(err, check.IsNil)
	c.Assert(cli, check.NotNil)
	expected := &rest.Config{
		APIPath: "/api",
		Host:    "addr1",
		TLSClientConfig: rest.TLSClientConfig{
			CAData:   []byte("cacert"),
			CertData: []byte("clientcert"),
			KeyData:  []byte("clientkey"),
		},
		Timeout: defaultTimeout,
	}
	expected.ContentConfig = cfg.ContentConfig
	c.Assert(cfg, check.DeepEquals, expected)
}

func (s *S) TestClusterGetRestConfigMultipleAddrsRandom(c *check.C) {
	c1 := Cluster{
		Name:      "c1",
		Addresses: []string{"addr1", "addr2"},
		Default:   true,
	}
	err := c1.Save()
	c.Assert(err, check.IsNil)
	rand.Seed(3)
	cfg, err := c1.getRestConfig()
	c.Assert(err, check.IsNil)
	c.Assert(cfg.Host, check.Equals, "addr1")
	cfg, err = c1.getRestConfig()
	c.Assert(err, check.IsNil)
	c.Assert(cfg.Host, check.Equals, "addr2")
}
