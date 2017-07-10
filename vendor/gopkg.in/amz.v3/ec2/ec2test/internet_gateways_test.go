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

func (s *S) TestAddInternetGateway(c *C) {
	igw, err := s.srv.AddInternetGateway(ec2.InternetGateway{
		VPCId: "missing",
	})
	c.Assert(err, ErrorMatches, `VPC "missing" not found`)
	c.Assert(igw, DeepEquals, ec2.InternetGateway{})
	s.assertVPCsExist(c, true)

	// Added as not attached to a VPC when VPCId is empty.
	toAdd := ec2.InternetGateway{
		Id:              "should-be-ignored",
		VPCId:           "",
		AttachmentState: "insane",
	}
	added, err := s.srv.AddInternetGateway(toAdd)
	c.Assert(err, IsNil)
	c.Assert(added.Id, Matches, `^igw-[0-9a-f]+$`)
	toAdd.Id = added.Id // the only field that should differ
	c.Assert(added, DeepEquals, toAdd)

	filter := ec2.NewFilter()
	filter.Add("attachment.state", "insane")
	resp, err := s.ec2.InternetGateways(nil, filter)
	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.InternetGateways, HasLen, 1)
	c.Assert(resp.InternetGateways[0], DeepEquals, added)

	// Added as attached to a VPC when VPCId is valid.
	toAdd = ec2.InternetGateway{
		Id:              "should-be-ignored",
		VPCId:           "vpc-0", // testing default VPC
		AttachmentState: "available",
	}
	added, err = s.srv.AddInternetGateway(toAdd)
	c.Assert(err, IsNil)
	c.Assert(added.Id, Matches, `^igw-[0-9a-f]+$`)
	toAdd.Id = added.Id // the only field that should differ
	c.Assert(added, DeepEquals, toAdd)

	resp, err = s.ec2.InternetGateways([]string{added.Id}, nil)
	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.InternetGateways, HasLen, 1)
	c.Assert(resp.InternetGateways[0], DeepEquals, added)
}

func (s *S) TestUpdateInternetGateway(c *C) {
	// AddDefaultVPCAndSubnets() called in SetUpTest() creates an IGW
	// as well.
	s.assertInternetGatewaysExist(c, true)

	err := s.srv.UpdateInternetGateway(ec2.InternetGateway{})
	c.Assert(err, ErrorMatches, "missing internet gateway id")

	err = s.srv.UpdateInternetGateway(ec2.InternetGateway{
		Id: "missing",
	})
	c.Assert(err, ErrorMatches, `internet gateway "missing" not found`)

	err = s.srv.UpdateInternetGateway(ec2.InternetGateway{
		Id:    "igw-0", // testing default IGW
		VPCId: "missing",
	})
	c.Assert(err, ErrorMatches, `VPC "missing" not found`)

	toUpdate := ec2.InternetGateway{
		Id:              "igw-0", // testing default IGW
		VPCId:           "vpc-0", // testing default VPC
		AttachmentState: "insane",
	}
	err = s.srv.UpdateInternetGateway(toUpdate)
	c.Assert(err, IsNil)

	resp, err := s.ec2.InternetGateways(nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.InternetGateways, HasLen, 1)
	c.Assert(resp.InternetGateways[0], DeepEquals, toUpdate)
}

func (s *S) TestRemoveInternetGateway(c *C) {
	resp, err := s.ec2.InternetGateways(nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.InternetGateways, HasLen, 1)
	c.Assert(resp.InternetGateways[0].Id, Equals, "igw-0") // default subnet

	err = s.srv.RemoveInternetGateway("")
	c.Assert(err, ErrorMatches, "missing internet gateway id")

	err = s.srv.RemoveInternetGateway("missing")
	c.Assert(err, ErrorMatches, `internet gateway "missing" not found`)

	err = s.srv.RemoveInternetGateway("igw-0") // the default IGW added
	c.Assert(err, IsNil)
	s.assertInternetGatewaysExist(c, false)
}
