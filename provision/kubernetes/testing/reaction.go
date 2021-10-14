// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"bufio"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/provision"
	tsuruv1 "github.com/tsuru/tsuru/provision/kubernetes/pkg/apis/tsuru/v1"
	faketsuru "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned/fake"
	tsuruv1client "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned/typed/tsuru/v1"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/rebuild"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	provTypes "github.com/tsuru/tsuru/types/provision"
	check "gopkg.in/check.v1"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	extensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	fakeapiextensions "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apimachinery/pkg/util/httpstream/spdy"
	fakevpa "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned/fake"
	informers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	ktesting "k8s.io/client-go/testing"
	fakeBackendConfig "k8s.io/ingress-gce/pkg/backendconfig/client/clientset/versioned/fake"
	fakemetrics "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

const (
	trueStr = "true"
)

type ClusterInterface interface {
	CoreV1() v1core.CoreV1Interface
	RestConfig() *rest.Config
	AppNamespace(context.Context, appTypes.App) (string, error)
	PoolNamespace(string) string
	Namespace() string
	GetCluster() *provTypes.Cluster
}

type KubeMock struct {
	client        *ClientWrapper
	Stream        map[string]StreamResult
	LogHook       func(w io.Writer, r *http.Request)
	DefaultHook   func(w http.ResponseWriter, r *http.Request)
	p             provision.Provisioner
	factory       informers.SharedInformerFactory
	HandleSize    bool
	IgnorePool    bool
	IgnoreAppName bool
}

func NewKubeMock(cluster *ClientWrapper, p provision.Provisioner, factory informers.SharedInformerFactory) *KubeMock {
	stream := make(map[string]StreamResult)
	return &KubeMock{
		client:      cluster,
		Stream:      stream,
		LogHook:     nil,
		DefaultHook: nil,
		p:           p,
		factory:     factory,
	}
}

type ClientWrapper struct {
	*fake.Clientset
	ApiExtensionsClientset *fakeapiextensions.Clientset
	TsuruClientset         *faketsuru.Clientset
	MetricsClientset       *fakemetrics.Clientset
	VPAClientset           *fakevpa.Clientset
	BackendClientset       *fakeBackendConfig.Clientset
	ClusterInterface
}

func (c *ClientWrapper) TsuruV1() tsuruv1client.TsuruV1Interface {
	return c.TsuruClientset.TsuruV1()
}

func (c *ClientWrapper) ApiextensionsV1() apiextensionsv1.ApiextensionsV1Interface {
	return c.ApiExtensionsClientset.ApiextensionsV1()
}

func (c *ClientWrapper) CoreV1() v1core.CoreV1Interface {
	core := c.Clientset.CoreV1()
	return &clientCoreWrapper{core, c.ClusterInterface}
}

type clientCoreWrapper struct {
	v1core.CoreV1Interface
	cluster ClusterInterface
}

func (c *clientCoreWrapper) Pods(namespace string) v1core.PodInterface {
	pods := c.CoreV1Interface.Pods(namespace)
	return &clientPodsWrapper{pods, c.cluster}
}

type clientPodsWrapper struct {
	v1core.PodInterface
	cluster ClusterInterface
}

func (c *clientPodsWrapper) GetLogs(name string, opts *apiv1.PodLogOptions) *rest.Request {
	cli, _ := rest.RESTClientFor(c.cluster.RestConfig())
	return cli.Get().Namespace(c.cluster.Namespace()).Name(name).Resource("pods").SubResource("log").VersionedParams(opts, kscheme.ParameterCodec)
}

type StreamResult struct {
	Stdin  string
	Resize string
	Urls   []url.URL
}

