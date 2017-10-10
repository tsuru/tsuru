// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/cluster"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/servicecommon"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

const alphaAffinityAnnotation = "scheduler.alpha.kubernetes.io/affinity"

type nodeContainerManager struct{}

func (m *nodeContainerManager) DeployNodeContainer(config *nodecontainer.NodeContainerConfig, pool string, filter servicecommon.PoolFilter, placementOnly bool) error {
	err := forEachCluster(func(cluster *clusterClient) error {
		return m.deployNodeContainerForCluster(cluster, config, pool, filter, placementOnly)
	})
	if err == cluster.ErrNoCluster {
		return nil
	}
	return err
}

func (m *nodeContainerManager) deployNodeContainerForCluster(client *clusterClient, config *nodecontainer.NodeContainerConfig, pool string, filter servicecommon.PoolFilter, placementOnly bool) error {
	dsName := daemonSetName(config.Name, pool)
	oldDs, err := client.Extensions().DaemonSets(client.Namespace()).Get(dsName, metav1.GetOptions{})
	if err != nil {
		if !k8sErrors.IsNotFound(err) {
			return errors.WithStack(err)
		}
		oldDs = nil
	}
	nodePoolLabelKey := tsuruLabelPrefix + provision.LabelNodePool
	nodeReq := apiv1.NodeSelectorRequirement{
		Key: nodePoolLabelKey,
	}
	if len(filter.Exclude) > 0 {
		nodeReq.Operator = apiv1.NodeSelectorOpNotIn
		nodeReq.Values = filter.Exclude
	} else {
		nodeReq.Operator = apiv1.NodeSelectorOpIn
		nodeReq.Values = filter.Include
	}
	selectors := []apiv1.NodeSelectorRequirement{
		{Key: nodePoolLabelKey, Operator: apiv1.NodeSelectorOpExists},
	}
	if len(nodeReq.Values) != 0 {
		selectors = append(selectors, nodeReq)
	}
	affinityAnnotation := map[string]string{}
	affinity := &apiv1.Affinity{
		NodeAffinity: &apiv1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &apiv1.NodeSelector{
				NodeSelectorTerms: []apiv1.NodeSelectorTerm{{
					MatchExpressions: selectors,
				}},
			},
		},
	}
	var affinityData []byte
	affinityData, err = json.Marshal(affinity)
	if err != nil {
		return errors.WithStack(err)
	}
	affinityAnnotation[alphaAffinityAnnotation] = string(affinityData)
	if oldDs != nil && placementOnly {
		if reflect.DeepEqual(oldDs.Spec.Template.ObjectMeta.Annotations, affinityAnnotation) &&
			reflect.DeepEqual(oldDs.Spec.Template.Spec.Affinity, affinity) {
			return nil
		}
		oldDs.Spec.Template.ObjectMeta.Annotations = affinityAnnotation
		oldDs.Spec.Template.Spec.Affinity = affinity
		_, err = client.Extensions().DaemonSets(client.Namespace()).Update(oldDs)
		return errors.WithStack(err)
	}
	ls := provision.NodeContainerLabels(provision.NodeContainerLabelsOpts{
		Name:         config.Name,
		CustomLabels: config.Config.Labels,
		Pool:         pool,
		Provisioner:  provisionerName,
		Prefix:       tsuruLabelPrefix,
	})
	envVars := make([]apiv1.EnvVar, len(config.Config.Env))
	for i, v := range config.Config.Env {
		parts := strings.SplitN(v, "=", 2)
		envVars[i].Name = parts[0]
		if len(parts) > 1 {
			envVars[i].Value = parts[1]
		}
	}
	var volumes []apiv1.Volume
	var volumeMounts []apiv1.VolumeMount
	if config.Name == nodecontainer.BsDefaultName {
		config.HostConfig.Binds = append(config.HostConfig.Binds,
			"/var/log:/var/log:rw",
			"/var/lib/docker/containers:/var/lib/docker/containers:ro",
			// This last one is for out of the box compatibility with minikube.
			"/mnt/sda1/var/lib/docker/containers:/mnt/sda1/var/lib/docker/containers:ro")
	}
	for i, b := range config.HostConfig.Binds {
		parts := strings.SplitN(b, ":", 3)
		vol := apiv1.Volume{
			Name: fmt.Sprintf("volume-%d", i),
			VolumeSource: apiv1.VolumeSource{
				HostPath: &apiv1.HostPathVolumeSource{
					Path: parts[0],
				},
			},
		}
		mount := apiv1.VolumeMount{
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
	var secCtx *apiv1.SecurityContext
	if config.HostConfig.Privileged {
		trueVar := true
		secCtx = &apiv1.SecurityContext{
			Privileged: &trueVar,
		}
	}
	restartPolicy := apiv1.RestartPolicyAlways
	switch config.HostConfig.RestartPolicy.Name {
	case docker.RestartOnFailure(0).Name:
		restartPolicy = apiv1.RestartPolicyOnFailure
	case docker.NeverRestart().Name:
		restartPolicy = apiv1.RestartPolicyNever
	}
	maxUnavailable := intstr.FromString("20%")
	ds := &v1beta1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dsName,
			Namespace: client.Namespace(),
		},
		Spec: v1beta1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: ls.ToNodeContainerSelector(),
			},
			UpdateStrategy: v1beta1.DaemonSetUpdateStrategy{
				Type: v1beta1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &v1beta1.RollingUpdateDaemonSet{
					MaxUnavailable: &maxUnavailable,
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      ls.ToLabels(),
					Annotations: affinityAnnotation,
				},
				Spec: apiv1.PodSpec{
					Affinity:      affinity,
					Volumes:       volumes,
					RestartPolicy: restartPolicy,
					HostNetwork:   config.HostConfig.NetworkMode == "host",
					Containers: []apiv1.Container{
						{
							Name:            config.Name,
							Image:           config.Image(),
							Command:         config.Config.Entrypoint,
							Args:            config.Config.Cmd,
							Env:             envVars,
							WorkingDir:      config.Config.WorkingDir,
							TTY:             config.Config.Tty,
							VolumeMounts:    volumeMounts,
							SecurityContext: secCtx,
						},
					},
				},
			},
		},
	}
	if oldDs != nil {
		_, err = client.Extensions().DaemonSets(client.Namespace()).Update(ds)
	} else {
		_, err = client.Extensions().DaemonSets(client.Namespace()).Create(ds)
	}
	return errors.WithStack(err)
}

func ensureNodeContainers() error {
	m := nodeContainerManager{}
	buf := &bytes.Buffer{}
	err := servicecommon.EnsureNodeContainersCreated(&m, buf)
	if err != nil {
		return errors.Wrapf(err, "unable to ensure node containers running: %s", buf.String())
	}
	return nil
}
