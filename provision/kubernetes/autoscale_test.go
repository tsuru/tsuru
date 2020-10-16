// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"sort"
	"strconv"

	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (s *S) TestProvisionerSetAutoScale(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	c.Assert(err, check.IsNil)
	wait()

	err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
		MinUnits:   1,
		MaxUnits:   2,
		AverageCPU: "500m",
	})
	c.Assert(err, check.IsNil)

	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	hpa, err := s.client.AutoscalingV2beta2().HorizontalPodAutoscalers(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	cpu := resource.MustParse("500m")
	c.Assert(hpa, check.DeepEquals, &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-web",
			Namespace: "default",
			Labels: map[string]string{
				"tsuru.io/app-name":        "myapp",
				"tsuru.io/app-platform":    "python",
				"tsuru.io/app-pool":        "test-default",
				"tsuru.io/app-process":     "web",
				"tsuru.io/app-version":     "1",
				"tsuru.io/builder":         "",
				"tsuru.io/is-build":        "false",
				"tsuru.io/is-deploy":       "false",
				"tsuru.io/is-isolated-run": "false",
				"tsuru.io/is-service":      "true",
				"tsuru.io/is-stopped":      "false",
				"tsuru.io/is-tsuru":        "true",
				"tsuru.io/provisioner":     "kubernetes",
				"version":                  "v1",
			},
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MinReplicas: func(i int32) *int32 { return &i }(1),
			MaxReplicas: int32(2),
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "myapp-web",
			},
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: "cpu",
						Target: autoscalingv2.MetricTarget{
							Type:         autoscalingv2.AverageValueMetricType,
							AverageValue: &cpu,
						},
					},
				},
			},
		},
	})
}

func (s *S) TestProvisionerSetAutoScaleMultipleVersions(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	versions := make([]appTypes.AppVersion, 4)
	for i := range versions {
		versions[i] = newSuccessfulVersion(c, a, map[string]interface{}{
			"processes": map[string]interface{}{
				"web": "python myapp.py",
			},
		})
	}

	err := s.p.AddUnits(context.TODO(), a, 1, "web", versions[0], nil)
	c.Assert(err, check.IsNil)
	wait()
	err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
		MinUnits:   1,
		MaxUnits:   2,
		AverageCPU: "500m",
	})
	c.Assert(err, check.IsNil)

	tests := []struct {
		scenario           func()
		expectedDeployment string
		expectedVersion    int
	}{
		{
			scenario:           func() {},
			expectedDeployment: "myapp-web",
			expectedVersion:    1,
		},
		{
			scenario: func() {
				err = s.p.AddUnits(context.TODO(), a, 1, "web", versions[1], nil)
				c.Assert(err, check.IsNil)
				wait()
			},
			expectedDeployment: "myapp-web",
			expectedVersion:    1,
		},
		{
			scenario: func() {
				err = s.p.AddUnits(context.TODO(), a, 1, "web", versions[2], nil)
				c.Assert(err, check.IsNil)
				wait()
				err = s.p.ToggleRoutable(context.TODO(), a, versions[2], true)
				c.Assert(err, check.IsNil)
			},
			expectedDeployment: "myapp-web",
			expectedVersion:    1,
		},
		{
			scenario: func() {
				err = s.p.AddUnits(context.TODO(), a, 1, "web", versions[3], nil)
				c.Assert(err, check.IsNil)
				wait()
			},
			expectedDeployment: "myapp-web",
			expectedVersion:    1,
		},
		{
			scenario: func() {
				err = s.p.Stop(context.TODO(), a, "web", versions[0])
				c.Assert(err, check.IsNil)
				wait()
			},
			expectedDeployment: "myapp-web-v3",
			expectedVersion:    3,
		},
		{
			scenario: func() {
				err = s.p.Stop(context.TODO(), a, "web", versions[2])
				c.Assert(err, check.IsNil)
				wait()
			},
			expectedDeployment: "myapp-web-v2",
			expectedVersion:    2,
		},
		{
			scenario: func() {
				err = s.p.Stop(context.TODO(), a, "web", versions[1])
				c.Assert(err, check.IsNil)
				wait()
			},
			expectedDeployment: "myapp-web-v4",
			expectedVersion:    4,
		},
	}

	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)

	for i, tt := range tests {
		c.Logf("test %d", i)
		tt.scenario()

		hpa, err := s.client.AutoscalingV2beta2().HorizontalPodAutoscalers(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
		c.Assert(err, check.IsNil)
		c.Assert(hpa.Spec.ScaleTargetRef.Name, check.Equals, tt.expectedDeployment)
		c.Assert(hpa.Labels["tsuru.io/app-version"], check.Equals, strconv.Itoa(tt.expectedVersion))
	}
}

func (s *S) TestProvisionerRemoveAutoScale(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	c.Assert(err, check.IsNil)
	wait()

	err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
		MinUnits:   1,
		MaxUnits:   2,
		AverageCPU: "500m",
	})
	c.Assert(err, check.IsNil)

	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	_, err = s.client.AutoscalingV2beta2().HorizontalPodAutoscalers(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	c.Assert(err, check.IsNil)

	err = s.p.RemoveAutoScale(context.TODO(), a, "web")
	c.Assert(err, check.IsNil)
	_, err = s.client.AutoscalingV2beta2().HorizontalPodAutoscalers(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	c.Assert(k8sErrors.IsNotFound(err), check.Equals, true)
}

func (s *S) TestProvisionerGetAutoScale(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python myapp.py",
			"worker": "python worker.py",
		},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	c.Assert(err, check.IsNil)
	wait()
	err = s.p.AddUnits(context.TODO(), a, 1, "worker", version, nil)
	c.Assert(err, check.IsNil)
	wait()

	err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
		MinUnits:   1,
		MaxUnits:   2,
		AverageCPU: "500m",
		Process:    "web",
	})
	c.Assert(err, check.IsNil)

	err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
		MinUnits:   2,
		MaxUnits:   4,
		AverageCPU: "200m",
		Process:    "worker",
	})
	c.Assert(err, check.IsNil)

	scales, err := s.p.GetAutoScale(context.TODO(), a)
	c.Assert(err, check.IsNil)
	sort.Slice(scales, func(i, j int) bool {
		return scales[i].Process < scales[j].Process
	})
	c.Assert(scales, check.DeepEquals, []provision.AutoScaleSpec{
		{
			MinUnits:   1,
			MaxUnits:   2,
			AverageCPU: "500m",
			Version:    1,
			Process:    "web",
		},
		{
			MinUnits:   2,
			MaxUnits:   4,
			AverageCPU: "200m",
			Version:    1,
			Process:    "worker",
		},
	})
}
