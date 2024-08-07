// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"testing"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/version"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/provision"
	kubeProv "github.com/tsuru/tsuru/provision/kubernetes"
	tsuruv1clientset "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned"
	faketsuru "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned/fake"
	kubeTesting "github.com/tsuru/tsuru/provision/kubernetes/testing"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/servicemanager"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	provTypes "github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/types/quota"
	"github.com/tsuru/tsuru/volume"
	"golang.org/x/crypto/bcrypt"
	check "gopkg.in/check.v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	fakeapiextensions "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/informers/internalinterfaces"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

type testProv interface {
	provision.Provisioner
	provision.JobProvisioner
	shutdown.Shutdownable
}

type S struct {
	b             *kubernetesBuilder
	user          *auth.User
	team          *authTypes.Team
	token         auth.Token
	client        *kubeTesting.ClientWrapper
	clusterClient *kubeProv.ClusterClient
	p             testProv
	mock          *kubeTesting.KubeMock
	mockService   servicemock.MockService
	factory       informers.SharedInformerFactory
	t             *testing.T
}

var suiteInstance = &S{}
var _ = check.Suite(suiteInstance)

func Test(t *testing.T) {
	suiteInstance.t = t
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

	storagev2.Reset()
}

func (s *S) TearDownSuite(c *check.C) {
}

func (s *S) TearDownTest(c *check.C) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := s.p.Shutdown(ctx)
	c.Assert(err, check.IsNil)
}

func (s *S) SetUpTest(c *check.C) {
	err := storagev2.ClearAllCollections(nil)
	c.Assert(err, check.IsNil)
	clus := &provTypes.Cluster{
		Name:        "c1",
		Addresses:   []string{"https://clusteraddr"},
		Default:     true,
		Provisioner: "kubernetes",
		CustomData:  map[string]string{},
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
		return s.client, nil
	}
	kubeProv.TsuruClientForConfig = func(conf *rest.Config) (tsuruv1clientset.Interface, error) {
		return s.client.TsuruClientset, nil
	}
	kubeProv.ExtensionsClientForConfig = func(conf *rest.Config) (apiextensionsclientset.Interface, error) {
		return s.client.ApiExtensionsClientset, nil
	}
	routertest.FakeRouter.Reset()
	err = pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "test-default",
		Default:     true,
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	plan := appTypes.Plan{
		Name:    "default",
		Default: true,
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
	s.mockService.Cluster.OnList = func() ([]provTypes.Cluster, error) {
		return []provTypes.Cluster{*clus}, nil
	}
	s.mockService.Cluster.OnFindByProvisioner = func(provName string) ([]provTypes.Cluster, error) {
		return []provTypes.Cluster{*clus}, nil
	}
	s.mockService.Cluster.OnFindByPool = func(provName, poolName string) (*provTypes.Cluster, error) {
		return clus, nil
	}
	s.mockService.Cluster.OnFindByPools = func(provName string, poolNames []string) (map[string]provTypes.Cluster, error) {
		ret := make(map[string]provTypes.Cluster)
		for _, pool := range poolNames {
			ret[pool] = *clus
		}
		return ret, nil
	}
	s.b = &kubernetesBuilder{}
	s.p = kubeProv.GetProvisioner()
	s.factory = informers.NewSharedInformerFactory(s.client, time.Minute)
	kubeProv.InformerFactory = func(client *kubeProv.ClusterClient, tweak internalinterfaces.TweakListOptionsFunc) (informers.SharedInformerFactory, error) {
		return s.factory, nil
	}
	s.mock = kubeTesting.NewKubeMock(s.client, s.p, s.p, s.factory)
	s.client.ApiExtensionsClientset.PrependReactor("create", "customresourcedefinitions", s.mock.CRDReaction(c))
	s.user = &auth.User{Email: "whiskeyjack@genabackis.com", Password: "123456", Quota: quota.UnlimitedQuota}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	_, err = nativeScheme.Create(context.TODO(), s.user)
	c.Assert(err, check.IsNil)
	s.team = &authTypes.Team{Name: "admin"}
	s.token, err = nativeScheme.Login(context.TODO(), map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	servicemanager.AppVersion, err = version.AppVersionService()
	c.Assert(err, check.IsNil)
	servicemanager.Volume, err = volume.VolumeService()
	c.Assert(err, check.IsNil)
}
