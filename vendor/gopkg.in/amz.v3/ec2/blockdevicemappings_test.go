//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2015 Canonical Ltd.
//

package ec2_test

import (
	"time"

	. "gopkg.in/check.v1"

	"gopkg.in/amz.v3/aws"
	"gopkg.in/amz.v3/ec2"
)

// Block device mapping tests run against either a local test server or
// live on EC2.

func (s *ServerTests) TestBlockDeviceMappings(c *C) {
	blockDeviceMappings := []ec2.BlockDeviceMapping{{
		DeviceName:          "/dev/sda2",
		VolumeSize:          8,
		DeleteOnTermination: true,
	}, {
		VirtualName: "ephemeral0",
		DeviceName:  "/dev/sdb",
	}}

	instList, err := s.ec2.RunInstances(&ec2.RunInstances{
		ImageId:             imageId,
		InstanceType:        "t1.micro",
		BlockDeviceMappings: blockDeviceMappings,
	})
	c.Assert(err, IsNil)
	inst := instList.Instances[0]
	c.Assert(inst, NotNil)
	instId := inst.InstanceId
	defer terminateInstances(c, s.ec2, []string{instId})

	// Block device mappings are not (typically?) included in the initial
	// RunInstanceResp, so we must periodically DescribeInstances.
	testAttempt := aws.AttemptStrategy{
		Total: 5 * time.Minute,
		Delay: 5 * time.Second,
	}
	var list *ec2.InstancesResp
	done := false
	for a := testAttempt.Start(); !done && a.Next(); {
		c.Logf("waiting for block device mappings to be processed")
		list, err = s.ec2.Instances([]string{instId}, nil)
		if err != nil {
			c.Fatalf("Instances returned: %v", err)
			return
		}
		inst = list.Reservations[0].Instances[0]
		if len(inst.BlockDeviceMappings) == 0 {
			c.Logf("BlockDeviceMappings is empty, retrying")
			continue
		}
		done = true
	}
	if !done {
		c.Fatalf("timeout while waiting for block device mappings")
	}

	// There should be one item for /dev/sda1; ephemeral devices
	// should not show up.
	c.Assert(inst.BlockDeviceMappings, HasLen, 2)
	c.Assert(inst.BlockDeviceMappings[0].DeviceName, Equals, "/dev/sda1")
	c.Assert(inst.BlockDeviceMappings[0].DeleteOnTermination, Equals, true)
	c.Assert(inst.BlockDeviceMappings[0].AttachTime, Not(Equals), "")
	c.Assert(inst.BlockDeviceMappings[0].Status, Not(Equals), "")
	c.Assert(inst.BlockDeviceMappings[0].VolumeId, Not(Equals), "")
	c.Assert(inst.BlockDeviceMappings[1].DeviceName, Equals, "/dev/sda2")
	c.Assert(inst.BlockDeviceMappings[1].DeleteOnTermination, Equals, true)
	c.Assert(inst.BlockDeviceMappings[1].AttachTime, Not(Equals), "")
	c.Assert(inst.BlockDeviceMappings[1].Status, Not(Equals), "")
	c.Assert(inst.BlockDeviceMappings[1].VolumeId, Not(Equals), "")
}
