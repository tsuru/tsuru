// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"math/rand"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/version"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	tsuruv1clientset "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned"
	faketsuru "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned/fake"
	kTesting "github.com/tsuru/tsuru/provision/kubernetes/testing"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/servicemanager"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/types/quota"
	"golang.org/x/crypto/bcrypt"
	check "gopkg.in/check.v1"
	apiv1 "k8s.io/api/core/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	fakeapiextensions "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/informers/internalinterfaces"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

type S struct {
	p             *kubernetesProvisioner
	conn          *db.Storage
	user          *auth.User
	team          *authTypes.Team
	token         auth.Token
	client        *kTesting.ClientWrapper
	clusterClient *ClusterClient
	t             *testing.T
	mock          *kTesting.KubeMock
	mockService   servicemock.MockService
	factory       informers.SharedInformerFactory
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
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "provision_kubernetes_tests_s")
	config.Set("queue:mongo-url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("queue:mongo-database", "queue_provision_kubernetes_tests")
	config.Set("queue:mongo-polling-interval", 0.01)
	config.Set("routers:fake:type", "fake")
	config.Set("routers:fake:default", true)
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	s.conn.Close()
}

func (s *S) TearDownTest(c *check.C) {
	stopClusterController(s.p, s.clusterClient)
}

func (s *S) SetUpTest(c *check.C) {
	err := dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
	clus := &provision.Cluster{
		Name:        "c1",
		Addresses:   []string{"https://clusteraddr"},
		Default:     true,
		Provisioner: provisionerName,
		CustomData:  map[string]string{},
	}
	s.clusterClient, err = NewClusterClient(clus)
	c.Assert(err, check.IsNil)
	s.client = &kTesting.ClientWrapper{
		Clientset:              fake.NewSimpleClientset(),
		ApiExtensionsClientset: fakeapiextensions.NewSimpleClientset(),
		TsuruClientset:         faketsuru.NewSimpleClientset(),
		ClusterInterface:       s.clusterClient,
	}
	s.clusterClient.Interface = s.client
	ClientForConfig = func(conf *rest.Config) (kubernetes.Interface, error) {
		return s.client, nil
	}
	TsuruClientForConfig = func(conf *rest.Config) (tsuruv1clientset.Interface, error) {
		return s.client.TsuruClientset, nil
	}
	ExtensionsClientForConfig = func(conf *rest.Config) (apiextensionsclientset.Interface, error) {
		return s.client.ApiExtensionsClientset, nil
	}
	routertest.FakeRouter.Reset()
	rand.Seed(0)
	err = pool.AddPool(pool.AddPoolOptions{
		Name:        "test-default",
		Default:     true,
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	s.factory = informers.NewSharedInformerFactory(s.client, 1)
	InformerFactory = func(client *ClusterClient, tweak internalinterfaces.TweakListOptionsFunc) (informers.SharedInformerFactory, error) {
		return s.factory, nil
	}
	s.p = &kubernetesProvisioner{
		clusterControllers: map[string]*clusterController{},
	}
	s.mock = kTesting.NewKubeMock(s.client, s.p, s.factory)
	s.client.ApiExtensionsClientset.PrependReactor("create", "customresourcedefinitions", s.mock.CRDReaction(c))
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
	clust := s.client.GetCluster()
	c.Assert(clust, check.NotNil)
	s.mockService.Cluster.OnFindByProvisioner = func(provName string) ([]provision.Cluster, error) {
		return []provision.Cluster{*clust}, nil
	}
	s.mockService.Cluster.OnFindByPool = func(provName, poolName string) (*provision.Cluster, error) {
		if provName == provisionerName {
			return clust, nil
		}
		return nil, provision.ErrNoCluster
	}
	s.mockService.Cluster.OnFindByPools = func(provName string, poolNames []string) (map[string]provision.Cluster, error) {
		ret := make(map[string]provision.Cluster)
		for _, pool := range poolNames {
			ret[pool] = *clust
		}
		return ret, nil
	}
	servicemanager.AppVersion, err = version.AppVersionService()
	c.Assert(err, check.IsNil)
}

func (s *S) waitNodeUpdate(c *check.C, fn func()) {
	s.mock.WaitNodeUpdate(c, fn)
}

func sortPods(pods []*apiv1.Pod) {
	sort.Slice(pods, func(i, j int) bool {
		return pods[i].Name < pods[j].Name
	})
}

func (s *S) waitPodUpdate(c *check.C, fn func()) {
	controller, err := getClusterController(s.p, s.clusterClient)
	c.Assert(err, check.IsNil)
	podInformer, err := controller.getPodInformer()
	c.Assert(err, check.IsNil)
	pods, err := podInformer.Lister().List(labels.Everything())
	c.Assert(err, check.IsNil)
	fn()
	timeout := time.After(5 * time.Second)
	for {
		podsAfter, err := podInformer.Lister().List(labels.Everything())
		c.Assert(err, check.IsNil)
		sortPods(pods)
		sortPods(podsAfter)
		if !reflect.DeepEqual(pods, podsAfter) {
			return
		}
		select {
		case <-time.After(100 * time.Millisecond):
		case <-timeout:
			c.Fatal("timeout waiting for node changes")
		}
	}
}
