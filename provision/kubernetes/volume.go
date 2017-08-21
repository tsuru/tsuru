// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"encoding/json"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/volume"
	"github.com/ugorji/go/codec"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiv1 "k8s.io/client-go/pkg/api/v1"
)

type volumeOptions struct {
	Plugin       string
	StorageClass string `json:"storage-class"`
	Capacity     resource.Quantity
	AccessModes  string `json:"access-modes"`
}

func createVolumesForApp(client *clusterClient, app provision.App) ([]apiv1.Volume, []apiv1.VolumeMount, error) {
	volumes, err := volume.ListByApp(app.GetName())
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	var kubeVolumes []apiv1.Volume
	var kubeMounts []apiv1.VolumeMount
	for i := range volumes {
		err = createVolumeForApp(client, app, &volumes[i])
		if err != nil {
			return nil, nil, err
		}
		volumes, mounts, err := bindsForVolume(&volumes[i])
		if err != nil {
			return nil, nil, err
		}
		kubeMounts = append(kubeMounts, mounts...)
		kubeVolumes = append(kubeVolumes, volumes...)
	}
	return kubeVolumes, kubeMounts, nil
}

func bindsForVolume(v *volume.Volume) ([]apiv1.Volume, []apiv1.VolumeMount, error) {
	var kubeVolumes []apiv1.Volume
	var kubeMounts []apiv1.VolumeMount
	binds, err := v.LoadBinds()
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
			break
		}
	}
	kubeVol := apiv1.Volume{
		Name: volumeName(v.Name),
		VolumeSource: apiv1.VolumeSource{
			PersistentVolumeClaim: &apiv1.PersistentVolumeClaimVolumeSource{
				ClaimName: volumeClaimName(v.Name),
				ReadOnly:  allReadOnly,
			},
		},
	}
	kubeVolumes = append(kubeVolumes, kubeVol)
	return kubeVolumes, kubeMounts, nil
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

func deleteVolume(client *clusterClient, name string) error {
	err := client.Core().PersistentVolumes().Delete(volumeName(name), &metav1.DeleteOptions{
		PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
	})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.WithStack(err)
	}
	err = client.Core().PersistentVolumeClaims(client.Namespace()).Delete(volumeClaimName(name), &metav1.DeleteOptions{
		PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
	})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.WithStack(err)
	}
	return nil
}

func createVolumeForApp(client *clusterClient, app provision.App, v *volume.Volume) error {
	opts, err := validateVolume(v)
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
	var storageClass *string
	var volName string
	var selector *metav1.LabelSelector
	if opts.Plugin != "" {
		_, err = client.Core().PersistentVolumes().Create(&apiv1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name:   volumeName(v.Name),
				Labels: labelSet.ToLabels(),
			},
			Spec: pvSpec,
		})
		if err != nil && !k8sErrors.IsAlreadyExists(err) {
			return err
		}
		selector = &metav1.LabelSelector{
			MatchLabels: labelSet.ToVolumeSelector(),
		}
		volName = volumeName(v.Name)
	}
	if opts.StorageClass != "" {
		storageClass = &opts.StorageClass
	}
	_, err = client.Core().PersistentVolumeClaims(client.Namespace()).Create(&apiv1.PersistentVolumeClaim{
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
			StorageClassName: storageClass,
		},
	})
	if err != nil && !k8sErrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}
