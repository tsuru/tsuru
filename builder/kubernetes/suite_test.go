// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/cluster"
	kubeProv "github.com/tsuru/tsuru/provision/kubernetes"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/router/routertest"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"golang.org/x/crypto/bcrypt"
	check "gopkg.in/check.v1"
	"k8s.io/api/apps/v1beta2"
	apiv1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apimachinery/pkg/util/httpstream/spdy"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	ktesting "k8s.io/client-go/testing"
)

type testProv interface {
	provision.Provisioner
	provision.BuilderDeployKubeClient
}

type S struct {
	b        *kubernetesBuilder
	conn     *db.Storage
	user     *auth.User
	team     *authTypes.Team
	token    auth.Token
	lastConf *rest.Config
	client   *clientWrapper
	p        testProv
	stream   map[string]streamResult
	logHook  func(w io.Writer, r *http.Request)
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
	//s.conn.Apps().Database.DropDatabase()
	s.conn.Close()
}

func (s *S) SetUpTest(c *check.C) {
	err := dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
	s.logHook = nil
	s.stream = make(map[string]streamResult)
	clus := &cluster.Cluster{
		Name:        "c1",
		Addresses:   []string{"https://clusteraddr"},
		Default:     true,
		Provisioner: "kubernetes",
	}
	err = clus.Save()
	c.Assert(err, check.IsNil)
	client, err := kubeProv.NewClusterClient(clus)
	c.Assert(err, check.IsNil)
	s.client = &clientWrapper{fake.NewSimpleClientset(), client}
	client.Interface = s.client
	kubeProv.ClientForConfig = func(conf *rest.Config) (kubernetes.Interface, error) {
		s.lastConf = conf
		return s.client, nil
	}
	routertest.FakeRouter.Reset()
	rand.Seed(0)
	err = pool.AddPool(pool.AddPoolOptions{
		Name:        "test-default",
		Default:     true,
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	p := appTypes.Plan{
		Name:     "default",
		Default:  true,
		CpuShare: 100,
	}
	err = app.SavePlan(p)
	c.Assert(err, check.IsNil)
	s.b = &kubernetesBuilder{}
	s.p = kubeProv.GetProvisioner()
	s.user = &auth.User{Email: "whiskeyjack@genabackis.com", Password: "123456", Quota: quota.Unlimited}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	_, err = nativeScheme.Create(s.user)
	c.Assert(err, check.IsNil)
	s.team = &authTypes.Team{Name: "admin"}
	err = auth.TeamService().Insert(*s.team)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
}

type streamResult struct {
	stdin  string
	resize string
	urls   []url.URL
}

type clientWrapper struct {
	*fake.Clientset
	*kubeProv.ClusterClient
}

func (c *clientWrapper) CoreV1() v1core.CoreV1Interface {
	core := c.Clientset.CoreV1()
	return &clientCoreWrapper{core, c.ClusterClient}
}

type clientCoreWrapper struct {
	v1core.CoreV1Interface
	cluster *kubeProv.ClusterClient
}

func (c *clientCoreWrapper) Pods(namespace string) v1core.PodInterface {
	pods := c.CoreV1Interface.Pods(namespace)
	return &clientPodsWrapper{pods, c.cluster}
}

type clientPodsWrapper struct {
	v1core.PodInterface
	cluster *kubeProv.ClusterClient
}

func (c *clientPodsWrapper) GetLogs(name string, opts *apiv1.PodLogOptions) *rest.Request {
	cli, _ := rest.RESTClientFor(c.cluster.RestConfig())
	return cli.Get().Namespace(c.cluster.Namespace()).Name(name).Resource("pods").SubResource("log").VersionedParams(opts, scheme.ParameterCodec)
}

func (s *S) defaultReactions(c *check.C) (*provisiontest.FakeApp, func(), func()) {
	srv, wg := s.createDeployReadyServer(c)
	s.mockfakeNodes(c, srv.URL)
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	a.Deploys = 1
	podReaction, deployPodReady := s.deployPodReaction(a, c)
	servReaction := s.serviceWithPortReaction(c)
	rollbackDeployment := s.deploymentReactions(c)
	s.client.PrependReactor("create", "pods", podReaction)
	s.client.PrependReactor("create", "services", servReaction)
	return a, func() {
			rollbackDeployment()
			deployPodReady.Wait()
			wg.Wait()
		}, func() {
			rollbackDeployment()
			deployPodReady.Wait()
			wg.Wait()
			if srv == nil {
				return
			}
			srv.Close()
			srv = nil
		}
}

func (s *S) createDeployReadyServer(c *check.C) (*httptest.Server, *sync.WaitGroup) {
	mu := sync.Mutex{}
	attachFn := func(w http.ResponseWriter, r *http.Request, cont string) {
		tty := r.FormValue("tty") == "true"
		stdin := r.FormValue("stdin") == "true"
		stdout := r.FormValue("stdout") == "true"
		stderr := r.FormValue("stderr") == "true"
		expected := 1
		if stdin {
			expected++
		}
		if stdout {
			expected++
		}
		if stderr || tty {
			expected++
		}
		_, err := httpstream.Handshake(r, w, []string{"v4.channel.k8s.io"})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		upgrader := spdy.NewResponseUpgrader()
		type streamAndReply struct {
			s httpstream.Stream
			r <-chan struct{}
		}
		streams := make(chan streamAndReply, expected)
		conn := upgrader.UpgradeResponse(w, r, func(stream httpstream.Stream, replySent <-chan struct{}) error {
			streams <- streamAndReply{s: stream, r: replySent}
			return nil
		})
		if conn == nil {
			return
		}
		defer conn.Close()
		waitStreamReply := func(replySent <-chan struct{}, notify chan<- struct{}) {
			<-replySent
			notify <- struct{}{}
		}
		replyChan := make(chan struct{})
		streamMap := map[string]httpstream.Stream{}
		receivedStreams := 0
		timeout := time.After(5 * time.Second)
	WaitForStreams:
		for {
			select {
			case stream := <-streams:
				streamType := stream.s.Headers().Get(apiv1.StreamType)
				streamMap[streamType] = stream.s
				go waitStreamReply(stream.r, replyChan)
			case <-replyChan:
				receivedStreams++
				if receivedStreams == expected {
					break WaitForStreams
				}
			case <-timeout:
				c.Fatalf("timeout waiting for channels, received %d of %d", receivedStreams, expected)
				return
			}
		}
		if resize := streamMap[apiv1.StreamTypeResize]; resize != nil {
			scanner := bufio.NewScanner(resize)
			if scanner.Scan() {
				mu.Lock()
				res := s.stream[cont]
				res.resize = scanner.Text()
				s.stream[cont] = res
				mu.Unlock()
			}
		}
		if stdin := streamMap[apiv1.StreamTypeStdin]; stdin != nil {
			data, _ := ioutil.ReadAll(stdin)
			mu.Lock()
			res := s.stream[cont]
			res.stdin = string(data)
			s.stream[cont] = res
			mu.Unlock()
		}
		if stderr := streamMap[apiv1.StreamTypeStderr]; stderr != nil {
			if s.logHook == nil {
				stderr.Write([]byte("stderr data"))
			}
		}
		if stdout := streamMap[apiv1.StreamTypeStdout]; stdout != nil {
			if s.logHook != nil {
				s.logHook(stdout, r)
				return
			}
			stdout.Write([]byte("stdout data"))
		}
	}
	wg := sync.WaitGroup{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wg.Add(1)
		defer wg.Done()
		cont := r.FormValue("container")
		mu.Lock()
		res := s.stream[cont]
		res.urls = append(res.urls, *r.URL)
		s.stream[cont] = res
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/attach") || strings.HasSuffix(r.URL.Path, "/exec") {
			attachFn(w, r, cont)
		} else if strings.HasSuffix(r.URL.Path, "/log") {
			if s.logHook != nil {
				s.logHook(w, r)
				return
			}
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "my log message")
		}
	}))
	return srv, &wg
}

