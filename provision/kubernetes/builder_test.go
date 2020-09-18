// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/safe"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	provTypes "github.com/tsuru/tsuru/types/provision"
	check "gopkg.in/check.v1"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktesting "k8s.io/client-go/testing"
)

func newEmptyVersion(c *check.C, app appTypes.App) appTypes.AppVersion {
	version, err := servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
		App: app,
	})
	c.Assert(err, check.IsNil)
	return version
}

func newVersion(c *check.C, app appTypes.App, customData map[string]interface{}) appTypes.AppVersion {
	version := newEmptyVersion(c, app)
	err := version.CommitBuildImage()
	c.Assert(err, check.IsNil)
	err = version.AddData(appTypes.AddVersionDataArgs{
		CustomData: customData,
	})
	c.Assert(err, check.IsNil)
	return version
}

func newCommittedVersion(c *check.C, app appTypes.App, customData map[string]interface{}) appTypes.AppVersion {
	version := newVersion(c, app, customData)
	err := version.CommitBaseImage()
	c.Assert(err, check.IsNil)
	return version
}

func newSuccessfulVersion(c *check.C, app appTypes.App, customData map[string]interface{}) appTypes.AppVersion {
	version := newCommittedVersion(c, app, customData)
	err := version.CommitSuccessful()
	c.Assert(err, check.IsNil)
	return version
}

func (s *S) TestBuildPod(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := s.p.Provision(context.TODO(), a)
	c.Assert(err, check.IsNil)
	s.client.Fake.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		containers := pod.Spec.Containers
		c.Assert(containers, check.HasLen, 2)
		sort.Slice(containers, func(i, j int) bool { return containers[i].Name < containers[j].Name })
		cmds := cleanCmds(containers[0].Command[2])
		c.Assert(cmds, check.Equals, `end() { touch /tmp/intercontainer/done; }
trap end EXIT
mkdir -p $(dirname /home/application/archive.tar.gz) && cat >/home/application/archive.tar.gz && tsuru_unit_agent   myapp "/var/lib/tsuru/deploy archive file:///home/application/archive.tar.gz" build`)
		c.Assert(containers[0].Env, check.DeepEquals, []apiv1.EnvVar{
			{Name: "DEPLOYAGENT_RUN_AS_SIDECAR", Value: "true"},
			{Name: "DEPLOYAGENT_DESTINATION_IMAGES", Value: "tsuru/app-myapp:mytag"},
			{Name: "DEPLOYAGENT_SOURCE_IMAGE", Value: ""},
			{Name: "DEPLOYAGENT_REGISTRY_AUTH_USER", Value: ""},
			{Name: "DEPLOYAGENT_REGISTRY_AUTH_PASS", Value: ""},
			{Name: "DEPLOYAGENT_REGISTRY_ADDRESS", Value: ""},
			{Name: "DEPLOYAGENT_INPUT_FILE", Value: "/home/application/archive.tar.gz"},
			{Name: "DEPLOYAGENT_RUN_AS_USER", Value: "1000"},
			{Name: "DEPLOYAGENT_DOCKERFILE_BUILD", Value: "false"},
		})
		return false, nil, nil
	})
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	buf := strings.NewReader("my upload data")
	client := KubeClient{}
	version, err := servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
		App:            a,
		CustomBuildTag: "mytag",
	})
	c.Assert(err, check.IsNil)
	err = client.BuildPod(context.TODO(), a, evt, ioutil.NopCloser(buf), version)
	c.Assert(err, check.IsNil)
}

