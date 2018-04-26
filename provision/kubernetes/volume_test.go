// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/volume"
	"gopkg.in/check.v1"
	apiv1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
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
	}
	err := v.Save()
	c.Assert(err, check.IsNil)
	err = v.BindApp(a.GetName(), "/mnt", false)
	c.Assert(err, check.IsNil)
	err = v.BindApp(a.GetName(), "/mnt2", false)
	c.Assert(err, check.IsNil)
	err = v.BindApp("otherapp", "/mnt", false)
	c.Assert(err, check.IsNil)
	volumes, mounts, err := createVolumesForApp(s.clusterClient, a)
	c.Assert(err, check.IsNil)
	expectedVolume := []apiv1.Volume{{
		Name: volumeName(v.Name),
		VolumeSource: apiv1.VolumeSource{
			PersistentVolumeClaim: &apiv1.PersistentVolumeClaimVolumeSource{
				ClaimName: volumeClaimName(v.Name),
				ReadOnly:  false,
			},
		},
	}}
	expectedMount := []apiv1.VolumeMount{
		{
			Name:      volumeName(v.Name),
			MountPath: "/mnt",
			ReadOnly:  false,
		},
		{
			Name:      volumeName(v.Name),
			MountPath: "/mnt2",
			ReadOnly:  false,
		},
	}
	c.Assert(volumes, check.DeepEquals, expectedVolume)
	c.Assert(mounts, check.DeepEquals, expectedMount)
	pv, err := s.client.CoreV1().PersistentVolumes().Get(volumeName(v.Name), metav1.GetOptions{})
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
	pvc, err := s.client.CoreV1().PersistentVolumeClaims(s.client.Namespace(a.Pool)).Get(volumeClaimName(v.Name), metav1.GetOptions{})
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
			Namespace: s.client.Namespace(a.Pool),
		},
		Spec: apiv1.PersistentVolumeClaimSpec{
			AccessModes: []apiv1.PersistentVolumeAccessMode{apiv1.ReadWriteMany},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"tsuru.io/volume-name": "v1"},
			},
			VolumeName:       volumeName(v.Name),
			StorageClassName: nil,
			Resources: apiv1.ResourceRequirements{
				Requests: apiv1.ResourceList{
					apiv1.ResourceStorage: expectedCap,
				},
			},
		},
	})
	volumes, mounts, err = createVolumesForApp(s.clusterClient, a)
	c.Assert(err, check.IsNil)
	c.Assert(volumes, check.DeepEquals, expectedVolume)
	c.Assert(mounts, check.DeepEquals, expectedMount)
}

