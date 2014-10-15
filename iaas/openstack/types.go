// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package openstack

const (
	JOB_STATUS_IN_BUILD   = "BUILD"
	JOB_STATUS_SUCCESSFUL = "RUNNING"
	JOB_STATUS_FAILED     = "ERROR"
)

type ApiParams map[string]string

type GetVMInfo struct {
	GetVMInfo struct {
		Info interface{} `json:"addresses"`
		Name string      `json:"name"`
		Az   string      `json:"OS-EXT-AZ:availability_zone"`
	} `json:"server"`
}

type preInfo struct {
	Server map[string]string `json:"server"`
}

type DeployVMresult struct {
	DeployVMresult struct {
		ID string `json:"id"`
	} `json:"server"`
}

type QueryJobResult struct {
	QueryJobResult struct {
		JobStatus string `json:"status"`
	} `json:"server"`
}
