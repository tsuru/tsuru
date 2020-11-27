// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"

	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
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

	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil)
	wait()

	s.client.MetricsClientset.PrependReactor("list", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		listAction := action.(ktesting.ListAction)
		c.Assert(listAction.GetListRestrictions().Labels.String(), check.DeepEquals, "tsuru.io/app-name=myapp")
		return true, &metricsv1beta1.PodMetricsList{
			Items: []metricsv1beta1.PodMetrics{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      a.GetName() + "-123",
						Namespace: "default",
						Labels: map[string]string{
							"tsuru.io/app-name": a.GetName(),
						},
					},
					Containers: []metricsv1beta1.ContainerMetrics{
						{
							Name: a.GetName() + "-123-web",
							Usage: corev1.ResourceList{
								"cpu":    resource.MustParse("2100m"),
								"memory": resource.MustParse("100Mi"),
							},
						},
						{
							Name: a.GetName() + "-123-sidecar",
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
	c.Assert(err, check.IsNil)

	metrics, err := s.p.UnitsMetrics(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(metrics, check.HasLen, 1)
	c.Assert(metrics, check.DeepEquals, []provision.UnitMetric{
		{
			ID:     "myapp-123",
			CPU:    "2200m",
			Memory: "110Mi",
		},
	})

}
