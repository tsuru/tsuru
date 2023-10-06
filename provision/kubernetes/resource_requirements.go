package kubernetes

import (
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	provisionTypes "github.com/tsuru/tsuru/types/provision"
)

type requirementsFactors struct {
	overCommit       float64
	cpuOverCommit    float64
	memoryOverCommit float64
	poolCPUBurst     float64
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
	cpuBurst := f.poolCPUBurst
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

func resourceRequirements(object provisionTypes.ResourceGetter, client *ClusterClient, factors requirementsFactors) (apiv1.ResourceRequirements, error) {
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
