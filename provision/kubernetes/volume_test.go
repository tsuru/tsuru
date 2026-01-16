// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tsuru/config"
	tsuruv1 "github.com/tsuru/tsuru/provision/kubernetes/pkg/apis/tsuru/v1"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/servicemanager"
	volumeTypes "github.com/tsuru/tsuru/types/volume"
	check "gopkg.in/check.v1"
	apiv1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (s *S) TestCreateVolumesForAppPlugin(_ *check.C) {
	config.Set("volume-plans:p1:kubernetes:plugin", "nfs")
	defer config.Unset("volume-plans")
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	err := s.p.Provision(context.TODO(), a)
	require.NoError(s.t, err)
	v := volumeTypes.Volume{
		Name: "v1",
		Opts: map[string]string{
			"path":         "/exports",
			"server":       "192.168.1.1",
			"capacity":     "20Gi",
			"access-modes": string(apiv1.ReadWriteMany),
		},
		Plan:      volumeTypes.VolumePlan{Name: "p1"},
		Pool:      "test-default",
		TeamOwner: "admin",
	}
	err = servicemanager.Volume.Create(context.TODO(), &v)
	require.NoError(s.t, err)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a.Name,
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	require.NoError(s.t, err)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a.Name,
		MountPoint: "/mnt2",
		ReadOnly:   false,
	})
	require.NoError(s.t, err)
	err = s.p.Provision(context.TODO(), provisiontest.NewFakeApp("otherapp", "python", 0))
	require.NoError(s.t, err)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    "otherapp",
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	require.NoError(s.t, err)
	volumes, mounts, err := createVolumesForApp(context.TODO(), s.clusterClient, a)
	require.NoError(s.t, err)
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
	assert.Equal(s.t, expectedVolume, volumes)
	assert.Equal(s.t, expectedMount, mounts)
	pv, err := s.client.CoreV1().PersistentVolumes().Get(context.TODO(), volumeName(v.Name), metav1.GetOptions{})
	require.NoError(s.t, err)
	expectedCap, err := resource.ParseQuantity("20Gi")
	require.NoError(s.t, err)
	require.EqualValues(s.t, &apiv1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: volumeName(v.Name),
			Labels: map[string]string{
				"tsuru.io/volume-name": "v1",
				"tsuru.io/volume-pool": "test-default",
				"tsuru.io/volume-plan": "p1",
				"tsuru.io/volume-team": "admin",
				"tsuru.io/is-tsuru":    "true",
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
	}, pv)
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	pvc, err := s.client.CoreV1().PersistentVolumeClaims(ns).Get(context.TODO(), volumeClaimName(v.Name), metav1.GetOptions{})
	require.NoError(s.t, err)
	emptyStr := ""
	require.EqualValues(s.t, &apiv1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: volumeClaimName(v.Name),
			Labels: map[string]string{
				"tsuru.io/volume-name": "v1",
				"tsuru.io/volume-pool": "test-default",
				"tsuru.io/volume-plan": "p1",
				"tsuru.io/volume-team": "admin",
				"tsuru.io/is-tsuru":    "true",
			},
			Namespace: ns,
		},
		Spec: apiv1.PersistentVolumeClaimSpec{
			AccessModes: []apiv1.PersistentVolumeAccessMode{apiv1.ReadWriteMany},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"tsuru.io/volume-name": "v1"},
			},
			VolumeName:       volumeName(v.Name),
			StorageClassName: &emptyStr,
			Resources: apiv1.VolumeResourceRequirements{
				Requests: apiv1.ResourceList{
					apiv1.ResourceStorage: expectedCap,
				},
			},
		},
	}, pvc)
	volumes, mounts, err = createVolumesForApp(context.TODO(), s.clusterClient, a)
	require.NoError(s.t, err)
	require.EqualValues(s.t, expectedVolume, volumes)
	require.EqualValues(s.t, expectedMount, mounts)
}

