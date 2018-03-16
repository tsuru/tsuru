// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"sort"
	"strings"

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
	fakePods, ok := s.client.Core().Pods(s.client.Namespace()).(*kfake.FakePods)
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
			{Name: "DEPLOYAGENT_INPUT_FILE", Value: "/home/application/archive.tar.gz"},
			{Name: "DEPLOYAGENT_RUN_AS_USER", Value: "1000"},
			{Name: "DEPLOYAGENT_REGISTRY_AUTH_USER", Value: ""},
			{Name: "DEPLOYAGENT_REGISTRY_AUTH_PASS", Value: ""},
			{Name: "DEPLOYAGENT_REGISTRY_ADDRESS", Value: ""},
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

func (s *S) TestImageTagPushAndInspect(c *check.C) {
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		exp := regexp.MustCompile("/api/v1/namespaces/default/pods/(.*)/attach")
		parts := exp.FindStringSubmatch(r.URL.Path)
		c.Assert(parts, check.HasLen, 2)
		switch parts[1] {
		case "myapp-v1-deploy":
			w.Write([]byte(`[{"Id":"1234"}]`))
		case "myapp-v1-build-procfile-inspect":
			w.Write([]byte(`web: make run`))
		case "myapp-v1-build-yamldata":
			w.Write([]byte("healthcheck:\n  path: /health\n  scheme: https"))
		}
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

func (s *S) TestImageTagPushAndInspectWithRegistryAuth(c *check.C) {
	config.Set("docker:registry", "registry.example.com")
	defer config.Unset("docker:registry")
	config.Set("docker:registry-auth:username", "user")
	defer config.Unset("docker:registry-auth:username")
	config.Set("docker:registry-auth:password", "pwd")
	defer config.Unset("docker:registry-auth:password")

	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		exp := regexp.MustCompile("/api/v1/namespaces/default/pods/(.*)/attach")
		parts := exp.FindStringSubmatch(r.URL.Path)
		c.Assert(parts, check.HasLen, 2)
		switch parts[1] {
		case "myapp-v1-deploy":
			w.Write([]byte(`[{"Id":"1234"}]`))
		case "myapp-v1-build-procfile-inspect":
			w.Write([]byte(`web: make run`))
		case "myapp-v1-build-yamldata":
			w.Write([]byte("healthcheck:\n  path: /health\n  scheme: https"))
		}
	}
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	fakePods, ok := s.client.Core().Pods(s.client.Namespace()).(*kfake.FakePods)
	c.Assert(ok, check.Equals, true)
	fakePods.Fake.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		containers := pod.Spec.Containers
		if containers[0].Name == "myapp-v1-deploy" {
			c.Assert(containers, check.HasLen, 1)
			cmds := cleanCmds(containers[0].Command[2])
			c.Assert(cmds, check.Equals, `cat >/dev/null &&
docker login -u "user" -p "pwd" "registry.example.com"
docker pull registry.example.com/tsuru/app-myapp:tag1 >/dev/null
docker inspect registry.example.com/tsuru/app-myapp:tag1
docker tag registry.example.com/tsuru/app-myapp:tag1 registry.example.com/tsuru/app-myapp:tag2
docker login -u "user" -p "pwd" "registry.example.com"
docker push registry.example.com/tsuru/app-myapp:tag2`)
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
		exp := regexp.MustCompile("/api/v1/namespaces/default/pods/(.*)/attach")
		parts := exp.FindStringSubmatch(r.URL.Path)
		c.Assert(parts, check.HasLen, 2)
		switch parts[1] {
		case "myapp-v1-deploy":
			w.Write([]byte(`[{"Id":"1234"}]`))
		case "myapp-v1-build-procfile-inspect":
			w.Write([]byte(`web: make run`))
		case "myapp-v1-build-yamldata":
			w.Write([]byte("healthcheck:\n  path: /health\n  scheme: https"))
		}
	}
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	fakePods, ok := s.client.Core().Pods(s.client.Namespace()).(*kfake.FakePods)
	c.Assert(ok, check.Equals, true)
	fakePods.Fake.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		containers := pod.Spec.Containers
		if containers[0].Name == "myapp-v1-deploy" {
			pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
			containers := pod.Spec.Containers
			c.Assert(containers, check.HasLen, 1)
			cmds := cleanCmds(containers[0].Command[2])
			c.Assert(cmds, check.Equals, `cat >/dev/null &&
docker pull otherregistry.example.com/tsuru/app-myapp:tag1 >/dev/null
docker inspect otherregistry.example.com/tsuru/app-myapp:tag1
docker tag otherregistry.example.com/tsuru/app-myapp:tag1 otherregistry.example.com/tsuru/app-myapp:tag2
docker push otherregistry.example.com/tsuru/app-myapp:tag2`)
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
