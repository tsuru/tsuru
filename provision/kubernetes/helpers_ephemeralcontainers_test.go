// Copyright 2024 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"

	provTypes "github.com/tsuru/tsuru/types/provision"
	check "gopkg.in/check.v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktesting "k8s.io/client-go/testing"
)

func (s *S) TestShouldReturnContainerNameOfDebug(c *check.C) {
	type Input struct {
		debugContainerName  string
		debugContainerImage string
		customData          map[string]string
		ephemeralContainer  []apiv1.EphemeralContainer
	}
	type expected struct {
		shouldCreateEphemeralContainer bool
		debugContainerName             string
		debugContainerImage            string
	}

	params := []struct {
		input    Input
		expected expected
	}{
		{
			input: Input{
				debugContainerName:  "tsuru-debugger",
				debugContainerImage: "tsuru/netshoot",
				customData:          map[string]string{},
			},
			expected: expected{
				shouldCreateEphemeralContainer: true,
				debugContainerName:             "tsuru-debugger",
				debugContainerImage:            "tsuru/netshoot",
			},
		},
		{
			input: Input{
				debugContainerName:  "tsuru-debugger",
				debugContainerImage: "tsuru/netshoot",
				customData:          map[string]string{"debug-container-image": "tsuru/debugger"},
			},
			expected: expected{
				shouldCreateEphemeralContainer: true,
				debugContainerName:             "tsuru-debugger",
				debugContainerImage:            "tsuru/debugger",
			},
		},
		{
			input: Input{
				debugContainerName:  "tsuru-debugger",
				debugContainerImage: "tsuru/netshoot",
				customData:          map[string]string{"debug-container-image": "tsuru/debugger"},
				ephemeralContainer: []apiv1.EphemeralContainer{
					{
						EphemeralContainerCommon: apiv1.EphemeralContainerCommon{
							Name:  "tsuru-debugger",
							Image: "tsuru/netshoot",
						},
					},
				},
			},
			expected: expected{
				shouldCreateEphemeralContainer: false,
				debugContainerName:             "tsuru-debugger",
			},
		},
		{
			input: Input{
				debugContainerName:  "tsuru-debugger",
				debugContainerImage: "tsuru/netshoot",
				ephemeralContainer: []apiv1.EphemeralContainer{
					{
						EphemeralContainerCommon: apiv1.EphemeralContainerCommon{
							Name:  "aaaa",
							Image: "aaaa",
						},
					},
				},
			},
			expected: expected{
				shouldCreateEphemeralContainer: true,
				debugContainerName:             "tsuru-debugger",
				debugContainerImage:            "tsuru/netshoot",
			},
		},
	}

	for _, p := range params {
		createEphemeralContainer := false
		s.client.PrependReactor("update", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
			updateAction, ok := action.(ktesting.UpdateAction)
			c.Assert(ok, check.Equals, true)
			pod, ok := updateAction.GetObject().(*apiv1.Pod)
			c.Assert(ok, check.Equals, true)
			c.Assert(len(pod.Spec.EphemeralContainers) > 0, check.Equals, true)
			indexEphemeralContainer := len(pod.Spec.EphemeralContainers) - 1
			c.Assert(pod.Spec.EphemeralContainers[indexEphemeralContainer].Name, check.Equals, p.expected.debugContainerName)
			c.Assert(pod.Spec.EphemeralContainers[indexEphemeralContainer].Image, check.Equals, p.expected.debugContainerImage)
			createEphemeralContainer = true
			return true, pod, nil
		})
		c1, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: p.input.customData})
		c.Assert(err, check.IsNil)
		pod := apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod1"},
		}
		if len(p.input.ephemeralContainer) != 0 {
			pod.Spec.EphemeralContainers = p.input.ephemeralContainer
		}
		containerName, err := getContainerName(context.Background(), s.clusterClient, &pod, execOpts{
			debug:  true,
			client: c1,
		})

		c.Assert(err, check.IsNil)
		c.Assert(containerName, check.Equals, p.expected.debugContainerName)
		c.Assert(createEphemeralContainer, check.Equals, p.expected.shouldCreateEphemeralContainer)
	}
}

func (s *S) TestShouldReturnContainerNameNotDebug(c *check.C) {
	pod := apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod1"},
		Spec: apiv1.PodSpec{
			Containers: []apiv1.Container{
				{Name: "container1"},
			},
		},
	}
	c1, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}})
	c.Assert(err, check.IsNil)
	containerName, err := getContainerName(context.Background(), s.clusterClient, &pod, execOpts{
		debug:  false,
		client: c1,
	})
	c.Assert(err, check.IsNil)
	c.Assert(containerName, check.Equals, "container1")
}