func (s *S) TestCreateVolumesForAppPluginNonPersistentEmptyDir(_ *check.C) {
	config.Set("volume-plans:p1:kubernetes:plugin", "emptyDir")
	defer config.Unset("volume-plans")
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	err := s.p.Provision(context.TODO(), a)
	require.NoError(s.t, err)
	v := volumeTypes.Volume{
		Name: "v1",
		Opts: map[string]string{
			"medium": "Memory",
		},
		Plan:      volumeTypes.VolumePlan{Name: "p1"},
		Pool:      "test-default",
		TeamOwner: "admin",
	}
	err = servicemanager.Volume.Create(context.TODO(), &v)
	require.NoError(s.t, err)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a.Name,
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	require.NoError(s.t, err)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a.Name,
		MountPoint: "/mnt2",
		ReadOnly:   false,
	})
	require.NoError(s.t, err)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    "otherapp",
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	require.NoError(s.t, err)
	volumes, mounts, err := createVolumesForApp(context.TODO(), s.clusterClient, a)
	require.NoError(s.t, err)
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
	require.EqualValues(s.t, expectedVolume, volumes)
	require.EqualValues(s.t, expectedMount, mounts)
	_, err = s.client.CoreV1().PersistentVolumes().Get(context.TODO(), volumeName(v.Name), metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	_, err = s.client.CoreV1().PersistentVolumeClaims(ns).Get(context.TODO(), volumeClaimName(v.Name), metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	volumes, mounts, err = createVolumesForApp(context.TODO(), s.clusterClient, a)
	require.NoError(s.t, err)
	require.EqualValues(s.t, expectedVolume, volumes)
	require.EqualValues(s.t, expectedMount, mounts)
}

func (s *S) TestCreateVolumesForAppPluginNonPersistentEphemeral(_ *check.C) {
	config.Set("volume-plans:p1:kubernetes:plugin", "ephemeral")
	config.Set("volume-plans:p1:kubernetes:storage-class", "my-storage-class")
	config.Set("volume-plans:p1:kubernetes:access-modes", "ReadWriteOnce")
	defer config.Unset("volume-plans")
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	err := s.p.Provision(context.TODO(), a)
	require.NoError(s.t, err)
	v := volumeTypes.Volume{
		Name: "v1",
		Opts: map[string]string{
			"capacity": "10Gi",
		},
		Plan: volumeTypes.VolumePlan{Name: "p1", Opts: map[string]interface{}{
			"storage-class": "my-storage-class",
		}},
		Pool:      "test-default",
		TeamOwner: "admin",
	}
	err = servicemanager.Volume.Create(context.TODO(), &v)
	require.NoError(s.t, err)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a.Name,
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	require.NoError(s.t, err)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a.Name,
		MountPoint: "/mnt2",
		ReadOnly:   false,
	})
	require.NoError(s.t, err)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    "otherapp",
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	require.NoError(s.t, err)
	volumes, mounts, err := createVolumesForApp(context.TODO(), s.clusterClient, a)
	require.NoError(s.t, err)
	expectedStorageClass := "my-storage-class"
	expectedCap, _ := resource.ParseQuantity("10Gi")
	expectedVolume := []apiv1.Volume{{
		Name: volumeName(v.Name),
		VolumeSource: apiv1.VolumeSource{
			Ephemeral: &apiv1.EphemeralVolumeSource{
				VolumeClaimTemplate: &apiv1.PersistentVolumeClaimTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"tsuru.io/volume-name": "v1",
							"tsuru.io/volume-pool": "test-default",
							"tsuru.io/volume-plan": "p1",
							"tsuru.io/volume-team": "admin",
							"tsuru.io/is-tsuru":    "true",
						},
					},
					Spec: apiv1.PersistentVolumeClaimSpec{
						StorageClassName: &expectedStorageClass,
						AccessModes:      []apiv1.PersistentVolumeAccessMode{apiv1.ReadWriteOnce},
						Resources: apiv1.VolumeResourceRequirements{
							Requests: apiv1.ResourceList{
								apiv1.ResourceStorage: expectedCap,
							},
						},
					},
				},
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
	require.EqualValues(s.t, expectedVolume, volumes)
	require.EqualValues(s.t, expectedMount, mounts)
	_, err = s.client.CoreV1().PersistentVolumes().Get(context.TODO(), volumeName(v.Name), metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	_, err = s.client.CoreV1().PersistentVolumeClaims(ns).Get(context.TODO(), volumeClaimName(v.Name), metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	volumes, mounts, err = createVolumesForApp(context.TODO(), s.clusterClient, a)
	require.NoError(s.t, err)
	require.EqualValues(s.t, expectedVolume, volumes)
	require.EqualValues(s.t, expectedMount, mounts)
}

func (s *S) TestCreateVolumesForAppStorageClass(_ *check.C) {
	config.Set("volume-plans:p1:kubernetes:storage-class", "my-class")
	config.Set("volume-plans:p1:kubernetes:capacity", "20Gi")
	config.Set("volume-plans:p1:kubernetes:access-modes", "ReadWriteMany")
	defer config.Unset("volume-plans")
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	err := s.p.Provision(context.TODO(), a)
	require.NoError(s.t, err)
	v := volumeTypes.Volume{
		Name:      "v1",
		Plan:      volumeTypes.VolumePlan{Name: "p1"},
		Pool:      "test-default",
		TeamOwner: "admin",
	}
	err = servicemanager.Volume.Create(context.TODO(), &v)
	require.NoError(s.t, err)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a.Name,
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	require.NoError(s.t, err)
	volumes, mounts, err := createVolumesForApp(context.TODO(), s.clusterClient, a)
	require.NoError(s.t, err)
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
	require.EqualValues(s.t, expectedVolume, volumes)
	require.EqualValues(s.t, expectedMount, mounts)
	_, err = s.client.CoreV1().PersistentVolumes().Get(context.TODO(), volumeName(v.Name), metav1.GetOptions{})
	require.ErrorContains(s.t, err, "persistentvolumes \"v1-tsuru\" not found")
	expectedClass := "my-class"
	expectedCap, err := resource.ParseQuantity("20Gi")
	require.NoError(s.t, err)
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	pvc, err := s.client.CoreV1().PersistentVolumeClaims(ns).Get(context.TODO(), volumeClaimName(v.Name), metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, &apiv1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: volumeClaimName(v.Name),
			Labels: map[string]string{
				"tsuru.io/volume-name": "v1",
				"tsuru.io/volume-pool": "test-default",
				"tsuru.io/volume-plan": "p1",
				"tsuru.io/volume-team": "admin",
				"tsuru.io/is-tsuru":    "true",
			},
			Namespace: ns,
		},
		Spec: apiv1.PersistentVolumeClaimSpec{
			AccessModes:      []apiv1.PersistentVolumeAccessMode{apiv1.ReadWriteMany},
			StorageClassName: &expectedClass,
			Resources: apiv1.VolumeResourceRequirements{
				Requests: apiv1.ResourceList{
					apiv1.ResourceStorage: expectedCap,
				},
			},
		},
	}, pvc)
	volumes, mounts, err = createVolumesForApp(context.TODO(), s.clusterClient, a)
	require.NoError(s.t, err)
	require.EqualValues(s.t, expectedVolume, volumes)
	require.EqualValues(s.t, expectedMount, mounts)
}

