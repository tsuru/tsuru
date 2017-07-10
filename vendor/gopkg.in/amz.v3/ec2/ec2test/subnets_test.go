//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2015 Canonical Ltd.
//

package ec2test_test

import (
	. "gopkg.in/check.v1"

	"gopkg.in/amz.v3/ec2"
)

func (s *S) TestAddSubnet(c *C) {
	// AddDefaultVPCAndSubnets() called by SetUpTest() adds a default
	// VPC.
	s.assertVPCsExist(c, true)
	subnet, err := s.srv.AddSubnet(ec2.Subnet{})
	c.Assert(err, ErrorMatches, "empty VPCId field")
	c.Assert(subnet, DeepEquals, ec2.Subnet{})

	subnet, err = s.srv.AddSubnet(ec2.Subnet{VPCId: "ignored"})
	c.Assert(err, ErrorMatches, "empty AvailZone field")
	c.Assert(subnet, DeepEquals, ec2.Subnet{})

	subnet, err = s.srv.AddSubnet(ec2.Subnet{
		VPCId:     "missing",
		AvailZone: "ignored",
	})
	c.Assert(err, ErrorMatches, `no such VPC "missing"`)
	c.Assert(subnet, DeepEquals, ec2.Subnet{})

	subnet, err = s.srv.AddSubnet(ec2.Subnet{
		VPCId:     "vpc-0", // testing default VPC
		AvailZone: "missing",
	})
	c.Assert(err, ErrorMatches, `no such availability zone "missing"`)
	c.Assert(subnet, DeepEquals, ec2.Subnet{})

	toAdd := ec2.Subnet{
		Id:                  "should-be-ignored",
		VPCId:               "vpc-0", // default VPC
		State:               "insane",
		CIDRBlock:           "0.1.2.0/24",
		AvailZone:           "us-east-1c", // testing zone
		AvailableIPCount:    42,           // used as-is
		DefaultForAZ:        false,
		MapPublicIPOnLaunch: true,
	}
	added, err := s.srv.AddSubnet(toAdd)
	c.Assert(added.Id, Matches, `^subnet-[0-9a-f]+$`)
	toAdd.Id = added.Id // only this differs
	c.Assert(added, DeepEquals, toAdd)

	filter := ec2.NewFilter()
	filter.Add("vpc-id", "vpc-0")
	filter.Add("state", "insane")
	resp, err := s.ec2.Subnets(nil, filter)
	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.Subnets, HasLen, 1)
	c.Assert(resp.Subnets[0], DeepEquals, added)
}

func (s *S) TestUpdateSubnet(c *C) {
	err := s.srv.UpdateSubnet(ec2.Subnet{})
	c.Assert(err, ErrorMatches, "missing subnet id")

	err = s.srv.UpdateSubnet(ec2.Subnet{
		Id:    "ignored-when-VPCId-is-empty",
		VPCId: "",
	})
	c.Assert(err, ErrorMatches, "missing VPC id")

	err = s.srv.UpdateSubnet(ec2.Subnet{
		Id:    "ignored-when-VPCId-invalid",
		VPCId: "missing",
	})
	c.Assert(err, ErrorMatches, `VPC "missing" not found`)

	err = s.srv.UpdateSubnet(ec2.Subnet{
		Id:    "missing",
		VPCId: "vpc-0", // testing default VPC
	})
	c.Assert(err, ErrorMatches, `subnet "missing" not found`)

	toUpdate := ec2.Subnet{
		Id:                  "subnet-0", // testing default subnet
		VPCId:               "vpc-0",    // default VPC
		State:               "insane",
		CIDRBlock:           "0.1.2.0/24",
		AvailZone:           "us-east-1c", // testing zone
		AvailableIPCount:    42,           // used as-is
		DefaultForAZ:        false,
		MapPublicIPOnLaunch: true,
	}
	err = s.srv.UpdateSubnet(toUpdate)
	c.Assert(err, IsNil)

	resp, err := s.ec2.Subnets([]string{"subnet-0"}, nil)
	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.Subnets, HasLen, 1)
	c.Assert(resp.Subnets[0], DeepEquals, toUpdate)
}

func (s *S) TestRemoveSubnet(c *C) {
	resp, err := s.ec2.Subnets(nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.Subnets, HasLen, 1)
	c.Assert(resp.Subnets[0].Id, Equals, "subnet-0") // default subnet

	err = s.srv.RemoveSubnet("")
	c.Assert(err, ErrorMatches, "missing subnet id")

	err = s.srv.RemoveSubnet("missing")
	c.Assert(err, ErrorMatches, `subnet "missing" not found`)

	err = s.srv.RemoveSubnet("subnet-0") // the default VPC added
	c.Assert(err, IsNil)

	// Ensure no subnets remain.
	resp, err = s.ec2.Subnets(nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.Subnets, HasLen, 0)
}
