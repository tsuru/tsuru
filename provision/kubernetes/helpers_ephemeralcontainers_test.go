// Copyright 2024 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"

	"github.com/stretchr/testify/require"
	provTypes "github.com/tsuru/tsuru/types/provision"
	check "gopkg.in/check.v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktesting "k8s.io/client-go/testing"
)

func (s *S) TestShouldReturnContainerNameOfDebug(_ *check.C) {
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
			require.True(s.t, ok)
			pod, ok := updateAction.GetObject().(*apiv1.Pod)
			require.True(s.t, ok)
			require.Greater(s.t, len(pod.Spec.EphemeralContainers), 0)
			indexEphemeralContainer := len(pod.Spec.EphemeralContainers) - 1
			require.Equal(s.t, p.expected.debugContainerName, pod.Spec.EphemeralContainers[indexEphemeralContainer].Name)
			require.Equal(s.t, p.expected.debugContainerImage, pod.Spec.EphemeralContainers[indexEphemeralContainer].Image)
			createEphemeralContainer = true
			return true, pod, nil
		})
		c1, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: p.input.customData})
		require.NoError(s.t, err)
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

		require.NoError(s.t, err)
		require.Equal(s.t, p.expected.debugContainerName, containerName)
		require.Equal(s.t, p.expected.shouldCreateEphemeralContainer, createEphemeralContainer)
	}
}

func (s *S) TestShouldReturnContainerNameNotDebug(_ *check.C) {
	pod := apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod1"},
		Spec: apiv1.PodSpec{
			Containers: []apiv1.Container{
				{Name: "container1"},
			},
		},
	}
	c1, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}})
	require.NoError(s.t, err)
	containerName, err := getContainerName(context.Background(), s.clusterClient, &pod, execOpts{
		debug:  false,
		client: c1,
	})
	require.NoError(s.t, err)
	require.Equal(s.t, "container1", containerName)
}