func (s *KubeMock) DefaultReactions(c *check.C) (*provisiontest.FakeApp, func(), func()) {
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	err := s.p.Provision(context.TODO(), a)
	c.Assert(err, check.IsNil)
	a.Deploys = 1
	err = rebuild.Initialize(func(appName string) (rebuild.RebuildApp, error) {
		return &app.App{
			Name:    appName,
			Pool:    "test-default",
			Routers: a.GetRouters(),
		}, nil
	})
	c.Assert(err, check.IsNil)
	podReaction, deployPodReady := s.deployPodReaction(a, c)
	servReaction := s.ServiceWithPortReaction(c, nil)
	rollbackDeployment := s.DeploymentReactions(c)
	s.client.PrependReactor("create", "pods", podReaction)
	s.client.PrependReactor("create", "services", servReaction)
	s.client.TsuruClientset.PrependReactor("create", "apps", s.AppReaction(a, c))
	srv, wg := s.CreateDeployReadyServer(c)
	s.MockfakeNodes(c, srv.URL)
	return a, func() {
			rollbackDeployment()
			deployPodReady.Wait()
			wg.Wait()
		}, func() {
			rebuild.Shutdown(context.Background())
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

func (s *KubeMock) NoNodeReactions(c *check.C) (*provisiontest.FakeApp, func(), func()) {
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	err := s.p.Provision(context.TODO(), a)
	c.Assert(err, check.IsNil)
	a.Deploys = 1
	podReaction, deployPodReady := s.deployPodReaction(a, c)
	servReaction := s.ServiceWithPortReaction(c, nil)
	rollbackDeployment := s.DeploymentReactions(c)
	s.client.PrependReactor("create", "pods", podReaction)
	s.client.PrependReactor("create", "services", servReaction)
	s.client.TsuruClientset.PrependReactor("create", "apps", s.AppReaction(a, c))
	return a, func() {
			rollbackDeployment()
			deployPodReady.Wait()
		}, func() {
			rollbackDeployment()
			deployPodReady.Wait()
		}
}

func (s *KubeMock) NoAppReactions(c *check.C) (func(), func()) {
	podReaction, podReady := s.buildPodReaction(c)
	servReaction := s.ServiceWithPortReaction(c, nil)
	rollbackDeployment := s.DeploymentReactions(c)
	s.client.PrependReactor("create", "pods", podReaction)
	s.client.PrependReactor("create", "services", servReaction)
	srv, wg := s.CreateDeployReadyServer(c)
	s.MockfakeNodes(c, srv.URL)
	return func() {
			rollbackDeployment()
			podReady.Wait()
			wg.Wait()
		}, func() {
			rollbackDeployment()
			podReady.Wait()
			wg.Wait()
			if srv == nil {
				return
			}
			srv.Close()
			srv = nil
		}
}

func (s *KubeMock) CreateDeployReadyServer(c *check.C) (*httptest.Server, *sync.WaitGroup) {
	mu := sync.Mutex{}
	attachFn := func(w http.ResponseWriter, r *http.Request, cont string) {
		tty := r.FormValue("tty") == trueStr
		stdin := r.FormValue("stdin") == trueStr
		stdout := r.FormValue("stdout") == trueStr
		stderr := r.FormValue("stderr") == trueStr
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
			if s.HandleSize && scanner.Scan() {
				mu.Lock()
				res := s.Stream[cont]
				res.Resize = scanner.Text()
				s.Stream[cont] = res
				mu.Unlock()
			}
		}
		if stdin := streamMap[apiv1.StreamTypeStdin]; stdin != nil {
			data, _ := ioutil.ReadAll(stdin)
			mu.Lock()
			res := s.Stream[cont]
			res.Stdin = string(data)
			s.Stream[cont] = res
			mu.Unlock()
		}
		if stderr := streamMap[apiv1.StreamTypeStderr]; stderr != nil {
			if s.LogHook == nil {
				stderr.Write([]byte("stderr data"))
			}
		}
		if stdout := streamMap[apiv1.StreamTypeStdout]; stdout != nil {
			if s.LogHook != nil {
				s.LogHook(stdout, r)
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
		res := s.Stream[cont]
		res.Urls = append(res.Urls, *r.URL)
		s.Stream[cont] = res
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/attach") || strings.HasSuffix(r.URL.Path, "/exec") {
			attachFn(w, r, cont)
		} else if strings.HasSuffix(r.URL.Path, "/log") {
			if s.LogHook != nil {
				s.LogHook(w, r)
				return
			}
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "my log message")
		} else if s.DefaultHook != nil {
			s.DefaultHook(w, r)
		} else if r.URL.Path == "/api/v1/pods" {
			s.ListPodsHandler(c)(w, r)
		}
	}))
	return srv, &wg
}

