package migrate

import (
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision/cluster"
	kubeProv "github.com/tsuru/tsuru/provision/kubernetes"
	tsuruv1 "github.com/tsuru/tsuru/provision/kubernetes/pkg/apis/tsuru/v1"
	tsuruv1clientset "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned"
	faketsuru "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned/fake"
	kubeTesting "github.com/tsuru/tsuru/provision/kubernetes/testing"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/servicemanager"
	apptypes "github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/types/quota"
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
	mock          *kubeTesting.KubeMock
}

var _ = check.Suite(&S{})

func Test(t *testing.T) { check.TestingT(t) }

func (s *S) SetUpSuite(c *check.C) {
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "kubernetes_migrate_tests_s")
	config.Set("routers:fake:type", "fake")
	config.Set("routers:fake:default", true)
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
		Pools:       []string{"kube", "test-default"},
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
	s.mock = kubeTesting.NewKubeMock(s.client, kubeProv.GetProvisioner())
	s.client.ApiExtensionsClientset.PrependReactor("create", "customresourcedefinitions", s.mock.CRDReaction(c))
	kubeProv.ExtensionsClientForConfig = func(conf *rest.Config) (apiextensionsclientset.Interface, error) {
		return s.client.ApiExtensionsClientset, nil
	}
	err = pool.AddPool(pool.AddPoolOptions{
		Name:        "test-default",
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	err = pool.AddPool(pool.AddPoolOptions{
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
}

func (s *S) SetUpTest(c *check.C) {
	err := s.conn.Apps().DropCollection()
	c.Assert(err, check.IsNil)
	appList, err := s.client.TsuruV1().Apps("default").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	for _, a := range appList.Items {
		err = s.client.TsuruV1().Apps("default").Delete(a.GetName(), &metav1.DeleteOptions{})
		c.Assert(err, check.IsNil)
	}
}

func (s *S) TearDownSuite(c *check.C) {
	s.conn.Close()
}

func (s *S) TestMigrateAppsCRDs(c *check.C) {
	apps := []app.App{
		{Name: "app-kube", Pool: "kube"},
		{Name: "app-kube2", Pool: "kube"},
		{Name: "app-kube-failed", Pool: "kube-failed"},
		{Name: "app-docker", Pool: "docker"},
	}
	for _, a := range apps {
		err := s.conn.Apps().Insert(a)
		c.Assert(err, check.IsNil)
	}
	err := MigrateAppsCRDs()
	c.Assert(err, check.NotNil)
	appList, err := s.client.TsuruV1().Apps("default").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(appList.Items), check.Equals, 2)
	c.Assert(appList.Items[0].Name, check.Equals, "app-kube")
	c.Assert(appList.Items[1].Name, check.Equals, "app-kube2")
}

func (s *S) TestMigrateAppsCRDsDeployedApp(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := s.conn.Apps().Insert(app.App{Name: a.GetName(), Pool: a.GetPool()})
	c.Assert(err, check.IsNil)
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	u := &auth.User{Email: "whiskeyjack@genabackis.com", Password: "123456", Quota: quota.UnlimitedQuota}
	_, err = nativeScheme.Create(u)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-myapp:v1", customData)
	c.Assert(err, check.IsNil)
	_, err = kubeProv.GetProvisioner().Deploy(a, "tsuru/app-myapp:v1", evt)
	c.Assert(err, check.IsNil)
	err = s.client.TsuruV1().Apps("default").Delete(a.GetName(), &metav1.DeleteOptions{})
	c.Assert(err, check.IsNil)
	appList, err := s.client.TsuruV1().Apps("default").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(appList.Items), check.Equals, 0)
	err = MigrateAppsCRDs()
	c.Assert(err, check.IsNil)
	appList, err = s.client.TsuruV1().Apps("default").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(appList.Items), check.Equals, 1)
	c.Assert(appList.Items[0].Spec, check.DeepEquals, tsuruv1.AppSpec{
		NamespaceName: "default",
		Deployments:   []string{"myapp-web"},
		Services:      []string{"myapp-web", "myapp-web-units"},
	})
}
