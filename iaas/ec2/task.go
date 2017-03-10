// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ec2

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/log"
)

type ec2WaitTask struct {
	iaas *EC2IaaS
}

func (t *ec2WaitTask) Name() string {
	return fmt.Sprintf("ec2-wait-machine-%s", t.iaas.base.IaaSName)
}

func (t *ec2WaitTask) Run(job monsterqueue.Job) {
	params := job.Parameters()
	regionOrEndpoint := getRegionOrEndpoint(map[string]string{
		"region":   params["region"].(string),
		"endpoint": params["endpoint"].(string),
	}, true)
	machineId := params["machineId"].(string)
	var timeout int
	switch val := params["timeout"].(type) {
	case int:
		timeout = val
	case float64:
		timeout = int(val)
	}
	networkIdx := -1
	if idx, ok := params["networkIndex"]; ok {
		switch val := idx.(type) {
		case int:
			networkIdx = val
		case float64:
			networkIdx = int(val)
		}
	}
	ec2Inst, err := t.iaas.createEC2Handler(regionOrEndpoint)
	if err != nil {
		job.Error(err)
		return
	}
	var dnsName string
	var notifiedSuccess bool
	t0 := time.Now()
	for {
		if time.Since(t0) > time.Duration(2*timeout)*time.Second {
			job.Error(errors.New("hard timeout"))
			break
		}
		log.Debugf("ec2: waiting for dnsname for instance %s", machineId)
		input := ec2.DescribeInstancesInput{
			InstanceIds: []*string{aws.String(machineId)},
		}
		resp, err := ec2Inst.DescribeInstances(&input)
		if err != nil {
			log.Debug("ec2: api error")
			time.Sleep(1000 * time.Millisecond)
			continue
		}
		if len(resp.Reservations) == 0 || len(resp.Reservations[0].Instances) == 0 {
			job.Error(err)
			break
		}
		instance := resp.Reservations[0].Instances[0]
		if networkIdx < 0 {
			dnsName = aws.StringValue(instance.PublicDnsName)
		} else {
			if len(instance.NetworkInterfaces) <= networkIdx {
				job.Error(errors.New("invalid network-index. "))
				break
			}
			dnsName = aws.StringValue(instance.NetworkInterfaces[networkIdx].PrivateDnsName)
		}
		if dnsName != "" {
			notifiedSuccess, _ = job.Success(dnsName)
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !notifiedSuccess {
		input := ec2.TerminateInstancesInput{
			InstanceIds: []*string{aws.String(machineId)},
		}
		ec2Inst.TerminateInstances(&input)
	}
}