func (s *KubeMock) ListPodsHandler(c *check.C, funcs ...func(r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.URL.Path, check.Equals, "/api/v1/pods")
		for _, f := range funcs {
			f(r)
		}
		nlist, err := s.client.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
		c.Assert(err, check.IsNil)
		response := apiv1.PodList{}
		namespaces := []string{}
		if len(nlist.Items) == 0 {
			namespaces = []string{"default"}
		}
		for _, n := range nlist.Items {
			namespaces = append(namespaces, n.GetName())
		}
		for _, n := range namespaces {
			podlist, errList := s.client.CoreV1().Pods(n).List(context.TODO(), metav1.ListOptions{LabelSelector: r.Form.Get("labelSelector")})
			c.Assert(errList, check.IsNil)
			response.Items = append(response.Items, podlist.Items...)
		}
		w.Header().Add("Content-type", "application/json")
		err = json.NewEncoder(w).Encode(response)
		c.Assert(err, check.IsNil)
	}
}

func SortNodes(nodes []*apiv1.Node) {
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Name < nodes[j].Name
	})
}

func (s *KubeMock) WaitNodeUpdate(c *check.C, fn func()) {
	s.WaitNodeUpdateCount(c, false, fn)
}

func (s *KubeMock) WaitNodeUpdateCount(c *check.C, countOnly bool, fn func()) {
	nodes, err := s.p.(provision.NodeProvisioner).ListNodes(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	var rawNodes []*apiv1.Node
	for _, n := range nodes {
		rawNodes = append(rawNodes, n.(interface{ RawNode() *apiv1.Node }).RawNode())
	}
	fn()
	timeout := time.After(5 * time.Second)
	for {
		nodes, err = s.p.(provision.NodeProvisioner).ListNodes(context.TODO(), nil)
		c.Assert(err, check.IsNil)
		var rawNodesAfter []*apiv1.Node
		for _, n := range nodes {
			rawNodesAfter = append(rawNodesAfter, n.(interface{ RawNode() *apiv1.Node }).RawNode())
		}
		if countOnly {
			if len(rawNodes) != len(rawNodesAfter) {
				return
			}
		} else {
			SortNodes(rawNodes)
			SortNodes(rawNodesAfter)
			if !reflect.DeepEqual(rawNodes, rawNodesAfter) {
				return
			}
		}
		select {
		case <-time.After(100 * time.Millisecond):
		case <-timeout:
			c.Fatal("timeout waiting for node changes")
		}
	}
}

func (s *KubeMock) MockfakeNodes(c *check.C, urls ...string) {
	if len(urls) > 0 {
		s.client.GetCluster().Addresses = urls
		s.client.ClusterInterface.RestConfig().Host = urls[0]
	}
	for i := 1; i <= 2; i++ {
		s.WaitNodeUpdate(c, func() {
			_, err := s.client.CoreV1().Nodes().Create(context.TODO(), &apiv1.Node{
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
			}, metav1.CreateOptions{})
			c.Assert(err, check.IsNil)
		})
	}
}

func (s *KubeMock) AppReaction(a provision.App, c *check.C) ktesting.ReactionFunc {
	return func(action ktesting.Action) (bool, runtime.Object, error) {
		if !s.IgnoreAppName {
			app := action.(ktesting.CreateAction).GetObject().(*tsuruv1.App)
			c.Assert(app.GetName(), check.Equals, a.GetName())
		}
		return false, nil, nil
	}
}

func (s *KubeMock) CRDReaction(c *check.C) ktesting.ReactionFunc {
	return func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		crd, ok := obj.(*extensionsv1beta1.CustomResourceDefinition)
		if ok {
			crd.Status.Conditions = []extensionsv1beta1.CustomResourceDefinitionCondition{
				{Type: extensionsv1beta1.Established, Status: extensionsv1beta1.ConditionTrue},
			}
		} else {
			crdV1, ok := obj.(*extensionsv1.CustomResourceDefinition)
			if !ok {
				return false, nil, errors.Errorf("invalid crd object %#v", obj)
			}
			crdV1.Status.Conditions = []extensionsv1.CustomResourceDefinitionCondition{
				{Type: extensionsv1.Established, Status: extensionsv1.ConditionTrue},
			}
		}
		return false, nil, nil
	}
}

