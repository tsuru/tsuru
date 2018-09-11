// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cluster

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	provTypes "github.com/tsuru/tsuru/types/provision"
	"gopkg.in/check.v1"
)

type S struct {
	conn *db.Storage
}

var _ = check.Suite(&S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "provision_kubernetes_cluster_tests_s")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *S) SetUpTest(c *check.C) {
	provisiontest.ProvisionerInstance.Reset()
	err := dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	s.conn.Close()
}

type TestIaaS struct{}

func (TestIaaS) DeleteMachine(m *iaas.Machine) error {
	m.Status = "destroyed"
	return nil
}

func (TestIaaS) CreateMachine(params map[string]string) (*iaas.Machine, error) {
	m := iaas.Machine{
		Id:      params["id"],
		Status:  "running",
		Address: params["id"] + ".somewhere.com",
	}
	if params["pool"] != "" {
		m.Id += "-" + params["pool"]
	}
	return &m, nil
}

func newTestIaaS(string) iaas.IaaS {
	return TestIaaS{}
}

func (s *S) TestClusterServiceCreateWithCreateData(c *check.C) {
	iaas.RegisterIaasProvider("test-iaas", newTestIaaS)
	kubeCluster := provTypes.Cluster{
		Name:        "c1",
		Addresses:   []string{},
		Provisioner: "fake",
		Default:     true,
		CreateData: map[string]string{
			"id":   "test1",
			"iaas": "test-iaas",
		},
	}
	cs := &clusterService{
		storage: &provTypes.MockClusterStorage{
			OnUpsert: func(clust provTypes.Cluster) error {
				c.Assert(clust.Name, check.Equals, kubeCluster.Name)
				c.Assert(clust.Provisioner, check.Equals, kubeCluster.Provisioner)
				c.Assert(clust.Addresses, check.DeepEquals, []string{"http://test1.somewhere.com:2375"})
				return nil
			},
		},
	}

	err := cs.Create(kubeCluster)
	c.Assert(err, check.IsNil)
}

func (s *S) TestClusterServiceCreateError(c *check.C) {
	mycluster := provTypes.Cluster{Name: "cluster1", Provisioner: "fake", Pools: []string{"mypool"}}
	cs := &clusterService{
		storage: &provTypes.MockClusterStorage{
			OnUpsert: func(_ provTypes.Cluster) error {
				return errors.New("storage error")
			},
		},
	}

	err := cs.Create(mycluster)
	c.Assert(err, check.NotNil)
}

func (s *S) TestClusterServiceCreateNameValidation(c *check.C) {
	mycluster := provTypes.Cluster{Provisioner: "fake", Pools: []string{"mypool"}}
	cs := &clusterService{
		storage: &provTypes.MockClusterStorage{},
	}
	invalidNameMsg := "Invalid cluster name, cluster name should have at most 40 " +
		"characters, containing only lower case letters, numbers or dashes, " +
		"starting with a letter."
	tests := []struct {
		name, err string
	}{
		{" ", "cluster name is mandatory"},
		{"1c", invalidNameMsg},
		{"c_1", invalidNameMsg},
		{"C1", invalidNameMsg},
		{"41-characters-ccccccccccccccccccccccccccc", invalidNameMsg},
	}
	for _, tt := range tests {
		mycluster.Name = tt.name
		err := cs.Create(mycluster)
		c.Check(err, check.ErrorMatches, tt.err)
	}
}

func (s *S) TestClusterServiceUpdate(c *check.C) {
	mycluster := provTypes.Cluster{Name: "cluster1", Provisioner: "fake", Pools: []string{"mypool"}}
	cs := &clusterService{
		storage: &provTypes.MockClusterStorage{
			OnUpsert: func(clust provTypes.Cluster) error {
				c.Assert(clust.Name, check.Equals, mycluster.Name)
				c.Assert(clust.Provisioner, check.Equals, mycluster.Provisioner)
				return nil
			},
		},
	}

	err := cs.Update(mycluster)
	c.Assert(err, check.IsNil)
}

func (s *S) TestClusterServiceUpdateError(c *check.C) {
	mycluster := provTypes.Cluster{Name: "cluster1", Provisioner: "fake", Pools: []string{"mypool"}}
	cs := &clusterService{
		storage: &provTypes.MockClusterStorage{
			OnUpsert: func(_ provTypes.Cluster) error {
				return errors.New("storage error")
			},
		},
	}

	err := cs.Update(mycluster)
	c.Assert(err, check.NotNil)
}

