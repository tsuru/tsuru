/*
Copyright The CBI Authors.

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

package controller

import (
	"context"
	"encoding/json"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	cbiv1alpha1 "github.com/containerbuilding/cbi/pkg/apis/cbi/v1alpha1"
	api "github.com/containerbuilding/cbi/pkg/plugin/api"
)

func jobName(buildJob *cbiv1alpha1.BuildJob) string {
	return buildJob.Name + "-job"
}

func objectMeta(buildJob *cbiv1alpha1.BuildJob) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      jobName(buildJob),
		Namespace: buildJob.Namespace,
		OwnerReferences: []metav1.OwnerReference{
			*metav1.NewControllerRef(buildJob, schema.GroupVersionKind{
				Group:   cbiv1alpha1.SchemeGroupVersion.Group,
				Version: cbiv1alpha1.SchemeGroupVersion.Version,
				Kind:    "BuildJob",
			}),
		},
	}
}

func newJob(ctx context.Context, pluginClient api.PluginClient, buildJob *cbiv1alpha1.BuildJob) (*batchv1.Job, error) {
	buildJobJSON, err := json.Marshal(buildJob)
	if err != nil {
		return nil, err
	}
	specReq := &api.SpecRequest{
		BuildJobJson: buildJobJSON,
	}
	specRes, err := pluginClient.Spec(ctx, specReq)
	var pts corev1.PodTemplateSpec
	if err := json.Unmarshal(specRes.PodTemplateSpecJson, &pts); err != nil {
		return nil, err
	}
	j := &batchv1.Job{
		ObjectMeta: objectMeta(buildJob),
		Spec: batchv1.JobSpec{
			Template: pts,
		},
	}
	return j, nil
}