func (s *S) mockfakeNodes(c *check.C, urls ...string) {
	if len(urls) > 0 {
		s.client.Cluster.Addresses = urls
		s.client.ClusterClient.RestConfig().Host = urls[0]
		err := s.client.Cluster.Save()
		c.Assert(err, check.IsNil)
	}
	for i := 1; i <= 2; i++ {
		_, err := s.client.CoreV1().Nodes().Create(&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("n%d", i),
				Labels: map[string]string{
					"tsuru.io/pool": "test-default",
				},
			},
			Status: apiv1.NodeStatus{
				Addresses: []apiv1.NodeAddress{
					{
						Type:    apiv1.NodeInternalIP,
						Address: fmt.Sprintf("192.168.99.%d", i),
					},
					{
						Type:    apiv1.NodeExternalIP,
						Address: fmt.Sprintf("200.0.0.%d", i),
					},
				},
			},
		})
		c.Assert(err, check.IsNil)
	}
}

func (s *S) deployPodReaction(a provision.App, c *check.C) (ktesting.ReactionFunc, *sync.WaitGroup) {
	wg := sync.WaitGroup{}
	return func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		c.Assert(pod.Spec.NodeSelector, check.DeepEquals, map[string]string{
			"tsuru.io/pool": a.GetPool(),
		})
		c.Assert(pod.ObjectMeta.Labels, check.NotNil)
		c.Assert(pod.ObjectMeta.Labels["tsuru.io/is-tsuru"], check.Equals, "true")
		c.Assert(pod.ObjectMeta.Labels["tsuru.io/app-name"], check.Equals, a.GetName())
		c.Assert(pod.ObjectMeta.Labels["tsuru.io/app-platform"], check.Equals, a.GetPlatform())
		c.Assert(pod.ObjectMeta.Labels["tsuru.io/app-pool"], check.Equals, a.GetPool())
		c.Assert(pod.ObjectMeta.Labels["tsuru.io/router-type"], check.Equals, "fake")
		c.Assert(pod.ObjectMeta.Labels["tsuru.io/router-name"], check.Equals, "fake")
		c.Assert(pod.ObjectMeta.Labels["tsuru.io/provisioner"], check.Equals, "kubernetes")
		if !strings.HasSuffix(pod.Name, "-build") {
			return false, nil, nil
		}
		pod.Status.StartTime = &metav1.Time{Time: time.Now()}
		pod.Status.Phase = apiv1.PodSucceeded
		pod.Spec.NodeName = "n1"
		toRegister := false
		for _, cont := range pod.Spec.Containers {
			if strings.Contains(strings.Join(cont.Command, " "), "unit_agent") {
				toRegister = true
			}
		}
		if toRegister {
			pod.Status.Phase = apiv1.PodRunning
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := s.p.RegisterUnit(a, pod.Name, map[string]interface{}{
					"processes": map[string]interface{}{
						"web":    "python myapp.py",
						"worker": "python myworker.py",
					},
				})
				c.Assert(err, check.IsNil)
				pod.Status.Phase = apiv1.PodSucceeded
				_, err = s.client.CoreV1().Pods(s.client.Namespace()).Update(pod)
				c.Assert(err, check.IsNil)
			}()
		}
		return false, nil, nil
	}, &wg
}

