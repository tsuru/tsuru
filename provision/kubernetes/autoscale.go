// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"strconv"
	"strings"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
	provTypes "github.com/tsuru/tsuru/types/provision"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	vpav1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	vpaclientset "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned"
	k8sutilsptr "k8s.io/utils/ptr"
)

const (
	vpaCRDName = "verticalpodautoscalers.autoscaling.k8s.io"
)

var errNoDeploy = errors.New("no routable version found for app, at least one deploy is required before configuring autoscale")

func (p *kubernetesProvisioner) GetVerticalAutoScaleRecommendations(ctx context.Context, a *appTypes.App) ([]provTypes.RecommendedResources, error) {
	client, err := clusterForPool(ctx, a.Pool)
	if err != nil {
		return nil, err
	}
	hasVPA, err := vpaCRDExists(ctx, client)
	if err != nil {
		return nil, err
	}
	if !hasVPA {
		return nil, nil
	}

	controller, err := getClusterController(p, client)
	if err != nil {
		return nil, err
	}
	vpaInformer, err := controller.getVPAInformer()
	if err != nil {
		return nil, err
	}

	ls, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
		App: a,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix: tsuruLabelPrefix,
		},
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return nil, err
	}
	vpas, err := vpaInformer.Lister().VerticalPodAutoscalers(ns).List(labels.SelectorFromSet(labels.Set(ls.ToHPASelector())))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var specs []provTypes.RecommendedResources
	for _, vpa := range vpas {
		specs = append(specs, vpaToRecommended(*vpa))
	}
	return specs, nil
}

func vpaToRecommended(vpa vpav1.VerticalPodAutoscaler) provTypes.RecommendedResources {
	ls := labelSetFromMeta(&vpa.ObjectMeta)
	rec := provTypes.RecommendedResources{
		Process: ls.AppProcess(),
	}
	if vpa.Status.Recommendation == nil {
		return rec
	}
	for _, contRec := range vpa.Status.Recommendation.ContainerRecommendations {
		if contRec.ContainerName != vpa.Name {
			continue
		}
		rec.Recommendations = append(rec.Recommendations, provTypes.RecommendedProcessResources{
			Type:   "target",
			CPU:    contRec.Target.Cpu().String(),
			Memory: contRec.Target.Memory().String(),
		})
		rec.Recommendations = append(rec.Recommendations, provTypes.RecommendedProcessResources{
			Type:   "uncappedTarget",
			CPU:    contRec.UncappedTarget.Cpu().String(),
			Memory: contRec.UncappedTarget.Memory().String(),
		})
		rec.Recommendations = append(rec.Recommendations, provTypes.RecommendedProcessResources{
			Type:   "lowerBound",
			CPU:    contRec.LowerBound.Cpu().String(),
			Memory: contRec.LowerBound.Memory().String(),
		})
		rec.Recommendations = append(rec.Recommendations, provTypes.RecommendedProcessResources{
			Type:   "upperBound",
			CPU:    contRec.UpperBound.Cpu().String(),
			Memory: contRec.UpperBound.Memory().String(),
		})
	}
	return rec
}

func (p *kubernetesProvisioner) GetAutoScale(ctx context.Context, a *appTypes.App) ([]provTypes.AutoScaleSpec, error) {
	client, err := clusterForPool(ctx, a.Pool)
	if err != nil {
		return nil, err
	}

	kedaClient, err := KEDAClientForConfig(client.restConfig)
	if err != nil {
		return nil, err
	}

	ls, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
		App: a,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix: tsuruLabelPrefix,
		},
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return nil, err
	}

	var specs []provTypes.AutoScaleSpec

	hpaList, err := client.AutoscalingV2().HorizontalPodAutoscalers(ns).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(ls.ToHPASelector())).String(),
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	for _, hpa := range hpaList.Items {
		scaledObjectName := kedaScaledObjectName(hpa)
		if scaledObjectName == "" {
			specs = append(specs, hpaToSpec(hpa))
		}
	}

	scaledObjects, err := kedaClient.KedaV1alpha1().ScaledObjects(ns).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(labels.Set(ls.ToHPASelector())).String()})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, scaledObject := range scaledObjects.Items {
		specs = append(specs, scaledObjectToSpec(scaledObject))
	}
	return specs, nil
}

