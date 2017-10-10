// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"math/rand"
	"os"
	"strings"
	"testing"

	"github.com/fsouza/go-dockerclient"
	dockerTesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	fakebuilder "github.com/tsuru/tsuru/builder/fake"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/cluster"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/router/routertest"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/types"
	"github.com/tsuru/tsuru/types/service"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/check.v1"
)

type S struct {
	p          *swarmProvisioner
	b          *fakebuilder.FakeBuilder
	conn       *db.Storage
	user       *auth.User
	team       *types.Team
	token      auth.Token
	clusterSrv *dockerTesting.DockerServer
	clusterCli *clusterClient
}

var _ = check.Suite(&S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "provision_swarm_tests_s")
	config.Set("routers:fake:type", "fake")
	config.Set("routers:fake:default", true)
	config.Set("docker:registry", "registry.tsuru.io")
	config.Set("host", "http://tsuruhost")
	config.Set("docker:repository-namespace", "tsuru")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	s.conn.Close()
}

func (s *S) SetUpTest(c *check.C) {
	routertest.FakeRouter.Reset()
	rand.Seed(0)
	config.Set("swarm:swarm-port", 0)
	err := dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
	err = pool.AddPool(pool.AddPoolOptions{Name: "bonehunters", Default: true, Provisioner: "swarm"})
	c.Assert(err, check.IsNil)
	p := appTypes.Plan{
		Name:     "default",
		Default:  true,
		CpuShare: 100,
	}
	err = app.SavePlan(p)
	c.Assert(err, check.IsNil)
	s.p = &swarmProvisioner{}
	err = s.p.Initialize()
	c.Assert(err, check.IsNil)
	s.b = &fakebuilder.FakeBuilder{}
	s.user = &auth.User{Email: "whiskeyjack@genabackis.com", Password: "123456", Quota: quota.Unlimited}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	_, err = nativeScheme.Create(s.user)
	c.Assert(err, check.IsNil)
	s.team = &types.Team{Name: "admin"}
	err = service.Team().Insert(*s.team)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownTest(c *check.C) {
	if s.clusterSrv != nil {
		s.clusterSrv.Stop()
		s.clusterSrv = nil
	}
}

func (s *S) addTLSCluster(c *check.C) {
	var err error
	caPath := tmpFileWith(c, testCA)
	certPath := tmpFileWith(c, testServerCert)
	keyPath := tmpFileWith(c, testServerKey)
	defer os.Remove(certPath)
	defer os.Remove(keyPath)
	defer os.Remove(caPath)
	s.clusterSrv, err = dockerTesting.NewTLSServer("127.0.0.1:0", nil, nil, dockerTesting.TLSConfig{
		RootCAPath:  caPath,
		CertPath:    certPath,
		CertKeyPath: keyPath,
	})
	c.Assert(err, check.IsNil)
	url := strings.Replace(s.clusterSrv.URL(), "http://", "https://", 1)
	clust := &cluster.Cluster{
		Addresses:   []string{url},
		Default:     true,
		Name:        "c1",
		Provisioner: provisionerName,
		CaCert:      testCA,
		ClientCert:  testCert,
		ClientKey:   testKey,
	}
	s.initCluster(c, clust)
}

func (s *S) addCluster(c *check.C) {
	var err error
	s.clusterSrv, err = dockerTesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	clust := &cluster.Cluster{
		Addresses:   []string{s.clusterSrv.URL()},
		Default:     true,
		Name:        "c1",
		Provisioner: provisionerName,
	}
	s.initCluster(c, clust)
}

func (s *S) initCluster(c *check.C, clust *cluster.Cluster) {
	err := clust.Save()
	c.Assert(err, check.IsNil)
	s.clusterCli, err = newClusterClient(clust)
	c.Assert(err, check.IsNil)
	dockerInfo, err := s.clusterCli.Info()
	c.Assert(err, check.IsNil)
	nodeData, err := s.clusterCli.InspectNode(dockerInfo.Swarm.NodeID)
	c.Assert(err, check.IsNil)
	nodeData.Spec.Annotations.Labels = provision.NodeLabels(provision.NodeLabelsOpts{
		Addr:   s.clusterSrv.URL(),
		Pool:   "bonehunters",
		Prefix: tsuruLabelPrefix,
	}).ToLabels()
	err = s.clusterCli.UpdateNode(dockerInfo.Swarm.NodeID, docker.UpdateNodeOptions{
		Version:  nodeData.Version.Index,
		NodeSpec: nodeData.Spec,
	})
	c.Assert(err, check.IsNil)
}
