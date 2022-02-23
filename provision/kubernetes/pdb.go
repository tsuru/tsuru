// Copyright 2021 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"math"
	"reflect"

	"github.com/tsuru/tsuru/provision"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
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
	existingPDB, err := client.PolicyV1beta1().PodDisruptionBudgets(pdb.Namespace).Get(ctx, pdb.Name, metav1.GetOptions{})
	if k8sErrors.IsNotFound(err) {
		_, err = client.PolicyV1beta1().PodDisruptionBudgets(pdb.Namespace).Create(ctx, pdb, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if reflect.DeepEqual(pdb.Spec, existingPDB.Spec) {
		return nil
	}
	// NOTE: Kubernetes 1.14 or below does not allow updating PDB resources as so
	// we've to recreate the object to get around that.
	err = client.PolicyV1beta1().PodDisruptionBudgets(existingPDB.Namespace).Delete(ctx, existingPDB.Name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	_, err = client.PolicyV1beta1().PodDisruptionBudgets(pdb.Namespace).Create(ctx, pdb, metav1.CreateOptions{})
	return err
}

func allPDBsForApp(ctx context.Context, client *ClusterClient, app provision.App) ([]policyv1beta1.PodDisruptionBudget, error) {
	ns, err := client.AppNamespace(ctx, app)
	if err != nil {
		return nil, err
	}
	pdbList, err := client.PolicyV1beta1().PodDisruptionBudgets(ns).
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
		err = client.PolicyV1beta1().PodDisruptionBudgets(ns).Delete(ctx, pdb.Name, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func newPDB(ctx context.Context, client *ClusterClient, app provision.App, process string) (*policyv1beta1.PodDisruptionBudget, error) {
	if client.disablePDB(app.GetPool()) {
		return nil, nil
	}
	maxUnavailableByProcess := intstr.FromString("10%")
	autoscaleSpecs, err := getAutoScale(ctx, client, app, process)
	if err != nil {
		return nil, err
	}
	var minAvailableByProcess *intstr.IntOrString
	if len(autoscaleSpecs) > 0 {
		minAvailableByProcess = intOrStringPtr(intstr.FromInt(int(math.Floor(float64(autoscaleSpecs[0].MinUnits) * 0.9))))
	}
	ns, err := client.AppNamespace(ctx, app)
	if err != nil {
		return nil, err
	}
	routableLabels := pdbLabels(app, process)
	routableLabels.SetIsRoutable()
	if minAvailableByProcess != nil {
		return &policyv1beta1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pdbNameForApp(app, process),
				Namespace: ns,
				Labels:    pdbLabels(app, process).ToLabels(),
			},
			Spec: policyv1beta1.PodDisruptionBudgetSpec{
				MinAvailable: minAvailableByProcess,
				Selector:     &metav1.LabelSelector{MatchLabels: routableLabels.ToRoutableSelector()},
			},
		}, nil
	}
	return &policyv1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pdbNameForApp(app, process),
			Namespace: ns,
			Labels:    pdbLabels(app, process).ToLabels(),
		},
		Spec: policyv1beta1.PodDisruptionBudgetSpec{
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