func (s *S) TestCreateVolumeAppNamespace(_ *check.C) {
	config.Set("volume-plans:p1:kubernetes:plugin", "nfs")
	defer config.Unset("volume-plans")
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	err := s.p.Provision(context.TODO(), a)
	require.NoError(s.t, err)
	appCR := tsuruv1.App{
		ObjectMeta: metav1.ObjectMeta{
			Name: a.Name,
		},
		Spec: tsuruv1.AppSpec{
			NamespaceName: "custom-namespace",
		},
	}
	_, err = s.client.TsuruV1().Apps(s.client.Namespace()).Update(context.TODO(), &appCR, metav1.UpdateOptions{})
	require.NoError(s.t, err)
	v := volumeTypes.Volume{
		Name: "v1",
		Opts: map[string]string{
			"path":         "/exports",
			"server":       "192.168.1.1",
			"capacity":     "20Gi",
			"access-modes": string(apiv1.ReadWriteMany),
		},
		Plan:      volumeTypes.VolumePlan{Name: "p1"},
		Pool:      "test-default",
		TeamOwner: "admin",
	}
	err = servicemanager.Volume.Create(context.TODO(), &v)
	require.NoError(s.t, err)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a.Name,
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	require.NoError(s.t, err)
	_, _, err = createVolumesForApp(context.TODO(), s.clusterClient, a)
	require.NoError(s.t, err)
	pvc, err := s.client.CoreV1().PersistentVolumeClaims("custom-namespace").Get(context.TODO(), volumeClaimName(v.Name), metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, metav1.ObjectMeta{
		Name: volumeClaimName(v.Name),
		Labels: map[string]string{
			"tsuru.io/volume-name": "v1",
			"tsuru.io/volume-pool": "test-default",
			"tsuru.io/volume-plan": "p1",
			"tsuru.io/volume-team": "admin",
			"tsuru.io/is-tsuru":    "true",
		},
		Namespace: "custom-namespace",
	}, pvc.ObjectMeta)
}