func (s *S) TestCreateVolumesForAppPluginUpdatePV(c *check.C) {
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
	}
	err := v.Save()
	c.Assert(err, check.IsNil)
	err = v.BindApp(a.GetName(), "/mnt", false)
	c.Assert(err, check.IsNil)
	err = v.BindApp(a.GetName(), "/mnt2", false)
	c.Assert(err, check.IsNil)
	err = v.BindApp("otherapp", "/mnt", false)
	c.Assert(err, check.IsNil)
	volumes, mounts, err := createVolumesForApp(s.clusterClient, a)
	c.Assert(err, check.IsNil)
	expectedVolume := []apiv1.Volume{{
		Name: volumeName(v.Name),
		VolumeSource: apiv1.VolumeSource{
			PersistentVolumeClaim: &apiv1.PersistentVolumeClaimVolumeSource{
				ClaimName: volumeClaimName(v.Name),
				ReadOnly:  false,
			},
		},
	}}
	expectedMount := []apiv1.VolumeMount{
		{
			Name:      volumeName(v.Name),
			MountPath: "/mnt",
			ReadOnly:  false,
		},
		{
			Name:      volumeName(v.Name),
			MountPath: "/mnt2",
			ReadOnly:  false,
		},
	}
	c.Assert(volumes, check.DeepEquals, expectedVolume)
	c.Assert(mounts, check.DeepEquals, expectedMount)
	pv, err := s.client.CoreV1().PersistentVolumes().Get(volumeName(v.Name), metav1.GetOptions{})
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
	pvc, err := s.client.CoreV1().PersistentVolumeClaims(s.client.Namespace(a.Pool)).Get(volumeClaimName(v.Name), metav1.GetOptions{})
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
			Namespace: s.client.Namespace(a.Pool),
		},
		Spec: apiv1.PersistentVolumeClaimSpec{
			AccessModes: []apiv1.PersistentVolumeAccessMode{apiv1.ReadWriteMany},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"tsuru.io/volume-name": "v1"},
			},
			VolumeName:       volumeName(v.Name),
			StorageClassName: nil,
			Resources: apiv1.ResourceRequirements{
				Requests: apiv1.ResourceList{
					apiv1.ResourceStorage: expectedCap,
				},
			},
		},
	})
	volumes, mounts, err = createVolumesForApp(s.clusterClient, a)
	c.Assert(err, check.IsNil)
	c.Assert(volumes, check.DeepEquals, expectedVolume)
	c.Assert(mounts, check.DeepEquals, expectedMount)

	err = pool.AddPool(pool.AddPoolOptions{
		Name:        "test-prod",
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	v = volume.Volume{
		Name: "v1",
		Opts: map[string]string{
			"path":         "/exports/changed",
			"server":       "192.168.1.1",
			"capacity":     "10Gi",
			"access-modes": string(apiv1.ReadOnlyMany),
		},
		Plan:      volume.VolumePlan{Name: "p1"},
		Pool:      "test-prod",
		TeamOwner: "admin",
	}
	err = v.Save()
	c.Assert(err, check.IsNil)
	err = v.UnbindApp(a.GetName(), "/mnt")
	c.Assert(err, check.IsNil)
	err = v.UnbindApp(a.GetName(), "/mnt2")
	c.Assert(err, check.IsNil)
	err = v.BindApp(a.GetName(), "/mnt/changed", false)
	c.Assert(err, check.IsNil)
	volumes, mounts, err = createVolumesForApp(s.clusterClient, a)
	c.Assert(err, check.IsNil)
	expectedVolume = []apiv1.Volume{{
		Name: volumeName(v.Name),
		VolumeSource: apiv1.VolumeSource{
			PersistentVolumeClaim: &apiv1.PersistentVolumeClaimVolumeSource{
				ClaimName: volumeClaimName(v.Name),
				ReadOnly:  false,
			},
		},
	}}
	expectedMount = []apiv1.VolumeMount{
		{
			Name:      volumeName(v.Name),
			MountPath: "/mnt/changed",
			ReadOnly:  false,
		},
	}
	c.Assert(volumes, check.DeepEquals, expectedVolume)
	c.Assert(mounts, check.DeepEquals, expectedMount)
	pv, err = s.client.CoreV1().PersistentVolumes().Get(volumeName(v.Name), metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	expectedCap, err = resource.ParseQuantity("10Gi")
	c.Assert(err, check.IsNil)
	c.Assert(pv, check.DeepEquals, &apiv1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: volumeName(v.Name),
			Labels: map[string]string{
				"tsuru.io/volume-name": "v1",
				"tsuru.io/volume-pool": "test-prod",
				"tsuru.io/volume-plan": "p1",
				"tsuru.io/is-tsuru":    "true",
				"tsuru.io/provisioner": "kubernetes",
			},
		},
		Spec: apiv1.PersistentVolumeSpec{
			PersistentVolumeSource: apiv1.PersistentVolumeSource{
				NFS: &apiv1.NFSVolumeSource{
					Path:   "/exports/changed",
					Server: "192.168.1.1",
				},
			},
			AccessModes: []apiv1.PersistentVolumeAccessMode{apiv1.ReadOnlyMany},
			Capacity: apiv1.ResourceList{
				apiv1.ResourceStorage: expectedCap,
			},
		},
	})
	pvc, err = s.client.CoreV1().PersistentVolumeClaims(s.client.Namespace(a.Pool)).Get(volumeClaimName(v.Name), metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pvc, check.DeepEquals, &apiv1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: volumeClaimName(v.Name),
			Labels: map[string]string{
				"tsuru.io/volume-name": "v1",
				"tsuru.io/volume-pool": "test-prod",
				"tsuru.io/volume-plan": "p1",
				"tsuru.io/is-tsuru":    "true",
				"tsuru.io/provisioner": "kubernetes",
			},
			Namespace: s.client.Namespace(a.Pool),
		},
		Spec: apiv1.PersistentVolumeClaimSpec{
			AccessModes: []apiv1.PersistentVolumeAccessMode{apiv1.ReadOnlyMany},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"tsuru.io/volume-name": "v1"},
			},
			VolumeName:       volumeName(v.Name),
			StorageClassName: nil,
			Resources: apiv1.ResourceRequirements{
				Requests: apiv1.ResourceList{
					apiv1.ResourceStorage: expectedCap,
				},
			},
		},
	})
}