func kedaScaledObjectName(hpa autoscalingv2.HorizontalPodAutoscaler) string {
	return hpa.Labels["scaledobject.keda.sh/name"]
}

func scaledObjectToSpec(scaledObject kedav1alpha1.ScaledObject) provTypes.AutoScaleSpec {
	ls := labelSetFromMeta(&scaledObject.ObjectMeta)
	behavior := getBehaviorNoFail(scaledObject)
	spec := provTypes.AutoScaleSpec{
		MaxUnits: uint(*scaledObject.Spec.MaxReplicaCount),
		MinUnits: uint(*scaledObject.Spec.MinReplicaCount),
		Process:  ls.AppProcess(),
		Version:  ls.AppVersion(),
		Behavior: provTypes.BehaviorAutoScaleSpec{
			ScaleDown: &provTypes.ScaleDownPolicy{
				PercentagePolicyValue: getPercentagePolicy(behavior),
				UnitsPolicyValue:      getUnitPolicy(behavior),
				StabilizationWindow:   getStabilizationWindow(behavior),
			},
		},
	}

	for _, metric := range scaledObject.Spec.Triggers {
		switch metric.Type {
		case "cron":
			minReplicas, _ := strconv.Atoi(metric.Metadata["desiredReplicas"])

			spec.Schedules = append(spec.Schedules, provTypes.AutoScaleSchedule{
				MinReplicas: minReplicas,
				Start:       metric.Metadata["start"],
				End:         metric.Metadata["end"],
				Timezone:    metric.Metadata["timezone"],
				Name:        metric.Metadata["scheduleName"],
			})

		case "prometheus":
			thresholdValue, _ := strconv.ParseFloat(metric.Metadata["threshold"], 64)
			activationThresholdValue, _ := strconv.ParseFloat(metric.Metadata["activationThreshold"], 64)

			spec.Prometheus = append(spec.Prometheus, provTypes.AutoScalePrometheus{
				Name:                metric.Metadata["prometheusMetricName"],
				Query:               metric.Metadata["query"],
				Threshold:           thresholdValue,
				ActivationThreshold: activationThresholdValue,
				PrometheusAddress:   metric.Metadata["serverAddress"],
			})

		case "cpu":
			cpuValue := metric.Metadata["value"]
			if metric.MetricType == autoscalingv2.UtilizationMetricType {
				// percentage based, so multiple by 10
				spec.AverageCPU = fmt.Sprintf("%s0m", cpuValue)
			} else if metric.MetricType == autoscalingv2.AverageValueMetricType {
				spec.AverageCPU = fmt.Sprintf("%sm", cpuValue)
			}
		}
	}

	return spec
}

func hpaToSpec(hpa autoscalingv2.HorizontalPodAutoscaler) provTypes.AutoScaleSpec {
	ls := labelSetFromMeta(&hpa.ObjectMeta)
	spec := provTypes.AutoScaleSpec{
		MaxUnits: uint(hpa.Spec.MaxReplicas),
		Process:  ls.AppProcess(),
		Version:  ls.AppVersion(),
		Behavior: provTypes.BehaviorAutoScaleSpec{
			ScaleDown: &provTypes.ScaleDownPolicy{
				PercentagePolicyValue: getPercentagePolicy(hpa.Spec.Behavior),
				UnitsPolicyValue:      getUnitPolicy(hpa.Spec.Behavior),
				StabilizationWindow:   getStabilizationWindow(hpa.Spec.Behavior),
			},
		},
	}
	if hpa.Spec.MinReplicas != nil {
		spec.MinUnits = uint(*hpa.Spec.MinReplicas)
	}

	cpuValue := int64(0)
	if len(hpa.Spec.Metrics) > 0 && hpa.Spec.Metrics[0].Resource != nil {
		if hpa.Spec.Metrics[0].Resource.Target.AverageUtilization != nil {
			cpuValue = int64(*hpa.Spec.Metrics[0].Resource.Target.AverageUtilization)
			cpuValue = cpuValue * 10
		} else if hpa.Spec.Metrics[0].Resource.Target.AverageValue != nil {
			cpuValue = hpa.Spec.Metrics[0].Resource.Target.AverageValue.MilliValue()
		}
	}

	if cpuValue > 0 {
		spec.AverageCPU = fmt.Sprintf("%dm", cpuValue)
	}

	return spec
}