func UpdatePodContainerStatus(pod *apiv1.Pod, running bool) {
	for _, cont := range pod.Spec.Containers {
		contStatus := apiv1.ContainerStatus{
			Name:  cont.Name,
			State: apiv1.ContainerState{},
			Ready: running,
		}
		if running {
			contStatus.State.Running = &apiv1.ContainerStateRunning{}
		} else {
			contStatus.State.Terminated = &apiv1.ContainerStateTerminated{
				ExitCode: 0,
			}
		}
		pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, contStatus)
	}
}

func (s *KubeMock) deployPodReaction(a provision.App, c *check.C) (ktesting.ReactionFunc, *sync.WaitGroup) {
	wg := sync.WaitGroup{}
	return func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		defer func() {
			err := s.factory.Core().V1().Pods().Informer().GetStore().Add(pod)
			c.Assert(err, check.IsNil)
		}()
		if !s.IgnorePool {
			c.Assert(pod.Spec.NodeSelector, check.DeepEquals, map[string]string{
				"tsuru.io/pool": a.GetPool(),
			})
		}
		c.Assert(pod.ObjectMeta.Labels, check.NotNil)
		c.Assert(pod.ObjectMeta.Labels["tsuru.io/is-tsuru"], check.Equals, trueStr)
		c.Assert(pod.ObjectMeta.Labels["tsuru.io/app-name"], check.Equals, a.GetName())
		c.Assert(pod.ObjectMeta.Labels["tsuru.io/app-platform"], check.Equals, a.GetPlatform())
		if !s.IgnorePool {
			c.Assert(pod.ObjectMeta.Labels["tsuru.io/app-pool"], check.Equals, a.GetPool())
		}
		c.Assert(pod.ObjectMeta.Labels["tsuru.io/provisioner"], check.Equals, "kubernetes")
		if !strings.HasSuffix(pod.Name, "-deploy") {
			return false, nil, nil
		}
		pod.Status.StartTime = &metav1.Time{Time: time.Now()}
		pod.Status.Phase = apiv1.PodSucceeded
		pod.Status.HostIP = "192.168.99.1"
		pod.Spec.NodeName = "n1"
		toRegister := false
		for _, cont := range pod.Spec.Containers {
			if strings.Contains(strings.Join(cont.Command, " "), "unit_agent") {
				toRegister = true
			}
		}
		if toRegister {
			UpdatePodContainerStatus(pod, true)
			pod.Status.Phase = apiv1.PodRunning
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := s.p.RegisterUnit(context.TODO(), a, pod.Name, map[string]interface{}{
					"processes": map[string]interface{}{
						"web":    "python myapp.py",
						"worker": "python myworker.py",
					},
				})
				c.Assert(err, check.IsNil)
				pod.Status.Phase = apiv1.PodSucceeded
				ns, err := s.client.AppNamespace(context.TODO(), a)
				c.Assert(err, check.IsNil)
				UpdatePodContainerStatus(pod, false)
				_, err = s.client.CoreV1().Pods(ns).Update(context.TODO(), pod, metav1.UpdateOptions{})
				c.Assert(err, check.IsNil)
				err = s.factory.Core().V1().Pods().Informer().GetStore().Update(pod)
				c.Assert(err, check.IsNil)
			}()
		}
		return false, nil, nil
	}, &wg
}