func (s *S) TestBuildPodWithPoolNamespaces(c *check.C) {
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := s.p.Provision(context.TODO(), a)
	c.Assert(err, check.IsNil)
	var counter int32
	nsName, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	s.client.PrependReactor("create", "namespaces", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		atomic.AddInt32(&counter, 1)
		ns, ok := action.(ktesting.CreateAction).GetObject().(*apiv1.Namespace)
		c.Assert(ok, check.Equals, true)
		c.Assert(ns.ObjectMeta.Name, check.Equals, nsName)
		return false, nil, nil
	})
	s.client.Fake.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		containers := pod.Spec.Containers
		c.Assert(containers, check.HasLen, 2)
		sort.Slice(containers, func(i, j int) bool { return containers[i].Name < containers[j].Name })
		cmds := cleanCmds(containers[0].Command[2])
		c.Assert(cmds, check.Equals, `end() { touch /tmp/intercontainer/done; }
trap end EXIT
mkdir -p $(dirname /home/application/archive.tar.gz) && cat >/home/application/archive.tar.gz && tsuru_unit_agent   myapp "/var/lib/tsuru/deploy archive file:///home/application/archive.tar.gz" build`)
		c.Assert(containers[0].Env, check.DeepEquals, []apiv1.EnvVar{
			{Name: "DEPLOYAGENT_RUN_AS_SIDECAR", Value: "true"},
			{Name: "DEPLOYAGENT_DESTINATION_IMAGES", Value: "tsuru/app-myapp:mytag"},
			{Name: "DEPLOYAGENT_SOURCE_IMAGE", Value: ""},
			{Name: "DEPLOYAGENT_REGISTRY_AUTH_USER", Value: ""},
			{Name: "DEPLOYAGENT_REGISTRY_AUTH_PASS", Value: ""},
			{Name: "DEPLOYAGENT_REGISTRY_ADDRESS", Value: ""},
			{Name: "DEPLOYAGENT_INPUT_FILE", Value: "/home/application/archive.tar.gz"},
			{Name: "DEPLOYAGENT_RUN_AS_USER", Value: "1000"},
			{Name: "DEPLOYAGENT_DOCKERFILE_BUILD", Value: "false"},
		})
		return false, nil, nil
	})
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	buf := strings.NewReader("my upload data")
	client := KubeClient{}
	version, err := servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
		App:            a,
		CustomBuildTag: "mytag",
	})
	c.Assert(err, check.IsNil)
	err = client.BuildPod(context.TODO(), a, evt, ioutil.NopCloser(buf), version)
	c.Assert(err, check.IsNil)
	c.Assert(atomic.LoadInt32(&counter), check.Equals, int32(1))
}

func (s *S) TestImageTagPushAndInspect(c *check.C) {
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		output := `{
"image": {"Id":"1234"},
"procfile": "web: make run",
"tsuruYaml": {"healthcheck": {"path": "/health",  "scheme": "https"}}
}`
		w.Write([]byte(output))
	}
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	client := KubeClient{}
	evt := s.newTestEvent(c, a)
	version := newEmptyVersion(c, a)
	procData, err := client.ImageTagPushAndInspect(context.TODO(), a, evt, "tsuru/app-myapp:tag1", version)
	c.Assert(err, check.IsNil)
	c.Assert(procData.Image.ID, check.Equals, "1234")
	c.Assert(procData.Procfile, check.Equals, "web: make run")
	c.Assert(procData.TsuruYaml.Healthcheck.Path, check.Equals, "/health")
	c.Assert(procData.TsuruYaml.Healthcheck.Scheme, check.Equals, "https")
}

func (s *S) TestImageTagPushAndInspectWithPoolNamespaces(c *check.C) {
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		output := `{
"image": {"Id":"1234"},
"procfile": "web: make run",
"tsuruYaml": {"healthcheck": {"path": "/health",  "scheme": "https"}}
}`
		w.Write([]byte(output))
	}
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	var counter int32
	nsName, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	s.client.PrependReactor("create", "namespaces", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		atomic.AddInt32(&counter, 1)
		ns, ok := action.(ktesting.CreateAction).GetObject().(*apiv1.Namespace)
		c.Assert(ok, check.Equals, true)
		c.Assert(ns.ObjectMeta.Name, check.Equals, nsName)
		return false, nil, nil
	})
	client := KubeClient{}
	evt := s.newTestEvent(c, a)
	version := newEmptyVersion(c, a)
	_, err = client.ImageTagPushAndInspect(context.TODO(), a, evt, "tsuru/app-myapp:tag1", version)
	c.Assert(err, check.IsNil)
	c.Assert(atomic.LoadInt32(&counter), check.Equals, int32(1))
}