func (s *S) serviceWithPortReaction(c *check.C) ktesting.ReactionFunc {
	return func(action ktesting.Action) (bool, runtime.Object, error) {
		srv := action.(ktesting.CreateAction).GetObject().(*apiv1.Service)
		srv.Spec.Ports = []apiv1.ServicePort{
			{
				NodePort: int32(30000),
			},
		}
		return false, nil, nil
	}
}

func (s *S) deploymentReactions(c *check.C) func() {
	depReaction, depPodReady := s.deploymentWithPodReaction(c)
	s.client.PrependReactor("create", "deployments", depReaction)
	s.client.PrependReactor("update", "deployments", depReaction)
	return func() {
		depPodReady.Wait()
	}
}

func (s *S) deploymentWithPodReaction(c *check.C) (ktesting.ReactionFunc, *sync.WaitGroup) {
	wg := sync.WaitGroup{}
	var counter int32
	return func(action ktesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "" {
			return false, nil, nil
		}
		wg.Add(1)
		dep := action.(ktesting.CreateAction).GetObject().(*v1beta2.Deployment)
		var specReplicas int32
		if dep.Spec.Replicas != nil {
			specReplicas = *dep.Spec.Replicas
		}
		dep.Status.UpdatedReplicas = specReplicas
		dep.Status.Replicas = specReplicas
		go func() {
			defer wg.Done()
			pod := &apiv1.Pod{
				ObjectMeta: dep.Spec.Template.ObjectMeta,
				Spec:       dep.Spec.Template.Spec,
			}
			pod.Status.Phase = apiv1.PodRunning
			pod.Status.StartTime = &metav1.Time{Time: time.Now()}
			pod.ObjectMeta.Namespace = dep.Namespace
			pod.Spec.NodeName = "n1"
			err := cleanupPods(s.client.ClusterClient, metav1.ListOptions{
				LabelSelector: labels.SelectorFromSet(labels.Set(dep.Spec.Selector.MatchLabels)).String(),
			})
			c.Assert(err, check.IsNil)
			for i := int32(1); i <= specReplicas; i++ {
				id := atomic.AddInt32(&counter, 1)
				pod.ObjectMeta.Name = fmt.Sprintf("%s-pod-%d-%d", dep.Name, id, i)
				_, err = s.client.CoreV1().Pods(dep.Namespace).Create(pod)
				c.Assert(err, check.IsNil)
			}
		}()
		return false, nil, nil
	}, &wg
}

func cleanupPods(client *kubeProv.ClusterClient, opts metav1.ListOptions) error {
	pods, err := client.CoreV1().Pods(client.Namespace()).List(opts)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, pod := range pods.Items {
		err = client.CoreV1().Pods(client.Namespace()).Delete(pod.Name, &metav1.DeleteOptions{})
		if err != nil && !k8sErrors.IsNotFound(err) {
			return errors.WithStack(err)
		}
	}
	return nil
}
