// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"encoding/json"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/set"
	"github.com/tsuru/tsuru/volume"
	"github.com/ugorji/go/codec"
	apiv1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type volumeOptions struct {
	Plugin       string
	StorageClass string `json:"storage-class"`
	Capacity     resource.Quantity
	AccessModes  string `json:"access-modes"`
}

var allowedNonPersistentVolumes = set.FromValues("emptyDir")

func (opts *volumeOptions) isPersistent() bool {
	return !allowedNonPersistentVolumes.Includes(opts.Plugin)
}

func createVolumesForApp(client *ClusterClient, app provision.App) ([]apiv1.Volume, []apiv1.VolumeMount, error) {
	volumes, err := volume.ListByApp(app.GetName())
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	var kubeVolumes []apiv1.Volume
	var kubeMounts []apiv1.VolumeMount
	for i := range volumes {
		opts, err := validateVolume(&volumes[i])
		if err != nil {
			return nil, nil, err
		}
		if opts.isPersistent() {
			err = createVolume(client, &volumes[i], opts, app)
			if err != nil {
				return nil, nil, err
			}
		}
		volume, mounts, err := bindsForVolume(&volumes[i], opts, app.GetName())
		if err != nil {
			return nil, nil, err
		}
		kubeMounts = append(kubeMounts, mounts...)
		kubeVolumes = append(kubeVolumes, *volume)
	}
	return kubeVolumes, kubeMounts, nil
}

func bindsForVolume(v *volume.Volume, opts *volumeOptions, appName string) (*apiv1.Volume, []apiv1.VolumeMount, error) {
	var kubeMounts []apiv1.VolumeMount
	binds, err := v.LoadBindsForApp(appName)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	allReadOnly := true
	for _, b := range binds {
		kubeMounts = append(kubeMounts, apiv1.VolumeMount{
			Name:      volumeName(v.Name),
			MountPath: b.ID.MountPoint,
			ReadOnly:  b.ReadOnly,
		})
		if !b.ReadOnly {
			allReadOnly = false
		}
	}
	kubeVol := apiv1.Volume{
		Name: volumeName(v.Name),
	}
	if opts.isPersistent() {
		kubeVol.VolumeSource = apiv1.VolumeSource{
			PersistentVolumeClaim: &apiv1.PersistentVolumeClaimVolumeSource{
				ClaimName: volumeClaimName(v.Name),
				ReadOnly:  allReadOnly,
			},
		}
	} else {
		kubeVol.VolumeSource, err = nonPersistentVolume(v, opts)
		if err != nil {
			return nil, nil, err
		}
	}
	return &kubeVol, kubeMounts, nil
}

func nonPersistentVolume(v *volume.Volume, opts *volumeOptions) (apiv1.VolumeSource, error) {
	var volumeSrc apiv1.VolumeSource
	data, err := json.Marshal(map[string]interface{}{
		opts.Plugin: v.Opts,
	})
	if err != nil {
		return volumeSrc, errors.WithStack(err)
	}
	h := &codec.JsonHandle{}
	dec := codec.NewDecoderBytes(data, h)
	err = dec.Decode(&volumeSrc)
	if err != nil {
		return volumeSrc, errors.WithStack(err)
	}
	return volumeSrc, nil
}

func validateVolume(v *volume.Volume) (*volumeOptions, error) {
	var opts volumeOptions
	err := v.UnmarshalPlan(&opts)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if opts.Plugin != "" && opts.StorageClass != "" {
		return nil, errors.New("both volume plan plugin and storage-class cannot be set")
	}
	if opts.Plugin == "" && opts.StorageClass == "" {
		return nil, errors.New("both volume plan plugin and storage-class are empty")
	}
	if !opts.isPersistent() {
		return &opts, nil
	}
	if capRaw, ok := v.Opts["capacity"]; ok {
		delete(v.Opts, "capacity")
		opts.Capacity, err = resource.ParseQuantity(capRaw)
		if err != nil {
			return nil, errors.Wrap(err, "unable to parse `capacity` opt")
		}
	}
	if opts.Capacity.IsZero() {
		return nil, errors.New("capacity is mandatory either in plan or as volume opts")
	}
	if accessModesRaw, ok := v.Opts["access-modes"]; ok {
		delete(v.Opts, "access-modes")
		opts.AccessModes = accessModesRaw
	}
	if opts.AccessModes == "" {
		return nil, errors.New("access-modes is mandatory either in plan or as volume opts")
	}
	return &opts, nil
}