func (p *kubernetesProvisioner) deleteAllAutoScale(ctx context.Context, a *appTypes.App) error {
	scaleSpecs, err := p.GetAutoScale(ctx, a)
	if err != nil {
		return err
	}
	for _, spec := range scaleSpecs {
		err = p.RemoveAutoScale(ctx, a, spec.Process)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *kubernetesProvisioner) deleteHPAByVersionAndProcess(ctx context.Context, a *appTypes.App, process string, version int) error {
	scaleSpecs, err := p.GetAutoScale(ctx, a)
	if err != nil {
		return err
	}
	for _, spec := range scaleSpecs {
		if strings.Compare(process, spec.Process) == 0 && spec.Version == version {
			err = p.RemoveAutoScale(ctx, a, spec.Process)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *kubernetesProvisioner) RemoveAutoScale(ctx context.Context, a *appTypes.App, process string) error {
	client, err := clusterForPool(ctx, a.Pool)
	if err != nil {
		return err
	}

	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return err
	}

	depInfo, err := minimumAutoScaleVersion(ctx, client, a, process)
	if err != nil {
		return err
	}

	hpaName := hpaNameForApp(a, depInfo.process)

	err = removeKEDAScaleObject(ctx, client, ns, hpaName)
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.WithStack(err)
	}

	err = client.AutoscalingV2().HorizontalPodAutoscalers(ns).Delete(ctx, hpaName, metav1.DeleteOptions{})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.WithStack(err)
	}

	return nil
}

func (p *kubernetesProvisioner) SwapAutoScale(ctx context.Context, a *appTypes.App, versionStr string) error {
	version, _ := strconv.Atoi(versionStr)
	return p.swapAutoScale(ctx, a, version)
}

func (p *kubernetesProvisioner) swapAutoScale(ctx context.Context, a *appTypes.App, version int) error {
	client, err := clusterForPool(ctx, a.Pool)
	if err != nil {
		return err
	}

	depGroups, err := deploymentsDataForApp(ctx, client, a)
	if err != nil {
		return err
	}
	if _, ok := depGroups.versioned[version]; !ok {
		return errors.New("could not swap the autoscale, make sure the version provided is currently deployed")
	}

	scaleSpecs, err := p.GetAutoScale(ctx, a)
	if err != nil {
		return err
	}
	if len(scaleSpecs) == 0 {
		return errors.New("Cannot swap the autoscale, make sure the app has an autoscale configured")
	}
	multiErr := tsuruErrors.NewMultiError()
	for _, spec := range scaleSpecs {
		spec.Version = version
		err := setAutoScale(ctx, client, a, spec, true)
		if err != nil {
			multiErr.Add(err)
		}
	}
	return multiErr.ToError()
}

func removeKEDAScaleObject(ctx context.Context, client *ClusterClient, ns string, scaledObjectName string) error {
	kedaClient, err := KEDAClientForConfig(client.restConfig)
	if err != nil {
		return err
	}

	err = kedaClient.KedaV1alpha1().ScaledObjects(ns).Delete(ctx, scaledObjectName, metav1.DeleteOptions{})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.WithStack(err)
	}

	return nil
}

func (p *kubernetesProvisioner) SetAutoScale(ctx context.Context, a *appTypes.App, spec provTypes.AutoScaleSpec) error {
	client, err := clusterForPool(ctx, a.Pool)
	if err != nil {
		return err
	}
	return setAutoScale(ctx, client, a, spec, false)
}

