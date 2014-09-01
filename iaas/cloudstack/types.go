// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cloudstack

const (
	JOB_STATUS_IN_PROGRESS = 0
	JOB_STATUS_SUCCESSFUL  = 1
	JOB_STATUS_FAILED      = 2

	DISK_TYPE_ROOT     = "ROOT"
	DISK_TYPE_DATADISK = "DATADISK"
)

type ApiParams map[string]string

type ListVirtualMachinesResponse struct {
	ListVirtualMachinesResponse struct {
		VirtualMachine []VirtualMachine `json:"virtualmachine"`
	} `json:"listvirtualmachinesresponse"`
}

type VirtualMachine struct {
	Nic []NicStruct `json:"nic"`
}

type NicStruct struct {
	IpAddress string `json:"ipaddress"`
}

type DeployVirtualMachineResponse struct {
	DeployVirtualMachineResponse struct {
		ID    string `json:"id"`
		JobID string `json:"jobid"`
	} `json:"deployvirtualmachineresponse"`
}

type DestroyVirtualMachineResponse struct {
	DestroyVirtualMachineResponse struct {
		JobID string `json:"jobid"`
	} `json:"destroyvirtualmachineresponse"`
}

type QueryAsyncJobResultResponse struct {
	QueryAsyncJobResultResponse struct {
		JobStatus     int         `json:"jobstatus"`
		JobResult     interface{} `json:"jobresult"`
		JobResultType string      `json:"jobresulttype"`
		JobResultCode int         `json:"jobresultcode"`
	} `json:"queryasyncjobresultresponse"`
}

type VolumeResult struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type ListVolumesResponse struct {
	ListVolumesResponse struct {
		Volume []VolumeResult `json:"volume"`
	} `json:"listvolumesresponse"`
}

type DetachVolumeResponse struct {
	DetachVolumeResponse struct {
		JobID string `json:"jobid"`
	} `json:"detachvolumeresponse"`
}