func (s *S) TestClusterServiceUpdateValidationError(c *check.C) {
	cs := &clusterService{
		storage: &provTypes.MockClusterStorage{},
	}
	tests := []struct {
		c   provTypes.Cluster
		err string
	}{
		{
			c: provTypes.Cluster{
				Name:        "  ",
				Addresses:   []string{"addr1", "addr2"},
				Default:     true,
				Provisioner: "fake",
			},
			err: "cluster name is mandatory",
		},
		{
			c: provTypes.Cluster{
				Name:        "1c",
				Addresses:   []string{"addr1", "addr2"},
				Default:     true,
				Provisioner: "fake",
			},
			err: "",
		},
		{
			c: provTypes.Cluster{
				Name:        "c1",
				Addresses:   []string{"addr1"},
				Default:     false,
				Provisioner: "fake",
			},
			err: "either default or a list of pools must be set",
		},
		{
			c: provTypes.Cluster{
				Name:        "c1",
				Addresses:   []string{"addr1"},
				Default:     true,
				Pools:       []string{"p1"},
				Provisioner: "fake",
			},
			err: "cannot have both pools and default set",
		},
		{
			c: provTypes.Cluster{
				Name:        "c1",
				Addresses:   []string{"addr1"},
				Default:     true,
				Provisioner: "",
			},
			err: "provisioner name is mandatory",
		},
		{
			c: provTypes.Cluster{
				Name:        "c1",
				Addresses:   []string{"addr1"},
				Default:     true,
				Provisioner: "invalid",
			},
			err: "unknown provisioner: \"invalid\"",
		},
	}
	for _, tt := range tests {
		err := cs.Update(tt.c)
		if len(tt.err) == 0 {
			c.Check(err, check.IsNil)
		} else {
			c.Check(err, check.ErrorMatches, tt.err)
		}
	}
}

func (s *S) TestClusterServiceList(c *check.C) {
	clusters := []provTypes.Cluster{{Name: "cluster1"}, {Name: "cluster2"}}
	cs := &clusterService{
		storage: &provTypes.MockClusterStorage{
			OnFindAll: func() ([]provTypes.Cluster, error) {
				return clusters, nil
			},
		},
	}

	result, err := cs.List()
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, clusters)
}

func (s *S) TestClusterServiceFindByName(c *check.C) {
	cluster := provTypes.Cluster{Name: "cluster1"}
	cs := &clusterService{
		storage: &provTypes.MockClusterStorage{
			OnFindByName: func(name string) (*provTypes.Cluster, error) {
				c.Assert(name, check.Equals, cluster.Name)
				return &cluster, nil
			},
		},
	}

	result, err := cs.FindByName(cluster.Name)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.NotNil)
	c.Assert(*result, check.DeepEquals, cluster)
}

func (s *S) TestClusterServiceFindByNameNotFound(c *check.C) {
	cs := &clusterService{
		storage: &provTypes.MockClusterStorage{
			OnFindByName: func(_ string) (*provTypes.Cluster, error) {
				return nil, errors.New("not found")
			},
		},
	}

	result, err := cs.FindByName("unknown cluster")
	c.Assert(result, check.IsNil)
	c.Assert(err, check.ErrorMatches, "not found")
}

func (s *S) TestClusterServiceFindByProvisioner(c *check.C) {
	clusters := []provTypes.Cluster{{Name: "cluster1"}, {Name: "cluster2"}}
	cs := &clusterService{
		storage: &provTypes.MockClusterStorage{
			OnFindByProvisioner: func(prov string) ([]provTypes.Cluster, error) {
				c.Assert(prov, check.Equals, "kubernetes")
				return clusters, nil
			},
		},
	}

	result, err := cs.FindByProvisioner("kubernetes")
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, clusters)
}

func (s *S) TestClusterServiceFindByPool(c *check.C) {
	cluster := provTypes.Cluster{Name: "cluster1", Provisioner: "kubernetes", Pools: []string{"pool-a"}}
	cs := &clusterService{
		storage: &provTypes.MockClusterStorage{
			OnFindByPool: func(prov, pool string) (*provTypes.Cluster, error) {
				c.Assert(prov, check.Equals, cluster.Provisioner)
				c.Assert(pool, check.Equals, cluster.Pools[0])
				return &cluster, nil
			},
		},
	}

	result, err := cs.FindByPool(cluster.Provisioner, cluster.Pools[0])
	c.Assert(err, check.IsNil)
	c.Assert(result, check.NotNil)
	c.Assert(*result, check.DeepEquals, cluster)
}