func setAutoScale(ctx context.Context, client *ClusterClient, a *appTypes.App, spec provTypes.AutoScaleSpec, preserveVersions bool) error {
	depInfo, err := minimumAutoScaleVersion(ctx, client, a, spec.Process)
	if err != nil {
		return err
	}
	if preserveVersions {
		depGroups, err := deploymentsDataForApp(ctx, client, a)
		if err != nil {
			return err
		}
		if deps, ok := depGroups.versioned[spec.Version]; ok {
			for _, dep := range deps {
				// Only consider deployments for the same process
				if spec.Process != "" && dep.process != spec.Process {
					continue
				}
				if dep.replicas > 0 {
					depInfo.version = dep.version
					depInfo.isBase = dep.isBase
					break
				}
			}
		}
	}

	labels, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
		App:     a,
		Process: depInfo.process,
		Version: depInfo.version,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix: tsuruLabelPrefix,
		},
	})
	if err != nil {
		return errors.WithStack(err)
	}
	labels = labels.WithoutIsolated().WithoutRoutable()
	hpaName := hpaNameForApp(a, depInfo.process)

	if len(spec.Schedules) > 0 || len(spec.Prometheus) > 0 {
		err = setKEDAAutoscale(ctx, client, spec, a, depInfo, hpaName, labels, preserveVersions)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	minUnits := int32(spec.MinUnits)

	cpuValue, err := provision.CPUValueOfAutoScaleSpec(&spec, a)
	if err != nil {
		return errors.WithStack(err)
	}

	target := autoscalingv2.MetricTarget{}
	if a.Plan.GetMilliCPU() > 0 {
		target.Type = autoscalingv2.UtilizationMetricType
		val := int32(cpuValue)
		target.AverageUtilization = &val
	} else {
		target.Type = autoscalingv2.AverageValueMetricType
		target.AverageValue = resource.NewMilliQuantity(int64(cpuValue), resource.DecimalSI)
		// Fill string value for easier tests
		_ = target.AverageValue.String()
	}

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:   hpaName,
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
			// FIXME(cezarsa): We should probably support letting the users
			// customize the behavior directly. Meanwhile, we'll use a safer
			// default to prevent the autoscaler from scaling down too fast
			// poossibly disrupting the app.
			Behavior: buildHPABehavior(spec.Behavior.ScaleDown),
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name:   "cpu",
						Target: target,
					},
				},
			},
		},
	}

	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return err
	}

	existingHPA, err := client.AutoscalingV2().HorizontalPodAutoscalers(ns).Get(ctx, hpaName, metav1.GetOptions{})
	if k8sErrors.IsNotFound(err) {
		existingHPA = nil

		err = removeKEDAScaleObject(ctx, client, ns, hpaName)
		if err != nil {
			return err
		}
	} else if err != nil {
		return errors.WithStack(err)
	}

	if existingHPA != nil {
		if preserveVersions && !depInfo.isBase {
			hpa.Spec.ScaleTargetRef.Name = provision.AppProcessName(a, depInfo.process, depInfo.version, "")
		}
		hpa.ResourceVersion = existingHPA.ResourceVersion
		_, err = client.AutoscalingV2().HorizontalPodAutoscalers(ns).Update(ctx, hpa, metav1.UpdateOptions{})
	} else {
		_, err = client.AutoscalingV2().HorizontalPodAutoscalers(ns).Create(ctx, hpa, metav1.CreateOptions{})
	}
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func setKEDAAutoscale(ctx context.Context, client *ClusterClient, spec provTypes.AutoScaleSpec, a *appTypes.App, depInfo *deploymentInfo, hpaName string, labels *provision.LabelSet, preserveVersions bool) error {
	kedaClient, err := KEDAClientForConfig(client.restConfig)
	if err != nil {
		return err
	}

	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return err
	}

	// remove HPA managed by tsuru so KEDA can takeover AutoScaling
	err = client.AutoscalingV2().HorizontalPodAutoscalers(ns).Delete(ctx, hpaNameForApp(a, depInfo.process), metav1.DeleteOptions{})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.WithStack(err)
	}

	expectedKEDAScaledObject, err := newKEDAScaledObject(spec, a, depInfo, ns, hpaName, labels, preserveVersions)
	if err != nil {
		return err
	}

	observedKEDAScaledObject, err := kedaClient.KedaV1alpha1().ScaledObjects(ns).Get(ctx, hpaName, metav1.GetOptions{})
	if k8sErrors.IsNotFound(err) {
		_, err = kedaClient.KedaV1alpha1().ScaledObjects(ns).Create(ctx, expectedKEDAScaledObject, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}

	expectedKEDAScaledObject.ResourceVersion = observedKEDAScaledObject.ResourceVersion
	_, err = kedaClient.KedaV1alpha1().ScaledObjects(ns).Update(ctx, expectedKEDAScaledObject, metav1.UpdateOptions{})
	return err
}

