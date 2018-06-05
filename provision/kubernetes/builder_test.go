// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"io"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kfake "k8s.io/client-go/kubernetes/typed/core/v1/fake"
	ktesting "k8s.io/client-go/testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"gopkg.in/check.v1"
)

func (s *S) TestBuildPod(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := s.p.Provision(a)
	c.Assert(err, check.IsNil)
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	fakePods, ok := s.client.Core().Pods(ns).(*kfake.FakePods)
	c.Assert(ok, check.Equals, true)
	fakePods.Fake.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
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
	_, err = client.BuildPod(a, evt, ioutil.NopCloser(buf), "mytag")
	c.Assert(err, check.IsNil)
}

func (s *S) TestBuildPodWithPoolNamespaces(c *check.C) {
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := s.p.Provision(a)
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
	fakePods, ok := s.client.Core().Pods(nsName).(*kfake.FakePods)
	c.Assert(ok, check.Equals, true)
	fakePods.Fake.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
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
	_, err = client.BuildPod(a, evt, ioutil.NopCloser(buf), "mytag")
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
	img, procfileRaw, yamlData, err := client.ImageTagPushAndInspect(a, "tsuru/app-myapp:tag1", "tsuru/app-myapp:tag2")
	c.Assert(err, check.IsNil)
	c.Assert(img.ID, check.Equals, "1234")
	c.Assert(procfileRaw, check.Equals, "web: make run")
	c.Assert(yamlData.Healthcheck.Path, check.Equals, "/health")
	c.Assert(yamlData.Healthcheck.Scheme, check.Equals, "https")
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
	_, _, _, err = client.ImageTagPushAndInspect(a, "tsuru/app-myapp:tag1", "tsuru/app-myapp:tag2")
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
	nsName, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	fakePods, ok := s.client.Core().Pods(nsName).(*kfake.FakePods)
	c.Assert(ok, check.Equals, true)
	fakePods.Fake.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
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
				{Name: "DEPLOYAGENT_DESTINATION_IMAGES", Value: "registry.example.com/tsuru/app-myapp:tag2,registry.example.com/tsuru/app-myapp:latest"},
				{Name: "DEPLOYAGENT_SOURCE_IMAGE", Value: "registry.example.com/tsuru/app-myapp:tag1"},
				{Name: "DEPLOYAGENT_REGISTRY_AUTH_USER", Value: "user"},
				{Name: "DEPLOYAGENT_REGISTRY_AUTH_PASS", Value: "pwd"},
				{Name: "DEPLOYAGENT_REGISTRY_ADDRESS", Value: "registry.example.com"},
				{Name: "DEPLOYAGENT_INPUT_FILE", Value: ""},
				{Name: "DEPLOYAGENT_RUN_AS_USER", Value: "1000"},
			})
		}
		return false, nil, nil
	})

	client := KubeClient{}
	img, procfileRaw, yamlData, err := client.ImageTagPushAndInspect(a, "registry.example.com/tsuru/app-myapp:tag1", "registry.example.com/tsuru/app-myapp:tag2")
	c.Assert(err, check.IsNil)
	c.Assert(img.ID, check.Equals, "1234")
	c.Assert(procfileRaw, check.Equals, "web: make run")
	c.Assert(yamlData.Healthcheck.Path, check.Equals, "/health")
	c.Assert(yamlData.Healthcheck.Scheme, check.Equals, "https")
}

func (s *S) TestImageTagPushAndInspectWithRegistryAuthAndDifferentDomain(c *check.C) {
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
	nsName, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	fakePods, ok := s.client.Core().Pods(nsName).(*kfake.FakePods)
	c.Assert(ok, check.Equals, true)
	fakePods.Fake.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
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
				{Name: "DEPLOYAGENT_DESTINATION_IMAGES", Value: "otherregistry.example.com/tsuru/app-myapp:tag2,otherregistry.example.com/tsuru/app-myapp:latest"},
				{Name: "DEPLOYAGENT_SOURCE_IMAGE", Value: "otherregistry.example.com/tsuru/app-myapp:tag1"},
				{Name: "DEPLOYAGENT_REGISTRY_AUTH_USER", Value: ""},
				{Name: "DEPLOYAGENT_REGISTRY_AUTH_PASS", Value: ""},
				{Name: "DEPLOYAGENT_REGISTRY_ADDRESS", Value: ""},
				{Name: "DEPLOYAGENT_INPUT_FILE", Value: ""},
				{Name: "DEPLOYAGENT_RUN_AS_USER", Value: "1000"},
			})
		}
		return false, nil, nil
	})

	client := KubeClient{}
	img, procfileRaw, yamlData, err := client.ImageTagPushAndInspect(a, "otherregistry.example.com/tsuru/app-myapp:tag1", "otherregistry.example.com/tsuru/app-myapp:tag2")
	c.Assert(err, check.IsNil)
	c.Assert(img.ID, check.Equals, "1234")
	c.Assert(procfileRaw, check.Equals, "web: make run")
	c.Assert(yamlData.Healthcheck.Path, check.Equals, "/health")
	c.Assert(yamlData.Healthcheck.Scheme, check.Equals, "https")
}