func (s *S) TestCreateVolumesForAppPluginNonPersistent(c *check.C) {
	config.Set("volume-plans:p1:kubernetes:plugin", "emptyDir")
	defer config.Unset("volume-plans")
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	v := volume.Volume{
		Name: "v1",
		Opts: map[string]string{
			"medium": "Memory",
		},
		Plan:      volume.VolumePlan{Name: "p1"},
		Pool:      "test-default",
		TeamOwner: "admin",
	}
	err := v.Save()
	c.Assert(err, check.IsNil)
	err = v.BindApp(a.GetName(), "/mnt", false)
	c.Assert(err, check.IsNil)
	err = v.BindApp(a.GetName(), "/mnt2", false)
	c.Assert(err, check.IsNil)
	err = v.BindApp("otherapp", "/mnt", false)
	c.Assert(err, check.IsNil)
	volumes, mounts, err := createVolumesForApp(s.clusterClient, a)
	c.Assert(err, check.IsNil)
	expectedVolume := []apiv1.Volume{{
		Name: volumeName(v.Name),
		VolumeSource: apiv1.VolumeSource{
			EmptyDir: &apiv1.EmptyDirVolumeSource{
				Medium: apiv1.StorageMediumMemory,
			},
		},
	}}
	expectedMount := []apiv1.VolumeMount{
		{
			Name:      volumeName(v.Name),
			MountPath: "/mnt",
			ReadOnly:  false,
		},
		{
			Name:      volumeName(v.Name),
			MountPath: "/mnt2",
			ReadOnly:  false,
		},
	}
	c.Assert(volumes, check.DeepEquals, expectedVolume)
	c.Assert(mounts, check.DeepEquals, expectedMount)
	_, err = s.client.CoreV1().PersistentVolumes().Get(volumeName(v.Name), metav1.GetOptions{})
	c.Assert(k8sErrors.IsNotFound(err), check.Equals, true)
	_, err = s.client.CoreV1().PersistentVolumeClaims(s.client.Namespace(a.Pool)).Get(volumeClaimName(v.Name), metav1.GetOptions{})
	c.Assert(k8sErrors.IsNotFound(err), check.Equals, true)
	volumes, mounts, err = createVolumesForApp(s.clusterClient, a)
	c.Assert(err, check.IsNil)
	c.Assert(volumes, check.DeepEquals, expectedVolume)
	c.Assert(mounts, check.DeepEquals, expectedMount)
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
	}
	err := v.Save()
	c.Assert(err, check.IsNil)
	err = v.BindApp(a.GetName(), "/mnt", false)
	c.Assert(err, check.IsNil)
	volumes, mounts, err := createVolumesForApp(s.clusterClient, a)
	c.Assert(err, check.IsNil)
	expectedVolume := []apiv1.Volume{{
		Name: volumeName(v.Name),
		VolumeSource: apiv1.VolumeSource{
			PersistentVolumeClaim: &apiv1.PersistentVolumeClaimVolumeSource{
				ClaimName: volumeClaimName(v.Name),
				ReadOnly:  false,
			},
		},
	}}
	expectedMount := []apiv1.VolumeMount{{
		Name:      volumeName(v.Name),
		MountPath: "/mnt",
		ReadOnly:  false,
	}}
	c.Assert(volumes, check.DeepEquals, expectedVolume)
	c.Assert(mounts, check.DeepEquals, expectedMount)
	_, err = s.client.CoreV1().PersistentVolumes().Get(volumeName(v.Name), metav1.GetOptions{})
	c.Assert(err, check.ErrorMatches, "persistentvolumes \"v1-tsuru\" not found")
	expectedClass := "my-class"
	expectedCap, err := resource.ParseQuantity("20Gi")
	c.Assert(err, check.IsNil)
	pvc, err := s.client.CoreV1().PersistentVolumeClaims(s.client.Namespace(a.Pool)).Get(volumeClaimName(v.Name), metav1.GetOptions{})
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
			Namespace: s.client.Namespace(a.Pool),
		},
		Spec: apiv1.PersistentVolumeClaimSpec{
			AccessModes:      []apiv1.PersistentVolumeAccessMode{apiv1.ReadWriteMany},
			StorageClassName: &expectedClass,
			Resources: apiv1.ResourceRequirements{
				Requests: apiv1.ResourceList{
					apiv1.ResourceStorage: expectedCap,
				},
			},
		},
	})
	volumes, mounts, err = createVolumesForApp(s.clusterClient, a)
	c.Assert(err, check.IsNil)
	c.Assert(volumes, check.DeepEquals, expectedVolume)
	c.Assert(mounts, check.DeepEquals, expectedMount)
}

func (s *S) TestDeleteVolume(c *check.C) {
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
	}
	err := v.Save()
	c.Assert(err, check.IsNil)
	err = v.BindApp(a.GetName(), "/mnt", false)
	c.Assert(err, check.IsNil)
	_, _, err = createVolumesForApp(s.clusterClient, a)
	c.Assert(err, check.IsNil)
	err = deleteVolume(s.clusterClient, "v1")
	c.Assert(err, check.IsNil)
	_, err = s.client.CoreV1().PersistentVolumes().Get(volumeName(v.Name), metav1.GetOptions{})
	c.Assert(k8sErrors.IsNotFound(err), check.Equals, true)
	_, err = s.client.CoreV1().PersistentVolumeClaims(s.client.Namespace(a.Pool)).Get(volumeClaimName(v.Name), metav1.GetOptions{})
	c.Assert(k8sErrors.IsNotFound(err), check.Equals, true)
}
