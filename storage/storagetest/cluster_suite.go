// Copyright 2018 tsuru provisionors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"sort"

	clusterPkg "github.com/tsuru/tsuru/provision/cluster"
	"github.com/tsuru/tsuru/types/provision"
	"gopkg.in/check.v1"
)

type ClusterSuite struct {
	SuiteHooks
	ClusterStorage provision.ClusterStorage
}

func (s *ClusterSuite) TestUpsertNewCluster(c *check.C) {
	cluster := provision.Cluster{Name: "clustername", Addresses: []string{"1.2.3.4", "5.6.7.8"}}
	err := s.ClusterStorage.Upsert(cluster)
	c.Assert(err, check.IsNil)

	cl, err := s.ClusterStorage.FindByName(cluster.Name)
	c.Assert(err, check.IsNil)
	c.Assert(cl.Name, check.Equals, cluster.Name)
	c.Assert(cl.Addresses, check.DeepEquals, cluster.Addresses)
}

func (s *ClusterSuite) TestUpsertNewDefaultCluster(c *check.C) {
	err := s.ClusterStorage.Upsert(provision.Cluster{Name: "c1", Default: true})
	c.Assert(err, check.IsNil)
	err = s.ClusterStorage.Upsert(provision.Cluster{Name: "c2", Default: true})
	c.Assert(err, check.IsNil)

	cluster, err := s.ClusterStorage.FindByName("c1")
	c.Assert(err, check.IsNil)
	c.Assert(cluster.Default, check.Equals, false)
	cluster, err = s.ClusterStorage.FindByName("c2")
	c.Assert(err, check.IsNil)
	c.Assert(cluster.Default, check.Equals, true)
}

func (s *ClusterSuite) TestUpsertExistingCluster(c *check.C) {
	cluster := provision.Cluster{Name: "clustername"}
	err := s.ClusterStorage.Upsert(cluster)
	c.Assert(err, check.IsNil)
	cluster.Addresses = []string{"1.2.3.4"}
	cluster.Default = true
	err = s.ClusterStorage.Upsert(cluster)
	c.Assert(err, check.IsNil)

	cl, err := s.ClusterStorage.FindByName(cluster.Name)
	c.Assert(err, check.IsNil)
	c.Assert(cl.Addresses, check.DeepEquals, []string{"1.2.3.4"})
	c.Assert(cl.Default, check.Equals, true)
}

func (s *ClusterSuite) TestFindAllClusters(c *check.C) {
	err := s.ClusterStorage.Upsert(provision.Cluster{Name: "cluster-a"})
	c.Assert(err, check.IsNil)
	err = s.ClusterStorage.Upsert(provision.Cluster{Name: "cluster-b"})
	c.Assert(err, check.IsNil)
	clusters, err := s.ClusterStorage.FindAll()
	c.Assert(err, check.IsNil)
	c.Assert(clusters, check.HasLen, 2)
	names := []string{clusters[0].Name, clusters[1].Name}
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{"cluster-a", "cluster-b"})
}

func (s *ClusterSuite) TestFindClusterByName(c *check.C) {
	cluster := provision.Cluster{Name: "mycluster"}
	err := s.ClusterStorage.Upsert(cluster)
	c.Assert(err, check.IsNil)

	clust, err := s.ClusterStorage.FindByName(cluster.Name)
	c.Assert(err, check.IsNil)
	c.Assert(clust.Name, check.Equals, cluster.Name)

	clust, err = s.ClusterStorage.FindByName("wat")
	c.Assert(err, check.Equals, clusterPkg.ErrClusterNotFound)
	c.Assert(clust, check.IsNil)
}

func (s *ClusterSuite) TestFindClusterByProvisioner(c *check.C) {
	err := s.ClusterStorage.Upsert(provision.Cluster{Name: "kubecluster1", Provisioner: "kubernetes"})
	c.Assert(err, check.IsNil)
	err = s.ClusterStorage.Upsert(provision.Cluster{Name: "swarmcluster", Provisioner: "swarm"})
	c.Assert(err, check.IsNil)
	err = s.ClusterStorage.Upsert(provision.Cluster{Name: "kubecluster2", Provisioner: "kubernetes"})
	c.Assert(err, check.IsNil)

	clusters, err := s.ClusterStorage.FindByProvisioner("kubernetes")
	c.Assert(err, check.IsNil)
	c.Assert(clusters, check.HasLen, 2)
	names := []string{clusters[0].Name, clusters[1].Name}
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{"kubecluster1", "kubecluster2"})

	clusters, err = s.ClusterStorage.FindByProvisioner("swarm")
	c.Assert(err, check.IsNil)
	c.Assert(clusters, check.HasLen, 1)
	c.Assert(clusters[0].Name, check.Equals, "swarmcluster")

	clusters, err = s.ClusterStorage.FindByProvisioner("other")
	c.Assert(err, check.IsNil)
	c.Assert(clusters, check.HasLen, 0)
}

func (s *ClusterSuite) TestFindClusterByPool(c *check.C) {
	err := s.ClusterStorage.Upsert(provision.Cluster{Name: "kubecluster1", Provisioner: "kubernetes", Pools: []string{"pool-a", "pool-b"}})
	c.Assert(err, check.IsNil)
	err = s.ClusterStorage.Upsert(provision.Cluster{Name: "swarmcluster", Provisioner: "swarm", Pools: []string{"pool-b", "pool-d"}})
	c.Assert(err, check.IsNil)
	err = s.ClusterStorage.Upsert(provision.Cluster{Name: "kubecluster2", Provisioner: "kubernetes", Pools: []string{"pool-c"}})
	c.Assert(err, check.IsNil)

	cluster, err := s.ClusterStorage.FindByPool("kubernetes", "pool-b")
	c.Assert(err, check.IsNil)
	c.Assert(cluster.Name, check.Equals, "kubecluster1")

	cluster, err = s.ClusterStorage.FindByPool("swarm", "pool-b")
	c.Assert(err, check.IsNil)
	c.Assert(cluster.Name, check.Equals, "swarmcluster")

	cluster, err = s.ClusterStorage.FindByPool("swarm", "pool-a")
	c.Assert(err, check.Equals, clusterPkg.ErrNoCluster)
	c.Assert(cluster, check.IsNil)
}

func (s *ClusterSuite) TestDeleteCluster(c *check.C) {
	cluster := provision.Cluster{Name: "mycluster"}
	err := s.ClusterStorage.Upsert(cluster)
	c.Assert(err, check.IsNil)
	err = s.ClusterStorage.Delete(cluster)
	c.Assert(err, check.IsNil)
	clust, err := s.ClusterStorage.FindByName(cluster.Name)
	c.Assert(err, check.Equals, clusterPkg.ErrClusterNotFound)
	c.Assert(clust, check.IsNil)
}
