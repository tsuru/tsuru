// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ec2

import (
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/iaas"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
	"launchpad.net/goamz/ec2/ec2test"
	"launchpad.net/gocheck"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct {
	srv    *ec2test.Server
	region aws.Region
}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpTest(c *gocheck.C) {
	var err error
	s.srv, err = ec2test.NewServer()
	c.Assert(err, gocheck.IsNil)
	s.region = aws.Region{
		Name:        "myregion",
		EC2Endpoint: s.srv.URL(),
	}
	aws.Regions["myregion"] = s.region
	config.Set("iaas:ec2:key-id", "mykey")
	config.Set("iaas:ec2:secret-key", "mysecret")
}

func (s *S) TearDownTest(c *gocheck.C) {
	s.srv.Quit()
}

func (s *S) TestCreateEC2Handler(c *gocheck.C) {
	iaas := &EC2IaaS{}
	handler, err := iaas.createEC2Handler(aws.APNortheast)
	c.Assert(err, gocheck.IsNil)
	c.Assert(handler.Region, gocheck.DeepEquals, aws.APNortheast)
	c.Assert(handler.Auth.AccessKey, gocheck.Equals, "mykey")
	c.Assert(handler.Auth.SecretKey, gocheck.Equals, "mysecret")
}

func (s *S) TestCreateMachine(c *gocheck.C) {
	params := map[string]string{
		"region": "myregion",
		"image":  "ami-xxxxxx",
		"type":   "m1.micro",
	}
	iaas := &EC2IaaS{}
	m, err := iaas.CreateMachine(params)
	m.CreationParams = map[string]string{"region": "myregion"}
	defer iaas.DeleteMachine(m)
	c.Assert(err, gocheck.IsNil)
	c.Assert(m.Id, gocheck.Matches, `i-\d`)
	c.Assert(m.Address, gocheck.Matches, `i-\d.testing.invalid`)
	c.Assert(m.Status, gocheck.Equals, "pending")
}

func (s *S) TestCreateMachineDefaultRegion(c *gocheck.C) {
	defaultRegionServer, err := ec2test.NewServer()
	c.Assert(err, gocheck.IsNil)
	region := aws.Region{
		Name:        defaultRegion,
		EC2Endpoint: defaultRegionServer.URL(),
	}
	aws.Regions[defaultRegion] = region
	params := map[string]string{
		"image": "ami-xxxxxx",
		"type":  "m1.micro",
	}
	expectedParams := map[string]string{
		"image":  "ami-xxxxxx",
		"type":   "m1.micro",
		"region": defaultRegion,
	}
	iaas := &EC2IaaS{}
	m, err := iaas.CreateMachine(params)
	c.Assert(err, gocheck.IsNil)
	c.Assert(params, gocheck.DeepEquals, expectedParams)
	m.CreationParams = params
	defer iaas.DeleteMachine(m)
	c.Assert(m.Id, gocheck.Matches, `i-\d`)
	c.Assert(m.Address, gocheck.Matches, `i-\d.testing.invalid`)
	c.Assert(m.Status, gocheck.Equals, "pending")
}

func (s *S) TestWaitForDnsName(c *gocheck.C) {
	iaas := &EC2IaaS{}
	handler, err := iaas.createEC2Handler(s.region)
	c.Assert(err, gocheck.IsNil)
	options := ec2.RunInstances{
		ImageId:      "ami-xxx",
		InstanceType: "m1.small",
		MinCount:     1,
		MaxCount:     1,
	}
	resp, err := handler.RunInstances(&options)
	c.Assert(err, gocheck.IsNil)
	instance := &resp.Instances[0]
	instance.DNSName = ""
	instance, err = waitForDnsName(handler, instance)
	c.Assert(err, gocheck.IsNil)
	c.Assert(instance.DNSName, gocheck.Matches, `i-\d.testing.invalid`)
}

func (s *S) TestCreateMachineValidations(c *gocheck.C) {
	iaas := &EC2IaaS{}
	_, err := iaas.CreateMachine(map[string]string{
		"region": "invalid-region",
	})
	c.Assert(err, gocheck.ErrorMatches, `region "invalid-region" not found`)
	_, err = iaas.CreateMachine(map[string]string{
		"region": "myregion",
	})
	c.Assert(err, gocheck.ErrorMatches, "image param required")
	_, err = iaas.CreateMachine(map[string]string{
		"region": "myregion",
		"image":  "ami-xxxxx",
	})
	c.Assert(err, gocheck.ErrorMatches, "type param required")
}

func (s *S) TestDeleteMachine(c *gocheck.C) {
	insts := s.srv.NewInstances(1, "m1.small", "ami-x", ec2.InstanceState{}, nil)
	m := iaas.Machine{
		Id:             insts[0],
		CreationParams: map[string]string{"region": "myregion"},
	}
	iaas := &EC2IaaS{}
	err := iaas.DeleteMachine(&m)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestDeleteMachineValidations(c *gocheck.C) {
	insts := s.srv.NewInstances(1, "m1.small", "ami-x", ec2.InstanceState{}, nil)
	ec2Iaas := &EC2IaaS{}
	m := &iaas.Machine{
		Id:             insts[0],
		CreationParams: map[string]string{"region": "invalid"},
	}
	err := ec2Iaas.DeleteMachine(m)
	c.Assert(err, gocheck.ErrorMatches, `region "invalid" not found`)
	m = &iaas.Machine{
		Id:             insts[0],
		CreationParams: map[string]string{},
	}
	err = ec2Iaas.DeleteMachine(m)
	c.Assert(err, gocheck.ErrorMatches, `region creation param required`)
}

func (s *S) TestClone(c *gocheck.C) {
	var iaas EC2IaaS
	clonned := iaas.Clone("something")
	c.Assert(clonned, gocheck.FitsTypeOf, &iaas)
	clonnedIaas, _ := clonned.(*EC2IaaS)
	c.Assert(iaas.iaasName, gocheck.Equals, "")
	c.Assert(clonnedIaas.iaasName, gocheck.Equals, "something")
}

func (s *S) TestGetConfigString(c *gocheck.C) {
	var iaas EC2IaaS
	config.Set("iaas:ec2:url", "default_url")
	val, err := iaas.getConfigString("url")
	c.Assert(err, gocheck.IsNil)
	c.Assert(val, gocheck.Equals, "default_url")
	iaas2 := iaas.Clone("something").(*EC2IaaS)
	val, err = iaas2.getConfigString("url")
	c.Assert(err, gocheck.IsNil)
	c.Assert(val, gocheck.Equals, "default_url")
	config.Set("iaas:custom:something:url", "custom_url")
	val, err = iaas2.getConfigString("url")
	c.Assert(err, gocheck.IsNil)
	c.Assert(val, gocheck.Equals, "custom_url")
}
