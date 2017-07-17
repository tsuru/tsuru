// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/volume"
	"gopkg.in/check.v1"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (s *S) TestCreateVolumesForAppPlugin(c *check.C) {
	config.Set("volume-plans:p1:kubernetes:plugin", "nfs")
	defer config.Unset("volume-plans")
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	v := volume.Volume{
		Name: "v1",
		Opts: map[string]string{
			"path":         "/exports",
			"server":       "192.168.1.1",
			"capacity":     "20Gi",
			"access-modes": string(apiv1.ReadWriteMany),
		},
		Plan:      volume.VolumePlan{Name: "p1"},
		Pool:      "test-default",
		TeamOwner: "admin",
		Apps:      []string{a.GetName()},
	}
	err := v.Save()
	c.Assert(err, check.IsNil)
	err = createVolumesForApp(s.client.clusterClient, a)
	c.Assert(err, check.IsNil)
	pv, err := s.client.Core().PersistentVolumes().Get(volumeName(v.Name), metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	expectedCap, err := resource.ParseQuantity("20Gi")
	c.Assert(err, check.IsNil)
	c.Assert(pv, check.DeepEquals, &apiv1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: volumeName(v.Name),
			Labels: map[string]string{
				"tsuru.io/volume-name": "v1",
				"tsuru.io/volume-pool": "test-default",
				"tsuru.io/volume-plan": "p1",
				"tsuru.io/is-tsuru":    "true",
				"tsuru.io/provisioner": "kubernetes",
			},
		},
		Spec: apiv1.PersistentVolumeSpec{
			PersistentVolumeSource: apiv1.PersistentVolumeSource{
				NFS: &apiv1.NFSVolumeSource{
					Path:   "/exports",
					Server: "192.168.1.1",
				},
			},
			AccessModes: []apiv1.PersistentVolumeAccessMode{apiv1.ReadWriteMany},
			Capacity: apiv1.ResourceList{
				apiv1.ResourceStorage: expectedCap,
			},
		},
	})
	pvc, err := s.client.Core().PersistentVolumeClaims(s.client.Namespace()).Get(volumeClaimName(v.Name), metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pvc, check.DeepEquals, &apiv1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: volumeClaimName(v.Name),
			Labels: map[string]string{
				"tsuru.io/volume-name": "v1",
				"tsuru.io/volume-pool": "test-default",
				"tsuru.io/volume-plan": "p1",
				"tsuru.io/is-tsuru":    "true",
				"tsuru.io/provisioner": "kubernetes",
			},
			Namespace: s.client.Namespace(),
		},
		Spec: apiv1.PersistentVolumeClaimSpec{
			AccessModes: []apiv1.PersistentVolumeAccessMode{apiv1.ReadWriteMany},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"tsuru.io/volume-name": "v1"},
			},
			VolumeName:       volumeName(v.Name),
			StorageClassName: nil,
		},
	})
	err = createVolumesForApp(s.client.clusterClient, a)
	c.Assert(err, check.IsNil)
}

func (s *S) TestCreateVolumesForAppStorageClass(c *check.C) {
	config.Set("volume-plans:p1:kubernetes:storage-class", "my-class")
	config.Set("volume-plans:p1:kubernetes:capacity", "20Gi")
	config.Set("volume-plans:p1:kubernetes:access-modes", "ReadWriteMany")
	defer config.Unset("volume-plans")
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	v := volume.Volume{
		Name:      "v1",
		Plan:      volume.VolumePlan{Name: "p1"},
		Pool:      "test-default",
		TeamOwner: "admin",
		Apps:      []string{a.GetName()},
	}
	err := v.Save()
	c.Assert(err, check.IsNil)
	err = createVolumesForApp(s.client.clusterClient, a)
	c.Assert(err, check.IsNil)
	_, err = s.client.Core().PersistentVolumes().Get(volumeName(v.Name), metav1.GetOptions{})
	c.Assert(err, check.ErrorMatches, "persistentvolumes \"v1-tsuru\" not found")
	expectedClass := "my-class"
	pvc, err := s.client.Core().PersistentVolumeClaims(s.client.Namespace()).Get(volumeClaimName(v.Name), metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pvc, check.DeepEquals, &apiv1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: volumeClaimName(v.Name),
			Labels: map[string]string{
				"tsuru.io/volume-name": "v1",
				"tsuru.io/volume-pool": "test-default",
				"tsuru.io/volume-plan": "p1",
				"tsuru.io/is-tsuru":    "true",
				"tsuru.io/provisioner": "kubernetes",
			},
			Namespace: s.client.Namespace(),
		},
		Spec: apiv1.PersistentVolumeClaimSpec{
			AccessModes:      []apiv1.PersistentVolumeAccessMode{apiv1.ReadWriteMany},
			StorageClassName: &expectedClass,
		},
	})
	err = createVolumesForApp(s.client.clusterClient, a)
	c.Assert(err, check.IsNil)
}