func (s *S) TestImageTagPushAndInspectWithRegistryAuth(c *check.C) {
	config.Set("docker:registry", "registry.example.com")
	defer config.Unset("docker:registry")
	config.Set("docker:registry-auth:username", "user")
	defer config.Unset("docker:registry-auth:username")
	config.Set("docker:registry-auth:password", "pwd")
	defer config.Unset("docker:registry-auth:password")

	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		output := `{
"image": {"Id":"1234"},
"procfile": "web: make run",
"tsuruYaml": {"healthcheck": {"path": "/health",  "scheme": "https"}}
}`
		w.Write([]byte(output))
	}
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newEmptyVersion(c, a)
	s.client.Fake.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		containers := pod.Spec.Containers
		if containers[0].Name == "myapp-v1-deploy" {
			c.Assert(containers, check.HasLen, 2)
			cmds := cleanCmds(containers[0].Command[2])
			c.Assert(cmds, check.Equals, `while [ ! -f /tmp/intercontainer/done ]; do sleep 5; done`)
			cmds = cleanCmds(containers[1].Command[2])
			c.Assert(cmds, check.Equals, `end() { touch /tmp/intercontainer/done; }
trap end EXIT
cat >/dev/null && /bin/deploy-agent`)
			c.Assert(containers[1].Env, check.DeepEquals, []apiv1.EnvVar{
				{Name: "DEPLOYAGENT_RUN_AS_SIDECAR", Value: "true"},
				{Name: "DEPLOYAGENT_DESTINATION_IMAGES", Value: version.BaseImageName() + ",registry.example.com/tsuru/app-myapp:latest"},
				{Name: "DEPLOYAGENT_SOURCE_IMAGE", Value: "registry.example.com/tsuru/app-myapp:tag1"},
				{Name: "DEPLOYAGENT_REGISTRY_AUTH_USER", Value: "user"},
				{Name: "DEPLOYAGENT_REGISTRY_AUTH_PASS", Value: "pwd"},
				{Name: "DEPLOYAGENT_REGISTRY_ADDRESS", Value: "registry.example.com"},
				{Name: "DEPLOYAGENT_INPUT_FILE", Value: ""},
				{Name: "DEPLOYAGENT_RUN_AS_USER", Value: "1000"},
				{Name: "DEPLOYAGENT_DOCKERFILE_BUILD", Value: "false"},
			})
		}
		return false, nil, nil
	})

	client := KubeClient{}
	evt := s.newTestEvent(c, a)
	procData, err := client.ImageTagPushAndInspect(context.TODO(), a, evt, "registry.example.com/tsuru/app-myapp:tag1", version)
	c.Assert(err, check.IsNil)
	c.Assert(procData.Image.ID, check.Equals, "1234")
	c.Assert(procData.Procfile, check.Equals, "web: make run")
	c.Assert(procData.TsuruYaml.Healthcheck.Path, check.Equals, "/health")
	c.Assert(procData.TsuruYaml.Healthcheck.Scheme, check.Equals, "https")
}

func (s *S) TestImageTagPushAndInspectWithKubernetesConfig(c *check.C) {
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		output := `{
"image": {"Id":"1234"},
"procfile": "web: make run",
"tsuruYaml": {"kubernetes": {"groups": {"pod1": {"web": {"ports": [{"name": "http", "protocol": "TCP", "port": 8080, "target_port": 8888}]}}}}}
}`
		w.Write([]byte(output))
	}
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	client := KubeClient{}
	evt := s.newTestEvent(c, a)
	version := newEmptyVersion(c, a)
	procData, err := client.ImageTagPushAndInspect(context.TODO(), a, evt, "tsuru/app-myapp:tag1", version)
	c.Assert(err, check.IsNil)
	c.Assert(procData.Image.ID, check.Equals, "1234")
	c.Assert(procData.Procfile, check.Equals, "web: make run")
	c.Assert(procData.TsuruYaml.Kubernetes.Groups["pod1"]["web"].Ports[0].Name, check.Equals, "http")
	c.Assert(procData.TsuruYaml.Kubernetes.Groups["pod1"]["web"].Ports[0].Protocol, check.Equals, "TCP")
	c.Assert(procData.TsuruYaml.Kubernetes.Groups["pod1"]["web"].Ports[0].Port, check.Equals, 8080)
	c.Assert(procData.TsuruYaml.Kubernetes.Groups["pod1"]["web"].Ports[0].TargetPort, check.Equals, 8888)
}

