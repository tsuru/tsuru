// Copyright 2018 tsuru provisionors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"context"
	"sort"

	"github.com/tsuru/tsuru/types/provision"
	check "gopkg.in/check.v1"
	clientcmdAPI "k8s.io/client-go/tools/clientcmd/api"
)

type ClusterSuite struct {
	SuiteHooks
	ClusterStorage provision.ClusterStorage
}

func (s *ClusterSuite) TestUpsertNewCluster(c *check.C) {
	cluster := provision.Cluster{Name: "clustername", Addresses: []string{"1.2.3.4", "5.6.7.8"}}
	err := s.ClusterStorage.Upsert(context.TODO(), cluster)
	c.Assert(err, check.IsNil)

	cl, err := s.ClusterStorage.FindByName(context.TODO(), cluster.Name)
	c.Assert(err, check.IsNil)
	c.Assert(cl.Name, check.Equals, cluster.Name)
	c.Assert(cl.Addresses, check.DeepEquals, cluster.Addresses)
}

func (s *ClusterSuite) TestUpsertNewClusterWithKubeConfig(c *check.C) {
	cluster := provision.Cluster{
		Name:      "clustername",
		Addresses: []string{"1.2.3.4", "5.6.7.8"},
		KubeConfig: &provision.KubeConfig{
			Cluster: clientcmdAPI.Cluster{
				Server: "https://1.2.3.4",
			},
			AuthInfo: clientcmdAPI.AuthInfo{
				ClientCertificateData: []byte("cert"),
				Exec: &clientcmdAPI.ExecConfig{
					Command: "cmd",
					Args:    []string{"arg1", "arg2"},
				},
			},
		},
	}
	err := s.ClusterStorage.Upsert(context.TODO(), cluster)
	c.Assert(err, check.IsNil)

	cl, err := s.ClusterStorage.FindByName(context.TODO(), cluster.Name)
	c.Assert(err, check.IsNil)

	c.Assert(cl.Name, check.Equals, cluster.Name)
	c.Assert(cl.KubeConfig.Cluster.Server, check.Equals, "https://1.2.3.4")
	c.Assert(cl.KubeConfig.AuthInfo.ClientCertificateData, check.DeepEquals, []byte("cert"))
	c.Assert(cl.KubeConfig.AuthInfo.Exec.Command, check.Equals, "cmd")
	c.Assert(cl.KubeConfig.AuthInfo.Exec.Args, check.DeepEquals, []string{"arg1", "arg2"})
}

func (s *ClusterSuite) TestUpsertNewDefaultCluster(c *check.C) {
	err := s.ClusterStorage.Upsert(context.TODO(), provision.Cluster{Name: "c1", Default: true})
	c.Assert(err, check.IsNil)
	err = s.ClusterStorage.Upsert(context.TODO(), provision.Cluster{Name: "c2", Default: true})
	c.Assert(err, check.IsNil)

	cluster, err := s.ClusterStorage.FindByName(context.TODO(), "c1")
	c.Assert(err, check.IsNil)
	c.Assert(cluster.Default, check.Equals, false)
	cluster, err = s.ClusterStorage.FindByName(context.TODO(), "c2")
	c.Assert(err, check.IsNil)
	c.Assert(cluster.Default, check.Equals, true)
}

func (s *ClusterSuite) TestUpsertExistingCluster(c *check.C) {
	cluster := provision.Cluster{Name: "clustername"}
	err := s.ClusterStorage.Upsert(context.TODO(), cluster)
	c.Assert(err, check.IsNil)
	cluster.Addresses = []string{"1.2.3.4"}
	cluster.Default = true
	err = s.ClusterStorage.Upsert(context.TODO(), cluster)
	c.Assert(err, check.IsNil)

	cl, err := s.ClusterStorage.FindByName(context.TODO(), cluster.Name)
	c.Assert(err, check.IsNil)
	c.Assert(cl.Addresses, check.DeepEquals, []string{"1.2.3.4"})
	c.Assert(cl.Default, check.Equals, true)
}

func (s *ClusterSuite) TestFindAllClusters(c *check.C) {
	err := s.ClusterStorage.Upsert(context.TODO(), provision.Cluster{Name: "cluster-a"})
	c.Assert(err, check.IsNil)
	err = s.ClusterStorage.Upsert(context.TODO(), provision.Cluster{Name: "cluster-b"})
	c.Assert(err, check.IsNil)
	clusters, err := s.ClusterStorage.FindAll(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(clusters, check.HasLen, 2)
	names := []string{clusters[0].Name, clusters[1].Name}
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{"cluster-a", "cluster-b"})
}

func (s *ClusterSuite) TestFindAllClustersNoCluster(c *check.C) {
	_, err := s.ClusterStorage.FindAll(context.TODO())
	c.Assert(err, check.Equals, provision.ErrNoCluster)
}

