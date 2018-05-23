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

package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v2"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	aev1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	crd "github.com/containerbuilding/cbi/pkg/apis/cbi/v1alpha1"
	"github.com/containerbuilding/cbi/pkg/plugin"
)

var generateManifests = &cli.Command{
	Name:      "generate-manifests",
	Usage:     "Generate Kubernetes manifests for deploying CBI.",
	ArgsUsage: "[flags] REGISTRY TAG",
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:  "plugin",
			Usage: "Plugin names (first=highest priority, last=lowest priority)",
			Value: cli.NewStringSlice("docker", "buildkit", "buildah", "kaniko", "img"),
		},
	},
	Action: generateManifestsAction,
}

func generateManifestsAction(clicontext *cli.Context) error {
	registry := clicontext.Args().Get(0)
	if registry == "" {
		return errors.New("REGISTRY missing")
	}
	if strings.HasSuffix(registry, "/") {
		return errors.New("REGISTRY must not contain a trailing slash")
	}
	tag := clicontext.Args().Get(1)
	if tag == "" {
		return errors.New("TAG missing")
	}

	var (
		manifests      []*Manifest
		serviceAccount *corev1.ServiceAccount
		pluginServices []*corev1.Service
	)
	manifestGenerators := []func() (*Manifest, error){
		GenerateCRD,
		func() (*Manifest, error) {
			o, e := GenerateServiceAccount()
			if e == nil {
				serviceAccount = o.Object.(*corev1.ServiceAccount)
			}
			return o, e
		},
		func() (*Manifest, error) {
			return GenerateClusterRoleBinding(serviceAccount)
		},
	}
	for _, f := range clicontext.StringSlice("plugin") {
		p := f // iterator for the closure
		var args func() []string
		switch p {
		case "docker":
			dockerImage := "docker:18.03"
			args = func() []string {
				return []string{"-docker-image=" + dockerImage}
			}
		case "buildkit":
			buildkitImage := "tonistiigi/buildkit:latest"
			var (
				buildkitdDepl *appsv1.Deployment
				buildkitdSvc  *corev1.Service
			)
			manifestGenerators = append(manifestGenerators,
				func() (*Manifest, error) {
					o, e := GenerateBuildKitDaemonDeployment(buildkitImage)
					if e == nil {
						buildkitdDepl = o.Object.(*appsv1.Deployment)
					}
					return o, e
				},
				func() (*Manifest, error) {
					o, e := GenerateService(buildkitdDepl)
					if e == nil {
						buildkitdSvc = o.Object.(*corev1.Service)
					}
					return o, e
				},
			)

			args = func() []string {
				return []string{
					"-buildctl-image=" + buildkitImage,
					fmt.Sprintf("-buildkitd-addr=tcp://%s:%d",
						buildkitdSvc.ObjectMeta.Name,
						buildkitdSvc.Spec.Ports[0].Port)}
			}
		case "buildah":
			args = func() []string {
				return []string{fmt.Sprintf("-buildah-image=%s/buildah:%s", registry, tag)}
			}
		case "kaniko":
			args = func() []string {
				return []string{"-kaniko-image=gcr.io/kaniko-project/executor:latest"}
			}
		case "img":
			args = func() []string {
				return []string{"-img-image=r.j3ss.co/img:latest"}
			}
		default:
			return fmt.Errorf("unknown plugin: %s", p)
		}
		var depl *appsv1.Deployment
		manifestGenerators = append(manifestGenerators,
			func() (*Manifest, error) {
				o, e := GeneratePluginDeployment(p, registry, tag, args())
				if e == nil {
					depl = o.Object.(*appsv1.Deployment)
				}
				return o, e
			},
			func() (*Manifest, error) {
				o, e := GenerateService(depl)
				if e == nil {
					pluginServices = append(pluginServices, o.Object.(*corev1.Service))
				}
				return o, e
			},
		)
	}
	manifestGenerators = append(manifestGenerators,
		func() (*Manifest, error) {
			return GenerateCBIDDeployment(registry, tag, serviceAccount.ObjectMeta.Name, pluginServices)
		})
	for _, f := range manifestGenerators {
		m, err := f()
		if err != nil {
			return err
		}
		manifests = append(manifests, m)
	}
	return WriteManifests(os.Stdout, manifests)
}

func GenerateCRD() (*Manifest, error) {
	o := aev1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: aev1.SchemeGroupVersion.String(),
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "buildjobs." + crd.SchemeGroupVersion.Group,
		},
		Spec: aev1.CustomResourceDefinitionSpec{
			Group:   crd.SchemeGroupVersion.Group,
			Version: crd.SchemeGroupVersion.Version,
			Names: aev1.CustomResourceDefinitionNames{
				Kind:   "BuildJob",
				Plural: "buildjobs",
			},
			Scope: aev1.NamespaceScoped,
			// TODO: add Validation
		},
	}
	return &Manifest{
		Description: "CRD (BuildJob)",
		Object:      &o,
	}, nil
}