func deleteVolume(client *ClusterClient, name string) error {
	v, err := volume.Load(name)
	if err != nil {
		if err == volume.ErrVolumeNotFound {
			return nil
		}
		return err
	}
	namespace, err := getNamespaceForVolume(client, v)
	if err != nil {
		return err
	}
	err = client.CoreV1().PersistentVolumes().Delete(volumeName(name), &metav1.DeleteOptions{
		PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
	})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.WithStack(err)
	}
	err = client.CoreV1().PersistentVolumeClaims(namespace).Delete(volumeClaimName(name), &metav1.DeleteOptions{
		PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
	})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.WithStack(err)
	}
	return nil
}

func createVolume(client *ClusterClient, v *volume.Volume, opts *volumeOptions, app provision.App) error {
	namespace, err := getNamespaceForVolume(client, v)
	if err != nil {
		return err
	}
	labelSet := provision.VolumeLabels(provision.VolumeLabelsOpts{
		Name:        v.Name,
		Provisioner: provisionerName,
		Prefix:      tsuruLabelPrefix,
		Pool:        v.Pool,
		Plan:        v.Plan.Name,
	})
	data, err := json.Marshal(map[string]interface{}{
		opts.Plugin: v.Opts,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	h := &codec.JsonHandle{}
	dec := codec.NewDecoderBytes(data, h)
	pvSpec := apiv1.PersistentVolumeSpec{}
	err = dec.Decode(&pvSpec)
	if err != nil {
		return errors.WithStack(err)
	}
	pvSpec.Capacity = apiv1.ResourceList{
		apiv1.ResourceStorage: opts.Capacity,
	}
	for _, am := range strings.Split(opts.AccessModes, ",") {
		pvSpec.AccessModes = append(pvSpec.AccessModes, apiv1.PersistentVolumeAccessMode(am))
	}
	var volName string
	var selector *metav1.LabelSelector
	if opts.Plugin != "" {
		pv := &apiv1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name:   volumeName(v.Name),
				Labels: labelSet.ToLabels(),
			},
			Spec: pvSpec,
		}
		_, err = client.CoreV1().PersistentVolumes().Create(pv)
		if err != nil && !k8sErrors.IsAlreadyExists(err) {
			return errors.WithStack(err)
		}
		selector = &metav1.LabelSelector{
			MatchLabels: labelSet.ToVolumeSelector(),
		}
		volName = volumeName(v.Name)
	}
	pvc := &apiv1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:   volumeClaimName(v.Name),
			Labels: labelSet.ToLabels(),
		},
		Spec: apiv1.PersistentVolumeClaimSpec{
			Resources: apiv1.ResourceRequirements{
				Requests: pvSpec.Capacity,
			},
			AccessModes:      pvSpec.AccessModes,
			Selector:         selector,
			VolumeName:       volName,
			StorageClassName: &opts.StorageClass,
		},
	}
	_, err = client.CoreV1().PersistentVolumeClaims(namespace).Create(pvc)
	if err != nil && !k8sErrors.IsAlreadyExists(err) {
		return errors.WithStack(err)
	}
	return nil
}

func volumeExists(client *ClusterClient, name string) (bool, error) {
	v, err := volume.Load(name)
	if err != nil {
		if err == volume.ErrVolumeNotFound {
			return false, nil
		}
		return false, err
	}
	namespace, err := getNamespaceForVolume(client, v)
	if err != nil {
		return false, err
	}
	_, pvErr := client.CoreV1().PersistentVolumes().Get(volumeName(name), metav1.GetOptions{})
	_, pvcErr := client.CoreV1().PersistentVolumeClaims(namespace).Get(volumeClaimName(name), metav1.GetOptions{})
	if k8sErrors.IsNotFound(pvErr) && k8sErrors.IsNotFound(pvcErr) {
		return false, nil
	}
	if pvErr != nil {
		return false, errors.WithStack(pvErr)
	}
	if pvcErr != nil {
		return false, errors.WithStack(pvcErr)
	}
	return true, nil
}

func getNamespaceForVolume(client *ClusterClient, v *volume.Volume) (string, error) {
	binds, err := v.LoadBinds()
	if err != nil {
		return "", err
	}
	if len(binds) == 0 {
		return client.PoolNamespace(v.Pool), nil
	}
	var namespace string
	for _, b := range binds {
		ns, err := client.appNamespaceByName(b.ID.App)
		if err != nil {
			return "", err
		}
		if namespace == "" {
			namespace = ns
			continue
		}
		if ns != namespace {
			return "", errors.New("multiple namespaces for volume")
		}
	}
	return namespace, nil
}
