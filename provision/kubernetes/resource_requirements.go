package kubernetes

import (
	"context"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
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

func (f *requirementsFactors) cpuLimits(resourceCPUBurst float64, cpuMilli int64) resource.Quantity {
	cpuBurst := f.poolCPUBurst

	if resourceCPUBurst > 1 {
		cpuBurst = resourceCPUBurst
	}

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

func resourceRequirements(plan *appTypes.Plan, pool string, client *ClusterClient, factors requirementsFactors) (apiv1.ResourceRequirements, error) {
	resourceLimits := apiv1.ResourceList{}
	resourceRequests := apiv1.ResourceList{}
	memory := plan.GetMemory()
	if memory != 0 {
		resourceLimits[apiv1.ResourceMemory] = factors.memoryLimits(memory)
		resourceRequests[apiv1.ResourceMemory] = factors.memoryRequests(memory)
	}
	cpuMilli := int64(plan.GetMilliCPU())
	cpuBurst := plan.GetCPUBurst()
	if cpuMilli != 0 {
		resourceLimits[apiv1.ResourceCPU] = factors.cpuLimits(cpuBurst, cpuMilli)
		resourceRequests[apiv1.ResourceCPU] = factors.cpuRequests(cpuMilli)
	}
	ephemeral, err := client.ephemeralStorage(pool)
	if err != nil {
		return apiv1.ResourceRequirements{}, err
	}
	if ephemeral.Value() > 0 {
		resourceRequests[apiv1.ResourceEphemeralStorage] = *resource.NewQuantity(0, resource.DecimalSI)
		resourceLimits[apiv1.ResourceEphemeralStorage] = ephemeral
	}

	return apiv1.ResourceRequirements{Limits: resourceLimits, Requests: resourceRequests}, nil
}

func planForProcess(ctx context.Context, a *appTypes.App, process string) (appTypes.Plan, error) {
	p := getProcess(a, process)
	if p == nil || p.Plan == "" {
		plan := a.Plan
		return plan, nil
	}

	plan, err := servicemanager.Plan.FindByName(ctx, p.Plan)
	if err != nil {
		return appTypes.Plan{}, errors.WithMessage(err, "Could not fetch plan")
	}

	return *plan, nil
}

func getProcess(app *appTypes.App, process string) *appTypes.Process {
	for _, p := range app.Processes {
		if p.Name == process {
			return &p
		}
	}
	return nil
}
