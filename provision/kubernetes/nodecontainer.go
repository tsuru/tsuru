// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/provisioncommon"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"k8s.io/client-go/kubernetes"
	k8sErrors "k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/unversioned"
	"k8s.io/client-go/pkg/api/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

type nodeContainerManager struct {
	client kubernetes.Interface
}

func (m *nodeContainerManager) DeployNodeContainer(config *nodecontainer.NodeContainerConfig, pool string, filter servicecommon.PoolFilter, placementOnly bool) error {
	dsName := daemonSetName(config.Name, pool)
	oldDs, err := m.client.Extensions().DaemonSets(tsuruNamespace).Get(dsName)
	if err != nil {
		if !k8sErrors.IsNotFound(err) {
			return errors.WithStack(err)
		}
		oldDs = nil
	}
	nodeReq := v1.NodeSelectorRequirement{
		Key: labelNodePoolName,
	}
	if len(filter.Exclude) > 0 {
		nodeReq.Operator = v1.NodeSelectorOpNotIn
		nodeReq.Values = filter.Exclude
	} else {
		nodeReq.Operator = v1.NodeSelectorOpIn
		nodeReq.Values = filter.Include
	}
	affinityAnnotation := map[string]string{}
	if len(nodeReq.Values) != 0 {
		affinity := v1.Affinity{
			NodeAffinity: &v1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
					NodeSelectorTerms: []v1.NodeSelectorTerm{{
						MatchExpressions: []v1.NodeSelectorRequirement{nodeReq},
					}},
				},
			},
		}
		var affinityData []byte
		affinityData, err = json.Marshal(affinity)
		if err != nil {
			return errors.WithStack(err)
		}
		affinityAnnotation["scheduler.alpha.kubernetes.io/affinity"] = string(affinityData)
	}
	if oldDs != nil && placementOnly {
		oldDs.Spec.Template.ObjectMeta.Annotations = affinityAnnotation
		_, err = m.client.Extensions().DaemonSets(tsuruNamespace).Update(oldDs)
		return errors.WithStack(err)
	}
	ls := provisioncommon.NodeContainerLabels(config.Name, pool, provisionerName, config.Config.Labels)
	envVars := make([]v1.EnvVar, len(config.Config.Env))
	for i, v := range config.Config.Env {
		parts := strings.SplitN(v, "=", 2)
		envVars[i].Name = parts[0]
		if len(parts) > 1 {
			envVars[i].Value = parts[1]
		}
	}
	var volumes []v1.Volume
	var volumeMounts []v1.VolumeMount
	for i, b := range config.HostConfig.Binds {
		parts := strings.SplitN(b, ":", 3)
		vol := v1.Volume{
			Name: fmt.Sprintf("volume-%d", i),
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: parts[0],
				},
			},
		}
		mount := v1.VolumeMount{
			Name: vol.Name,
		}
		if len(parts) > 1 {
			mount.MountPath = parts[1]
		}
		if len(parts) > 2 {
			mount.ReadOnly = parts[2] == "ro"
		}
		volumes = append(volumes, vol)
		volumeMounts = append(volumeMounts, mount)
	}
	ds := &extensions.DaemonSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      dsName,
			Namespace: tsuruNamespace,
		},
		Spec: extensions.DaemonSetSpec{
			Selector: &unversioned.LabelSelector{
				MatchLabels: ls.ToNodeContainerSelector(),
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels:      ls.ToLabels(),
					Annotations: affinityAnnotation,
				},
				Spec: v1.PodSpec{
					Volumes:       volumes,
					RestartPolicy: v1.RestartPolicyAlways,
					Containers: []v1.Container{
						{
							Name:         config.Name,
							Image:        config.Image(),
							Command:      config.Config.Entrypoint,
							Args:         config.Config.Cmd,
							Env:          envVars,
							WorkingDir:   config.Config.WorkingDir,
							TTY:          config.Config.Tty,
							VolumeMounts: volumeMounts,
						},
					},
				},
			},
		},
	}
	if oldDs != nil {
		_, err = m.client.Extensions().DaemonSets(tsuruNamespace).Update(ds)
	} else {
		_, err = m.client.Extensions().DaemonSets(tsuruNamespace).Create(ds)
	}
	return errors.WithStack(err)
}