func newKEDAScaledObject(spec provTypes.AutoScaleSpec, a *appTypes.App, depInfo *deploymentInfo, ns string, hpaName string, labels *provision.LabelSet, preserveVersions bool) (*kedav1alpha1.ScaledObject, error) {
	kedaTriggers := []kedav1alpha1.ScaleTriggers{}

	if spec.AverageCPU != "" {
		cpu, err := provision.CPUValueOfAutoScaleSpec(&spec, a)
		if err != nil {
			return nil, err
		}

		cpuTrigger := kedav1alpha1.ScaleTriggers{
			Type: "cpu",
		}

		if a.Plan.GetMilliCPU() > 0 {
			cpuTrigger.MetricType = autoscalingv2.UtilizationMetricType
		} else {
			cpuTrigger.MetricType = autoscalingv2.AverageValueMetricType
		}

		cpuTrigger.Metadata = map[string]string{
			"value": strconv.Itoa(cpu),
		}
		kedaTriggers = append(kedaTriggers, cpuTrigger)
	}

	for _, schedule := range spec.Schedules {
		timezone := schedule.Timezone
		if timezone == "" {
			timezone = "UTC"
		}
		kedaTriggers = append(kedaTriggers, kedav1alpha1.ScaleTriggers{
			Type: "cron",
			Metadata: map[string]string{
				"scheduleName":    schedule.Name,
				"desiredReplicas": strconv.Itoa(schedule.MinReplicas),
				"start":           schedule.Start,
				"end":             schedule.End,
				"timezone":        timezone,
			},
		})
	}

	for _, prometheus := range spec.Prometheus {
		prometheusTrigger, err := buildPrometheusTrigger(ns, prometheus)
		if err != nil {
			return nil, err
		}

		kedaTriggers = append(kedaTriggers, *prometheusTrigger)
	}

	var scaledObjectAnnotation map[string]string
	if depInfo.replicas == 0 {
		// this is to disable the scale object when the deployment is scaled to 0 (app stop)
		scaledObjectAnnotation = map[string]string{
			AnnotationKEDAPausedReplicas: "0",
		}
	}

	targetRefName := depInfo.dep.Name
	if preserveVersions && !depInfo.isBase {
		targetRefName = provision.AppProcessName(a, depInfo.process, depInfo.version, "")
	}
	return &kedav1alpha1.ScaledObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:        hpaName,
			Namespace:   ns,
			Labels:      labels.ToLabels(),
			Annotations: scaledObjectAnnotation,
		},
		Spec: kedav1alpha1.ScaledObjectSpec{
			ScaleTargetRef: &kedav1alpha1.ScaleTarget{
				Name:       targetRefName,
				Kind:       "Deployment",
				APIVersion: appsv1.SchemeGroupVersion.String(),
			},
			MinReplicaCount: k8sutilsptr.To(int32(spec.MinUnits)),
			MaxReplicaCount: k8sutilsptr.To(int32(spec.MaxUnits)),
			Triggers:        kedaTriggers,
			Advanced: &kedav1alpha1.AdvancedConfig{
				HorizontalPodAutoscalerConfig: &kedav1alpha1.HorizontalPodAutoscalerConfig{
					Behavior: buildHPABehavior(spec.Behavior.ScaleDown),
				},
			},
		},
	}, nil
}

