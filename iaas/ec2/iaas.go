// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ec2

import (
	"fmt"
	"time"

	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
)

const (
	maxWaitTime   = time.Duration(1 * time.Minute)
	defaultRegion = "us-east-1"
)

func init() {
	iaas.RegisterIaasProvider("ec2", NewEC2IaaS())
}

type EC2IaaS struct {
	iaas.NamedIaaS
}

func NewEC2IaaS() *EC2IaaS {
	return &EC2IaaS{NamedIaaS: iaas.NamedIaaS{BaseIaaSName: "ec2"}}
}

func (i *EC2IaaS) createEC2Handler(region aws.Region) (*ec2.EC2, error) {
	keyId, err := i.NamedIaaS.GetConfigString("key-id")
	if err != nil {
		return nil, err
	}
	secretKey, err := i.NamedIaaS.GetConfigString("secret-key")
	if err != nil {
		return nil, err
	}
	auth := aws.Auth{AccessKey: keyId, SecretKey: secretKey}
	return ec2.New(auth, region), nil
}

func waitForDnsName(ec2Inst *ec2.EC2, instance *ec2.Instance) (*ec2.Instance, error) {
	t0 := time.Now()
	for instance.DNSName == "" {
		instId := instance.InstanceId
		if time.Now().Sub(t0) > maxWaitTime {
			return nil, fmt.Errorf("ec2: time out waiting for instance %s to start", instId)
		}
		log.Debugf("ec2: waiting for dnsname for instance %s", instId)
		time.Sleep(500 * time.Millisecond)
		resp, err := ec2Inst.Instances([]string{instance.InstanceId}, ec2.NewFilter())
		if err != nil {
			return nil, err
		}
		if len(resp.Reservations) == 0 || len(resp.Reservations[0].Instances) == 0 {
			return nil, fmt.Errorf("No instances returned")
		}
		instance = &resp.Reservations[0].Instances[0]
	}
	return instance, nil
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

func (i *EC2IaaS) Clone(name string) iaas.IaaS {
	clone := *i
	clone.IaaSName = name
	return &clone
}

func (i *EC2IaaS) DeleteMachine(m *iaas.Machine) error {
	regionName, ok := m.CreationParams["region"]
	if !ok {
		return fmt.Errorf("region creation param required")
	}
	region, ok := aws.Regions[regionName]
	if !ok {
		return fmt.Errorf("region %q not found", regionName)
	}
	ec2Inst, err := i.createEC2Handler(region)
	if err != nil {
		return err
	}
	_, err = ec2Inst.TerminateInstances([]string{m.Id})
	return err
}

func (i *EC2IaaS) CreateMachine(params map[string]string) (*iaas.Machine, error) {
	if _, ok := params["region"]; !ok {
		params["region"] = defaultRegion
	}
	regionName := params["region"]
	region, ok := aws.Regions[regionName]
	if !ok {
		return nil, fmt.Errorf("region %q not found", regionName)
	}
	imageId, ok := params["image"]
	if !ok {
		return nil, fmt.Errorf("image param required")
	}
	instanceType, ok := params["type"]
	if !ok {
		return nil, fmt.Errorf("type param required")
	}
	keyName, _ := params["keyName"]
	options := ec2.RunInstances{
		ImageId:      imageId,
		InstanceType: instanceType,
		UserData:     []byte(iaas.UserData),
		MinCount:     1,
		MaxCount:     1,
		KeyName:      keyName,
	}
	securityGroup, ok := params["securityGroup"]
	if ok {
		options.SecurityGroups = []ec2.SecurityGroup{
			{Name: securityGroup},
		}
	}
	ec2Inst, err := i.createEC2Handler(region)
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
	instance, err := waitForDnsName(ec2Inst, &resp.Instances[0])
	if err != nil {
		ec2Inst.TerminateInstances([]string{instance.InstanceId})
		return nil, err
	}
	machine := iaas.Machine{
		Id:      instance.InstanceId,
		Status:  instance.State.Name,
		Address: instance.DNSName,
	}
	return &machine, nil
}
