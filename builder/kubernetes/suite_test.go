// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"math/rand"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision"
	kubeProv "github.com/tsuru/tsuru/provision/kubernetes"
	tsuruv1clientset "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned"
	faketsuru "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned/fake"
	kubeTesting "github.com/tsuru/tsuru/provision/kubernetes/testing"
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
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	fakeapiextensions "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

type testProv interface {
	provision.Provisioner
	provision.BuilderDeployKubeClient
}

type S struct {
	b             *kubernetesBuilder
	conn          *db.Storage
	user          *auth.User
	team          *authTypes.Team
	token         auth.Token
	lastConf      *rest.Config
	client        *kubeTesting.ClientWrapper
	clusterClient *kubeProv.ClusterClient
	p             testProv
	mock          *kubeTesting.KubeMock
	mockService   servicemock.MockService
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
	config.Set("database:name", "builder_kubernetes_tests_s")
	config.Set("routers:fake:type", "fake")
	config.Set("routers:fake:default", true)
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	s.conn.Close()
}

func (s *S) SetUpTest(c *check.C) {
	err := dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
	clus := &provTypes.Cluster{
		Name:        "c1",
		Addresses:   []string{"https://clusteraddr"},
		Default:     true,
		Provisioner: "kubernetes",
	}
	s.clusterClient, err = kubeProv.NewClusterClient(clus)
	c.Assert(err, check.IsNil)
	s.client = &kubeTesting.ClientWrapper{
		Clientset:              fake.NewSimpleClientset(),
		ClusterInterface:       s.clusterClient,
		ApiExtensionsClientset: fakeapiextensions.NewSimpleClientset(),
		TsuruClientset:         faketsuru.NewSimpleClientset(),
	}
	s.clusterClient.Interface = s.client
	kubeProv.ClientForConfig = func(conf *rest.Config) (kubernetes.Interface, error) {
		s.lastConf = conf
		return s.client, nil
	}
	kubeProv.TsuruClientForConfig = func(conf *rest.Config) (tsuruv1clientset.Interface, error) {
		return s.client.TsuruClientset, nil
	}
	kubeProv.ExtensionsClientForConfig = func(conf *rest.Config) (apiextensionsclientset.Interface, error) {
		return s.client.ApiExtensionsClientset, nil
	}
	s.client.ApiExtensionsClientset.PrependReactor("create", "customresourcedefinitions", s.mock.CRDReaction(c))
	routertest.FakeRouter.Reset()
	rand.Seed(0)
	err = pool.AddPool(pool.AddPoolOptions{
		Name:        "test-default",
		Default:     true,
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	plan := appTypes.Plan{
		Name:     "default",
		Default:  true,
		CpuShare: 100,
	}
	servicemock.SetMockService(&s.mockService)
	s.mockService.Plan = &appTypes.MockPlanService{
		OnList: func() ([]appTypes.Plan, error) {
			return []appTypes.Plan{plan}, nil
		},
		OnDefaultPlan: func() (*appTypes.Plan, error) {
			return &plan, nil
		},
	}
	s.mockService.Cluster.OnFindByPool = func(prov, pool string) (*provTypes.Cluster, error) {
		return clus, nil
	}
	s.b = &kubernetesBuilder{}
	s.p = kubeProv.GetProvisioner()
	s.mock = kubeTesting.NewKubeMock(s.client, s.p)
	s.user = &auth.User{Email: "whiskeyjack@genabackis.com", Password: "123456", Quota: quota.UnlimitedQuota}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	_, err = nativeScheme.Create(s.user)
	c.Assert(err, check.IsNil)
	s.team = &authTypes.Team{Name: "admin"}
	s.token, err = nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
}
