package kubernetes

import (
	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	provTypes "github.com/tsuru/tsuru/types/provision"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
)

func getBehaviorNoFail(scaledObject kedav1alpha1.ScaledObject) *autoscalingv2.HorizontalPodAutoscalerBehavior {
	if scaledObject.Spec.Advanced == nil {
		return &autoscalingv2.HorizontalPodAutoscalerBehavior{}
	}
	if scaledObject.Spec.Advanced.HorizontalPodAutoscalerConfig == nil {
		return &autoscalingv2.HorizontalPodAutoscalerBehavior{}
	}
	if scaledObject.Spec.Advanced.HorizontalPodAutoscalerConfig.Behavior == nil {
		return &autoscalingv2.HorizontalPodAutoscalerBehavior{}
	}
	return scaledObject.Spec.Advanced.HorizontalPodAutoscalerConfig.Behavior
}

func getPercentagePolicy(behavior *autoscalingv2.HorizontalPodAutoscalerBehavior) *int32 {
	if behavior == nil {
		return nil
	}
	if behavior.ScaleDown == nil {
		return nil
	}
	policies := behavior.ScaleDown.Policies
	for _, policy := range policies {
		if policy.Type == autoscalingv2.PercentScalingPolicy {
			return &policy.Value
		}
	}
	return nil
}

func getUnitPolicy(behavior *autoscalingv2.HorizontalPodAutoscalerBehavior) *int32 {
	if behavior == nil {
		return nil
	}
	if behavior.ScaleDown == nil {
		return nil
	}
	policies := behavior.ScaleDown.Policies
	for _, policy := range policies {
		if policy.Type == autoscalingv2.PodsScalingPolicy {
			return &policy.Value
		}
	}
	return nil
}

func getStabilizationWindow(behavior *autoscalingv2.HorizontalPodAutoscalerBehavior) *int32 {
	if behavior == nil {
		return nil
	}
	if behavior.ScaleDown == nil {
		return nil
	}
	return behavior.ScaleDown.StabilizationWindowSeconds
}

func getBehaviorPercentageNoFail(param *provTypes.ScaleDownPoliciy, valueDefault int32) int32 {
	if param == nil {
		return valueDefault
	}
	if param.PercentagePolicyValue == nil {
		return valueDefault
	}
	return *param.PercentagePolicyValue
}

func getBehaviorUnitsNoFail(param *provTypes.ScaleDownPoliciy, valueDefault int32) int32 {
	if param == nil {
		return valueDefault
	}
	if param.UnitsPolicyValue == nil {
		return valueDefault
	}
	return *param.UnitsPolicyValue
}
