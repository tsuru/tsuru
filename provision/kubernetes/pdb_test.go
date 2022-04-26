// Copyright 2021 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"

	"github.com/tsuru/tsuru/app"
	check "gopkg.in/check.v1"
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
					MaxUnavailable: intOrStringPtr(intstr.FromString("10%")),
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