func buildPrometheusTrigger(ns string, prometheus provTypes.AutoScalePrometheus) (*kedav1alpha1.ScaleTriggers, error) {
	if prometheus.PrometheusAddress == "" {
		defaultPrometheusAddress, err := buildDefaultPrometheusAddress(ns)
		if err != nil {
			return nil, err
		}

		prometheus.PrometheusAddress = defaultPrometheusAddress
	}

	var authenticationRef *kedav1alpha1.AuthenticationRef
	if strings.HasPrefix(prometheus.PrometheusAddress, "https://monitoring.googleapis.com") {
		authenticationRef = &kedav1alpha1.AuthenticationRef{
			Kind: "ClusterTriggerAuthentication",
			Name: "gcp-credentials",
		}
	}

	return &kedav1alpha1.ScaleTriggers{
		Type:              "prometheus",
		AuthenticationRef: authenticationRef,
		Metadata: map[string]string{
			"serverAddress":        prometheus.PrometheusAddress,
			"query":                prometheus.Query,
			"threshold":            strconv.FormatFloat(prometheus.Threshold, 'f', -1, 64),
			"activationThreshold":  strconv.FormatFloat(prometheus.ActivationThreshold, 'f', -1, 64),
			"prometheusMetricName": prometheus.Name,
		},
	}, nil
}