func (s *KubeMock) buildPodReaction(c *check.C) (ktesting.ReactionFunc, *sync.WaitGroup) {
	wg := sync.WaitGroup{}
	return func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		c.Assert(pod.Spec.Affinity, check.NotNil)
		c.Assert(pod.ObjectMeta.Labels, check.NotNil)
		c.Assert(pod.ObjectMeta.Labels["tsuru.io/is-tsuru"], check.Equals, trueStr)
		c.Assert(pod.ObjectMeta.Labels["tsuru.io/provisioner"], check.Equals, "kubernetes")
		c.Assert(pod.ObjectMeta.Annotations, check.NotNil)
		if !strings.HasSuffix(pod.Name, "-image-build") {
			return false, nil, nil
		}
		pod.Status.StartTime = &metav1.Time{Time: time.Now()}
		pod.Status.Phase = apiv1.PodSucceeded
		pod.Status.HostIP = "192.168.99.1"
		pod.Spec.NodeName = "n1"
		return false, nil, nil
	}, &wg
}

func (s *KubeMock) ServiceWithPortReaction(c *check.C, ports []apiv1.ServicePort) ktesting.ReactionFunc {
	return func(action ktesting.Action) (bool, runtime.Object, error) {
		srv := action.(ktesting.CreateAction).GetObject().(*apiv1.Service)
		defer func() {
			err := s.factory.Core().V1().Services().Informer().GetStore().Add(srv)
			c.Assert(err, check.IsNil)
		}()
		if len(srv.Spec.Ports) > 0 && srv.Spec.Ports[0].NodePort != int32(0) {
			return false, nil, nil
		}
		if len(ports) == 0 {
			srv.Spec.Ports = []apiv1.ServicePort{
				{
					NodePort: int32(30000),
				},
			}
		} else {
			srv.Spec.Ports = ports
		}
		return false, nil, nil
	}
}

func (s *KubeMock) DeploymentReactions(c *check.C) func() {
	depReaction, depPodReady := s.deploymentWithPodReaction(c)
	lastReactor := s.client.ReactionChain[len(s.client.ReactionChain)-1]
	s.client.PrependReactor("create", "deployments", depReaction)
	s.client.PrependReactor("update", "deployments", depReaction)
	s.client.PrependReactor("patch", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		_, ret, err := lastReactor.React(action)
		if err != nil {
			return false, nil, err
		}
		depPodReady.Add(1)
		patchAction := action.(ktesting.PatchAction)
		go func() {
			defer depPodReady.Done()
			dep, _ := s.client.AppsV1().Deployments(patchAction.GetNamespace()).Get(context.TODO(), patchAction.GetName(), metav1.GetOptions{})
			s.client.AppsV1().Deployments(patchAction.GetNamespace()).Update(context.TODO(), dep, metav1.UpdateOptions{})
		}()
		return true, ret, nil
	})
	return func() {
		depPodReady.Wait()
	}
}

func (s *KubeMock) deploymentWithPodReaction(c *check.C) (ktesting.ReactionFunc, *sync.WaitGroup) {
	var counter int32
	wg := sync.WaitGroup{}
	return func(action ktesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "" {
			return false, nil, nil
		}
		dep := action.(ktesting.CreateAction).GetObject().(*appsv1.Deployment)
		if dep.Annotations == nil {
			dep.Annotations = make(map[string]string)
		}
		var specReplicas int32
		if dep.Spec.Replicas != nil {
			specReplicas = *dep.Spec.Replicas
		}
		dep.Status.UpdatedReplicas = specReplicas
		dep.Status.Replicas = specReplicas
		if dep.Spec.Paused {
			return false, nil, nil
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.deployWithPodReaction(c, dep, specReplicas, &counter)
		}()
		return false, nil, nil
	}, &wg
}

