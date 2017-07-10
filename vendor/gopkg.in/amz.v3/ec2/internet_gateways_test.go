//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2015 Canonical Ltd.
//

package ec2_test

import (
	. "gopkg.in/check.v1"

	"gopkg.in/amz.v3/ec2"
)

// Internet Gateway tests with example responses

func (s *S) TestInternetGatewaysExample(c *C) {
	testServer.Response(200, nil, DescribeInternetGatewaysExample)

	ids := []string{"igw-eaad4883EXAMPLE"}
	resp, err := s.ec2.InternetGateways(ids, nil)
	req := testServer.WaitRequest()

	c.Assert(req.Form["Action"], DeepEquals, []string{"DescribeInternetGateways"})
	c.Assert(req.Form["InternetGatewayId.1"], DeepEquals, ids)

	c.Assert(err, IsNil)
	c.Assert(resp.RequestId, Equals, "59dbff89-35bd-4eac-99ed-be587EXAMPLE")
	c.Assert(resp.InternetGateways, HasLen, 1)
	igw := resp.InternetGateways[0]
	c.Check(igw.Id, Equals, "igw-eaad4883EXAMPLE")
	c.Check(igw.VPCId, Equals, "vpc-11ad4878")
	c.Check(igw.AttachmentState, Equals, "available")
	c.Check(igw.Tags, HasLen, 0)
}

// Internet Gateway tests that run either against the local test
// server or live EC2 servers.

func (s *ServerTests) TestDefaultVPCInternetGateway(c *C) {
	defaultVPCId, _ := s.getDefaultVPCIdAndSubnets(c)

	filter := ec2.NewFilter()
	filter.Add("attachment.vpc-id", defaultVPCId)
	// Look it up by VPC id filter.
	resp1, err := s.ec2.InternetGateways(nil, filter)
	c.Assert(err, IsNil)
	// There should be only one IGW attached to a VPC.
	c.Assert(resp1.InternetGateways, HasLen, 1)
	igw := resp1.InternetGateways[0]
	c.Assert(igw.Id, Matches, `^igw-[0-9a-f]+$`)
	defaultVPCIGWId := igw.Id
	c.Assert(igw.VPCId, Equals, defaultVPCId)
	// "available" should always be the state for the default VPC's IGW.
	c.Assert(igw.AttachmentState, Equals, "available")

	// Look it up by IGW id and no filter.
	resp2, err := s.ec2.InternetGateways([]string{defaultVPCIGWId}, nil)
	c.Assert(err, IsNil)
	resp2.RequestId = resp1.RequestId // the only difference.
	c.Assert(resp1, DeepEquals, resp2)
}
