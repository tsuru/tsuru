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

package img

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	crd "github.com/containerbuilding/cbi/pkg/apis/cbi/v1alpha1"
	pluginapi "github.com/containerbuilding/cbi/pkg/plugin/api"
	"github.com/containerbuilding/cbi/pkg/plugin/base"
	"github.com/containerbuilding/cbi/pkg/plugin/base/cbipluginhelper"
	"github.com/containerbuilding/cbi/pkg/plugin/base/registryutil"
)

type Img struct {
	Image  string
	Helper cbipluginhelper.Helper
}

var _ base.Backend = &Img{}

func (b *Img) Info(ctx context.Context, req *pluginapi.InfoRequest) (*pluginapi.InfoResponse, error) {
	res := &pluginapi.InfoResponse{
		Labels: map[string]string{
			pluginapi.LPluginName:         "img",
			pluginapi.LLanguageDockerfile: "",
		},
	}
	for k, v := range cbipluginhelper.Labels {
		res.Labels[k] = v
	}
	return res, nil
}

func (b *Img) commonPodSpec(buildJob crd.BuildJob) corev1.PodSpec {
	push := "0"
	if buildJob.Spec.Registry.Push {
		push = "1"
	}
	// TODO(AkihiroSuda): support non-privileged
	privileged := true
	podSpec := corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyNever,
		Containers: []corev1.Container{
			{
				Name:    "img-job",
				Image:   b.Image,
				Command: []string{
					// needs to be "docker-build-push.sh", not img itself.
				},
				Env: []corev1.EnvVar{
					{
						Name:  "DBP_DOCKER_BINARY",
						Value: "img",
					},
					{
						Name:  "DBP_IMAGE_NAME",
						Value: buildJob.Spec.Registry.Target,
					},
					{
						Name:  "DBP_DIALECT",
						Value: "docker",
					},
					{
						Name:  "DBP_PUSH",
						Value: push,
					},
				},
				SecurityContext: &corev1.SecurityContext{
					Privileged: &privileged,
				},
			},
		},
	}
	return podSpec
}

func (b *Img) CreatePodTemplateSpec(ctx context.Context, buildJob crd.BuildJob) (*corev1.PodTemplateSpec, error) {
	if buildJob.Spec.Language.Kind != crd.LanguageKindDockerfile {
		return nil, fmt.Errorf("unsupported Spec.Language: %v", buildJob.Spec.Language)
	}
	podSpec := b.commonPodSpec(buildJob)
	if buildJob.Spec.Registry.Push && buildJob.Spec.Registry.SecretRef.Name != "" {
		if err := registryutil.InjectRegistrySecret(&podSpec, 0, "/root", buildJob.Spec.Registry.SecretRef); err != nil {
			return nil, err
		}
	}
	injector := cbipluginhelper.Injector{
		Helper:        b.Helper,
		TargetPodSpec: &podSpec,
	}
	dbpPath, err := injector.InjectFile("/docker-build-push.sh")
	if err != nil {
		return nil, err
	}
	podSpec.Containers[0].Command = []string{dbpPath}
	ctxInjector := cbipluginhelper.ContextInjector{
		Injector: injector,
	}
	ctxPath, err := ctxInjector.Inject(buildJob.Spec.Context)
	if err != nil {
		return nil, err
	}
	podSpec.Containers[0].Command = append(podSpec.Containers[0].Command, []string{
		ctxPath,
	}...)
	return &corev1.PodTemplateSpec{
		Spec: podSpec,
	}, nil
}
