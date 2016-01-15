// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cloudstack

const (
	jobInProgress = iota * 2
	jobFailed
)

const diskDataDisk = "DATADISK"

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

type ListZonesResponse struct {
	ListZonesResponse struct {
		Count int `json:"count"`
	} `json:"listzonesresponse"`
}

type CreateTagsResponse struct {
	Createtagsresponse struct {
		Displaytext string `json:"displaytext"`
		Success     string `json:"success"`
	} `json:"createtagsresponse"`
}