func GenerateServiceAccount() (*Manifest, error) {
	o := corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cbi",
			Namespace: metav1.NamespaceDefault,
		},
	}
	return &Manifest{
		Description: "ServiceAccount used by CBI controller daemon",
		Object:      &o,
	}, nil
}

func GenerateClusterRoleBinding(sa *corev1.ServiceAccount) (*Manifest, error) {
	o := rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: rbacv1.SchemeGroupVersion.String(),
			Kind:       "ClusterRoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "cbi",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      sa.ObjectMeta.Name,
				Namespace: sa.ObjectMeta.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			// FIXME
			Name: "cluster-admin",
		},
	}
	return &Manifest{
		Description: "ClusterRoleBinding for binding a role to the service account. (FIXME: cluster-admin)",
		Object:      &o,
	}, nil
}

func GeneratePluginDeployment(pluginName, registry, tag string, args []string) (*Manifest, error) {
	labels := map[string]string{
		"app": "cbi-" + pluginName,
	}
	name := "cbi-" + pluginName
	fullArgs := append([]string{
		"-logtostderr",
		"-v=4",
		fmt.Sprintf("-helper-image=%s/cbipluginhelper:%s", registry, tag),
	}, args...)
	o := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  name,
							Image: fmt.Sprintf("%s/cbi-%s:%s", registry, pluginName, tag),
							Args:  fullArgs,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: int32(plugin.DefaultPort),
								},
							},
						},
					},
				},
			},
		},
	}
	if tag == "latest" {
		o.Spec.Template.Spec.Containers[0].ImagePullPolicy = corev1.PullAlways
	}
	return &Manifest{
		Description: fmt.Sprintf("Plugin: %s", pluginName),
		Object:      &o,
	}, nil
}

func GenerateService(depl *appsv1.Deployment) (*Manifest, error) {
	o := corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   depl.ObjectMeta.Name,
			Labels: depl.ObjectMeta.Labels,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				// FIXME: add all ports?
				{
					Port: depl.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort,
				},
			},
			Selector: depl.Spec.Template.ObjectMeta.Labels,
		},
	}
	return &Manifest{
		Description: fmt.Sprintf("Service for deployment %s", depl.ObjectMeta.Name),
		Object:      &o,
	}, nil
}

func GenerateBuildKitDaemonDeployment(imageWithTag string) (*Manifest, error) {
	labels := map[string]string{
		"app": "cbi-buildkit-buildkitd",
	}
	name := "cbi-buildkit-buildkitd"
	port := 1234
	privileged := true
	o := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  name,
							Image: imageWithTag,
							Args:  []string{"--addr", fmt.Sprintf("tcp://0.0.0.0:%d", port)},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: int32(port),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								Privileged: &privileged,
							},
						},
					},
				},
			},
		},
	}
	// FIXME
	if strings.HasSuffix(imageWithTag, ":latest") {
		o.Spec.Template.Spec.Containers[0].ImagePullPolicy = corev1.PullAlways
	}
	return &Manifest{
		Description: "BuildKit daemon",
		Object:      &o,
	}, nil
}

func GenerateCBIDDeployment(registry, tag, serviceAccountName string, pluginServices []*corev1.Service) (*Manifest, error) {
	labels := map[string]string{
		"app": "cbid",
	}
	name := "cbid"
	var pluginAddrs []string
	for _, svc := range pluginServices {
		pluginAddrs = append(pluginAddrs, svc.ObjectMeta.Name)
	}
	o := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccountName,
					Containers: []corev1.Container{
						{
							Name:  name,
							Image: fmt.Sprintf("%s/cbid:%s", registry, tag),
							Args: []string{
								"-logtostderr", "-v=4", "-cbi-plugins=" + strings.Join(pluginAddrs, ","),
							},
						},
					},
				},
			},
		},
	}
	if tag == "latest" {
		o.Spec.Template.Spec.Containers[0].ImagePullPolicy = corev1.PullAlways
	}
	return &Manifest{
		Description: fmt.Sprintf("CBI controller daemon. Plugin addresses=%v", pluginAddrs),
		Object:      &o,
	}, nil
}

func WriteManifests(w io.Writer, manifests []*Manifest) error {
	fmt.Fprintf(w, "# Autogenerated at %s.\n", time.Now().Format(time.UnixDate))
	fmt.Fprintf(w, "# Command: %v\n", os.Args)
	fmt.Fprintf(w, "# Contains %d manifests.\n", len(manifests))
	for i, m := range manifests {
		groupVersionKind := m.Object.GetObjectKind().GroupVersionKind()
		fmt.Fprintf(w, "# %2d. %s [%s]\n", i, groupVersionKind.Kind, m.Description)
	}
	for i, m := range manifests {
		fmt.Fprintf(w, "---\n")
		if m.Description != "" {
			fmt.Fprintf(w, "# %d. %s\n", i, m.Description)
		}
		d, err := yaml.Marshal(m.Object)
		if err != nil {
			return err
		}
		fmt.Fprintln(w, string(d))
	}
	return nil
}

type Manifest struct {
	Description string
	runtime.Object
}
