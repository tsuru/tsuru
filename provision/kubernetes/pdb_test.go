// Copyright 2021 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"

	"github.com/tsuru/tsuru/app"
	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (s *S) TestNewPDB(c *check.C) {
	tests := map[string]struct {
		app      *appTypes.App
		setup    func() (teardown func())
		expected *policyv1.PodDisruptionBudget
	}{
		"with default values": {
			app: &appTypes.App{Name: "myapp-01", TeamOwner: s.team.Name},
			expected: &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myapp-01-p1",
					Namespace: "default",
					Labels: map[string]string{
						"tsuru.io/is-tsuru":    "true",
						"tsuru.io/app-name":    "myapp-01",
						"tsuru.io/app-process": "p1",
						"tsuru.io/app-team":    "admin",
					},
				},
				Spec: policyv1.PodDisruptionBudgetSpec{
					MaxUnavailable: intOrStringPtr(intstr.FromString("10%")),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"tsuru.io/app-name":    "myapp-01",
							"tsuru.io/app-process": "p1",
							"tsuru.io/is-routable": "true",
						},
					},
				},
			},
		},
		"with custom maxUnavailable": {
			app: &app.App{
				Name:      "myapp-02",
				TeamOwner: s.team.Name,
				Metadata: appTypes.Metadata{
					Annotations: []appTypes.MetadataItem{
						{
							Name:  ResourceMetadataPrefix + "pdb-max-unavailable",
							Value: "30%",
						},
					},
				},
			},
			expected: &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myapp-02-p1",
					Namespace: "default",
					Labels: map[string]string{
						"tsuru.io/is-tsuru":    "true",
						"tsuru.io/app-name":    "myapp-02",
						"tsuru.io/app-process": "p1",
						"tsuru.io/app-team":    "admin",
					},
				},
				Spec: policyv1.PodDisruptionBudgetSpec{
					MaxUnavailable: intOrStringPtr(intstr.FromString("30%")),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"tsuru.io/app-name":    "myapp-02",
							"tsuru.io/app-process": "p1",
							"tsuru.io/is-routable": "true",
						},
					},
				},
			},
		},
		"when disable PDB for cluster/pool": {
			app: &app.App{Name: "myapp-03", TeamOwner: s.team.Name},
			setup: func() (teardown func()) {
				s.clusterClient.CustomData["test-default:disable-pdb"] = "true"
				return func() {
					delete(s.clusterClient.CustomData, "test-default:disable-pdb")
				}
			},
		},
	}
	for _, tt := range tests {
		var teardown func()
		if tt.setup != nil {
			teardown = tt.setup()
		}

		err := app.CreateApp(context.TODO(), tt.app, s.user)
		c.Assert(err, check.IsNil)

		pdb, err := newPDB(context.TODO(), s.clusterClient, tt.app, "p1")
		c.Assert(err, check.IsNil)
		c.Assert(pdb, check.DeepEquals, tt.expected)
		if teardown != nil {
			teardown()
		}
	}
}
