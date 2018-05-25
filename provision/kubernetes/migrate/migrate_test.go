package migrate

import (
	"sync"
	"testing"

	"github.com/tsuru/tsuru/servicemanager"
	apptypes "github.com/tsuru/tsuru/types/app"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision/cluster"
	kubeProv "github.com/tsuru/tsuru/provision/kubernetes"
	tsuruv1clientset "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned"
	faketsuru "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned/fake"
	kubeTesting "github.com/tsuru/tsuru/provision/kubernetes/testing"
	"github.com/tsuru/tsuru/provision/pool"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/check.v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	fakeapiextensions "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

type S struct {
	conn          *db.Storage
	clusterClient *kubeProv.ClusterClient
	client        *kubeTesting.ClientWrapper
}

var _ = check.Suite(&S{})

func Test(t *testing.T) { check.TestingT(t) }

func (s *S) SetUpSuite(c *check.C) {
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "kubernetes_migrate_tests_s")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	err = dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
	servicemanager.Cache = &apptypes.MockCacheService{}
	clus := &cluster.Cluster{
		Name:        "c1",
		Addresses:   []string{"https://clusteraddr"},
		Provisioner: "kubernetes",
		Pools:       []string{"kube"},
	}
	err = clus.Save()
	c.Assert(err, check.IsNil)
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
	once := sync.Once{}
	kubeProv.ExtensionsClientForConfig = func(conf *rest.Config) (apiextensionsclientset.Interface, error) {
		once.Do(func() {
			mock := kubeTesting.NewKubeMock(s.client, kubeProv.GetProvisioner())
			s.client.ApiExtensionsClientset.PrependReactor("create", "customresourcedefinitions", mock.CRDReaction(c))
		})
		return s.client.ApiExtensionsClientset, nil
	}
}

func (s *S) TearDownSuite(c *check.C) {
	s.conn.Close()
}

func (s *S) TestMigrateAppsCRDs(c *check.C) {
	err := pool.AddPool(pool.AddPoolOptions{
		Name:        "kube",
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	err = pool.AddPool(pool.AddPoolOptions{
		Name:        "kube-failed",
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	err = pool.AddPool(pool.AddPoolOptions{
		Name:        "docker",
		Provisioner: "docker",
	})
	c.Assert(err, check.IsNil)
	apps := []app.App{
		{Name: "app-kube", Pool: "kube"},
		{Name: "app-kube2", Pool: "kube"},
		{Name: "app-kube-failed", Pool: "kube-failed"},
		{Name: "app-docker", Pool: "docker"},
	}
	for _, a := range apps {
		err = s.conn.Apps().Insert(a)
		c.Assert(err, check.IsNil)
	}
	err = MigrateAppsCRDs()
	c.Assert(err, check.ErrorMatches, `.*no cluster.*`)
	appList, err := s.client.TsuruV1().Apps("default").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(appList.Items), check.Equals, 2)
	c.Assert(appList.Items[0].Name, check.Equals, "app-kube")
	c.Assert(appList.Items[1].Name, check.Equals, "app-kube2")
}
