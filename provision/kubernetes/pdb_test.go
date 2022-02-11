// Copyright 2021 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"

	"github.com/tsuru/tsuru/app"
	check "gopkg.in/check.v1"
	autoscalingv2beta2 "k8s.io/api/autoscaling/v2beta2"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (s *S) TestNewPDB(c *check.C) {
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	tests := map[string]struct {
		setup    func() (teardown func())
		expected *policyv1beta1.PodDisruptionBudget
	}{
		"with default values": {
			expected: &policyv1beta1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myapp-p1",
					Namespace: "default",
					Labels: map[string]string{
						"tsuru.io/is-tsuru":    "true",
						"tsuru.io/app-name":    "myapp",
						"tsuru.io/app-process": "p1",
						"tsuru.io/app-team":    "admin",
						"tsuru.io/provisioner": "kubernetes",
					},
				},
				Spec: policyv1beta1.PodDisruptionBudgetSpec{
					MinAvailable: intOrStringPtr(intstr.FromString("90%")),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"tsuru.io/app-name":    "myapp",
							"tsuru.io/app-process": "p1",
							"tsuru.io/is-routable": "true",
						},
					},
				},
			},
		},
		"when disable PDB for cluster/pool": {
			setup: func() (teardown func()) {
				s.clusterClient.CustomData["test-default:disable-pdb"] = "true"
				return func() {
					delete(s.clusterClient.CustomData, "test-default:disable-pdb")
				}
			},
			expected: nil,
		},
		"with min avaible from app's HPA": {
			setup: func() (teardown func()) {
				pdb, err := s.clusterClient.AutoscalingV2beta2().HorizontalPodAutoscalers("default").Create(context.TODO(), &autoscalingv2beta2.HorizontalPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "myapp-p1",
						Namespace: "default",
						Labels: map[string]string{
							"tsuru.io/is-tsuru":    "true",
							"tsuru.io/app-name":    "myapp",
							"tsuru.io/app-process": "p1",
							"tsuru.io/provisioner": "kubernetes",
						},
					},
					Spec: autoscalingv2beta2.HorizontalPodAutoscalerSpec{
						MinReplicas: int32Ptr(int32(10)),
						MaxReplicas: int32(100),
					},
				}, metav1.CreateOptions{})
				c.Assert(err, check.IsNil)
				return func() {
					err := s.clusterClient.AutoscalingV2beta2().HorizontalPodAutoscalers(pdb.Namespace).Delete(context.TODO(), pdb.Name, metav1.DeleteOptions{})
					c.Assert(err, check.IsNil)
				}
			},
			expected: &policyv1beta1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myapp-p1",
					Namespace: "default",
					Labels: map[string]string{
						"tsuru.io/is-tsuru":    "true",
						"tsuru.io/app-name":    "myapp",
						"tsuru.io/app-process": "p1",
						"tsuru.io/app-team":    "admin",
						"tsuru.io/provisioner": "kubernetes",
					},
				},
				Spec: policyv1beta1.PodDisruptionBudgetSpec{
					MinAvailable: intOrStringPtr(intstr.FromInt(9)),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"tsuru.io/app-name":    "myapp",
							"tsuru.io/app-process": "p1",
							"tsuru.io/is-routable": "true",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		var teardown func()
		if tt.setup != nil {
			teardown = tt.setup()
		}
		pdb, err := newPDB(context.TODO(), s.clusterClient, a, "p1")
		c.Assert(err, check.IsNil)
		c.Assert(pdb, check.DeepEquals, tt.expected)
		if teardown != nil {
			teardown()
		}
	}
}

func int32Ptr(n int32) *int32 { return &n }
