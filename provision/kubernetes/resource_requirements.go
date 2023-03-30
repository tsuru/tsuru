package kubernetes

import (
	"context"
	"fmt"
	"strconv"

	"github.com/tsuru/tsuru/provision"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type requirementsFactors struct {
	overCommit       float64
	cpuOverCommit    float64
	memoryOverCommit float64
	cpuBurst         float64
}

func (f *requirementsFactors) memoryLimits(memory int64) resource.Quantity {
	return *resource.NewQuantity(memory, resource.BinarySI)
}

func (f *requirementsFactors) memoryRequests(memory int64) resource.Quantity {
	memoryOvercommit := f.overCommit
	if f.memoryOverCommit != 0 {
		memoryOvercommit = f.memoryOverCommit
	}
	if memoryOvercommit < 1 {
		memoryOvercommit = 1 // memory cannot be less than 1
	}
	return *resource.NewQuantity(overcommitedValue(memory, memoryOvercommit), resource.BinarySI)
}

func (f *requirementsFactors) cpuLimits(cpuMilli int64) resource.Quantity {
	cpuBurst := f.cpuBurst
	if cpuBurst < 1 {
		cpuBurst = 1.0 // cpu cannot be less than 1
	}
	return *resource.NewMilliQuantity(burstValue(cpuMilli, cpuBurst), resource.DecimalSI)
}

func (f *requirementsFactors) cpuRequests(cpuMilli int64) resource.Quantity {
	cpuOvercommit := f.overCommit
	if f.cpuOverCommit != 0 {
		cpuOvercommit = f.cpuOverCommit
	}
	if cpuOvercommit < 1 {
		cpuOvercommit = 1 // cpu cannot be less than 1
	}
	return *resource.NewMilliQuantity(overcommitedValue(cpuMilli, cpuOvercommit), resource.DecimalSI)
}

func overcommitedValue(v int64, overcommit float64) int64 {
	if overcommit == 1 {
		return v
	}
	return int64(float64(v) / overcommit)
}

func burstValue(v int64, burst float64) int64 {
	if burst == 1 {
		return v
	}
	return int64(float64(v) * burst)
}

func resourceRequirements(object provision.ResourceGetter, client *ClusterClient, factors requirementsFactors) (apiv1.ResourceRequirements, error) {
	resourceLimits := apiv1.ResourceList{}
	resourceRequests := apiv1.ResourceList{}
	memory := object.GetMemory()
	if memory != 0 {
		resourceLimits[apiv1.ResourceMemory] = factors.memoryLimits(memory)
		resourceRequests[apiv1.ResourceMemory] = factors.memoryRequests(memory)
	}
	cpuMilli := int64(object.GetMilliCPU())
	if cpuMilli != 0 {
		resourceLimits[apiv1.ResourceCPU] = factors.cpuLimits(cpuMilli)
		resourceRequests[apiv1.ResourceCPU] = factors.cpuRequests(cpuMilli)
	}
	ephemeral, err := client.ephemeralStorage(object.GetPool())
	if err != nil {
		return apiv1.ResourceRequirements{}, err
	}
	if ephemeral.Value() > 0 {
		resourceRequests[apiv1.ResourceEphemeralStorage] = *resource.NewQuantity(0, resource.DecimalSI)
		resourceLimits[apiv1.ResourceEphemeralStorage] = ephemeral
	}

	return apiv1.ResourceRequirements{Limits: resourceLimits, Requests: resourceRequests}, nil
}

func resourceRequirementsForBuildPod(ctx context.Context, app provision.App, cluster *ClusterClient) (map[string]apiv1.ResourceRequirements, error) {
	k8sBuildPlans := make(map[string]apiv1.ResourceRequirements)
	// first, try to get the build plan from apps pool
	plans, err := getPoolBuildPlan(ctx, app.GetPool())
	if err != nil {
		return nil, err
	}
	// if pools build plan is nil, try to get it from the cluster
	if plans == nil {
		plans, err = getClusterBuildPlan(ctx, cluster)
		if err != nil {
			return nil, err
		}
		// if neither pool build plan or cluster build plan are set, return no error and nil
		if plans == nil {
			return nil, nil
		}
	}
	for planKey, planName := range plans {
		cpu, err := resource.ParseQuantity(fmt.Sprintf("%sm", strconv.Itoa(planName.CPUMilli)))
		if err != nil {
			return nil, err
		}
		memoryBytes, err := resource.ParseQuantity(strconv.FormatInt(planName.Memory, 10))
		if err != nil {
			return nil, err
		}
		k8sBuildPlans[planKey] = apiv1.ResourceRequirements{
			Limits: apiv1.ResourceList{
				"cpu":    cpu,
				"memory": memoryBytes,
			},
			Requests: apiv1.ResourceList{
				"cpu":    cpu,
				"memory": memoryBytes,
			},
		}
	}
	return k8sBuildPlans, nil
}