func (s *S) TestCreateVolumeMultipleNamespacesFail(_ *check.C) {
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	config.Set("volume-plans:p1:kubernetes:plugin", "nfs")
	defer config.Unset("volume-plans")
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	err := s.p.Provision(context.TODO(), a)
	require.NoError(s.t, err)
	v := volumeTypes.Volume{
		Name: "v1",
		Opts: map[string]string{
			"path":         "/exports",
			"server":       "192.168.1.1",
			"capacity":     "20Gi",
			"access-modes": string(apiv1.ReadWriteMany),
		},
		Plan:      volumeTypes.VolumePlan{Name: "p1"},
		Pool:      "test-default",
		TeamOwner: "admin",
	}
	err = servicemanager.Volume.Create(context.TODO(), &v)
	require.NoError(s.t, err)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a.Name,
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	require.NoError(s.t, err)
	err = s.p.Provision(context.TODO(), provisiontest.NewFakeAppWithPool("otherapp", "python", "otherpool", 0))
	require.NoError(s.t, err)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    "otherapp",
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	require.NoError(s.t, err)
	_, _, err = createVolumesForApp(context.TODO(), s.clusterClient, a)
	require.ErrorContains(s.t, err, `multiple namespaces for volume not allowed: "tsuru-otherpool" and "tsuru-test-default"`)
}

func (s *S) TestDeleteVolume(_ *check.C) {
	config.Set("volume-plans:p1:kubernetes:plugin", "nfs")
	defer config.Unset("volume-plans")
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	err := s.p.Provision(context.TODO(), a)
	require.NoError(s.t, err)
	v := volumeTypes.Volume{
		Name: "v1",
		Opts: map[string]string{
			"path":         "/exports",
			"server":       "192.168.1.1",
			"capacity":     "20Gi",
			"access-modes": string(apiv1.ReadWriteMany),
		},
		Plan: volumeTypes.VolumePlan{Name: "p1", Opts: map[string]interface{}{
			"storage-class": "myown",
		}},
		Pool:      "test-default",
		TeamOwner: "admin",
	}
	err = servicemanager.Volume.Create(context.TODO(), &v)
	require.NoError(s.t, err)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a.Name,
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	require.NoError(s.t, err)
	_, _, err = createVolumesForApp(context.TODO(), s.clusterClient, a)
	require.NoError(s.t, err)
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	err = deleteVolume(context.TODO(), s.clusterClient, "v1")
	require.NoError(s.t, err)
	_, err = s.client.CoreV1().PersistentVolumes().Get(context.TODO(), volumeName(v.Name), metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	_, err = s.client.CoreV1().PersistentVolumeClaims(ns).Get(context.TODO(), volumeClaimName(v.Name), metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
}

func (s *S) TestVolumeExists(_ *check.C) {
	config.Set("volume-plans:p1:kubernetes:plugin", "nfs")
	defer config.Unset("volume-plans")
	exists, err := volumeExists(context.TODO(), s.clusterClient, "v1")
	require.NoError(s.t, err)
	require.False(s.t, exists)
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	err = s.p.Provision(context.TODO(), a)
	require.NoError(s.t, err)
	v := volumeTypes.Volume{
		Name: "v1",
		Opts: map[string]string{
			"path":         "/exports",
			"server":       "192.168.1.1",
			"capacity":     "20Gi",
			"access-modes": string(apiv1.ReadWriteMany),
		},
		Plan: volumeTypes.VolumePlan{Name: "p1", Opts: map[string]interface{}{
			"storage-class": "mystorage-class",
		}},
		Pool:      "test-default",
		TeamOwner: "admin",
	}
	err = servicemanager.Volume.Create(context.TODO(), &v)
	require.NoError(s.t, err)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a.Name,
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	require.NoError(s.t, err)
	_, _, err = createVolumesForApp(context.TODO(), s.clusterClient, a)
	require.NoError(s.t, err)
	exists, err = volumeExists(context.TODO(), s.clusterClient, "v1")
	require.NoError(s.t, err)
	require.True(s.t, exists)
}