func (s *KubeMock) deployWithPodReaction(c *check.C, dep *appsv1.Deployment, specReplicas int32, counter *int32) {
	var revision int
	if dep.Annotations["deployment.kubernetes.io/revision"] != "" {
		revision, _ = strconv.Atoi(dep.Annotations["deployment.kubernetes.io/revision"])
	}
	revision++

	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        dep.Name + "-1",
			Namespace:   dep.Namespace,
			Labels:      dep.Labels,
			Annotations: map[string]string{},
		},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: dep.Spec.Replicas,
			Selector: dep.Spec.Selector.DeepCopy(),
			Template: *dep.Spec.Template.DeepCopy(),
		},
	}

	for k, v := range dep.Annotations {
		rs.Annotations[k] = v
	}
	rs.ObjectMeta.Annotations["deployment.kubernetes.io/revision"] = fmt.Sprintf("%d", revision)
	rs.OwnerReferences = []metav1.OwnerReference{
		*metav1.NewControllerRef(dep, appsv1.SchemeGroupVersion.WithKind("Deployment")),
	}
	rs.ObjectMeta.Name = dep.Name + "-" + shortMD5ForObject(rs.Spec.Template.Spec)
	_, _ = s.client.AppsV1().ReplicaSets(dep.Namespace).Create(context.TODO(), rs, metav1.CreateOptions{})
	_, err := s.client.AppsV1().ReplicaSets(dep.Namespace).Update(context.TODO(), rs, metav1.UpdateOptions{})
	c.Assert(err, check.IsNil)
	_ = s.factory.Apps().V1().ReplicaSets().Informer().GetStore().Add(rs)
	err = s.factory.Apps().V1().ReplicaSets().Informer().GetStore().Update(rs)
	c.Assert(err, check.IsNil)

	pod := &apiv1.Pod{
		ObjectMeta: dep.Spec.Template.ObjectMeta,
		Spec:       dep.Spec.Template.Spec,
	}
	pod.ObjectMeta.CreationTimestamp = metav1.Time{Time: time.Now()}
	pod.Status.Phase = apiv1.PodRunning
	pod.Status.StartTime = &metav1.Time{Time: time.Now()}
	pod.ObjectMeta.Namespace = dep.Namespace
	pod.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
		*metav1.NewControllerRef(rs, appsv1.SchemeGroupVersion.WithKind("ReplicaSet")),
	}
	pod.Spec.NodeName = "n1"
	pod.Status.HostIP = "192.168.99.1"
	err = cleanupPods(s.client.ClusterInterface, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(dep.Spec.Selector.MatchLabels)).String(),
	}, dep.Namespace, s.factory)
	c.Assert(err, check.IsNil)
	for i := int32(1); i <= specReplicas; i++ {
		id := atomic.AddInt32(counter, 1)
		pod.ObjectMeta.Name = fmt.Sprintf("%s-pod-%d-%d", dep.Name, id, i)
		_, err = s.client.CoreV1().Pods(dep.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
		c.Assert(err, check.IsNil)
		err = s.factory.Core().V1().Pods().Informer().GetStore().Add(pod)
		c.Assert(err, check.IsNil)
	}
}

func cleanupPods(client ClusterInterface, opts metav1.ListOptions, namespace string, factory informers.SharedInformerFactory) error {
	pods, err := client.CoreV1().Pods(namespace).List(context.TODO(), opts)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, pod := range pods.Items {
		err = client.CoreV1().Pods(namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
		if err != nil && !k8sErrors.IsNotFound(err) {
			return errors.WithStack(err)
		}
		err = factory.Core().V1().Pods().Informer().GetStore().Delete(&pod)
		if err != nil && !k8sErrors.IsNotFound(err) {
			return errors.WithStack(err)
		}
	}
	return nil
}

func shortMD5ForObject(i interface{}) string {
	b, _ := json.Marshal(i)
	m := md5.Sum(b)

	return fmt.Sprintf("%x", m)[0:10]
}
