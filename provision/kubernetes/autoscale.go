// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"

	"github.com/pkg/errors"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var errNoDeploy = errors.New("no routable version found for app, at least one deploy is required before configuring autoscale")

func (p *kubernetesProvisioner) GetAutoScale(ctx context.Context, a provision.App) ([]provision.AutoScaleSpec, error) {
	client, err := clusterForPool(ctx, a.GetPool())
	if err != nil {
		return nil, err
	}
	return getAutoScale(ctx, client, a, "")
}

func getAutoScale(ctx context.Context, client *ClusterClient, a provision.App, process string) ([]provision.AutoScaleSpec, error) {
	ns, err := client.AppNamespace(a)
	if err != nil {
		return nil, err
	}

	ls, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
		App:     a,
		Process: process,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix:      tsuruLabelPrefix,
			Provisioner: provisionerName,
		},
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	hpas, err := client.AutoscalingV2beta2().HorizontalPodAutoscalers(ns).List(metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(ls.ToHPASelector())).String(),
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var specs []provision.AutoScaleSpec
	for _, hpa := range hpas.Items {
		specs = append(specs, hpaToSpec(hpa))
	}
	return specs, nil
}

func hpaToSpec(hpa autoscalingv2.HorizontalPodAutoscaler) provision.AutoScaleSpec {
	ls := labelSetFromMeta(&hpa.ObjectMeta)
	spec := provision.AutoScaleSpec{
		MaxUnits: uint(hpa.Spec.MaxReplicas),
		Process:  ls.AppProcess(),
		Version:  ls.AppVersion(),
	}
	if hpa.Spec.MinReplicas != nil {
		spec.MinUnits = uint(*hpa.Spec.MinReplicas)
	}
	if len(hpa.Spec.Metrics) > 0 &&
		hpa.Spec.Metrics[0].Resource != nil &&
		hpa.Spec.Metrics[0].Resource.Target.AverageValue != nil {
		spec.AverageCPU = hpa.Spec.Metrics[0].Resource.Target.AverageValue.String()
	}
	return spec
}

func (p *kubernetesProvisioner) RemoveAutoScale(ctx context.Context, a provision.App, process string) error {
	client, err := clusterForPool(ctx, a.GetPool())
	if err != nil {
		return err
	}
	ns, err := client.AppNamespace(a)
	if err != nil {
		return err
	}
	err = client.AutoscalingV2beta2().HorizontalPodAutoscalers(ns).Delete(hpaNameForApp(a, process), &metav1.DeleteOptions{})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.WithStack(err)
	}
	return nil
}

func (p *kubernetesProvisioner) SetAutoScale(ctx context.Context, a provision.App, spec provision.AutoScaleSpec) error {
	client, err := clusterForPool(ctx, a.GetPool())
	if err != nil {
		return err
	}
	return setAutoScale(ctx, client, a, spec)
}

func setAutoScale(ctx context.Context, client *ClusterClient, a provision.App, spec provision.AutoScaleSpec) error {
	depGroups, err := deploymentsDataForApp(ctx, client, a)
	if err != nil {
		return err
	}

	minRoutableVersion := -1
	var depInfo *deploymentInfo

	for version, deps := range depGroups.versioned {
		if spec.Process == "" && len(deps) > 1 {
			return provision.InvalidProcessError{Msg: "process argument is required"}
		}
		for i, dep := range deps {
			if !dep.isRoutable {
				continue
			}
			if spec.Process != "" && spec.Process != dep.process {
				continue
			}
			if dep.dep.Spec.Replicas == nil || *dep.dep.Spec.Replicas == 0 {
				continue
			}
			if minRoutableVersion == -1 || version < minRoutableVersion {
				minRoutableVersion = version
				depInfo = &deps[i]
			}
		}
	}

	if depInfo == nil {
		return errNoDeploy
	}

	labels, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
		App:     a,
		Process: depInfo.process,
		Version: minRoutableVersion,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix:      tsuruLabelPrefix,
			Provisioner: provisionerName,
		},
	})
	if err != nil {
		return errors.WithStack(err)
	}
	cpu, err := resource.ParseQuantity(spec.AverageCPU)
	if err != nil {
		return errors.WithStack(err)
	}

	labels, _ = provision.SplitServiceLabelsAnnotations(labels)
	labels.WithoutIsolated().WithoutRoutable().WithoutVersion()
	minUnits := int32(spec.MinUnits)

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:   hpaNameForApp(a, depInfo.process),
			Labels: labels.ToLabels(),
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MinReplicas: &minUnits,
			MaxReplicas: int32(spec.MaxUnits),
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: appsv1.SchemeGroupVersion.String(),
				Kind:       "Deployment",
				Name:       depInfo.dep.Name,
			},
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: "cpu",
						Target: autoscalingv2.MetricTarget{
							Type:         autoscalingv2.AverageValueMetricType,
							AverageValue: &cpu,
						},
					},
				},
			},
		},
	}

	ns, err := client.AppNamespace(a)
	if err != nil {
		return err
	}

	existing, err := client.AutoscalingV2beta2().HorizontalPodAutoscalers(ns).Get(hpa.Name, metav1.GetOptions{})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.WithStack(err)
	}
	if existing.ResourceVersion != "" {
		hpa.ResourceVersion = existing.ResourceVersion
		_, err = client.AutoscalingV2beta2().HorizontalPodAutoscalers(ns).Update(hpa)
	} else {
		_, err = client.AutoscalingV2beta2().HorizontalPodAutoscalers(ns).Create(hpa)
	}
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func ensureAutoScale(ctx context.Context, client *ClusterClient, a provision.App, process string) error {
	autoScaleSpecs, err := getAutoScale(ctx, client, a, process)
	if err != nil {
		return err
	}
	if len(autoScaleSpecs) == 0 {
		return nil
	}

	multiErr := tsuruErrors.NewMultiError()
	for _, spec := range autoScaleSpecs {
		err = setAutoScale(ctx, client, a, spec)
		if err != nil && err != errNoDeploy {
			multiErr.Add(err)
		}
	}
	return multiErr.ToError()
}
