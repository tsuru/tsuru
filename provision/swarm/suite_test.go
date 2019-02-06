// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"math/rand"
	"os"
	"strings"
	"testing"

	docker "github.com/fsouza/go-dockerclient"
	dockerTesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/cluster"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/router/routertest"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	provTypes "github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/types/quota"
	"golang.org/x/crypto/bcrypt"
	check "gopkg.in/check.v1"
)

type S struct {
	p           *swarmProvisioner
	conn        *db.Storage
	user        *auth.User
	team        *authTypes.Team
	token       auth.Token
	clusterSrv  *dockerTesting.DockerServer
	clusterCli  *clusterClient
	mockService servicemock.MockService
}

var _ = check.Suite(&S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
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
	s.p = &swarmProvisioner{}
	err = s.p.Initialize()
	c.Assert(err, check.IsNil)
	s.user = &auth.User{Email: "whiskeyjack@genabackis.com", Password: "123456", Quota: quota.UnlimitedQuota}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	_, err = nativeScheme.Create(s.user)
	c.Assert(err, check.IsNil)
	s.team = &authTypes.Team{Name: "admin"}
	s.token, err = nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	servicemock.SetMockService(&s.mockService)
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{*s.team}, nil
	}
	s.mockService.Team.OnFindByName = func(_ string) (*authTypes.Team, error) {
		return s.team, nil
	}
	s.mockService.Team.OnFindByNames = func(_ []string) ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}

	plan := appTypes.Plan{
		Name:     "default",
		Default:  true,
		CpuShare: 100,
	}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{plan}, nil
	}
	s.mockService.Plan.OnDefaultPlan = func() (*appTypes.Plan, error) {
		return &plan, nil
	}
	s.mockService.UserQuota.OnGet = func(email string) (*quota.Quota, error) {
		c.Assert(email, check.Equals, s.user.Email)
		return &s.user.Quota, nil
	}
	s.mockService.UserQuota.OnInc = func(email string, q int) error {
		c.Assert(email, check.Equals, s.user.Email)
		return nil
	}
	s.mockService.UserQuota.OnInc = func(email string, q int) error {
		c.Assert(email, check.Equals, s.user.Email)
		return nil
	}
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
	clust := &provTypes.Cluster{
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
	clust := &provTypes.Cluster{
		Addresses:   []string{s.clusterSrv.URL()},
		Default:     true,
		Name:        "c1",
		Provisioner: provisionerName,
	}
	s.initCluster(c, clust)
}

func (s *S) initCluster(c *check.C, clust *provTypes.Cluster) {
	s.mockService.Cluster.OnFindByPool = func(prov, pool string) (*provTypes.Cluster, error) {
		return clust, nil
	}
	s.mockService.Cluster.OnFindByProvisioner = func(prov string) ([]provTypes.Cluster, error) {
		return []provTypes.Cluster{*clust}, nil
	}
	prov, err := provision.Get(clust.Provisioner)
	c.Assert(err, check.IsNil)
	if clusterProv, ok := prov.(cluster.ClusteredProvisioner); ok {
		err = clusterProv.InitializeCluster(clust)
		c.Assert(err, check.IsNil)
	}

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
