// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ec2

import (
	"errors"
	"fmt"
	"time"

	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/log"
	"gopkg.in/amz.v2/aws"
	"gopkg.in/amz.v2/ec2"
)

type ec2WaitTask struct {
	iaas *EC2IaaS
}

func (t *ec2WaitTask) Name() string {
	return fmt.Sprintf("ec2-wait-machine-%s", t.iaas.base.IaaSName)
}

func (t *ec2WaitTask) Run(job monsterqueue.Job) {
	params := job.Parameters()
	regionName := params["region"].(string)
	machineId := params["machineId"].(string)
	var timeout int
	switch val := params["timeout"].(type) {
	case int:
		timeout = val
	case float64:
		timeout = int(val)
	}
	region, ok := aws.Regions[regionName]
	if !ok {
		job.Error(fmt.Errorf("region %q not found", regionName))
		return
	}
	ec2Inst, err := t.iaas.createEC2Handler(region)
	if err != nil {
		job.Error(err)
		return
	}
	var dnsName string
	var notifiedSuccess bool
	t0 := time.Now()
	for {
		log.Debugf("ec2: waiting for dnsname for instance %s", machineId)
		resp, err := ec2Inst.Instances([]string{machineId}, ec2.NewFilter())
		if err != nil {
			job.Error(err)
			break
		}
		if len(resp.Reservations) == 0 || len(resp.Reservations[0].Instances) == 0 {
			job.Error(err)
			break
		}
		instance := &resp.Reservations[0].Instances[0]
		dnsName = instance.DNSName
		if dnsName != "" {
			notifiedSuccess, _ = job.Success(dnsName)
			break
		}
		if time.Now().Sub(t0) > time.Duration(2*timeout)*time.Second {
			job.Error(errors.New("hard timeout"))
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !notifiedSuccess {
		ec2Inst.TerminateInstances([]string{machineId})
	}
}
