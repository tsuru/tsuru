// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"

	"github.com/stretchr/testify/require"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	eventTypes "github.com/tsuru/tsuru/types/event"
	provTypes "github.com/tsuru/tsuru/types/provision"
	check "gopkg.in/check.v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktesting "k8s.io/client-go/testing"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

func (s *S) Test_MetricsProvisioner_UnitsMetrics(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	wait()

	s.client.MetricsClientset.PrependReactor("list", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		listAction := action.(ktesting.ListAction)
		require.Equal(s.t, "tsuru.io/app-name=myapp", listAction.GetListRestrictions().Labels.String())
		return true, &metricsv1beta1.PodMetricsList{
			Items: []metricsv1beta1.PodMetrics{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      a.Name + "-123",
						Namespace: "default",
						Labels: map[string]string{
							"tsuru.io/app-name": a.Name,
						},
					},
					Containers: []metricsv1beta1.ContainerMetrics{
						{
							Name: a.Name + "-123-web",
							Usage: corev1.ResourceList{
								"cpu":    resource.MustParse("2100m"),
								"memory": resource.MustParse("100Mi"),
							},
						},
						{
							Name: a.Name + "-123-sidecar",
							Usage: corev1.ResourceList{
								"cpu":    resource.MustParse("100m"),
								"memory": resource.MustParse("10Mi"),
							},
						},
					},
				},
			},
		}, nil
	})
	require.NoError(s.t, err)

	metrics, err := s.p.UnitsMetrics(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, metrics, 1)
	require.EqualValues(s.t, []provTypes.UnitMetric{
		{
			ID:     "myapp-123",
			CPU:    "2200m",
			Memory: "110Mi",
		},
	}, metrics)
}
