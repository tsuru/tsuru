// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ec2

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/queue"
)

const defaultRegion = "us-east-1"

func init() {
	iaas.RegisterIaasProvider("ec2", newEC2IaaS)
}

type EC2IaaS struct {
	base iaas.UserDataIaaS
}

func newEC2IaaS(name string) iaas.IaaS {
	return &EC2IaaS{base: iaas.UserDataIaaS{NamedIaaS: iaas.NamedIaaS{BaseIaaSName: "ec2", IaaSName: name}}}
}

func (i *EC2IaaS) createEC2Handler(regionOrEndpoint string) (*ec2.EC2, error) {
	keyId, err := i.base.GetConfigString("key-id")
	if err != nil {
		return nil, err
	}
	secretKey, err := i.base.GetConfigString("secret-key")
	if err != nil {
		return nil, err
	}
	var region, endpoint string
	if strings.HasPrefix(regionOrEndpoint, "http") {
		endpoint = regionOrEndpoint
		region = defaultRegion
	} else {
		region = regionOrEndpoint
	}
	config := aws.Config{
		Credentials: credentials.NewStaticCredentials(keyId, secretKey, ""),
		Region:      region,
		Endpoint:    endpoint,
	}
	return ec2.New(&config), nil
}

func (i *EC2IaaS) waitForDnsName(ec2Inst *ec2.EC2, instance *ec2.Instance) (*ec2.Instance, error) {
	rawWait, _ := i.base.GetConfigString("wait-timeout")
	maxWaitTime, _ := strconv.Atoi(rawWait)
	if maxWaitTime == 0 {
		maxWaitTime = 300
	}
	q, err := queue.Queue()
	if err != nil {
		return nil, err
	}
	taskName := fmt.Sprintf("ec2-wait-machine-%s", i.base.IaaSName)
	waitDuration := time.Duration(maxWaitTime) * time.Second
	job, err := q.EnqueueWait(taskName, monsterqueue.JobParams{
		"region":    ec2Inst.Config.Region,
		"endpoint":  ec2Inst.Config.Endpoint,
		"machineId": *instance.InstanceID,
		"timeout":   maxWaitTime,
	}, waitDuration)
	if err != nil {
		if err == monsterqueue.ErrQueueWaitTimeout {
			return nil, fmt.Errorf("ec2: time out after %v waiting for instance %s to start", waitDuration, *instance.InstanceID)
		}
		return nil, err
	}
	result, err := job.Result()
	if err != nil {
		return nil, err
	}
	instance.PublicDNSName = aws.String(result.(string))
	return instance, nil
}

func (i *EC2IaaS) Initialize() error {
	q, err := queue.Queue()
	if err != nil {
		return err
	}
	return q.RegisterTask(&ec2WaitTask{iaas: i})
}

func (i *EC2IaaS) Describe() string {
	return `EC2 IaaS required params:
  image=<image id>         Image AMI ID
  type=<instance type>     Your template uuid

Optional params:
  region=<region>          Chosen region, defaults to us-east-1
  securityGroup=<group>    Chosen security group
  keyName=<key name>       Key name for machine
`
}

func (i *EC2IaaS) DeleteMachine(m *iaas.Machine) error {
	regionOrEndpoint := getRegionOrEndpoint(m.CreationParams, false)
	if regionOrEndpoint == "" {
		return fmt.Errorf("region or endpoint creation param required")
	}
	ec2Inst, err := i.createEC2Handler(regionOrEndpoint)
	if err != nil {
		return err
	}
	input := ec2.TerminateInstancesInput{InstanceIDs: []*string{&m.Id}}
	_, err = ec2Inst.TerminateInstances(&input)
	return err
}

func (i *EC2IaaS) CreateMachine(params map[string]string) (*iaas.Machine, error) {
	regionOrEndpoint := getRegionOrEndpoint(params, true)
	imageId, ok := params["image"]
	if !ok {
		return nil, fmt.Errorf("image param required")
	}
	instanceType, ok := params["type"]
	if !ok {
		return nil, fmt.Errorf("type param required")
	}
	optimized, _ := params["ebs-optimized"]
	ebsOptimized, _ := strconv.ParseBool(optimized)
	userData, err := i.base.ReadUserData()
	if err != nil {
		return nil, err
	}
	keyName, _ := params["keyName"]
	options := ec2.RunInstancesInput{
		EBSOptimized: aws.Boolean(ebsOptimized),
		ImageID:      aws.String(imageId),
		InstanceType: aws.String(instanceType),
		KeyName:      aws.String(keyName),
		MinCount:     aws.Long(1),
		MaxCount:     aws.Long(1),
		UserData:     aws.String(userData),
	}
	securityGroup, ok := params["securityGroup"]
	if ok {
		options.SecurityGroups = []*string{aws.String(securityGroup)}
	}
	ec2Inst, err := i.createEC2Handler(regionOrEndpoint)
	if err != nil {
		return nil, err
	}
	resp, err := ec2Inst.RunInstances(&options)
	if err != nil {
		return nil, err
	}
	if len(resp.Instances) == 0 {
		return nil, fmt.Errorf("no instance created")
	}
	runInst := resp.Instances[0]
	instance, err := i.waitForDnsName(ec2Inst, runInst)
	if err != nil {
		return nil, err
	}
	machine := iaas.Machine{
		Id:      *instance.InstanceID,
		Status:  *instance.State.Name,
		Address: *instance.PublicDNSName,
	}
	return &machine, nil
}

func getRegionOrEndpoint(params map[string]string, useDefault bool) string {
	regionOrEndpoint := params["endpoint"]
	if regionOrEndpoint == "" {
		regionOrEndpoint = params["region"]
		if regionOrEndpoint == "" && useDefault {
			regionOrEndpoint = defaultRegion
		}
	}
	return regionOrEndpoint
}
