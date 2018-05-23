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

package registryutil

import (
	"github.com/cyphar/filepath-securejoin"
	corev1 "k8s.io/api/core/v1"
)

// InjectRegistrySecret injects .dockerconfigjson secret to ~/.docker/config.json
func InjectRegistrySecret(podSpec *corev1.PodSpec, containerIdx int, homeDir string, secretRef corev1.LocalObjectReference) error {
	volMountPath, err := securejoin.SecureJoin(homeDir, ".docker")
	if err != nil {
		return err
	}
	volName := "cbi-registrysecret"
	vol := corev1.Volume{
		Name: volName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: secretRef.Name,
				Items: []corev1.KeyToPath{
					{
						Key:  ".dockerconfigjson",
						Path: "config.json",
					},
				},
			},
		},
	}
	podSpec.Volumes = append(podSpec.Volumes, vol)
	podSpec.Containers[containerIdx].VolumeMounts = append(podSpec.Containers[containerIdx].VolumeMounts,
		corev1.VolumeMount{
			Name:      volName,
			MountPath: volMountPath,
			ReadOnly:  true,
		},
	)
	return nil
}