func (s *S) TestClusterServiceFindByPoolNotFound(c *check.C) {
	cs := &clusterService{
		storage: &provTypes.MockClusterStorage{
			OnFindByPool: func(_, _ string) (*provTypes.Cluster, error) {
				return nil, errors.New("not found")
			},
		},
	}

	result, err := cs.FindByPool("unknown prov", "unknown pool")
	c.Assert(result, check.IsNil)
	c.Assert(err, check.ErrorMatches, "not found")
}

func (s *S) TestClusterServiceDelete(c *check.C) {
	cluster := provTypes.Cluster{Name: "cluster1"}
	cs := &clusterService{
		storage: &provTypes.MockClusterStorage{
			OnDelete: func(clust provTypes.Cluster) error {
				c.Assert(clust, check.DeepEquals, cluster)
				return nil
			},
		},
	}

	err := cs.Delete(cluster)
	c.Assert(err, check.IsNil)
}

func (s *S) TestClusterServiceDeleteNotFound(c *check.C) {
	cs := &clusterService{
		storage: &provTypes.MockClusterStorage{
			OnDelete: func(_ provTypes.Cluster) error {
				return errors.New("not found")
			},
		},
	}

	err := cs.Delete(provTypes.Cluster{})
	c.Assert(err, check.ErrorMatches, "not found")
}

type initClusterProv struct {
	*provisiontest.FakeProvisioner
	callCluster *provTypes.Cluster
}

func (p *initClusterProv) InitializeCluster(c *provTypes.Cluster) error {
	p.callCluster = c
	return nil
}

func (s *S) TestClusterUpdateCallsProvInit(c *check.C) {
	inst := initClusterProv{FakeProvisioner: provisiontest.ProvisionerInstance}
	provision.Register("fake-cluster", func() (provision.Provisioner, error) {
		return &inst, nil
	})
	defer provision.Unregister("fake-cluster")
	c1 := provTypes.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Pools:       []string{"p1", "p2"},
		Provisioner: "fake-cluster",
	}
	cs := &clusterService{
		storage: &provTypes.MockClusterStorage{},
	}
	err := cs.Update(c1)
	c.Assert(err, check.IsNil)
	c.Assert(c1, check.DeepEquals, *inst.callCluster)
}

func (s *S) TestFindByPools(c *check.C) {
	prov := "prov1"
	clusters := []provTypes.Cluster{
		{Name: "cluster1", Provisioner: "kubernetes", Pools: []string{"poolA", "poolC"}},
		{Name: "cluster2", Provisioner: "kubernetes", Pools: []string{"poolB"}},
		{Name: "cluster3", Provisioner: "kubernetes", Default: true},
	}
	cs := &clusterService{
		storage: &provTypes.MockClusterStorage{
			OnFindByProvisioner: func(prov string) ([]provTypes.Cluster, error) {
				c.Assert(prov, check.Equals, prov)
				return clusters, nil
			},
		},
	}
	result, err := cs.FindByPools(prov, []string{"poolA", "poolB", "poolC", "poolD", "poolA"})
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, map[string]provTypes.Cluster{
		"poolA": clusters[0],
		"poolB": clusters[1],
		"poolC": clusters[0],
		"poolD": clusters[2],
	})
}

func (s *S) TestFindByPoolsNotFound(c *check.C) {
	prov := "prov1"
	clusters := []provTypes.Cluster{
		{Name: "cluster1", Provisioner: "kubernetes", Pools: []string{"poolA", "poolC"}},
		{Name: "cluster2", Provisioner: "kubernetes", Pools: []string{"poolB"}},
	}
	cs := &clusterService{
		storage: &provTypes.MockClusterStorage{
			OnFindByProvisioner: func(prov string) ([]provTypes.Cluster, error) {
				c.Assert(prov, check.Equals, prov)
				return clusters, nil
			},
		},
	}
	_, err := cs.FindByPools(prov, []string{"poolA", "poolB", "poolC", "poolD"})
	c.Assert(err, check.ErrorMatches, `unable to find cluster for pool "poolD"`)
}