func (s *ClusterSuite) TestFindClusterByName(c *check.C) {
	cluster := provision.Cluster{Name: "mycluster"}
	err := s.ClusterStorage.Upsert(context.TODO(), cluster)
	c.Assert(err, check.IsNil)

	clust, err := s.ClusterStorage.FindByName(context.TODO(), cluster.Name)
	c.Assert(err, check.IsNil)
	c.Assert(clust.Name, check.Equals, cluster.Name)

	clust, err = s.ClusterStorage.FindByName(context.TODO(), "wat")
	c.Assert(err, check.Equals, provision.ErrClusterNotFound)
	c.Assert(clust, check.IsNil)
}

func (s *ClusterSuite) TestFindClusterByProvisioner(c *check.C) {
	err := s.ClusterStorage.Upsert(context.TODO(), provision.Cluster{Name: "kubecluster1", Provisioner: "kubernetes"})
	c.Assert(err, check.IsNil)
	err = s.ClusterStorage.Upsert(context.TODO(), provision.Cluster{Name: "swarmcluster", Provisioner: "swarm"})
	c.Assert(err, check.IsNil)
	err = s.ClusterStorage.Upsert(context.TODO(), provision.Cluster{Name: "kubecluster2", Provisioner: "kubernetes"})
	c.Assert(err, check.IsNil)

	clusters, err := s.ClusterStorage.FindByProvisioner(context.TODO(), "kubernetes")
	c.Assert(err, check.IsNil)
	c.Assert(clusters, check.HasLen, 2)
	names := []string{clusters[0].Name, clusters[1].Name}
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{"kubecluster1", "kubecluster2"})

	clusters, err = s.ClusterStorage.FindByProvisioner(context.TODO(), "swarm")
	c.Assert(err, check.IsNil)
	c.Assert(clusters, check.HasLen, 1)
	c.Assert(clusters[0].Name, check.Equals, "swarmcluster")

	_, err = s.ClusterStorage.FindByProvisioner(context.TODO(), "other")
	c.Assert(err, check.Equals, provision.ErrNoCluster)
}

func (s *ClusterSuite) TestFindClusterByProvisionerNoCluster(c *check.C) {
	_, err := s.ClusterStorage.FindByProvisioner(context.TODO(), "other")
	c.Assert(err, check.Equals, provision.ErrNoCluster)
}

func (s *ClusterSuite) TestFindClusterByPool(c *check.C) {
	ctx := context.TODO()
	err := s.ClusterStorage.Upsert(ctx, provision.Cluster{Name: "kubecluster1", Provisioner: "kubernetes", Pools: []string{"pool-a", "pool-b"}})
	c.Assert(err, check.IsNil)
	err = s.ClusterStorage.Upsert(ctx, provision.Cluster{Name: "swarmcluster", Provisioner: "swarm", Pools: []string{"pool-b", "pool-d"}})
	c.Assert(err, check.IsNil)
	err = s.ClusterStorage.Upsert(ctx, provision.Cluster{Name: "kubecluster2", Provisioner: "kubernetes", Pools: []string{"pool-c"}})
	c.Assert(err, check.IsNil)

	cluster, err := s.ClusterStorage.FindByPool(ctx, "kubernetes", "pool-b")
	c.Assert(err, check.IsNil)
	c.Assert(cluster.Name, check.Equals, "kubecluster1")

	cluster, err = s.ClusterStorage.FindByPool(ctx, "swarm", "pool-b")
	c.Assert(err, check.IsNil)
	c.Assert(cluster.Name, check.Equals, "swarmcluster")

	cluster, err = s.ClusterStorage.FindByPool(ctx, "swarm", "pool-a")
	c.Assert(err, check.Equals, provision.ErrNoCluster)
	c.Assert(cluster, check.IsNil)
}

func (s *ClusterSuite) TestFindClusterByPoolNoCluster(c *check.C) {
	ctx := context.TODO()

	_, err := s.ClusterStorage.FindByPool(ctx, "swarm", "pool-a")
	c.Assert(err, check.Equals, provision.ErrNoCluster)
}

func (s *ClusterSuite) TestDeleteCluster(c *check.C) {
	ctx := context.TODO()
	cluster := provision.Cluster{Name: "mycluster"}
	err := s.ClusterStorage.Upsert(ctx, cluster)
	c.Assert(err, check.IsNil)
	err = s.ClusterStorage.Delete(ctx, cluster)
	c.Assert(err, check.IsNil)
	clust, err := s.ClusterStorage.FindByName(ctx, cluster.Name)
	c.Assert(err, check.Equals, provision.ErrClusterNotFound)
	c.Assert(clust, check.IsNil)
}