func buildDefaultPrometheusAddress(ns string) (string, error) {
	prometheusAddressTemplate, err := config.GetString("kubernetes:keda:prometheus-address-template")
	if err != nil {
		return "", err
	}

	tmpl, err := template.New("prometheusAddress").Parse(prometheusAddressTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer

	err = tmpl.Execute(&buf, map[string]string{
		"namespace": ns,
	})
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func buildHPABehavior(behaviorSpec *provTypes.ScaleDownPolicy) *autoscalingv2.HorizontalPodAutoscalerBehavior {
	// setting ground to allow user customization regarding down scale behavior
	policyMin := autoscalingv2.MinChangePolicySelect
	policies := getPoliciesFromBehavior(behaviorSpec)
	behavior := &autoscalingv2.HorizontalPodAutoscalerBehavior{
		ScaleDown: &autoscalingv2.HPAScalingRules{
			SelectPolicy: &policyMin,
			Policies:     policies,
		},
	}
	settingValueStabilizationWindow(behavior, behaviorSpec)
	return behavior
}

func settingValueStabilizationWindow(behavior *autoscalingv2.HorizontalPodAutoscalerBehavior, behaviorSpec *provTypes.ScaleDownPolicy) {
	if behaviorSpec != nil && behaviorSpec.StabilizationWindow != nil {
		behavior.ScaleDown.StabilizationWindowSeconds = behaviorSpec.StabilizationWindow
		return
	}
	behavior.ScaleDown.StabilizationWindowSeconds = k8sutilsptr.To(int32(300))
}

func getPoliciesFromBehavior(behaviorSpec *provTypes.ScaleDownPolicy) (policies []autoscalingv2.HPAScalingPolicy) {
	return []autoscalingv2.HPAScalingPolicy{
		{
			Type:          autoscalingv2.PercentScalingPolicy,
			Value:         getBehaviorPercentageNoFail(behaviorSpec, 10),
			PeriodSeconds: 60,
		},
		{
			Type:          autoscalingv2.PodsScalingPolicy,
			Value:         getBehaviorUnitsNoFail(behaviorSpec, 3),
			PeriodSeconds: 60,
		},
	}
}

func minimumAutoScaleVersion(ctx context.Context, client *ClusterClient, a *appTypes.App, process string) (*deploymentInfo, error) {
	depGroups, err := deploymentsDataForApp(ctx, client, a)
	if err != nil {
		return nil, err
	}

	minRoutableVersion := -1
	minNonRoutableVersion := -1
	var depInfoRoutable, depInfoNonRoutable *deploymentInfo

	for version, deps := range depGroups.versioned {
		if process == "" && len(deps) > 1 {
			return nil, provision.InvalidProcessError{Msg: "process argument is required"}
		}
		for i, dep := range deps {
			if process != "" && process != dep.process {
				continue
			}
			if dep.dep.Spec.Replicas == nil {
				continue
			}
			if dep.isRoutable {
				if minRoutableVersion == -1 || version < minRoutableVersion {
					minRoutableVersion = version
					depInfoRoutable = &deps[i]
				}
			} else {
				if minNonRoutableVersion == -1 || version < minNonRoutableVersion {
					minNonRoutableVersion = version
					depInfoNonRoutable = &deps[i]
				}
			}
		}
	}

	depInfo := depInfoRoutable
	if depInfo == nil {
		depInfo = depInfoNonRoutable
	}
	if depInfo == nil {
		return nil, errNoDeploy
	}
	return depInfo, nil
}

func vpaCRDExists(ctx context.Context, client *ClusterClient) (bool, error) {
	return crdExists(ctx, client, vpaCRDName)
}

func ensureAutoScale(ctx context.Context, client *ClusterClient, a *appTypes.App, process string) error {
	multiErr := tsuruErrors.NewMultiError()

	err := ensureHPA(ctx, client, a, process)
	if err != nil {
		multiErr.Add(err)
	}

	err = ensureVPAIfEnabled(ctx, client, a, process)
	if err != nil {
		multiErr.Add(err)
	}

	return multiErr.ToError()
}

func ensureVPAIfEnabled(ctx context.Context, client *ClusterClient, a *appTypes.App, process string) error {
	hasVPA, err := vpaCRDExists(ctx, client)
	if err != nil {
		return err
	}

	if !hasVPA {
		return nil
	}

	rawEnableVPA, _ := provision.GetAppMetadata(a, process).Annotation(AnnotationEnableVPA)
	if enableVPA, _ := strconv.ParseBool(rawEnableVPA); enableVPA {
		err = ensureVPA(ctx, client, a, process)
	} else {
		err = ensureVPADeleted(ctx, client, a, process)
	}
	return err
}

func ensureVPA(ctx context.Context, client *ClusterClient, a *appTypes.App, process string) error {
	cli, err := VPAClientForConfig(client.RestConfig())
	if err != nil {
		return err
	}
	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return err
	}
	depInfo, err := minimumAutoScaleVersion(ctx, client, a, process)
	if err != nil {
		return err
	}

	labels, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
		App:     a,
		Process: depInfo.process,
		Version: depInfo.version,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix: tsuruLabelPrefix,
		},
	})
	if err != nil {
		return errors.WithStack(err)
	}
	labels = labels.WithoutIsolated().WithoutRoutable()

	vpaUpdateOff := vpav1.UpdateModeOff
	vpa := &vpav1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:   vpaNameForApp(a, process),
			Labels: labels.ToLabels(),
		},
		Spec: vpav1.VerticalPodAutoscalerSpec{
			TargetRef: &autoscalingv1.CrossVersionObjectReference{
				APIVersion: appsv1.SchemeGroupVersion.String(),
				Kind:       "Deployment",
				Name:       depInfo.dep.Name,
			},
			UpdatePolicy: &vpav1.PodUpdatePolicy{
				UpdateMode: &vpaUpdateOff,
			},
		},
	}

	existingVPA, err := cli.AutoscalingV1().VerticalPodAutoscalers(ns).Get(ctx, vpa.Name, metav1.GetOptions{})
	if k8sErrors.IsNotFound(err) {
		existingVPA = nil
	} else if err != nil {
		return errors.WithStack(err)
	}

	if existingVPA != nil {
		vpa.ResourceVersion = existingVPA.ResourceVersion
		_, err = cli.AutoscalingV1().VerticalPodAutoscalers(ns).Update(ctx, vpa, metav1.UpdateOptions{})
	} else {
		_, err = cli.AutoscalingV1().VerticalPodAutoscalers(ns).Create(ctx, vpa, metav1.CreateOptions{})
	}
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func ensureVPADeleted(ctx context.Context, client *ClusterClient, a *appTypes.App, process string) error {
	cli, err := VPAClientForConfig(client.RestConfig())
	if err != nil {
		return err
	}
	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return err
	}
	err = cli.AutoscalingV1().VerticalPodAutoscalers(ns).Delete(ctx, vpaNameForApp(a, process), metav1.DeleteOptions{})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.WithStack(err)
	}
	return nil
}

func ensureHPA(ctx context.Context, client *ClusterClient, a *appTypes.App, process string) error {
	autoScaleSpecs, err := getAutoScale(ctx, client, a, process)
	if err != nil {
		return err
	}
	if len(autoScaleSpecs) == 0 {
		return nil
	}

	multiErr := tsuruErrors.NewMultiError()
	for _, spec := range autoScaleSpecs {
		err = setAutoScale(ctx, client, a, spec, true)
		if err != nil && err != errNoDeploy {
			multiErr.Add(err)
		}
	}
	return multiErr.ToError()
}