func (s *S) TestBuildImage(c *check.C) {
	_, rollback := s.mock.NoAppReactions(c)
	defer rollback()
	s.client.Fake.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		containers := pod.Spec.Containers
		c.Assert(containers, check.HasLen, 1)
		cmds := cleanCmds(containers[0].Command[2])
		c.Assert(cmds, check.Equals, `mkdir -p $(dirname /data/context.tar.gz) && cat >/data/context.tar.gz && tsuru_unit_agent`)
		c.Assert(containers[0].Env, check.DeepEquals, []apiv1.EnvVar{
			{Name: "DEPLOYAGENT_RUN_AS_SIDECAR", Value: "true"},
			{Name: "DEPLOYAGENT_DESTINATION_IMAGES", Value: "tsuru/myplatform:latest"},
			{Name: "DEPLOYAGENT_SOURCE_IMAGE", Value: ""},
			{Name: "DEPLOYAGENT_REGISTRY_AUTH_USER", Value: ""},
			{Name: "DEPLOYAGENT_REGISTRY_AUTH_PASS", Value: ""},
			{Name: "DEPLOYAGENT_REGISTRY_ADDRESS", Value: ""},
			{Name: "DEPLOYAGENT_INPUT_FILE", Value: "/data/context.tar.gz"},
			{Name: "DEPLOYAGENT_RUN_AS_USER", Value: "1000"},
			{Name: "DEPLOYAGENT_DOCKERFILE_BUILD", Value: "true"},
		})
		return false, nil, nil
	})
	inputStream := strings.NewReader("FROM tsuru/myplatform")
	client := KubeClient{}
	out := &safe.Buffer{}
	err := client.BuildImage(context.TODO(), "myplatform", []string{"tsuru/myplatform:latest"}, ioutil.NopCloser(inputStream), out)
	c.Assert(err, check.IsNil)
}

func (s *S) TestBuildImageNoDefaultPool(c *check.C) {
	s.mockService.Cluster.OnFindByPool = func(provName, poolName string) (*provTypes.Cluster, error) {
		return nil, provTypes.ErrNoCluster
	}
	_, rollback := s.mock.NoAppReactions(c)
	defer rollback()
	s.client.Fake.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		containers := pod.Spec.Containers
		c.Assert(containers, check.HasLen, 1)
		cmds := cleanCmds(containers[0].Command[2])
		c.Assert(cmds, check.Equals, `mkdir -p $(dirname /data/context.tar.gz) && cat >/data/context.tar.gz && tsuru_unit_agent`)
		c.Assert(containers[0].Env, check.DeepEquals, []apiv1.EnvVar{
			{Name: "DEPLOYAGENT_RUN_AS_SIDECAR", Value: "true"},
			{Name: "DEPLOYAGENT_DESTINATION_IMAGES", Value: "tsuru/myplatform:latest"},
			{Name: "DEPLOYAGENT_SOURCE_IMAGE", Value: ""},
			{Name: "DEPLOYAGENT_REGISTRY_AUTH_USER", Value: ""},
			{Name: "DEPLOYAGENT_REGISTRY_AUTH_PASS", Value: ""},
			{Name: "DEPLOYAGENT_REGISTRY_ADDRESS", Value: ""},
			{Name: "DEPLOYAGENT_INPUT_FILE", Value: "/data/context.tar.gz"},
			{Name: "DEPLOYAGENT_RUN_AS_USER", Value: "1000"},
			{Name: "DEPLOYAGENT_DOCKERFILE_BUILD", Value: "true"},
		})
		return false, nil, nil
	})
	inputStream := strings.NewReader("FROM tsuru/myplatform")
	client := KubeClient{}
	out := &safe.Buffer{}
	err := client.BuildImage(context.Background(), "myplatform", []string{"tsuru/myplatform:latest"}, ioutil.NopCloser(inputStream), out)
	c.Assert(err, check.IsNil)
}

func (s *S) TestDownloadFromContainer(c *check.C) {
	expectedFile := []byte("file content")
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		w.Write(expectedFile)
	}
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	client := KubeClient{}
	evt := s.newTestEvent(c, a)
	archiveReader, err := client.DownloadFromContainer(context.TODO(), a, evt, "tsuru/app-myapp:tag1")
	c.Assert(err, check.IsNil)
	c.Assert(archiveReader, check.NotNil)
	archive, err := ioutil.ReadAll(archiveReader)
	c.Assert(err, check.IsNil)
	c.Assert(archive, check.DeepEquals, expectedFile)
}

func (s *S) newTestEvent(c *check.C, a provision.App) *event.Event {
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	return evt
}
