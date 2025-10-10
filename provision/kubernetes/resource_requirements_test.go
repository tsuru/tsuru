package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/require"
	appTypes "github.com/tsuru/tsuru/types/app"
	provTypes "github.com/tsuru/tsuru/types/provision"
)

func TestGetResourceRequirements(t *testing.T) {
	type testCase struct {
		test                   string
		factors                requirementsFactors
		expectedLimitMemory    string
		expectedRequestsMemory string
		expectedLimitCPU       string
		expectedRequestsCPU    string
	}

	clusterClient := &ClusterClient{Cluster: &provTypes.Cluster{}}
	testsCases := []testCase{
		{
			test: "Normal factors",
			factors: requirementsFactors{
				overCommit: 1,
			},
			expectedLimitMemory:    "10Ki",
			expectedRequestsMemory: "10Ki",
			expectedLimitCPU:       "1",
			expectedRequestsCPU:    "1",
		},
		{
			test: "Over Commit 2",
			factors: requirementsFactors{
				overCommit: 2,
			},
			expectedLimitMemory:    "10Ki",
			expectedRequestsMemory: "5Ki",
			expectedLimitCPU:       "1",
			expectedRequestsCPU:    "500m",
		},
		{
			test: "CPU Over Commit 3 - Memory Over Commit 2",
			factors: requirementsFactors{
				overCommit:       1,
				cpuOverCommit:    3,
				memoryOverCommit: 2,
			},
			expectedLimitMemory:    "10Ki",
			expectedRequestsMemory: "5Ki",
			expectedLimitCPU:       "1",
			expectedRequestsCPU:    "333m",
		},
		{
			test: "Pool CPU Burst 1.1",
			factors: requirementsFactors{
				poolCPUBurst: 1.1,
				overCommit:   1,
			},
			expectedLimitMemory:    "10Ki",
			expectedRequestsMemory: "10Ki",
			expectedLimitCPU:       "1100m",
			expectedRequestsCPU:    "1",
		},
		{
			test: "Pool CPU Burst 2",
			factors: requirementsFactors{
				poolCPUBurst: 2,
				overCommit:   1,
			},
			expectedLimitMemory:    "10Ki",
			expectedRequestsMemory: "10Ki",
			expectedLimitCPU:       "2",
			expectedRequestsCPU:    "1",
		},
		{
			test: "Pool CPU Burst 2 - Over Commit 2",
			factors: requirementsFactors{
				poolCPUBurst: 2,
				overCommit:   2,
			},
			expectedLimitMemory:    "10Ki",
			expectedRequestsMemory: "5Ki",
			expectedLimitCPU:       "2",
			expectedRequestsCPU:    "500m",
		},
	}

	for _, testCase := range testsCases {
		t.Run(testCase.test, func(t *testing.T) {
			requirements, err := resourceRequirements(&appTypes.Plan{
				Memory:   10 * 1024,
				CPUMilli: 1000,
			}, "", clusterClient, testCase.factors)
			require.NoError(t, err)

			memoryLimits := requirements.Limits["memory"]
			require.Equal(t, testCase.expectedLimitMemory, memoryLimits.String())

			memoryRequests := requirements.Requests["memory"]
			require.Equal(t, testCase.expectedRequestsMemory, memoryRequests.String())

			cpuLimits := requirements.Limits["cpu"]
			require.Equal(t, testCase.expectedLimitCPU, cpuLimits.String())

			cpuRequests := requirements.Requests["cpu"]
			require.Equal(t, testCase.expectedRequestsCPU, cpuRequests.String())
		})
	}
}

func TestGetCPULimits(t *testing.T) {
	t.Run("Empty and default values", func(t *testing.T) {
		rf := &requirementsFactors{}
		result := rf.cpuLimits(0, 1000)
		require.Equal(t, "1", result.String())
	})

	t.Run("When we have burst per pool", func(t *testing.T) {
		rf := &requirementsFactors{poolCPUBurst: 1.2}
		result := rf.cpuLimits(0, 1000)
		require.Equal(t, "1200m", result.String())
	})

	t.Run("When we have burst per resource", func(t *testing.T) {
		rf := &requirementsFactors{poolCPUBurst: 1.2}
		result := rf.cpuLimits(1.3, 1000)
		require.Equal(t, "1300m", result.String())
	})

	t.Run("When we have burst per resource without pool default", func(t *testing.T) {
		rf := &requirementsFactors{}
		result := rf.cpuLimits(1.3, 1000)
		require.Equal(t, "1300m", result.String())
	})
}
