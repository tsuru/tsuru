/*
Copyright 2016 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
	http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/kubernetes"
)

// annotationsToSkip lists the annotations that should be preserved from the deployment and not
// copied from the replicaset when rolling a deployment back
var annotationsToSkip = map[string]bool{
	corev1.LastAppliedConfigAnnotation: true,
	appsv1.DeprecatedRollbackTo:        true,

	"deployment.kubernetes.io/revision":         true,
	"deployment.kubernetes.io/revision-history": true,
	"deployment.kubernetes.io/desired-replicas": true,
	"deployment.kubernetes.io/max-replicas":     true,
}

type DeploymentRollbacker struct {
	c kubernetes.Interface
}

func (r *DeploymentRollbacker) Rollback(ctx context.Context, w io.Writer, obj *appsv1.Deployment) error {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return fmt.Errorf("failed to create accessor for kind %v: %s", obj.GetObjectKind(), err.Error())
	}
	name := accessor.GetName()
	namespace := accessor.GetNamespace()

	// TODO: Fix this after kubectl has been removed from core. It is not possible to convert the runtime.Object
	// to the external appsv1 Deployment without round-tripping through an internal version of Deployment. We're
	// currently getting rid of all internal versions of resources. So we specifically request the appsv1 version
	// here. This follows the same pattern as for DaemonSet and StatefulSet.
	deployment, err := r.c.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to retrieve Deployment %s: %v", name, err)
	}

	rsForRevision, err := r.deploymentRevision(ctx, deployment)
	if err != nil {
		return err
	}

	if deployment.Spec.Paused {
		return fmt.Errorf("you cannot rollback a paused deployment; resume it first with 'kubectl rollout resume deployment/%s' and try again", name)
	}

	// Skip if the revision already matches current Deployment
	if equalIgnoreHash(&rsForRevision.Spec.Template, &deployment.Spec.Template) {
		fmt.Fprintf(w, "skipped rollback (current template already matches revision), deployment:%s, replicaset: %s\n", deployment.ObjectMeta.Name, rsForRevision.ObjectMeta.Name)
		return nil
	}

	// remove hash label before patching back into the deployment
	delete(rsForRevision.Spec.Template.Labels, appsv1.DefaultDeploymentUniqueLabelKey)

	// compute deployment annotations
	annotations := map[string]string{}
	for k := range annotationsToSkip {
		if v, ok := deployment.Annotations[k]; ok {
			annotations[k] = v
		}
	}
	for k, v := range rsForRevision.Annotations {
		if !annotationsToSkip[k] {
			annotations[k] = v
		}
	}

	fmt.Fprintf(w, "Deployment: %s, restoring podTemplate from replicaSet: %s\n", deployment.ObjectMeta.Name, rsForRevision.ObjectMeta.Name)

	// make patch to restore
	patchType, patch, err := getDeploymentPatch(&rsForRevision.Spec.Template, annotations)
	if err != nil {
		return fmt.Errorf("failed restoring revision %d: %v", 0, err)
	}

	// Restore revision
	if _, err = r.c.AppsV1().Deployments(namespace).Patch(ctx, name, patchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("failed restoring revision %d: %v", 0, err)
	}
	return nil
}

func (r *DeploymentRollbacker) deploymentRevision(ctx context.Context, deployment *appsv1.Deployment) (*appsv1.ReplicaSet, error) {

	allRSs, err := getAllReplicasets(ctx, r.c, deployment)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve replica sets from deployment %s: %v", deployment.Name, err)
	}

	var (
		latestReplicaSet   *appsv1.ReplicaSet
		latestRevision     = int64(-1)
		previousReplicaSet *appsv1.ReplicaSet
		previousRevision   = int64(-1)
	)
	for i, rs := range allRSs {
		if v, err := revision(&rs); err == nil {
			if latestRevision < v {
				// newest one we've seen so far
				previousRevision = latestRevision
				previousReplicaSet = latestReplicaSet
				latestRevision = v
				latestReplicaSet = &allRSs[i]
			} else if previousRevision < v {
				// second newest one we've seen so far
				previousRevision = v
				previousReplicaSet = &allRSs[i]
			}
		}
	}

	if previousReplicaSet == nil {
		return nil, fmt.Errorf("no rollout history found for deployment %q", deployment.Name)
	}
	return previousReplicaSet, nil
}

// equalIgnoreHash returns true if two given podTemplateSpec are equal, ignoring the diff in value of Labels[pod-template-hash]
// We ignore pod-template-hash because:
// 1. The hash result would be different upon podTemplateSpec API changes
//    (e.g. the addition of a new field will cause the hash code to change)
// 2. The deployment template won't have hash labels
func equalIgnoreHash(template1, template2 *corev1.PodTemplateSpec) bool {
	t1Copy := template1.DeepCopy()
	t2Copy := template2.DeepCopy()
	// Remove hash labels from template.Labels before comparing
	delete(t1Copy.Labels, appsv1.DefaultDeploymentUniqueLabelKey)
	delete(t2Copy.Labels, appsv1.DefaultDeploymentUniqueLabelKey)
	return apiequality.Semantic.DeepEqual(t1Copy, t2Copy)
}

// getPatch returns a patch that can be applied to restore a Deployment to a
// previous version. If the returned error is nil the patch is valid.
func getDeploymentPatch(podTemplate *corev1.PodTemplateSpec, annotations map[string]string) (types.PatchType, []byte, error) {
	// Create a patch of the Deployment that replaces spec.template
	patch, err := json.Marshal([]interface{}{
		map[string]interface{}{
			"op":    "replace",
			"path":  "/spec/template",
			"value": podTemplate,
		},
		map[string]interface{}{
			"op":    "replace",
			"path":  "/metadata/annotations",
			"value": annotations,
		},
	})
	return types.JSONPatchType, patch, err
}

func revision(rs *appsv1.ReplicaSet) (int64, error) {
	acc, err := meta.Accessor(rs)
	if err != nil {
		return 0, err
	}
	v, ok := acc.GetAnnotations()[replicaDepRevision]
	if !ok {
		return 0, nil
	}
	return strconv.ParseInt(v, 10, 64)
}
