// Copyright 2021 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"fmt"
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
	pdb.ResourceVersion = existingPDB.ResourceVersion
	_, err = client.PolicyV1beta1().PodDisruptionBudgets(pdb.Namespace).Update(ctx, pdb, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func removePDB(ctx context.Context, client *ClusterClient, app provision.App, process string) error {
	ns, err := client.AppNamespace(ctx, app)
	if err != nil {
		return err
	}

	fmt.Printf("Removing PDB from process: %s\n", pdbNameForApp(app, process))

	err = client.PolicyV1beta1().
		PodDisruptionBudgets(ns).
		Delete(ctx, pdbNameForApp(app, process), metav1.DeleteOptions{})

	if err != nil && !k8sErrors.IsNotFound(err) {
		return err
	}

	return nil
}

func allPDBsForApp(ctx context.Context, client *ClusterClient, app provision.App) ([]policyv1beta1.PodDisruptionBudget, error) {
	ns, err := client.AppNamespace(ctx, app)
	if err != nil {
		return nil, err
	}

	ls := provision.PDBLabels(provision.PDBLabelsOpts{
		App:    app,
		Prefix: tsuruLabelPrefix,
	}).ToPDBSelector()

	pdbList, err := client.PolicyV1beta1().
		PodDisruptionBudgets(ns).
		List(ctx, metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(labels.Set(ls)).String(),
		})

	if err != nil {
		return nil, err
	}

	var pdbs []policyv1beta1.PodDisruptionBudget
	for _, pdb := range pdbList.Items {
		pdbs = append(pdbs, pdb)
	}

	return pdbs, nil
}

func removeAllPDBs(ctx context.Context, client *ClusterClient, app provision.App) error {
	ns, err := client.AppNamespace(ctx, app)
	if err != nil {
		return err
	}

	ls := provision.PDBLabels(provision.PDBLabelsOpts{
		App:    app,
		Prefix: tsuruLabelPrefix,
	}).ToPDBSelector()

	return client.PolicyV1beta1().
		PodDisruptionBudgets(ns).
		DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(labels.Set(ls)).String(),
		})
}

func newPDB(ctx context.Context, client *ClusterClient, app provision.App, process string) (*policyv1beta1.PodDisruptionBudget, error) {
	ns, err := client.AppNamespace(ctx, app)
	if err != nil {
		return nil, err
	}

	defaultMinAvailable := client.minAvailablePDB(app.GetPool())

	autoscaleSpecs, err := getAutoScale(ctx, client, app, process)
	if err != nil {
		return nil, err
	}

	var minAvailableByProcess *intstr.IntOrString
	if len(autoscaleSpecs) > 0 {
		minAvailableByProcess = intOrStringPtr(intstr.FromInt(int(autoscaleSpecs[0].MinUnits)))
	}

	routableLabels := pdbLabels(app, process)
	routableLabels.SetIsRoutable()

	return &policyv1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pdbNameForApp(app, process),
			Namespace: ns,
			Labels:    pdbLabels(app, process).ToLabels(),
		},
		Spec: policyv1beta1.PodDisruptionBudgetSpec{
			MinAvailable: intstr.ValueOrDefault(minAvailableByProcess, defaultMinAvailable),
			Selector:     &metav1.LabelSelector{MatchLabels: routableLabels.ToRoutableSelector()},
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
