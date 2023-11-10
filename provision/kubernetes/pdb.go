// Copyright 2021 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"reflect"

	"github.com/tsuru/tsuru/provision"
	policyv1 "k8s.io/api/policy/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func ensurePDB(ctx context.Context, client *ClusterClient, app provision.App, process string) error {
	pdb, err := newPDB(ctx, client, app, process)
	if err != nil {
		return err
	}
	if pdb == nil {
		return nil
	}
	existingPDB, err := client.PolicyV1().PodDisruptionBudgets(pdb.Namespace).Get(ctx, pdb.Name, metav1.GetOptions{})
	if k8sErrors.IsNotFound(err) {
		_, err = client.PolicyV1().PodDisruptionBudgets(pdb.Namespace).Create(ctx, pdb, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if reflect.DeepEqual(pdb.Spec, existingPDB.Spec) {
		return nil
	}
	pdb.ResourceVersion = existingPDB.ResourceVersion
	_, err = client.PolicyV1().PodDisruptionBudgets(existingPDB.Namespace).Update(ctx, pdb, metav1.UpdateOptions{})
	return err
}

func allPDBsForApp(ctx context.Context, client *ClusterClient, app provision.App) ([]policyv1.PodDisruptionBudget, error) {
	ns, err := client.AppNamespace(ctx, app)
	if err != nil {
		return nil, err
	}
	pdbList, err := client.PolicyV1().PodDisruptionBudgets(ns).
		List(ctx, metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(labels.Set(provision.PDBLabels(provision.PDBLabelsOpts{
				App:    app,
				Prefix: tsuruLabelPrefix,
			}).ToPDBSelector())).String(),
		})
	if err != nil {
		return nil, err
	}
	return pdbList.Items, nil
}

func removeAllPDBs(ctx context.Context, client *ClusterClient, app provision.App) error {
	ns, err := client.AppNamespace(ctx, app)
	if err != nil {
		return err
	}
	pdbs, err := allPDBsForApp(ctx, client, app)
	if err != nil {
		return err
	}
	for _, pdb := range pdbs {
		err = client.PolicyV1().PodDisruptionBudgets(ns).Delete(ctx, pdb.Name, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func newPDB(ctx context.Context, client *ClusterClient, app provision.App, process string) (*policyv1.PodDisruptionBudget, error) {
	if client.disablePDB(app.GetPool()) {
		return nil, nil
	}

	maxUnavailable := "10%"
	if value, ok := app.GetMetadata(process).Annotation("app.tsuru.io/k8s-pdb-max-unavailable"); ok {
		maxUnavailable = value
	}
	maxUnavailableByProcess := intstr.FromString(maxUnavailable)

	ns, err := client.AppNamespace(ctx, app)
	if err != nil {
		return nil, err
	}
	routableLabels := pdbLabels(app, process)
	routableLabels.SetIsRoutable()

	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pdbNameForApp(app, process),
			Namespace: ns,
			Labels:    pdbLabels(app, process).ToLabels(),
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &maxUnavailableByProcess,
			Selector:       &metav1.LabelSelector{MatchLabels: routableLabels.ToRoutableSelector()},
		},
	}, nil
}

func pdbLabels(app provision.App, process string) *provision.LabelSet {
	return provision.PDBLabels(provision.PDBLabelsOpts{
		App:         app,
		Process:     process,
		Provisioner: provisionerName,
		Prefix:      tsuruLabelPrefix,
	})
}

func intOrStringPtr(v intstr.IntOrString) *intstr.IntOrString { return &v }
