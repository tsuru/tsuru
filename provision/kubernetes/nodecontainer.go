// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"github.com/tsuru/tsuru/servicemanager"
	provTypes "github.com/tsuru/tsuru/types/provision"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type nodeContainerManager struct {
	app provision.App
}

func (m *nodeContainerManager) DeployNodeContainer(config *nodecontainer.NodeContainerConfig, pool string, filter servicecommon.PoolFilter, placementOnly bool) error {
	if m.app != nil {
		client, err := clusterForPool(m.app.GetPool())
		if err != nil {
			return err
		}
		return m.deployNodeContainerForCluster(client, *config, pool, filter, placementOnly)
	}
	err := forEachCluster(func(cluster *ClusterClient) error {
		return m.deployNodeContainerForCluster(cluster, *config, pool, filter, placementOnly)
	})
	if err == provTypes.ErrNoCluster {
		return nil
	}
	return err
}

func (m *nodeContainerManager) deployNodeContainerForCluster(client *ClusterClient, config nodecontainer.NodeContainerConfig, pool string, filter servicecommon.PoolFilter, placementOnly bool) error {
	belongs, err := poolBelongsToCluster(client.Cluster, pool)
	if err != nil {
		return err
	}
	if !belongs {
		return nil
	}
	err = ensurePoolNamespace(client, pool)
	if err != nil {
		return err
	}
	dsName := daemonSetName(config.Name, pool)
	ns := client.PoolNamespace(pool)
	oldDs, err := client.AppsV1().DaemonSets(ns).Get(dsName, metav1.GetOptions{})
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
	affinity := &apiv1.Affinity{
		NodeAffinity: &apiv1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &apiv1.NodeSelector{
				NodeSelectorTerms: []apiv1.NodeSelectorTerm{{
					MatchExpressions: selectors,
				}},
			},
		},
	}
	singlePool, err := client.SinglePool()
	if err != nil {
		return err
	}
	if singlePool && pool != "" {
		affinity = &apiv1.Affinity{}
	}
	if oldDs != nil && placementOnly {
		if reflect.DeepEqual(oldDs.Spec.Template.Spec.Affinity, affinity) {
			return nil
		}
		oldDs.Spec.Template.Spec.Affinity = affinity
		_, err = client.AppsV1().DaemonSets(ns).Update(oldDs)
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
	serviceAccountName := serviceAccountNameForNodeContainer(config)
	accountLabels := provision.ServiceAccountLabels(provision.ServiceAccountLabelsOpts{
		NodeContainerName: config.Name,
		Provisioner:       provisionerName,
		Prefix:            tsuruLabelPrefix,
	})
	err = ensureServiceAccount(client, serviceAccountName, accountLabels, ns)
	if err != nil {
		return err
	}
	pullSecrets, err := getImagePullSecrets(client, config.Image())
	if err != nil {
		return err
	}
	serviceLinks := false
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dsName,
			Namespace: ns,
			Labels:    ls.ToLabels(),
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: ls.ToNodeContainerSelector(),
			},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{
					MaxUnavailable: &maxUnavailable,
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls.ToLabels(),
				},
				Spec: apiv1.PodSpec{
					EnableServiceLinks: &serviceLinks,
					HostPID:            config.HostConfig.PidMode == "host",
					ImagePullSecrets:   pullSecrets,
					ServiceAccountName: serviceAccountName,
					Affinity:           affinity,
					Volumes:            volumes,
					RestartPolicy:      restartPolicy,
					HostNetwork:        config.HostConfig.NetworkMode == "host",
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
					Tolerations: []apiv1.Toleration{
						{
							Key:      tsuruNodeDisabledTaint,
							Operator: apiv1.TolerationOpExists,
						},
					},
				},
			},
		},
	}
	if oldDs != nil {
		_, err = client.AppsV1().DaemonSets(ns).Update(ds)
	} else {
		_, err = client.AppsV1().DaemonSets(ns).Create(ds)
	}
	return errors.WithStack(err)
}

func ensureNodeContainers(a provision.App) error {
	m := nodeContainerManager{
		app: a,
	}
	buf := &bytes.Buffer{}
	err := servicecommon.EnsureNodeContainersCreated(&m, buf)
	if err != nil {
		return errors.Wrapf(err, "unable to ensure node containers running: %s", buf.String())
	}
	return nil
}

func poolBelongsToCluster(cluster *provTypes.Cluster, poolName string) (bool, error) {
	if poolName == "" {
		return true, nil
	}
	poolData, err := pool.GetPoolByName(poolName)
	if err != nil {
		if err == pool.ErrPoolNotFound {
			return false, nil
		}
		return false, err
	}
	poolCluster, err := servicemanager.Cluster.FindByPool(poolData.Provisioner, poolName)
	if err != nil {
		if err == provTypes.ErrNoCluster {
			return false, nil
		}
		return false, err
	}
	return poolCluster.Name == cluster.Name, nil
}
