// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/servicecommon"
	k8sErrors "k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/unversioned"
	"k8s.io/client-go/pkg/api/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

type nodeContainerManager struct{}

func (m *nodeContainerManager) DeployNodeContainer(config *nodecontainer.NodeContainerConfig, pool string, filter servicecommon.PoolFilter, placementOnly bool) error {
	return forEachCluster(func(cluster *Cluster) error {
		return m.deployNodeContainerForCluster(cluster, config, pool, filter, placementOnly)
	})
}

func (m *nodeContainerManager) deployNodeContainerForCluster(client *Cluster, config *nodecontainer.NodeContainerConfig, pool string, filter servicecommon.PoolFilter, placementOnly bool) error {
	dsName := daemonSetName(config.Name, pool)
	oldDs, err := client.Extensions().DaemonSets(client.namespace()).Get(dsName)
	if err != nil {
		if !k8sErrors.IsNotFound(err) {
			return errors.WithStack(err)
		}
		oldDs = nil
	}
	nodeReq := v1.NodeSelectorRequirement{
		Key: provision.LabelNodePool,
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
		_, err = client.Extensions().DaemonSets(client.namespace()).Update(oldDs)
		return errors.WithStack(err)
	}
	ls := provision.NodeContainerLabels(provision.NodeContainerLabelsOpts{
		Name:         config.Name,
		CustomLabels: config.Config.Labels,
		Pool:         pool,
		Provisioner:  provisionerName,
		Prefix:       tsuruLabelPrefix,
	})
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
	if config.Name == nodecontainer.BsDefaultName {
		config.HostConfig.Binds = append(config.HostConfig.Binds,
			"/var/log:/var/log:rw",
			"/var/lib/docker/containers:/var/lib/docker/containers:ro",
			// This last one is for out of the box compatibility with minikube.
			"/mnt/sda1/var/lib/docker/containers:/mnt/sda1/var/lib/docker/containers:ro")
	}
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
	var secCtx *v1.SecurityContext
	if config.HostConfig.Privileged {
		trueVar := true
		secCtx = &v1.SecurityContext{
			Privileged: &trueVar,
		}
	}
	restartPolicy := v1.RestartPolicyAlways
	switch config.HostConfig.RestartPolicy.Name {
	case docker.RestartOnFailure(0).Name:
		restartPolicy = v1.RestartPolicyOnFailure
	case docker.NeverRestart().Name:
		restartPolicy = v1.RestartPolicyNever
	}
	ds := &extensions.DaemonSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      dsName,
			Namespace: client.namespace(),
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
					RestartPolicy: restartPolicy,
					HostNetwork:   config.HostConfig.NetworkMode == "host",
					Containers: []v1.Container{
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
		// TODO(cezarsa): This is only needed because kubernetes <=1.5 does not
		// support rolling updating daemon sets. Once 1.6 is out we can drop
		// the cleanup call and configure DaemonSetUpdateStrategy accordingly.
		err = cleanupDaemonSet(client, config.Name, pool)
		if err != nil {
			return err
		}
		_, err = client.Extensions().DaemonSets(client.namespace()).Create(ds)
	} else {
		_, err = client.Extensions().DaemonSets(client.namespace()).Create(ds)
	}
	return errors.WithStack(err)
}
