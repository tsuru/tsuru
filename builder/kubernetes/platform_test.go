// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"

	"github.com/tsuru/tsuru/safe"
	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktesting "k8s.io/client-go/testing"
)

func (s *S) TestPlatformBuild(c *check.C) {
	_, rollback := s.mock.NoAppReactions(c)
	defer rollback()
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
		})
		return false, nil, nil
	})
	opts := appTypes.PlatformOptions{
		Name:      "myplatform",
		ImageName: "tsuru/myplatform:v1",
		ExtraTags: []string{"latest"},
		Data:      []byte("dockerfile data"),
		Output:    &safe.Buffer{},
		Ctx:       context.Background(),
	}
	err := s.b.PlatformBuild(opts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
}
