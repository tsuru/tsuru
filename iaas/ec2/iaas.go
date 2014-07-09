// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ec2

import (
	"fmt"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/iaas"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
)

const userData = `#!/bin/bash
curl -sL https://raw.github.com/tsuru/now/master/run.bash | bash
`

const iaasName = "ec2"

func init() {
	iaas.RegisterIaasProvider(iaasName, &EC2IaaS{})
}

func createEC2Handler(region aws.Region) (*ec2.EC2, error) {
	keyId, err := config.GetString("iaas:ec2:key_id")
	if err != nil {
		return nil, err
	}
	secretKey, err := config.GetString("iaas:ec2:secret_key")
	if err != nil {
		return nil, err
	}
	auth := aws.Auth{AccessKey: keyId, SecretKey: secretKey}
	return ec2.New(auth, region), nil
}

type EC2IaaS struct{}

func (i *EC2IaaS) DeleteMachine(m *iaas.Machine) error {
	regionName, ok := m.CreationParams["region"]
	if !ok {
		return fmt.Errorf("region creation param required")
	}
	region, ok := aws.Regions[regionName]
	if !ok {
		return fmt.Errorf("region %q not found", regionName)
	}
	ec2Inst, err := createEC2Handler(region)
	if err != nil {
		return err
	}
	_, err = ec2Inst.TerminateInstances([]string{m.Id})
	return err
}

func (i *EC2IaaS) CreateMachine(params map[string]string) (*iaas.Machine, error) {
	regionName, ok := params["region"]
	if !ok {
		regionName = "us-east-1"
	}
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
	options := ec2.RunInstances{
		ImageId:      imageId,
		InstanceType: instanceType,
		UserData:     []byte(userData),
		MinCount:     1,
		MaxCount:     1,
	}
	ec2Inst, err := createEC2Handler(region)
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
	instance := resp.Instances[0]
	machine := iaas.Machine{
		Id:      instance.InstanceId,
		Status:  instance.State.Name,
		Address: instance.DNSName,
	}
	return &machine, nil
}