func getAutoScale(ctx context.Context, client *ClusterClient, a *appTypes.App, process string) ([]provTypes.AutoScaleSpec, error) {
	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return nil, err
	}

	ls, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
		App:     a,
		Process: process,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix: tsuruLabelPrefix,
		},
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	hpas, err := client.AutoscalingV2().HorizontalPodAutoscalers(ns).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(ls.ToHPASelector())).String(),
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var specs []provTypes.AutoScaleSpec
	for _, hpa := range hpas.Items {
		scaledObjectName := kedaScaledObjectName(hpa)
		if scaledObjectName != "" {
			kedaClient, err := KEDAClientForConfig(client.restConfig)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			observedKEDAScaledObject, err := kedaClient.KedaV1alpha1().ScaledObjects(ns).Get(ctx, scaledObjectName, metav1.GetOptions{})
			if err != nil {
				return nil, errors.WithStack(err)
			}
			specs = append(specs, scaledObjectToSpec(*observedKEDAScaledObject))
		} else {
			specs = append(specs, hpaToSpec(hpa))
		}
	}
	return specs, nil
}

func allVPAsForApp(ctx context.Context, clusterClient *ClusterClient, vpaClient vpaclientset.Interface, a *appTypes.App) (*vpav1.VerticalPodAutoscalerList, error) {
	ns, err := clusterClient.AppNamespace(ctx, a)
	if err != nil {
		return nil, err
	}
	ls, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
		App: a,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix: tsuruLabelPrefix,
		},
	})
	if err != nil {
		return nil, err
	}
	existingVPAs, err := vpaClient.AutoscalingV1().VerticalPodAutoscalers(ns).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(ls.ToHPASelector())).String(),
	})
	if err != nil {
		return nil, err
	}

	return existingVPAs, nil
}

func vpasForVersion(ctx context.Context, clusterClient *ClusterClient, vpaClient vpaclientset.Interface, a *appTypes.App, version int) (*vpav1.VerticalPodAutoscalerList, error) {
	ns, err := clusterClient.AppNamespace(ctx, a)
	if err != nil {
		return nil, err
	}
	ls, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
		App: a,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix: tsuruLabelPrefix,
		},
		Version: version,
	})
	if err != nil {
		return nil, err
	}
	vpasForVersion, err := vpaClient.AutoscalingV1().VerticalPodAutoscalers(ns).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(ls.ToHPASelector())).String(),
	})
	if err != nil {
		return nil, err
	}

	return vpasForVersion, nil
}

func deleteAllVPA(ctx context.Context, client *ClusterClient, a *appTypes.App) error {
	vpaCli, err := VPAClientForConfig(client.RestConfig())
	if err != nil {
		return err
	}
	vpaList, err := allVPAsForApp(ctx, client, vpaCli, a)
	if k8sErrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if vpaList.Items != nil {
		for _, vpa := range vpaList.Items {
			err = vpaCli.AutoscalingV1().VerticalPodAutoscalers(vpa.Namespace).Delete(ctx, vpa.Name, metav1.DeleteOptions{})
			if err != nil && !k8sErrors.IsNotFound(err) {
				return errors.WithStack(err)
			}
		}
	}
	return nil
}

func deleteVPAsByVersion(ctx context.Context, client *ClusterClient, a *appTypes.App, version int) error {
	vpaCli, err := VPAClientForConfig(client.RestConfig())
	if err != nil {
		return err
	}
	vpaList, err := vpasForVersion(ctx, client, vpaCli, a, version)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	for _, vpa := range vpaList.Items {
		err = vpaCli.AutoscalingV1().VerticalPodAutoscalers(vpa.Namespace).Delete(ctx, vpa.Name, metav1.DeleteOptions{})
		if err != nil && !k8sErrors.IsNotFound(err) {
			return errors.WithStack(err)
		}
	}

	return nil
}
