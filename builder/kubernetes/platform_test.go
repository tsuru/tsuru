// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"

	"github.com/tsuru/tsuru/safe"
	appTypes "github.com/tsuru/tsuru/types/app"
	imgTypes "github.com/tsuru/tsuru/types/app/image"
	check "gopkg.in/check.v1"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktesting "k8s.io/client-go/testing"
)

func (s *S) TestPlatformBuild(c *check.C) {
	_, rollback := s.mock.NoAppReactions(c)
	defer rollback()
	s.mockService.PlatformImage.OnNewImage = func(reg imgTypes.ImageRegistry, plat string, version int) (string, error) {
		c.Assert(reg, check.Equals, imgTypes.ImageRegistry(""))
		c.Assert(plat, check.Equals, "myplatform")
		c.Assert(version, check.Equals, 1)
		return "tsuru/myplatform:v1", nil
	}
	s.client.Fake.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		containers := pod.Spec.Containers
		c.Assert(containers, check.HasLen, 1)
		c.Assert(containers[0].Env, check.DeepEquals, []apiv1.EnvVar{
			{Name: "DEPLOYAGENT_RUN_AS_SIDECAR", Value: "true"},
			{Name: "DEPLOYAGENT_DESTINATION_IMAGES", Value: "tsuru/myplatform:v1,tsuru/myplatform:latest"},
			{Name: "DEPLOYAGENT_SOURCE_IMAGE", Value: ""},
			{Name: "DEPLOYAGENT_REGISTRY_AUTH_USER", Value: ""},
			{Name: "DEPLOYAGENT_REGISTRY_AUTH_PASS", Value: ""},
			{Name: "DEPLOYAGENT_REGISTRY_ADDRESS", Value: ""},
			{Name: "DEPLOYAGENT_INPUT_FILE", Value: "/data/context.tar.gz"},
			{Name: "DEPLOYAGENT_RUN_AS_USER", Value: "1000"},
			{Name: "DEPLOYAGENT_DOCKERFILE_BUILD", Value: "true"},
			{Name: "DEPLOYAGENT_INSECURE_REGISTRY", Value: "false"},
			{Name: "BUILDKITD_FLAGS", Value: "--oci-worker-no-process-sandbox"},
			{Name: "BUILDCTL_CONNECT_RETRIES_MAX", Value: "50"},
		})
		return false, nil, nil
	})
	opts := appTypes.PlatformOptions{
		Name:      "myplatform",
		Version:   1,
		ExtraTags: []string{"latest"},
		Data:      []byte("dockerfile data"),
		Output:    &safe.Buffer{},
	}
	_, err := s.b.PlatformBuild(context.TODO(), opts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
}
